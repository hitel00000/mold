package resource

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry maintains an in-memory map of registered Resource IRs.
type Registry struct {
	mu        sync.RWMutex
	resources map[string]*Resource
}

// NewRegistry initializes an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		resources: make(map[string]*Resource),
	}
}

// Register adds a Resource IR to the Registry.
func (r *Registry) Register(res *Resource) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if res == nil || res.Name == "" {
		return fmt.Errorf("cannot register resource with empty name")
	}
	if _, exists := r.resources[res.Name]; exists {
		return fmt.Errorf("resource '%s' is already registered", res.Name)
	}
	r.resources[res.Name] = res
	return nil
}

// Get retrieves a Resource IR by its name.
func (r *Registry) Get(name string) (*Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, ok := r.resources[name]
	return res, ok
}

// List returns a slice of all registered Resource IRs.
func (r *Registry) List() []*Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Resource, 0, len(r.resources))
	for _, res := range r.resources {
		list = append(list, res)
	}
	return list
}

// LoadAll loads all YAML files in the given directory, validates each resource,
// checks cross-resource relation targets, and populates a new Registry atomically.
func LoadAll(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	tempMap := make(map[string]*Resource)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		res, err := LoadFromFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %w", entry.Name(), err)
		}

		if err := Validate(res); err != nil {
			return nil, fmt.Errorf("validation failed for %s: %w", entry.Name(), err)
		}

		if _, duplicate := tempMap[res.Name]; duplicate {
			return nil, fmt.Errorf("duplicate resource name '%s' found in %s", res.Name, entry.Name())
		}
		tempMap[res.Name] = res
	}

	// Validate cross-resource relations
	for _, res := range tempMap {
		err := ValidateTargetResources(res, func(target string) bool {
			_, exists := tempMap[target]
			return exists
		})
		if err != nil {
			return nil, err
		}
	}

	reg := NewRegistry()
	for _, res := range tempMap {
		if err := reg.Register(res); err != nil {
			return nil, err
		}
	}

	return reg, nil
}
