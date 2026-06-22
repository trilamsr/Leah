package widget

import (
	"fmt"
	"sync"
)

// Registry maps widget kind → adapter. Thread-safe so Wave 4 fsnotify
// hot-reload can swap adapters without daemon restart.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]WidgetAdapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]WidgetAdapter)}
}

// Register binds an adapter to its Type(). Errors on duplicate kind.
func (r *Registry) Register(a WidgetAdapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kind := a.Type()
	if _, dup := r.adapters[kind]; dup {
		return fmt.Errorf("widget kind %q already registered", kind)
	}
	r.adapters[kind] = a
	return nil
}

// Lookup returns the adapter for kind, ok=false if absent.
func (r *Registry) Lookup(kind string) (WidgetAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[kind]
	return a, ok
}
