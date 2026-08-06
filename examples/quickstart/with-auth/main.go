package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/hitel00000/mold/authglue"
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
	mux.HandleFunc("/signup", authglue.SignupHandler(app))
	mux.HandleFunc("/auth/google/callback", authglue.OAuthCallbackHandler(app, "google", authglue.UnsafeTestStubOAuthVerifier("google")))
	mux.HandleFunc("/posts/create", createPostHandler(app))
	// Everything else (/api/*, /view/*, /_mold/*) falls through to Mold's
	// own handler. runtime.App implements http.Handler via ServeHTTP.
	mux.Handle("/", app)

	log.Println("listening on http://localhost:8080 (signup at /signup, OAuth callback at /auth/google/callback)")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
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
		_ = json.NewEncoder(w).Encode(map[string]any{"data": post})
	}
}
