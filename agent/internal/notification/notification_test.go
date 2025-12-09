package notification

import (
	"context"
	"testing"
	"time"
)

// MockNotificationModule is a mock implementation for testing
type MockNotificationModule struct {
	name        string
	sendError   error
	lastURL     string
	lastPayload NotificationPayload
}

func (m *MockNotificationModule) Name() string {
	return m.name
}

func (m *MockNotificationModule) Send(ctx context.Context, url string, payload NotificationPayload) error {
	m.lastURL = url
	m.lastPayload = payload
	return m.sendError
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if registry.modules == nil {
		t.Fatal("Registry modules map is nil")
	}
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name        string
		moduleName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid module",
			moduleName: "test",
			wantErr:    false,
		},
		{
			name:        "empty name",
			moduleName:  "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			module := &MockNotificationModule{name: tt.moduleName}

			err := registry.Register(module)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("Register() error = %v, should contain %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	registry := NewRegistry()
	module1 := &MockNotificationModule{name: "test"}
	module2 := &MockNotificationModule{name: "test"}

	err := registry.Register(module1)
	if err != nil {
		t.Fatalf("First Register() failed: %v", err)
	}

	err = registry.Register(module2)
	if err == nil {
		t.Fatal("Register() should fail for duplicate module")
	}
	if !contains(err.Error(), "already registered") {
		t.Errorf("Register() error = %v, should contain 'already registered'", err)
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	module := &MockNotificationModule{name: "test"}
	registry.Register(module)

	tests := []struct {
		name        string
		moduleName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "existing module",
			moduleName: "test",
			wantErr:    false,
		},
		{
			name:        "non-existing module",
			moduleName:  "nonexistent",
			wantErr:     true,
			errContains: "not registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registry.Get(tt.moduleName)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got == nil {
				t.Error("Get() returned nil module")
			}
			if err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("Get() error = %v, should contain %v", err, tt.errContains)
				}
			}
		})
	}
}

func TestRegistry_IsRegistered(t *testing.T) {
	registry := NewRegistry()
	module := &MockNotificationModule{name: "test"}
	registry.Register(module)

	tests := []struct {
		name       string
		moduleName string
		want       bool
	}{
		{
			name:       "registered module",
			moduleName: "test",
			want:       true,
		},
		{
			name:       "unregistered module",
			moduleName: "nonexistent",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registry.IsRegistered(tt.moduleName)
			if got != tt.want {
				t.Errorf("IsRegistered() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	// Empty registry
	list := registry.List()
	if len(list) != 0 {
		t.Errorf("List() on empty registry = %v, want empty slice", list)
	}

	// Add modules
	registry.Register(&MockNotificationModule{name: "discord"})
	registry.Register(&MockNotificationModule{name: "slack"})

	list = registry.List()
	if len(list) != 2 {
		t.Errorf("List() length = %d, want 2", len(list))
	}

	// Check both modules are in the list
	hasDiscord := false
	hasSlack := false
	for _, name := range list {
		if name == "discord" {
			hasDiscord = true
		}
		if name == "slack" {
			hasSlack = true
		}
	}
	if !hasDiscord || !hasSlack {
		t.Errorf("List() = %v, should contain both 'discord' and 'slack'", list)
	}
}

func TestNotificationPayload(t *testing.T) {
	now := time.Now()
	payload := NotificationPayload{
		Event:     EventComplete,
		NodeName:  "test-node",
		Timestamp: now,
		Message:   "Test message",
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	if payload.Event != EventComplete {
		t.Errorf("Event = %v, want %v", payload.Event, EventComplete)
	}
	if payload.NodeName != "test-node" {
		t.Errorf("NodeName = %v, want test-node", payload.NodeName)
	}
	if payload.Timestamp != now {
		t.Errorf("Timestamp = %v, want %v", payload.Timestamp, now)
	}
	if payload.Message != "Test message" {
		t.Errorf("Message = %v, want 'Test message'", payload.Message)
	}
	if payload.Details["key"] != "value" {
		t.Errorf("Details[key] = %v, want 'value'", payload.Details["key"])
	}
}

func TestNotificationEvent_Constants(t *testing.T) {
	if EventFailure != "failure" {
		t.Errorf("EventFailure = %v, want 'failure'", EventFailure)
	}
	if EventSkip != "skip" {
		t.Errorf("EventSkip = %v, want 'skip'", EventSkip)
	}
	if EventComplete != "complete" {
		t.Errorf("EventComplete = %v, want 'complete'", EventComplete)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
