package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
schedule: "*/5 * * * *"
notifications:
  failure: true
  skip: false
  complete: true
  discord:
    url: https://discord.com/api/webhooks/test
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: testpass
  ssl_mode: require
nodes:
  ethereum-mainnet:
    protocol: ethereum
    type: archive
    schedule: "0 */6 * * *"
    rpc_url: http://localhost:8545
    beacon_url: http://localhost:5052
  arbitrum-one:
    protocol: arbitrum
    type: archive
    schedule: "0 */12 * * *"
    rpc_url: http://localhost:8547
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify global schedule
	if config.Schedule != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *', got '%s'", config.Schedule)
	}

	// Verify database config
	if config.Database.Host != "localhost" {
		t.Errorf("Expected database host 'localhost', got '%s'", config.Database.Host)
	}
	if config.Database.Port != 5432 {
		t.Errorf("Expected database port 5432, got %d", config.Database.Port)
	}

	// Verify nodes
	if len(config.Nodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(config.Nodes))
	}

	ethNode, exists := config.Nodes["ethereum-mainnet"]
	if !exists {
		t.Fatal("ethereum-mainnet node not found")
	}
	if ethNode.Protocol != "ethereum" {
		t.Errorf("Expected protocol 'ethereum', got '%s'", ethNode.Protocol)
	}
	if ethNode.Type != "archive" {
		t.Errorf("Expected type 'archive', got '%s'", ethNode.Type)
	}
}

func TestLoadConfigWithDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config without global schedule (should default to "* * * * *")
	configContent := `
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
    schedule: "0 */6 * * *"
    rpc_url: http://localhost:8545
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify default schedule
	if config.Schedule != "* * * * *" {
		t.Errorf("Expected default schedule '* * * * *', got '%s'", config.Schedule)
	}
}

func TestLoadConfigInvalidFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `
this is not: valid: yaml: content
  - broken
