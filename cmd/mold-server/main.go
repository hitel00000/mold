package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hitel00000/mold/runtime"
)

func main() {
	resourceDir := filepath.Join("..", "drink-log", "resources")
	dbPath := filepath.Join(os.TempDir(), "mold_drink_log_e2e_server.db")
	_ = os.Remove(dbPath)

	cfg := runtime.Config{ResourceDir: resourceDir, DBPath: dbPath}
	app, err := runtime.New(cfg)
	if err != nil {
		log.Fatalf("Failed to bootstrap Mold runtime: %v", err)
	}
	defer app.Close()

	// Seed User #1 for E2E authentication
	ctx := context.Background()
	_, err = app.CreateRecord(ctx, "User", map[string]any{
		"provider":         "google",
		"provider_user_id": "google-user-12345",
		"email":            "tester@example.com",
		"display_name":     "Tester User",
	})
	if err != nil {
		log.Printf("Warning seeding user: %v", err)
	}

	cookieVal, _, err := app.IssueSessionForUser(ctx, 1, "user")
	if err != nil {
		log.Fatalf("Failed to issue session: %v", err)
	}

	// Dynamic handler wrapper for CORS & Auth Injection for Frontend E2E
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Inject valid session cookie if missing for seamless E2E testing
		if r.Header.Get("Cookie") == "" {
			r.Header.Set("Cookie", cookieVal)
		}

		app.ServeHTTP(w, r)
	})

	port := 8787
	log.Printf("Starting Mold Native Server on http://127.0.0.1:%d ...", port)
	if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), handler); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
