package transport

import (
	"encoding/json"
	"net/http"
)

type SuccessEnvelope struct {
	Data any `json:"data"`
}

type ListSuccessEnvelope struct {
	Data any      `json:"data"`
	Meta ListMeta `json:"meta"`
}

type ListMeta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// WriteJSON sends a JSON response with status code.
func WriteJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

// WriteSuccess writes a standard single data success response {"data": ...}.
func WriteSuccess(w http.ResponseWriter, statusCode int, data any) {
	WriteJSON(w, statusCode, SuccessEnvelope{Data: data})
}

// WriteListSuccess writes a standard list data success response {"data": [...], "meta": {...}}.
func WriteListSuccess(w http.ResponseWriter, statusCode int, data any, total, limit, offset int) {
	WriteJSON(w, statusCode, ListSuccessEnvelope{
		Data: data,
		Meta: ListMeta{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	})
}

// WriteError writes a structured error response {"error": {"code": ..., "message": ..., "details": ...}}.
func WriteError(w http.ResponseWriter, statusCode int, code, message string, details any) {
	WriteJSON(w, statusCode, ErrorEnvelope{
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
