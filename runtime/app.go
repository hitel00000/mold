package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/hitel00000/mold/adapters/fsblob"
	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
	"github.com/hitel00000/mold/view"
)

// App is the bootstrapped Mold application container encapsulation.
type App struct {
	config     Config
	store      *sqlite.Store
	sessionMgr *auth.SessionManager
	blobStore  storage.BlobStore

	mu          sync.RWMutex
	router      *transport.Router
	viewHandler *view.ViewHandler
}

// New initializes and bootstraps a new App instance using the given Config.
func New(cfg Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	store, err := sqlite.Open(cfg.DBPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("runtime: failed to open sqlite database: %w", err)
	}

	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("runtime: failed to initialize session manager: %w", err)
	}

	var bs storage.BlobStore
	if cfg.BlobDir != "" {
		bs, err = fsblob.New(cfg.BlobDir)
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("runtime: failed to initialize blob store: %w", err)
		}
	}

	app := &App{
		config:     cfg,
		store:      store,
		sessionMgr: sm,
		blobStore:  bs,
	}

	if err := app.buildAndAttach(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}

	return app, nil
}

// buildAndAttach loads resources, ensures DB schemas, wires up router & view handler,
// and sets up the atomic reload function callback.
func (a *App) buildAndAttach(ctx context.Context) error {
	resReg, err := resource.LoadAll(a.config.ResourceDir)
	if err != nil {
		return fmt.Errorf("runtime: failed to load resources: %w", err)
	}

	transReg := transport.NewRegistry()
	for _, r := range resReg.List() {
		if err := a.store.EnsureSchema(ctx, r); err != nil {
			return fmt.Errorf("runtime: failed to ensure schema for %s: %w", r.Name, err)
		}
		transReg.Register(r, a.store)
	}

	router := transport.NewRouter(transReg)
	router.SetSessionManager(a.sessionMgr)
	if a.blobStore != nil {
		router.SetBlobStore(a.blobStore)
	}

	vh, err := view.NewViewHandler(router, a.config.Overrides)
	if err != nil {
		return fmt.Errorf("runtime: failed to initialize view handler: %w", err)
	}

	router.SetReloadFunc(func() (*transport.Registry, error) {
		return a.reload()
	})

	a.mu.Lock()
	a.router = router
	a.viewHandler = vh
	a.mu.Unlock()

	return nil
}

// reload handles atomic resource reloading triggered via POST /_mold/reload.
func (a *App) reload() (*transport.Registry, error) {
	ctx := context.Background()
	newResReg, err := resource.LoadAll(a.config.ResourceDir)
	if err != nil {
		return nil, err
	}

	newTransReg := transport.NewRegistry()
	for _, r := range newResReg.List() {
		if err := a.store.EnsureSchema(ctx, r); err != nil {
			return nil, err
		}
		newTransReg.Register(r, a.store)
	}

	newRouter := transport.NewRouter(newTransReg)
	newRouter.SetSessionManager(a.sessionMgr)
	if a.blobStore != nil {
		newRouter.SetBlobStore(a.blobStore)
	}

	newVh, err := view.NewViewHandler(newRouter, a.config.Overrides)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.router = newRouter
	a.viewHandler = newVh
	a.mu.Unlock()

	return newTransReg, nil
}

// ServeHTTP implements http.Handler for App, dispatching requests to API router or HTML view handler.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	router := a.router
	vh := a.viewHandler
	a.mu.RUnlock()

	if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/_mold") {
		router.ServeHTTP(w, r)
	} else {
		vh.ServeHTTP(w, r)
	}
}

// Listen starts an HTTP server on the specified address.
func (a *App) Listen(addr string) error {
	return http.ListenAndServe(addr, a)
}

// Close releases resources associated with the App container (such as the database connection).
func (a *App) Close() error {
	if a.store != nil {
		return a.store.Close()
	}
	return nil
}

// Store returns the underlying sqlite store (useful for seeding or direct store access in tests/setup).
func (a *App) Store() *sqlite.Store {
	return a.store
}
