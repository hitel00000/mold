// Package main demonstrates how to add a public "signup" endpoint safely on
// top of a User resource whose generic REST create permission is locked down
// to role:admin.
//
// Why this file exists: Mold's IR has no field-level permission (see
// docs/ir-spec.md, section 5: "Field 단위 권한은 1차 스코프에서 제외"). If a
// User resource exposes a `role` field and permissions.create is `public`,
// any anonymous client can POST {"role": "admin", ...} directly to
// /api/users and self-elevate. The fix is to never expose the generic
// create endpoint publicly for a resource that carries a privileged field;
// instead, write a small application-level handler (same pattern as the
// Google OAuth callback glue in examples/drink-log-pilot) that whitelists
// exactly which fields it forwards to runtime.App.CreateRecord, and hardcodes
// the rest (role: "user" here).
//
// NOTE: as with the rest of examples/quickstart, this has not been compiled
// against the actual mold module in this environment. Verify method
// signatures (runtime.App.CreateRecord, runtime.App.IssueSessionForUser,
// runtime.App.ServeHTTP) against your checked-out repo.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/hitel00000/mold/runtime"
)

func main() {
	app, err := runtime.New(runtime.Config{
		ResourceDir: "./resources",
		DBPath:      "./mold-quickstart.db",
	})
	if err != nil {
		log.Fatalf("failed to start Mold: %v", err)
	}
	defer app.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/signup", signupHandler(app))
	mux.HandleFunc("/posts/create", createPostHandler(app))
	// Everything else (/api/*, /view/*, /_mold/*) falls through to Mold's
	// own handler. runtime.App implements http.Handler via ServeHTTP.
	mux.Handle("/", app)

	log.Println("listening on http://localhost:8080 (signup at /signup)")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// signupHandler only forwards email/password/name to CreateRecord. `role` is
// never read from the client request — it is always fixed to "user" here,
// so this endpoint cannot be used to mint an admin account.
func signupHandler(app *runtime.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req signupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"code":"INVALID_JSON","message":"failed to parse json body"}}`, http.StatusBadRequest)
			return
		}
		if req.Email == "" || req.Password == "" || req.Name == "" {
			http.Error(w, `{"error":{"code":"INVALID_INPUT","message":"email, password, name required"}}`, http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		user, err := app.CreateRecord(ctx, "User", map[string]any{
			"email":    req.Email,
			"password": req.Password,
			"name":     req.Name,
			"role":     "user", // hardcoded — never taken from the request
		})
		if err != nil {
			http.Error(w, `{"error":{"code":"SIGNUP_FAILED","message":"`+err.Error()+`"}}`, http.StatusBadRequest)
			return
		}

		userID, ok := user["id"].(int64)
		if !ok {
			// depending on the sqlite driver this may come back as int/float64;
			// adjust the type switch to match runtime.App.CreateRecord's actual
			// return type in your checked-out repo.
			http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"unexpected id type"}}`, http.StatusInternalServerError)
			return
		}

		cookie, _, err := app.IssueSessionForUser(ctx, userID, "user")
		if err != nil {
			http.Error(w, `{"error":{"code":"SESSION_ISSUE_FAILED","message":"`+err.Error()+`"}}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Set-Cookie", cookie)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"data": user})
	}
}

// --- Post creation: same problem, same fix -------------------------------
//
// Post's `author_id` field has the same gap as User's `role`: it's a plain
// field, not a server-managed one, so a logged-in client can set
// author_id to anyone's id when POSTing to /api/posts directly. The fix is
// identical — don't expose the generic create endpoint for this purpose;
// force the ownership field server-side from the session instead.
//
// We use app.SessionUser(r) — Mold's in-process Escape Hatch for session
// user lookup — to read the authenticated userID and role from the request.

type createPostRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func createPostHandler(app *runtime.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID, _, ok := app.SessionUser(r)
		if !ok {
			http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"login required"}}`, http.StatusUnauthorized)
			return
		}

		var req createPostRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"code":"INVALID_JSON","message":"failed to parse json body"}}`, http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		post, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title":     req.Title,
			"body":      req.Body,
			"author_id": userID, // always from session, never from the client
		})
		if err != nil {
			http.Error(w, `{"error":{"code":"CREATE_FAILED","message":"`+err.Error()+`"}}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"data": post})
	}
}