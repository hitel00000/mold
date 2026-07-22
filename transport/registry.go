package transport

import (
	"sync/atomic"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

type ResourceEntry struct {
	Resource *resource.Resource
	Store    storage.Store
}

// Registry maps table names to Resource IR and Storage instances.
type Registry struct {
	entries map[string]ResourceEntry // table_name -> ResourceEntry
}

// NewRegistry creates a new empty Registry instance.
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]ResourceEntry),
	}
}

// Register registers a Resource IR and its corresponding Store.
func (r *Registry) Register(res *resource.Resource, store storage.Store) {
	if res == nil || store == nil {
		return
	}
	r.entries[res.Table] = ResourceEntry{
		Resource: res,
		Store:    store,
	}
}

// Lookup finds a ResourceEntry by table name.
func (r *Registry) Lookup(table string) (ResourceEntry, bool) {
	entry, exists := r.entries[table]
	return entry, exists
}

// Entries returns a copy of all registered table entries for iteration.
func (r *Registry) Entries() map[string]ResourceEntry {
	if r == nil {
		return nil
	}
	copyMap := make(map[string]ResourceEntry)
	for k, v := range r.entries {
		copyMap[k] = v
	}
	return copyMap
}

// Router maintains an atomic pointer to the current Registry snapshot.
type Router struct {
	registryPointer atomic.Pointer[Registry]
	reloadFn        func() (*Registry, error)
}

// NewRouter initializes a Router with an initial Registry snapshot.
func NewRouter(initialRegistry *Registry) *Router {
	r := &Router{}
	if initialRegistry == nil {
		initialRegistry = NewRegistry()
	}
	r.registryPointer.Store(initialRegistry)
	return r
}

// SwapRegistry atomically replaces the active Registry snapshot.
func (r *Router) SwapRegistry(newRegistry *Registry) {
	if newRegistry == nil {
		newRegistry = NewRegistry()
	}
	r.registryPointer.Store(newRegistry)
}

// CurrentRegistry retrieves the active Registry snapshot atomically.
func (r *Router) CurrentRegistry() *Registry {
	return r.registryPointer.Load()
}

// SetReloadFunc configures the callback invoked on POST /_mold/reload.
func (r *Router) SetReloadFunc(fn func() (*Registry, error)) {
	r.reloadFn = fn
}
