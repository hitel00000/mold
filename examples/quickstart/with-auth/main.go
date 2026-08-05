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
		DBPath:      "./mold-quickstart-with-auth.db",
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

// signupHandler handles user registration. Since User.yaml sets `client_writable: false`
// and `default: "user"` on the `role` field, privilege escalation is automatically prevented
// by Mold runtime even with `permissions.create: public`.
// This custom handler exists specifically to issue a session cookie (`app.IssueSessionForUser`)
// immediately upon successful registration for instant auto-login.
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
		})
		if err != nil {
			http.Error(w, `{"error":{"code":"SIGNUP_FAILED","message":"`+err.Error()+`"}}`, http.StatusBadRequest)
			return
		}

		userID, ok := user["id"].(int64)
		if !ok {
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

// --- Post creation: force author_id from session -------------------------

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
