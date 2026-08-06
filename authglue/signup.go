package authglue

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
)

// SignupHandler returns an http.HandlerFunc that handles user registration (`POST /signup`).
// It uses app.CreateRecord to create the User record, app.SanitizeRecord to remove sensitive
// password hashes from the response, and app.IssueSessionForUser to issue a session cookie
// for immediate auto-login.
//
// Security Rationale:
//   - Role Escalation Prevention: User.yaml specifies `client_writable: false` and `default: "user"`
//     for the `role` field. If a client attempts to pass `"role": "admin"` in the raw payload,
//     app.CreateRecord returns `resource.ErrClientWriteForbidden`, which this handler converts
//     into an HTTP 400 Bad Request (`CLIENT_WRITE_FORBIDDEN`).
//   - Response Sanitization: The user record returned by app.CreateRecord is sanitized using
//     app.SanitizeRecord("User", user) before JSON serialization, preventing password hash leakage.
func SignupHandler(app *runtime.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}

		var rawPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&rawPayload); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "failed to parse json body")
			return
		}
		if rawPayload == nil {
			rawPayload = make(map[string]any)
		}

		// Basic required field checks for email signup
		emailVal, _ := rawPayload["email"].(string)
		passVal, _ := rawPayload["password"].(string)
		if emailVal == "" || passVal == "" {
			writeError(w, http.StatusBadRequest, "INVALID_INPUT", "email and password are required")
			return
		}

		ctx := r.Context()
		created, err := app.CreateRecord(ctx, "User", rawPayload)
		if err != nil {
			if errors.Is(err, resource.ErrClientWriteForbidden) {
				writeError(w, http.StatusBadRequest, "CLIENT_WRITE_FORBIDDEN", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "SIGNUP_FAILED", err.Error())
			return
		}

		// Sanitize record to prevent sensitive/deprecated fields (e.g. password hash) from leaking
		sanitized, err := app.SanitizeRecord("User", created)
		if err != nil {
			sanitized = created
			delete(sanitized, "password")
		}

		userID, ok := extractUserID(sanitized["id"])
		if !ok {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "unexpected user id type")
			return
		}

		userRole, _ := sanitized["role"].(string)
		if userRole == "" {
			userRole = "user"
		}

		cookieVal, _, err := app.IssueSessionForUser(ctx, userID, userRole)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "SESSION_ISSUE_FAILED", err.Error())
			return
		}

		w.Header().Set("Set-Cookie", cookieVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": sanitized})
	}
}

func extractUserID(val any) (int64, bool) {
	switch v := val.(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}
