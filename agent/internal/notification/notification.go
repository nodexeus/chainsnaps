package notification

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NotificationEvent represents the type of event triggering a notification
type NotificationEvent string

const (
	EventFailure  NotificationEvent = "failure"
	EventSkip     NotificationEvent = "skip"
	EventComplete NotificationEvent = "complete"
)

// NotificationPayload contains event details for notification delivery
type NotificationPayload struct {
	Event     NotificationEvent      `json:"event"`
	NodeName  string                 `json:"node_name"`
	Timestamp time.Time              `json:"timestamp"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
}

// NotificationModule defines the interface for notification delivery
type NotificationModule interface {
	// Name returns the notification type identifier (e.g., "discord", "slack")
	Name() string

	// Send delivers a notification using the configured URL
	Send(ctx context.Context, url string, payload NotificationPayload) error
}

// Registry manages notification module registration and retrieval
type Registry struct {
	mu      sync.RWMutex
	modules map[string]NotificationModule
}

// NewRegistry creates a new notification registry
func NewRegistry() *Registry {
	return &Registry{
		modules: make(map[string]NotificationModule),
	}
}

// Register adds a notification module to the registry
func (r *Registry) Register(module NotificationModule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := module.Name()
	if name == "" {
		return fmt.Errorf("notification module name cannot be empty")
	}

	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("notification module %s is already registered", name)
	}

	r.modules[name] = module
	return nil
}

// Get retrieves a notification module by name
func (r *Registry) Get(name string) (NotificationModule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, exists := r.modules[name]
	if !exists {
		return nil, fmt.Errorf("notification module %s is not registered", name)
	}

	return module, nil
}

// IsRegistered checks if a notification module is registered
func (r *Registry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.modules[name]
	return exists
}

// List returns all registered notification type names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}
