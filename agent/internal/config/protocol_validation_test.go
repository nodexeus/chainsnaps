package config

import (
	"os"
	"testing"
)

// mockProtocolValidator is a mock implementation for testing
type mockProtocolValidator struct {
	registered map[string]bool
}

func (m *mockProtocolValidator) IsRegistered(name string) bool {
	return m.registered[name]
}

// mockNotificationValidator is a mock implementation for testing
type mockNotificationValidator struct {
	registered map[string]bool
}

func (m *mockNotificationValidator) IsRegistered(name string) bool {
	return m.registered[name]
}

func TestProtocolValidation(t *testing.T) {
	// Create a temporary config file
	configContent := `
schedule: "0 * * * * *"
database:
  host: localhost
  port: 5432
  database: testdb
  user: testuser
  password: testpass
  ssl_mode: disable
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Test with registered protocol
	SetProtocolValidator(&mockProtocolValidator{
		registered: map[string]bool{"ethereum": true},
	})

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error with registered protocol, got %v", err)
	}

	if config.Nodes["test-node"].Protocol != "ethereum" {
		t.Errorf("expected protocol 'ethereum', got '%s'", config.Nodes["test-node"].Protocol)
	}

	// Test with unregistered protocol
	SetProtocolValidator(&mockProtocolValidator{
		registered: map[string]bool{},
	})

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error with unregistered protocol")
	}

	// Reset validator
	SetProtocolValidator(nil)
}

func TestNotificationValidation(t *testing.T) {
	// Create a temporary config file with notifications
	configContent := `
schedule: "0 * * * * *"
notifications:
  failure: true
  skip: false
  complete: true
  discord:
    url: https://discord.com/api/webhooks/test
database:
  host: localhost
  port: 5432
  database: testdb
  user: testuser
  password: testpass
  ssl_mode: disable
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(configContent); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	tmpFile.Close()

	// Test with registered notification type
	SetNotificationValidator(&mockNotificationValidator{
		registered: map[string]bool{"discord": true},
	})

	config, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("expected no error with registered notification type, got %v", err)
	}

	if config.Notifications.GetNotificationURL("discord") != "https://discord.com/api/webhooks/test" {
		t.Errorf("expected discord notification URL, got '%s'", config.Notifications.GetNotificationURL("discord"))
	}

	// Test with unregistered notification type
	SetNotificationValidator(&mockNotificationValidator{
		registered: map[string]bool{},
	})

	_, err = LoadConfig(tmpFile.Name())
	if err == nil {
		t.Fatal("expected error with unregistered notification type")
	}

	// Reset validator
	SetNotificationValidator(nil)
}
