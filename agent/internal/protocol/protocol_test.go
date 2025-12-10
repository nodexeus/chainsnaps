package protocol

import (
	"context"
	"testing"

	"github.com/nodexeus/agent/internal/config"
)

// mockProtocolModule is a mock implementation for testing
type mockProtocolModule struct {
	name string
}

func (m *mockProtocolModule) Name() string {
	return m.name
}

func (m *mockProtocolModule) CollectMetrics(ctx context.Context, cfg config.NodeConfig) (map[string]interface{}, error) {
	return map[string]interface{}{"test": "value"}, nil
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	module := &mockProtocolModule{name: "test"}

	err := registry.Register(module)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Test duplicate registration
	err = registry.Register(module)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	module := &mockProtocolModule{name: "test"}

	err := registry.Register(module)
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	retrieved, err := registry.Get("test")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if retrieved.Name() != "test" {
		t.Errorf("expected name 'test', got '%s'", retrieved.Name())
	}

	// Test non-existent module
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent module")
	}
}

func TestRegistry_IsRegistered(t *testing.T) {
	registry := NewRegistry()
	module := &mockProtocolModule{name: "test"}

	if registry.IsRegistered("test") {
		t.Fatal("expected false for unregistered module")
	}

	err := registry.Register(module)
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	if !registry.IsRegistered("test") {
		t.Fatal("expected true for registered module")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()
	module1 := &mockProtocolModule{name: "test1"}
	module2 := &mockProtocolModule{name: "test2"}

	registry.Register(module1)
	registry.Register(module2)

	names := registry.List()
	if len(names) != 2 {
		t.Errorf("expected 2 modules, got %d", len(names))
	}

	// Check both names are present
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	if !found["test1"] || !found["test2"] {
		t.Error("expected both test1 and test2 in list")
	}
}

func TestEthereumModule_Name(t *testing.T) {
	module := NewEthereumModule()
	if module.Name() != "ethereum" {
		t.Errorf("expected name 'ethereum', got '%s'", module.Name())
	}
}

func TestArbitrumModule_Name(t *testing.T) {
	module := NewArbitrumModule()
	if module.Name() != "arbitrum" {
		t.Errorf("expected name 'arbitrum', got '%s'", module.Name())
	}
}
func TestEthereumModule_HexToInt64(t *testing.T) {
	module := NewEthereumModule()

	tests := []struct {
		name     string
		hexStr   string
		expected int64
		hasError bool
	}{
		{
			name:     "hex with 0x prefix",
			hexStr:   "0x16df431",
			expected: 23983153,
			hasError: false,
		},
		{
			name:     "hex without 0x prefix",
			hexStr:   "16df431",
			expected: 23983153,
			hasError: false,
		},
		{
			name:     "simple hex",
			hexStr:   "0xff",
			expected: 255,
			hasError: false,
		},
		{
			name:     "zero",
			hexStr:   "0x0",
			expected: 0,
			hasError: false,
		},
		{
			name:     "invalid hex",
			hexStr:   "0xgg",
			expected: 0,
			hasError: true,
		},
		{
			name:     "empty string",
			hexStr:   "",
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := module.hexToInt64(tt.hexStr)

			if tt.hasError {
				if err == nil {
					t.Errorf("expected error for input '%s', but got none", tt.hexStr)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input '%s': %v", tt.hexStr, err)
				}
				if result != tt.expected {
					t.Errorf("expected %d for input '%s', got %d", tt.expected, tt.hexStr, result)
				}
			}
		})
	}
}
