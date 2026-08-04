package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	resReg      *resource.Registry
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
	a.resReg = resReg
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
	a.resReg = newResReg
	a.mu.Unlock()

	return newTransReg, nil
}

// CreateRecord creates a new record for the specified resource by its Resource Name (e.g. "User", "Post").
// It retrieves the registered Resource IR from the internal Registry and delegates
// record creation to the underlying Store.
//
// Contract & Rationale:
// - Single Contract: Accepts ONLY the canonical Resource Name (res.Name in IR). It intentionally
//   does NOT fallback to SQL table names (res.Table) in adherence to Mold's core philosophy
//   ("Resource is the single source of truth", "Opinionated Framework: consistency over flexibility",
//   and the Maserati Principle against premature multi-way fallback lookups).
// - Verification Layer Separation: Checks schema existence in Registry before storage call.
//   If missing, wraps runtime.ErrResourceNotFound sentinel error so callers can programmatically
//   distinguish missing schemas from record validation errors.
// - Record Validation: Downstream record data validation is delegated to storage.Store.Create.
func (a *App) CreateRecord(ctx context.Context, resourceName string, record map[string]any) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	a.mu.RLock()
	resReg := a.resReg
	a.mu.RUnlock()

	if resReg == nil {
		return nil, ErrNotInitialized
	}

	// Schema existence check: verify if resource definition exists in Registry by Resource Name
	res, ok := resReg.Get(resourceName)
	if !ok || res == nil {
		return nil, fmt.Errorf("%w: %q", ErrResourceNotFound, resourceName)
	}

	created, err := a.store.Create(ctx, res, record)
	if err != nil {
		return nil, err
	}

	return created, nil
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

// IssueSessionForUser issues a session cookie value string and expiration time for an authenticated user ID and role.
// This is an in-process Escape Hatch for external authentication (e.g. OAuth verification handled outside Mold).
//
// Security Rationale & Trust Boundary:
// - In-process API ONLY: This method is intentionally NOT registered to any HTTP router endpoints.
//   It must be invoked directly by trusted server-side Go application code within the same process boundary.
// - Parity & Cookie Attributes: Returns the formatted cookie value string for '_mold_session' matching
//   Cloudflare target attributes (Expires, Max-Age, HttpOnly, Secure, SameSite=Lax) suitable for Set-Cookie header.
func (a *App) IssueSessionForUser(ctx context.Context, userID int64, role string) (string, time.Time, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if a.sessionMgr == nil {
		return "", time.Time{}, ErrNotInitialized
	}

	token, exp, err := a.sessionMgr.CreateSessionForUser(ctx, userID, role)
	if err != nil {
		return "", time.Time{}, err
	}

	maxAge := int(time.Until(exp).Seconds())
	cookieVal := fmt.Sprintf("%s=%s; Path=/; Expires=%s; Max-Age=%d; HttpOnly; Secure; SameSite=Lax",
		auth.SessionCookieName, token, exp.UTC().Format(http.TimeFormat), maxAge)
	return cookieVal, exp, nil
}

// SessionUser inspects the HTTP request's session cookie (_mold_session) and returns the authenticated user's ID and role.
// This is the read-direction in-process Escape Hatch for application-level glue handlers (e.g. /posts/create, /signup)
// to extract session identity and enforce server-side ownership fields.
//
// Signature & Return Values:
// - (userID int64, role string, ok bool)
// - Returns ok = false when the request is unauthenticated, the session cookie is missing/invalid/expired,
//   or the user ID cannot be parsed as int64.
// - Returns ok = true, userID, role when a valid active session exists.
//
// Security Rationale & Role Freshness:
// - In-process API ONLY: This method is intentionally NOT registered as an HTTP router endpoint.
//   It is invoked directly by trusted server-side Go handler code within the same process boundary.
// - Session-Cached Role Resolution: Returns the role cached in the '_mold_sessions' table. This adheres to Mold's
//   single-source-of-truth and vendor-independent identity design (supporting OAuth and non-local user sessions
//   without mandatory table joins or unexpected DB schema couplings).
func (a *App) SessionUser(r *http.Request) (userID int64, role string, ok bool) {
	if a == nil || a.sessionMgr == nil || r == nil {
		return 0, "", false
	}

	sess, err := a.sessionMgr.GetSessionFromRequest(r)
	if err != nil || sess == nil {
		return 0, "", false
	}

	var parsedID int64
	switch v := sess.UserID.(type) {
	case int64:
		parsedID = v
	case float64:
		parsedID = int64(v)
	case int:
		parsedID = int64(v)
	case string:
		idVal, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil {
			return 0, "", false
		}
		parsedID = idVal
	default:
		return 0, "", false
	}

	return parsedID, sess.Role, true
}

