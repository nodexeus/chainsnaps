package protocol

import (
	"context"
	"fmt"
	"sync"

	"github.com/yourusername/snapd/internal/config"
)

// ProtocolModule defines the interface for blockchain-specific metric collection
type ProtocolModule interface {
	// Name returns the protocol identifier (e.g., "ethereum", "arbitrum")
	Name() string

	// CollectMetrics executes protocol-specific RPC queries and returns metric data
	// Returns a map of metric names to values, or error if collection fails
	CollectMetrics(ctx context.Context, config config.NodeConfig) (map[string]interface{}, error)
}

// Registry manages protocol module registration and retrieval
type Registry struct {
	mu      sync.RWMutex
	modules map[string]ProtocolModule
}

// NewRegistry creates a new protocol registry
func NewRegistry() *Registry {
	return &Registry{
		modules: make(map[string]ProtocolModule),
	}
}

// Register adds a protocol module to the registry
func (r *Registry) Register(module ProtocolModule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := module.Name()
	if name == "" {
		return fmt.Errorf("protocol module name cannot be empty")
	}

	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("protocol module %s is already registered", name)
	}

	r.modules[name] = module
	return nil
}

// Get retrieves a protocol module by name
func (r *Registry) Get(name string) (ProtocolModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, exists := r.modules[name]
	if !exists {
		return nil, fmt.Errorf("protocol module %s is not registered", name)
	}

	return module, nil
}

// IsRegistered checks if a protocol module is registered
func (r *Registry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.modules[name]
	return exists
}

// List returns all registered protocol names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}
