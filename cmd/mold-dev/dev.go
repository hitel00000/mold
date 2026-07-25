package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hitel00000/mold/runtime"
)

// DevConfig configures the mold dev server.
type DevConfig struct {
	ResourceDir   string
	DBPath        string
	BlobDir       string
	Addr          string
	AdminEmail    string
	AdminPass     string
	SessionCookie string
	DebounceMs    int
	PollMs        int
}

// DevServer manages the runtime app, HTTP server, watcher, and reload client.
type DevServer struct {
	config  DevConfig
	app     *runtime.App
	server  *http.Server
	listen  net.Listener
	watcher *ResourceWatcher
	client  *ReloadClient

	mu        sync.Mutex
	running   bool
	baseURL   string
	closeOnce sync.Once
}

// NewDevServer initializes and configures a DevServer instance.
func NewDevServer(cfg DevConfig) (*DevServer, error) {
	if cfg.ResourceDir == "" {
		return nil, fmt.Errorf("ResourceDir must be specified")
	}
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("DBPath must be specified")
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0" // Random port by default if unspecified
	}
	if cfg.AdminEmail == "" {
		cfg.AdminEmail = "admin@mold.dev"
	}
	if cfg.AdminPass == "" {
		cfg.AdminPass = "adminpassword123"
	}
	if cfg.DebounceMs <= 0 {
		cfg.DebounceMs = 300
	}
	if cfg.PollMs <= 0 {
		cfg.PollMs = 200
	}

	app, err := runtime.New(runtime.Config{
		ResourceDir: cfg.ResourceDir,
		DBPath:      cfg.DBPath,
		BlobDir:     cfg.BlobDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to construct runtime app: %w", err)
	}

	return &DevServer{
		config: cfg,
		app:    app,
	}, nil
}

// Start opens the HTTP listener, starts background serving, authenticates session, and starts the file watcher.
func (ds *DevServer) Start() error {
	ln, err := net.Listen("tcp", ds.config.Addr)
	if err != nil {
		ds.app.Close()
		return fmt.Errorf("failed to listen on %s: %w", ds.config.Addr, err)
	}
	ds.listen = ln

	addrStr := ln.Addr().String()
	if strings.HasPrefix(addrStr, "[::]:") || strings.HasPrefix(addrStr, "0.0.0.0:") {
		parts := strings.Split(addrStr, ":")
		addrStr = "127.0.0.1:" + parts[len(parts)-1]
	}
	ds.baseURL = "http://" + addrStr

	ds.server = &http.Server{
		Handler: ds.app,
	}

	go func() {
		_ = ds.server.Serve(ln)
	}()

	client, err := NewReloadClient(ds.baseURL)
	if err != nil {
		ds.Close()
		return fmt.Errorf("failed to create reload client: %w", err)
	}
	ds.client = client

	// Setup authentication
	if ds.config.SessionCookie != "" {
		if err := client.SetSessionCookie(ds.config.SessionCookie); err != nil {
			log.Printf("[mold dev] Option B session cookie setup failed: %v", err)
		} else {
			log.Printf("[mold dev] Option B: session cookie directly attached")
		}
	} else {
		// Option A: attempt login via /login
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := client.Login(ctx, ds.config.AdminEmail, ds.config.AdminPass); err != nil {
			log.Printf("[mold dev] Option A: admin session login at %s/login not established (%v); reload will proceed with client jar", ds.baseURL, err)
		} else {
			log.Printf("[mold dev] Option A: authenticated as %s via /login", ds.config.AdminEmail)
		}
	}

	// Create and start file watcher
	ds.watcher = NewResourceWatcher(
		ds.config.ResourceDir,
		time.Duration(ds.config.PollMs)*time.Millisecond,
		time.Duration(ds.config.DebounceMs)*time.Millisecond,
		ds.handleFileChanges,
	)

	if err := ds.watcher.Start(); err != nil {
		ds.Close()
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	ds.running = true
	log.Printf("[mold dev] Listening on %s, watching %s (debounce: %dms)", ds.baseURL, ds.config.ResourceDir, ds.config.DebounceMs)
	return nil
}

func (ds *DevServer) handleFileChanges(changedFiles []string) {
	log.Printf("[mold dev] File change detected: [%s]", strings.Join(changedFiles, ", "))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := ds.client.TriggerReload(ctx)
	if err != nil {
		log.Printf("[mold dev] Reload HTTP call failed: %v", err)
		return
	}

	if resp.Success {
		log.Printf("[mold dev] Reload SUCCESS (%d OK) -> %s", resp.StatusCode, resp.Message)
	} else {
		log.Printf("[mold dev] Reload FAILED (%d) -> %s", resp.StatusCode, resp.Message)
	}
}

// BaseURL returns the HTTP server base URL.
func (ds *DevServer) BaseURL() string {
	return ds.baseURL
}

// App returns the underlying runtime app instance.
func (ds *DevServer) App() *runtime.App {
	return ds.app
}

// Close shuts down the server, watcher, and database cleanly.
func (ds *DevServer) Close() error {
	ds.closeOnce.Do(func() {
		if ds.watcher != nil {
			ds.watcher.Stop()
		}
		if ds.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = ds.server.Shutdown(ctx)
		}
		if ds.app != nil {
			_ = ds.app.Close()
		}
		ds.running = false
	})
	return nil
}