`

	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestValidateCronSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"valid every minute", "* * * * *", false},
		{"valid every 5 minutes", "*/5 * * * *", false},
		{"valid specific time", "0 */6 * * *", false},
		{"valid complex", "0 0 * * 1-5", false},
		{"invalid format", "invalid", true},
		{"invalid too many fields", "* * * * * *", true},
		{"invalid too few fields", "* * *", true},
		{"invalid range", "60 * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCronSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCronSchedule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "snapd",
				User:     "snapd",
				Password: "pass",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			config: DatabaseConfig{
				Port:     5432,
				Database: "snapd",
				User:     "snapd",
			},
			wantErr: true,
		},
		{
			name: "missing port",
			config: DatabaseConfig{
				Host:     "localhost",
				Database: "snapd",
				User:     "snapd",
			},
			wantErr: true,
		},
		{
			name: "missing database",
			config: DatabaseConfig{
				Host: "localhost",
				Port: 5432,
				User: "snapd",
			},
			wantErr: true,
		},
		{
			name: "missing user",
			config: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Database: "snapd",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("DatabaseConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  NodeConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: NodeConfig{
				Protocol: "ethereum",
				Type:     "archive",
				RPCUrl:   "http://localhost:8545",
				Schedule: "0 */6 * * *",
			},
			wantErr: false,
		},
		{
			name: "missing protocol",
			config: NodeConfig{
				Type:     "archive",
				RPCUrl:   "http://localhost:8545",
				Schedule: "0 */6 * * *",
			},
			wantErr: true,
		},
		{
			name: "missing rpc_url",
			config: NodeConfig{
				Protocol: "ethereum",
				Type:     "archive",
				Schedule: "0 */6 * * *",
			},
			wantErr: true,
		},
		{
			name: "missing schedule",
			config: NodeConfig{
				Protocol: "ethereum",
				Type:     "archive",
				RPCUrl:   "http://localhost:8545",
			},
			wantErr: true,
		},
		{
			name: "invalid schedule",
			config: NodeConfig{
				Protocol: "ethereum",
				RPCUrl:   "http://localhost:8545",
				Schedule: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid with schedule",
			config: NodeConfig{
				Protocol: "ethereum",
				RPCUrl:   "http://localhost:8545",
				Schedule: "0 */6 * * *",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotificationConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  NotificationConfig
		wantErr bool
	}{
		{
			name: "valid config with single type",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with multiple types",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
					"slack":   {URL: "https://hooks.slack.com/services/test"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty types map",
			config: NotificationConfig{
				Types: map[string]NotificationTypeConfig{},
			},
			wantErr: true,
		},
		{
			name: "missing url for type",
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

func TestGetNodeSchedule(t *testing.T) {
	config := &Config{
		Schedule: "* * * * *",
		Nodes: map[string]NodeConfig{
			"node-with-schedule": {
				Protocol: "ethereum",
				RPCUrl:   "http://localhost:8545",
				Schedule: "0 */6 * * *",
			},
			"another-node": {
				Protocol: "arbitrum",
				RPCUrl:   "http://localhost:8547",
				Schedule: "0 */12 * * *",
			},
		},
	}

	// Test node with schedule
	schedule := config.GetNodeSchedule("node-with-schedule")
	if schedule != "0 */6 * * *" {
		t.Errorf("Expected '0 */6 * * *', got '%s'", schedule)
	}

	// Test another node with different schedule
	schedule = config.GetNodeSchedule("another-node")
	if schedule != "0 */12 * * *" {
		t.Errorf("Expected '0 */12 * * *', got '%s'", schedule)
	}

	// Test nonexistent node (should return empty string)
	schedule = config.GetNodeSchedule("nonexistent")
	if schedule != "" {
		t.Errorf("Expected empty string for nonexistent node, got '%s'", schedule)
	}
}

func TestGetNodeNotifications(t *testing.T) {
	globalNotif := &NotificationConfig{
		Failure:  true,
		Skip:     false,
		Complete: true,
		Types: map[string]NotificationTypeConfig{
			"discord": {URL: "https://discord.com/api/webhooks/global"},
		},
	}

	nodeNotif := &NotificationConfig{
		Failure:  false,
		Skip:     true,
		Complete: false,
		Types: map[string]NotificationTypeConfig{
			"slack": {URL: "https://hooks.slack.com/services/test"},
		},
	}

	config := &Config{
		Notifications: globalNotif,
		Nodes: map[string]NodeConfig{
			"node-with-notif": {
				Protocol:      "ethereum",
				RPCUrl:        "http://localhost:8545",
				Notifications: nodeNotif,
			},
			"node-without-notif": {
				Protocol: "arbitrum",
				RPCUrl:   "http://localhost:8547",
			},
		},
	}

	// Test node with custom notifications
	notif := config.GetNodeNotifications("node-with-notif")
	if notif != nodeNotif {
		t.Error("Expected node-specific notifications")
	}
	if notif.GetNotificationURL("slack") != "https://hooks.slack.com/services/test" {
		t.Errorf("Expected slack URL, got '%s'", notif.GetNotificationURL("slack"))
	}

	// Test node without custom notifications (should use global)
	notif = config.GetNodeNotifications("node-without-notif")
	if notif != globalNotif {
		t.Error("Expected global notifications")
	}
	if notif.GetNotificationURL("discord") != "https://discord.com/api/webhooks/global" {
		t.Errorf("Expected discord URL, got '%s'", notif.GetNotificationURL("discord"))
	}

	// Test nonexistent node (should use global)
	notif = config.GetNodeNotifications("nonexistent")
	if notif != globalNotif {
		t.Error("Expected global notifications")
	}
}

func TestConfigValidateNoNodes(t *testing.T) {
	config := &Config{
		Schedule: "* * * * *",
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "snapd",
			User:     "snapd",
		},
		Nodes: map[string]NodeConfig{},
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected error for config with no nodes")
	}
}

func TestConfigValidateInvalidGlobalSchedule(t *testing.T) {
	config := &Config{
		Schedule: "invalid schedule",
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "snapd",
			User:     "snapd",
		},
		Nodes: map[string]NodeConfig{
			"test": {
				Protocol: "ethereum",
				RPCUrl:   "http://localhost:8545",
			},
		},
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected error for invalid global schedule")
	}
}

func TestNotificationConfig_GetNotificationURL(t *testing.T) {
	config := &NotificationConfig{
		Types: map[string]NotificationTypeConfig{
			"discord": {URL: "https://discord.com/api/webhooks/test"},
			"slack":   {URL: "https://hooks.slack.com/services/test"},
		},
	}

	tests := []struct {
		name             string
		notificationType string
		want             string
	}{
		{
			name:             "existing discord type",
			notificationType: "discord",
			want:             "https://discord.com/api/webhooks/test",
		},
		{
			name:             "existing slack type",
			notificationType: "slack",
			want:             "https://hooks.slack.com/services/test",
		},
		{
			name:             "non-existing type",
			notificationType: "email",
			want:             "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.GetNotificationURL(tt.notificationType)
			if got != tt.want {
				t.Errorf("GetNotificationURL(%s) = %s, want %s", tt.notificationType, got, tt.want)
			}
		})
	}
}

func TestNotificationConfig_GetNotificationTypes(t *testing.T) {
	tests := []struct {
		name   string
		config *NotificationConfig
		want   int
	}{
		{
			name: "multiple types",
			config: &NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
					"slack":   {URL: "https://hooks.slack.com/services/test"},
				},
			},
			want: 2,
		},
		{
			name: "single type",
			config: &NotificationConfig{
				Types: map[string]NotificationTypeConfig{
					"discord": {URL: "https://discord.com/api/webhooks/test"},
				},
			},
			want: 1,
		},
		{
			name:   "nil config",
			config: nil,
			want:   0,
		},
		{
			name: "empty types",
			config: &NotificationConfig{
				Types: map[string]NotificationTypeConfig{},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetNotificationTypes()
			if len(got) != tt.want {
				t.Errorf("GetNotificationTypes() returned %d types, want %d", len(got), tt.want)
			}
		})
	}
}
