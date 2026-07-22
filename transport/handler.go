package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// ServeHTTP implements http.Handler for dynamic dispatching of REST API endpoints.
func (rt *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/")
	pathParts := strings.Split(path, "/")

	// Reload API endpoint
	if req.URL.Path == "/_mold/reload" {
		if req.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only POST method is allowed for reload", nil)
			return
		}
		rt.handleReload(w, req)
		return
	}

	// API route matching: /api/{table} or /api/{table}/{id}
	if len(pathParts) < 2 || pathParts[0] != "api" || pathParts[1] == "" {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
		return
	}

	table := pathParts[1]
	// Block internal system tables starting with _mold_
	if strings.HasPrefix(table, "_mold_") {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
		return
	}

	var idStr string
	if len(pathParts) >= 3 {
		idStr = pathParts[2]
	}

	reg := rt.CurrentRegistry()
	entry, exists := reg.Lookup(table)
	if !exists {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("resource table '%s' not found", table), nil)
		return
	}

	res := entry.Resource
	store := entry.Store
	sess := rt.extractSession(req)

	if idStr == "" {
		switch req.Method {
		case http.MethodGet:
			rt.handleList(w, req, res, store, sess)
		case http.MethodPost:
			rt.handleCreate(w, req, res, store, sess)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("method %s not allowed on collection endpoint", req.Method), nil)
		}
	} else {
		idVal := parseID(idStr)
		switch req.Method {
		case http.MethodGet:
			rt.handleDetail(w, req, res, store, idVal, sess)
		case http.MethodPut, http.MethodPatch:
			rt.handleUpdate(w, req, res, store, idVal, sess)
		case http.MethodDelete:
			rt.handleDelete(w, req, res, store, idVal, sess)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("method %s not allowed on detail endpoint", req.Method), nil)
		}
	}
}

func (rt *Router) handleList(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, sess *auth.Session) {
	status, allowed, err := auth.Evaluate(sess, res, auth.ActionRead, nil, nil)
	if !allowed {
		rt.writeAuthError(w, status, err)
		return
	}

	limit := DefaultLimit
	offset := 0

	queryValues := req.URL.Query()
	if lStr := queryValues.Get("limit"); lStr != "" {
		if parsedL, err := strconv.Atoi(lStr); err == nil && parsedL > 0 {
			limit = parsedL
			if limit > MaxLimit {
				limit = MaxLimit
			}
		}
	}
	if oStr := queryValues.Get("offset"); oStr != "" {
		if parsedO, err := strconv.Atoi(oStr); err == nil && parsedO >= 0 {
			offset = parsedO
		}
	}

	// Fetch page
	q := storage.Query{
		Limit:  limit,
		Offset: offset,
	}
	records, err := store.List(req.Context(), res, q)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to list records: %v", err), nil)
		return
	}

	// Fetch total count without limit/offset for pagination metadata
	totalRecords, err := store.List(req.Context(), res, storage.Query{})
	totalCount := len(records)
	if err == nil {
		totalCount = len(totalRecords)
	}

	sanitized := SanitizeRecordList(res, records)
	if sanitized == nil {
		sanitized = []storage.Record{}
	}

	WriteListSuccess(w, http.StatusOK, sanitized, totalCount, limit, offset)
}

func (rt *Router) handleDetail(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, sess *auth.Session) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to fetch record: %v", err), nil)
		return
	}

	status, allowed, err := auth.Evaluate(sess, res, auth.ActionRead, rec, nil)
	if !allowed {
		rt.writeAuthError(w, status, err)
		return
	}

	sanitized := SanitizeRecord(res, rec)
	WriteSuccess(w, http.StatusOK, sanitized)
}

