package config

import (
	"os"
	"path/filepath"
	"testing"
)

// MockNotificationValidator is a mock implementation for testing
type MockNotificationValidator struct {
	registered map[string]bool
}

func (m *MockNotificationValidator) IsRegistered(name string) bool {
	return m.registered[name]
}

func TestNotificationValidation_WithValidator(t *testing.T) {
	// Set up mock validator
	validator := &MockNotificationValidator{
		registered: map[string]bool{
			"discord": true,
			"slack":   true,
		},
	}
	SetNotificationValidator(validator)
	defer SetNotificationValidator(nil)

	tests := []struct {
		name    string
		config  NotificationConfig
		wantErr bool
	}{
		{
			name: "registered notification type",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple registered notification types",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
					"slack":   {URL: "https://hooks.slack.com/services/test"},
				},
			},
			wantErr: false,
		},
		{
			name: "unregistered notification type",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"unregistered": {URL: "https://example.com/webhook"},
				},
			},
			wantErr: true,
		},
		{
			name: "mixed registered and unregistered types",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord":      {URL: "https://discord.com/api/webhooks/test"},
					"unregistered": {URL: "https://example.com/webhook"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty types map",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{},
			},
			wantErr: true,
		},
		{
			name: "missing URL for type",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: ""},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("NotificationConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_WithNotificationValidation(t *testing.T) {
	// Set up mock validator
	validator := &MockNotificationValidator{
		registered: map[string]bool{
			"discord": true,
		},
	}
	SetNotificationValidator(validator)
	defer SetNotificationValidator(nil)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with unregistered notification type
	configContent := `
schedule: "0 * * * * *"
notifications:
  failure: true
  skip: false
  complete: true
  unregistered:
    url: https://example.com/webhook
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: testpass
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for unregistered notification type, got nil")
	}
}

func TestLoadConfig_WithNodeNotificationValidation(t *testing.T) {
	// Set up mock validator
	validator := &MockNotificationValidator{
		registered: map[string]bool{
			"discord": true,
		},
	}
	SetNotificationValidator(validator)
	defer SetNotificationValidator(nil)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with unregistered notification type in node config
	configContent := `
schedule: "0 * * * * *"
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: testpass
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
    notifications:
      failure: true
      skip: false
      complete: true
      unregistered:
        url: https://example.com/webhook
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for unregistered notification type in node config, got nil")
	}
}

func TestLoadConfig_ValidNotificationTypes(t *testing.T) {
	// Set up mock validator
	validator := &MockNotificationValidator{
		registered: map[string]bool{
			"discord": true,
			"slack":   true,
		},
	}
	SetNotificationValidator(validator)
	defer SetNotificationValidator(nil)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with valid notification types
	configContent := `
schedule: "0 * * * * *"
notifications:
  failure: true
  skip: false
  complete: true
  discord:
    url: https://discord.com/api/webhooks/global
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: testpass
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
    notifications:
      failure: false
      skip: true
      complete: false
      slack:
        url: https://hooks.slack.com/services/test
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify global notifications
	if config.Notifications.GetNotificationURL("discord") != "https://discord.com/api/webhooks/global" {
		t.Errorf("Expected global discord URL, got '%s'", config.Notifications.GetNotificationURL("discord"))
	}

	// Verify node notifications
	node := config.Nodes["test-node"]
	if node.Notifications.GetNotificationURL("slack") != "https://hooks.slack.com/services/test" {
		t.Errorf("Expected node slack URL, got '%s'", node.Notifications.GetNotificationURL("slack"))
	}
}

func TestLoadConfig_MultipleNotificationTypes(t *testing.T) {
	// Set up mock validator
	validator := &MockNotificationValidator{
		registered: map[string]bool{
			"discord": true,
			"slack":   true,
			"email":   true,
		},
	}
	SetNotificationValidator(validator)
	defer SetNotificationValidator(nil)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config with multiple notification types
	configContent := `
schedule: "0 * * * * *"
notifications:
  failure: true
  skip: false
  complete: true
  discord:
    url: https://discord.com/api/webhooks/global
  slack:
    url: https://hooks.slack.com/services/global
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: testpass
nodes:
  test-node:
    protocol: ethereum
    type: archive
    schedule: "0 0 */6 * * *"
    url: http://localhost:8545
    notifications:
      failure: true
      skip: true
      complete: true
      discord:
        url: https://discord.com/api/webhooks/node-specific
      email:
        url: smtp://mail.example.com
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify global notifications has multiple types
	globalTypes := config.Notifications.GetNotificationTypes()
	if len(globalTypes) != 2 {
		t.Errorf("Expected 2 global notification types, got %d", len(globalTypes))
	}

	// Verify global notification URLs
	if config.Notifications.GetNotificationURL("discord") != "https://discord.com/api/webhooks/global" {
		t.Errorf("Expected global discord URL, got '%s'", config.Notifications.GetNotificationURL("discord"))
	}
	if config.Notifications.GetNotificationURL("slack") != "https://hooks.slack.com/services/global" {
		t.Errorf("Expected global slack URL, got '%s'", config.Notifications.GetNotificationURL("slack"))
	}

	// Verify node notifications override
	nodeNotif := config.GetNodeNotifications("test-node")
	nodeTypes := nodeNotif.GetNotificationTypes()
	if len(nodeTypes) != 2 {
		t.Errorf("Expected 2 node notification types, got %d", len(nodeTypes))
	}

	// Verify node notification URLs
	if nodeNotif.GetNotificationURL("discord") != "https://discord.com/api/webhooks/node-specific" {
		t.Errorf("Expected node discord URL, got '%s'", nodeNotif.GetNotificationURL("discord"))
	}
	if nodeNotif.GetNotificationURL("email") != "smtp://mail.example.com" {
		t.Errorf("Expected node email URL, got '%s'", nodeNotif.GetNotificationURL("email"))
	}

	// Verify node doesn't have slack (not in override)
	if nodeNotif.GetNotificationURL("slack") != "" {
		t.Errorf("Expected no slack URL for node, got '%s'", nodeNotif.GetNotificationURL("slack"))
	}
}
