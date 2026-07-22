package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

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

	if idStr == "" {
		switch req.Method {
		case http.MethodGet:
			rt.handleList(w, req, res, store)
		case http.MethodPost:
			rt.handleCreate(w, req, res, store)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("method %s not allowed on collection endpoint", req.Method), nil)
		}
	} else {
		idVal := parseID(idStr)
		switch req.Method {
		case http.MethodGet:
			rt.handleDetail(w, req, res, store, idVal)
		case http.MethodPut, http.MethodPatch:
			rt.handleUpdate(w, req, res, store, idVal)
		case http.MethodDelete:
			rt.handleDelete(w, req, res, store, idVal)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("method %s not allowed on detail endpoint", req.Method), nil)
		}
	}
}

func (rt *Router) handleList(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store) {
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

func (rt *Router) handleDetail(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any) {
	rec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("record with id '%v' not found in resource '%s'", id, res.Name), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("failed to fetch record: %v", err), nil)
		return
	}

	sanitized := SanitizeRecord(res, rec)
	WriteSuccess(w, http.StatusOK, sanitized)
}

func (rt *Router) handleCreate(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store) {
	var input map[string]any
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", fmt.Sprintf("invalid JSON payload: %v", err), nil)
		return
	}
	if input == nil {
		input = make(map[string]any)
	}

	created, err := store.Create(req.Context(), res, input)
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

func (rt *Router) handleUpdate(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any) {
	var input map[string]any
	if err := json.NewDecoder(req.Body).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "INVALID_JSON", fmt.Sprintf("invalid JSON payload: %v", err), nil)
		return
	}
	if input == nil {
		input = make(map[string]any)
	}

	updated, err := store.Update(req.Context(), res, id, input)
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

func (rt *Router) handleDelete(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any) {
	err := store.SoftDelete(req.Context(), res, id)
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
