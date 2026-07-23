package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	// Blob sub-endpoint routing (/api/{table}/{id}/upload or /api/{table}/{id}/blob)
	if len(pathParts) >= 4 && (pathParts[3] == "upload" || pathParts[3] == "blob") {
		rt.handleBlobSubendpoint(w, req, entry, idStr, pathParts)
		return
	}

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

func (rt *Router) handleBlobSubendpoint(w http.ResponseWriter, req *http.Request, entry ResourceEntry, idStr string, pathParts []string) {
	if rt.blobStore == nil {
		WriteError(w, http.StatusNotImplemented, "BLOB_STORE_NOT_CONFIGURED", "blob storage is not configured on server", nil)
		return
	}

	res := entry.Resource
	store := entry.Store
	sess := rt.extractSession(req)
	idVal := parseID(idStr)

	var fieldName string
	if len(pathParts) >= 5 && pathParts[4] != "" {
		fieldName = pathParts[4]
	} else {
		for _, f := range res.Fields {
			if f.Type == resource.TypeBlob {
				fieldName = f.Name
				break
			}
		}
	}

	if fieldName == "" {
		WriteError(w, http.StatusBadRequest, "BLOB_FIELD_NOT_FOUND", fmt.Sprintf("resource '%s' has no blob field specified", res.Name), nil)
		return
	}

	var targetField *resource.Field
	for _, f := range res.Fields {
		if f.Name == fieldName {
			targetField = &f
			break
		}
	}

	if targetField == nil || targetField.Type != resource.TypeBlob {
		WriteError(w, http.StatusBadRequest, "INVALID_BLOB_FIELD", fmt.Sprintf("field '%s' on resource '%s' is not of type blob", fieldName, res.Name), nil)
		return
	}

	opType := pathParts[3]

	switch opType {
	case "upload":
		if req.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "upload sub-endpoint requires POST method", nil)
			return
		}
		rt.handleBlobUpload(w, req, res, store, idVal, fieldName, sess)
	case "blob":
		switch req.Method {
		case http.MethodGet:
			rt.handleBlobGet(w, req, res, store, idVal, fieldName, sess)
		case http.MethodDelete:
			rt.handleBlobDelete(w, req, res, store, idVal, fieldName, sess)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("method %s not allowed on blob sub-endpoint", req.Method), nil)
		}
	}
}

func (rt *Router) handleBlobUpload(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, fieldName string, sess *auth.Session) {
	existingRec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s record with id '%v' not found", res.Name, id), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionUpdate, existingRec, nil)
	if !allowed {
		rt.writeAuthError(w, status, authErr)
		return
	}

	var fileReader io.Reader
	var fileSize int64
	var contentType string

	if strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
		err := req.ParseMultipartForm(32 << 20)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART_FORM", err.Error(), nil)
			return
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			if req.MultipartForm != nil && len(req.MultipartForm.File) > 0 {
				for _, files := range req.MultipartForm.File {
					if len(files) > 0 {
						file, err = files[0].Open()
						header = files[0]
						break
					}
				}
			}
		}
		if err != nil || file == nil {
			WriteError(w, http.StatusBadRequest, "FILE_REQUIRED", "multipart file parameter 'file' is required", nil)
			return
		}
		defer file.Close()
		fileReader = file
		fileSize = header.Size
		contentType = header.Header.Get("Content-Type")
	} else {
		fileReader = req.Body
		fileSize = req.ContentLength
		contentType = req.Header.Get("Content-Type")
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	ext := ""
	if parts := strings.Split(contentType, "/"); len(parts) == 2 {
		ext = "." + parts[1]
	}
	blobKey := fmt.Sprintf("blobs/%s/%v/%s_%d%s", res.Table, id, fieldName, time.Now().UnixNano(), ext)

	if err := rt.blobStore.Put(req.Context(), blobKey, fileReader, fileSize, contentType); err != nil {
		WriteError(w, http.StatusInternalServerError, "BLOB_STORE_FAILED", fmt.Sprintf("failed to store blob: %v", err), nil)
		return
	}

	updatePayload := map[string]any{
		fieldName: blobKey,
	}
	updatedRec, err := store.Update(req.Context(), res, id, updatePayload)
	if err != nil {
		_ = rt.blobStore.Delete(req.Context(), blobKey)
		WriteError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SanitizeRecord(res, updatedRec)
	WriteSuccess(w, http.StatusOK, updatedRec)
}

func (rt *Router) handleBlobGet(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, fieldName string, sess *auth.Session) {
	existingRec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s record with id '%v' not found", res.Name, id), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionRead, existingRec, nil)
	if !allowed {
		rt.writeAuthError(w, status, authErr)
		return
	}

	keyVal, ok := existingRec[fieldName].(string)
	if !ok || keyVal == "" {
		WriteError(w, http.StatusNotFound, "BLOB_NOT_FOUND", fmt.Sprintf("record '%v' has no blob stored in '%s'", id, fieldName), nil)
		return
	}

	reader, contentType, err := rt.blobStore.Get(req.Context(), keyVal)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "BLOB_NOT_FOUND", "blob data file not found in storage", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "BLOB_FETCH_FAILED", err.Error(), nil)
		return
	}
	defer reader.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (rt *Router) handleBlobDelete(w http.ResponseWriter, req *http.Request, res *resource.Resource, store storage.Store, id any, fieldName string, sess *auth.Session) {
	existingRec, err := store.Get(req.Context(), res, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("%s record with id '%v' not found", res.Name, id), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	status, allowed, authErr := auth.Evaluate(sess, res, auth.ActionDelete, existingRec, nil)
	if !allowed {
		rt.writeAuthError(w, status, authErr)
		return
	}

	if keyVal, ok := existingRec[fieldName].(string); ok && keyVal != "" {
		_ = rt.blobStore.Delete(req.Context(), keyVal)
	}

	updatePayload := map[string]any{
		fieldName: "",
	}
	updatedRec, err := store.Update(req.Context(), res, id, updatePayload)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "UPDATE_FAILED", err.Error(), nil)
		return
	}

	SanitizeRecord(res, updatedRec)
	WriteSuccess(w, http.StatusOK, updatedRec)
}
