package view

import (
	"fmt"
	"html/template"
	"sync"
)

// TemplateOverrides is a persistent registry for custom HTML template overrides.
// It survives across atomic reloads (POST /_mold/reload) and is shared across ViewHandler instances.
type TemplateOverrides struct {
	mu        sync.RWMutex
	overrides map[string]map[string]*template.Template
}

// NewTemplateOverrides initializes a new persistent TemplateOverrides registry.
func NewTemplateOverrides() *TemplateOverrides {
	return &TemplateOverrides{
		overrides: make(map[string]map[string]*template.Template),
	}
}

// SetCustomTemplateString parses and registers a custom template string for a target resource table and viewType ("list", "detail", "form").
// It uses template.Clone() based on Mold's baseLayout to guarantee 100% template tree isolation per resource.
func (to *TemplateOverrides) SetCustomTemplateString(table, viewType, tplStr string) error {
	if table == "" || viewType == "" {
		return fmt.Errorf("table and viewType must not be empty")
	}

	base, err := createBaseTemplate()
	if err != nil {
		return fmt.Errorf("failed to create base template for override: %w", err)
	}

	cloned, err := base.Clone()
	if err != nil {
		return fmt.Errorf("failed to clone base template: %w", err)
	}

	parsed, err := cloned.Parse(tplStr)
	if err != nil {
		return fmt.Errorf("failed to parse custom template string for table '%s', viewType '%s': %w", table, viewType, err)
	}

	to.mu.Lock()
	defer to.mu.Unlock()

	if to.overrides[table] == nil {
		to.overrides[table] = make(map[string]*template.Template)
	}
	to.overrides[table][viewType] = parsed
	return nil
}

// Get returns the custom template for table and viewType, or nil if none is registered.
func (to *TemplateOverrides) Get(table, viewType string) *template.Template {
	if to == nil {
		return nil
	}
	to.mu.RLock()
	defer to.mu.RUnlock()

	if resMap, exists := to.overrides[table]; exists {
		return resMap[viewType]
	}
	return nil
}
