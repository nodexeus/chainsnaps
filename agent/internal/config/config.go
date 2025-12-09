package config

import (
	"fmt"
	"os"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// ProtocolValidator is an interface for validating protocol names
type ProtocolValidator interface {
	IsRegistered(name string) bool
}

// NotificationValidator is an interface for validating notification types
type NotificationValidator interface {
	IsRegistered(name string) bool
}

var (
	protocolValidator     ProtocolValidator
	notificationValidator NotificationValidator
)

// SetProtocolValidator sets the protocol validator for configuration validation
func SetProtocolValidator(validator ProtocolValidator) {
	protocolValidator = validator
}

// SetNotificationValidator sets the notification validator for configuration validation
func SetNotificationValidator(validator NotificationValidator) {
	notificationValidator = validator
}

// Config represents the complete daemon configuration
type Config struct {
	Schedule      string                `yaml:"schedule"`
	Notifications *NotificationConfig   `yaml:"notifications"`
	Database      DatabaseConfig        `yaml:"database"`
	Nodes         map[string]NodeConfig `yaml:"nodes"`
}

// NodeConfig represents a single node's configuration
type NodeConfig struct {
	Protocol      string              `yaml:"protocol"`
	Type          string              `yaml:"type"`
	Schedule      string              `yaml:"schedule"`
	RPCUrl        string              `yaml:"rpc_url"`
	BeaconUrl     string              `yaml:"beacon_url,omitempty"`
	Notifications *NotificationConfig `yaml:"notifications,omitempty"`
}

// NotificationConfig represents notification settings
type NotificationConfig struct {
	Failure  bool                              `yaml:"failure"`
	Skip     bool                              `yaml:"skip"`
	Complete bool                              `yaml:"complete"`
	Types    map[string]NotificationTypeConfig `yaml:",inline"`
}

// NotificationTypeConfig represents a single notification type configuration
type NotificationTypeConfig struct {
	URL string `yaml:"url"`
}

// DatabaseConfig represents database connection settings
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`
}

// LoadConfig loads configuration from the specified file path
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults
	if config.Schedule == "" {
		config.Schedule = "* * * * *" // Default to every minute
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate global schedule
	if err := validateCronSchedule(c.Schedule); err != nil {
		return fmt.Errorf("invalid global schedule: %w", err)
	}

	// Validate database configuration
	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("invalid database config: %w", err)
	}

	// Validate global notifications if present
	if c.Notifications != nil {
		if err := c.Notifications.Validate(); err != nil {
			return fmt.Errorf("invalid global notifications config: %w", err)
		}
	}

	// Validate each node configuration
	if len(c.Nodes) == 0 {
		return fmt.Errorf("at least one node must be configured")
	}

	for name, node := range c.Nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("invalid config for node %s: %w", name, err)
		}
	}

	return nil
}

// Validate validates the database configuration
func (d *DatabaseConfig) Validate() error {
	if d.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if d.Port == 0 {
		return fmt.Errorf("database port is required")
	}
	if d.Database == "" {
		return fmt.Errorf("database name is required")
	}
	if d.User == "" {
		return fmt.Errorf("database user is required")
	}
	// Password can be empty if using other auth methods
	return nil
}

// Validate validates the node configuration
func (n *NodeConfig) Validate() error {
	if n.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if n.RPCUrl == "" {
		return fmt.Errorf("rpc_url is required")
	}
	if n.Schedule == "" {
		return fmt.Errorf("schedule is required")
	}

	// Validate protocol is registered if validator is set
	if protocolValidator != nil && !protocolValidator.IsRegistered(n.Protocol) {
		return fmt.Errorf("protocol %s is not registered", n.Protocol)
	}

	// Validate node schedule
	if err := validateCronSchedule(n.Schedule); err != nil {
		return fmt.Errorf("invalid node schedule: %w", err)
	}

	// Validate per-node notifications if present
	if n.Notifications != nil {
		if err := n.Notifications.Validate(); err != nil {
			return fmt.Errorf("invalid node notifications config: %w", err)
		}
	}

	return nil
}

// Validate validates the notification configuration
func (n *NotificationConfig) Validate() error {
	if len(n.Types) == 0 {
		return fmt.Errorf("at least one notification type is required")
	}

	// Validate each notification type
	for typeName, typeConfig := range n.Types {
		if typeConfig.URL == "" {
			return fmt.Errorf("notification url is required for type %s", typeName)
		}

		// Validate notification type is registered if validator is set
		if notificationValidator != nil && !notificationValidator.IsRegistered(typeName) {
			return fmt.Errorf("notification type %s is not registered", typeName)
		}
	}

	return nil
}

// validateCronSchedule validates a cron schedule expression
func validateCronSchedule(schedule string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("invalid cron schedule '%s': %w", schedule, err)
	}
	return nil
}

// GetNodeSchedule returns the schedule for a node
// Node schedule is required, so this always returns the node's schedule
func (c *Config) GetNodeSchedule(nodeName string) string {
	node, exists := c.Nodes[nodeName]
	if !exists {
		return ""
	}

	return node.Schedule
}

// GetNodeNotifications returns the effective notification config for a node
// (per-node notifications override global notifications)
func (c *Config) GetNodeNotifications(nodeName string) *NotificationConfig {
	node, exists := c.Nodes[nodeName]
	if !exists {
		return c.Notifications
	}

	if node.Notifications != nil {
		return node.Notifications
	}

	return c.Notifications
}

// GetNotificationURL returns the URL for a specific notification type from the config
// Returns empty string if the type is not configured
func (n *NotificationConfig) GetNotificationURL(notificationType string) string {
	if n == nil || n.Types == nil {
		return ""
	}

	typeConfig, exists := n.Types[notificationType]
	if !exists {
		return ""
	}

	return typeConfig.URL
}

// GetNotificationTypes returns all configured notification types
func (n *NotificationConfig) GetNotificationTypes() []string {
	if n == nil || n.Types == nil {
		return nil
	}

	types := make([]string, 0, len(n.Types))
	for typeName := range n.Types {
		types = append(types, typeName)
	}
	return types
}
