package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var cfg DevConfig

	flag.StringVar(&cfg.ResourceDir, "dir", "./resources", "Path to resource YAML directory")
	flag.StringVar(&cfg.DBPath, "db", "./mold.db", "Path to SQLite database file")
	flag.StringVar(&cfg.BlobDir, "blob", "", "Path to Blob storage directory (optional)")
	flag.StringVar(&cfg.Addr, "addr", "127.0.0.1:8080", "Address to listen on")
	flag.StringVar(&cfg.AdminEmail, "admin-email", "admin@mold.dev", "Admin user email for login (Option A)")
	flag.StringVar(&cfg.AdminPass, "admin-pass", "adminpassword123", "Admin user password for login (Option A)")
	flag.StringVar(&cfg.SessionCookie, "session-cookie", "", "Direct session cookie value (Option B)")
	flag.IntVar(&cfg.DebounceMs, "debounce", 300, "Debounce delay in milliseconds")
	flag.IntVar(&cfg.PollMs, "poll", 200, "Watcher polling interval in milliseconds")

	flag.Parse()

	if envCookie := os.Getenv("MOLD_SESSION_COOKIE"); envCookie != "" && cfg.SessionCookie == "" {
		cfg.SessionCookie = envCookie
	}

	ds, err := NewDevServer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing mold dev server: %v\n", err)
		os.Exit(1)
	}

	if err := ds.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting mold dev server: %v\n", err)
		os.Exit(1)
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	<-stopCh

	log.Println("[mold dev] Shutting down...")
	if err := ds.Close(); err != nil {
		log.Printf("[mold dev] Error during shutdown: %v\n", err)
	}
	log.Println("[mold dev] Goodbye.")
}