func (rt *Router) handleCreate(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, sess *auth.Session) {
	var input map[string]any
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", fmt.Sprintf("invalid JSON payload: %v", err), nil)
		return
	}
	if input == nil {
		input = make(map[string]any)
	}

	status, allowed, err := auth.Evaluate(sess, res, auth.ActionCreate, nil, input)
	if !allowed {
		rt.writeAuthError(w, status, err)
		return
	}

	// Auto-assign ownership_field if session exists and field is present in IR
	if sess != nil && res.Auth != nil && res.Auth.OwnershipField != "" {
		if _, exists := input[res.Auth.OwnershipField]; !exists {
			input[res.Auth.OwnershipField] = sess.UserID
		}
	}

	processedInput, err := auth.ProcessPasswordFields(res, input)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	created, err := store.Create(req.Context(), res, processedInput)
	if err != nil {
		if isFKConstraintError(err) {
			WriteError(w, http.StatusBadRequest, "INVALID_FOREIGN_KEY", fmt.Sprintf("referenced foreign key target does not exist: %v", err), nil)
			return
		}
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	sanitized := SanitizeRecord(res, created)
	WriteSuccess(w, http.StatusCreated, sanitized)
}

func (rt *Router) handleUpdate(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, sess *auth.Session) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to fetch record: %v", err), nil)
		return
	}

	var input map[string]any
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", fmt.Sprintf("invalid JSON payload: %v", err), nil)
		return
	}
	if input == nil {
		input = make(map[string]any)
	}

	status, allowed, err := auth.Evaluate(sess, res, auth.ActionUpdate, rec, input)
	if !allowed {
		rt.writeAuthError(w, status, err)
		return
	}

	processedInput, err := auth.ProcessPasswordFields(res, input)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	updated, err := store.Update(req.Context(), res, id, processedInput)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		if isFKConstraintError(err) {
			WriteError(w, http.StatusBadRequest, "INVALID_FOREIGN_KEY", fmt.Sprintf("referenced foreign key target does not exist: %v", err), nil)
			return
		}
		WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
		return
	}

	sanitized := SanitizeRecord(res, updated)
	WriteSuccess(w, http.StatusOK, sanitized)
}

func (rt *Router) handleDelete(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, sess *auth.Session) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to fetch record for deletion: %v", err), nil)
		return
	}

	status, allowed, err := auth.Evaluate(sess, res, auth.ActionDelete, rec, nil)
	if !allowed {
		rt.writeAuthError(w, status, err)
		return
	}

	err = store.SoftDelete(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to delete record: %v", err), nil)
		return
	}

	WriteSuccess(w, http.StatusOK, map[string]any{
		"deleted": true,
		"id":      id,
	})
}

func (rt *Router) handleReload(w http.ResponseWriter, req *http.Request) {
	if rt.reloadFn == nil {
		WriteError(w, http.StatusNotImplemented, "RELOAD_NOT_CONFIGURED", "reload callback function is not configured", nil)
		return
	}

	newReg, err := rt.reloadFn()
	if err != nil {
		WriteError(w, http.StatusBadRequest, "RELOAD_FAILED", fmt.Sprintf("failed to reload resource schemas: %v", err), nil)
		return
	}

	rt.SwapRegistry(newReg)
	WriteSuccess(w, http.StatusOK, map[string]any{
		"reloaded": true,
	})
}

func parseID(s string) any {
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	return s
}

func isFKConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "FOREIGN KEY constraint failed")
}

func (rt *Router) extractSession(req *http.Request) *auth.Session {
	if rt.sessionMgr == nil {
		return nil
	}
	cookie, err := req.Cookie(auth.SessionCookieName)
	if err != nil || cookie == nil || cookie.Value == "" {
		return nil
	}
	sess, err := rt.sessionMgr.GetSession(req.Context(), cookie.Value)
	if err != nil {
		return nil
	}
	return sess
}

func (rt *Router) writeAuthError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = auth.ErrForbidden
	}
	code := "FORBIDDEN"
	if status == http.StatusUnauthorized {
		code = "UNAUTHORIZED"
	} else if status == http.StatusNotFound {
		code = "NOT_FOUND"
	}
	WriteError(w, status, code, err.Error(), nil)
}
