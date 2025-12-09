package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNew_DefaultOutput(t *testing.T) {
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
	}

	logger := New(cfg)

	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	if logger.Logger == nil {
		t.Fatal("Expected underlying logrus.Logger to be set")
	}
}

func TestNew_CustomOutput(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	logger.Info("test message")

	if buf.Len() == 0 {
		t.Error("Expected output to be written to custom writer")
	}
}

func TestNew_LogLevel(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		expectedLevel logrus.Level
	}{
		{"debug level", "debug", logrus.DebugLevel},
		{"info level", "info", logrus.InfoLevel},
		{"warn level", "warn", logrus.WarnLevel},
		{"error level", "error", logrus.ErrorLevel},
		{"invalid level defaults to info", "invalid", logrus.InfoLevel},
		{"empty level defaults to info", "", logrus.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Level:       tt.level,
				ConsoleMode: false,
			}

			logger := New(cfg)

			if logger.GetLevel() != tt.expectedLevel {
				t.Errorf("Expected level %v, got %v", tt.expectedLevel, logger.GetLevel())
			}
		})
	}
}

func TestNew_ConsoleMode(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: true,
		Output:      buf,
	}

	logger := New(cfg)
	logger.Info("test message")

	output := buf.String()

	// Text formatter should produce human-readable output
	if !strings.Contains(output, "test message") {
		t.Error("Expected message to be in output")
	}

	// Text formatter should include timestamp
	if !strings.Contains(output, "level=info") {
		t.Error("Expected level to be in text format")
	}
}

func TestNew_DaemonMode(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	logger.Info("test message")

	output := buf.String()

	// JSON formatter should produce valid JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	// Check required fields
	if _, ok := logEntry["timestamp"]; !ok {
		t.Error("Expected 'timestamp' field in JSON output")
	}

	if _, ok := logEntry["level"]; !ok {
		t.Error("Expected 'level' field in JSON output")
	}

	if _, ok := logEntry["message"]; !ok {
		t.Error("Expected 'message' field in JSON output")
	}

	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["message"])
	}
}

func TestLogger_WithComponent(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	logger.WithComponent("test-component").Info("test message")

	output := buf.String()

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	if logEntry["component"] != "test-component" {
		t.Errorf("Expected component 'test-component', got %v", logEntry["component"])
	}
}

func TestLogger_WithError(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	testErr := errors.New("test error")
	logger.WithError(testErr).Error("error occurred")

	output := buf.String()

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	if logEntry["error"] != "test error" {
		t.Errorf("Expected error 'test error', got %v", logEntry["error"])
	}

	if logEntry["level"] != "error" {
		t.Errorf("Expected level 'error', got %v", logEntry["level"])
	}
}

func TestLogger_WithFields(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	logger.WithFields(logrus.Fields{
		"component": "test",
		"node":      "ethereum-mainnet",
		"count":     42,
	}).Info("test message")

	output := buf.String()

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	if logEntry["component"] != "test" {
		t.Errorf("Expected component 'test', got %v", logEntry["component"])
	}

	if logEntry["node"] != "ethereum-mainnet" {
		t.Errorf("Expected node 'ethereum-mainnet', got %v", logEntry["node"])
	}

	if logEntry["count"] != float64(42) {
		t.Errorf("Expected count 42, got %v", logEntry["count"])
	}
}

func TestLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  string
		logFunc   func(*Logger)
		shouldLog bool
	}{
		{
			name:      "debug logs when level is debug",
			logLevel:  "debug",
			logFunc:   func(l *Logger) { l.Debug("debug message") },
			shouldLog: true,
		},
		{
			name:      "debug does not log when level is info",
			logLevel:  "info",
			logFunc:   func(l *Logger) { l.Debug("debug message") },
			shouldLog: false,
		},
		{
			name:      "info logs when level is info",
			logLevel:  "info",
			logFunc:   func(l *Logger) { l.Info("info message") },
			shouldLog: true,
		},
		{
			name:      "warn logs when level is info",
			logLevel:  "info",
			logFunc:   func(l *Logger) { l.Warn("warn message") },
			shouldLog: true,
		},
		{
			name:      "error logs when level is info",
			logLevel:  "info",
			logFunc:   func(l *Logger) { l.Error("error message") },
			shouldLog: true,
		},
		{
			name:      "error logs when level is error",
			logLevel:  "error",
			logFunc:   func(l *Logger) { l.Error("error message") },
			shouldLog: true,
		},
		{
			name:      "info does not log when level is error",
			logLevel:  "error",
			logFunc:   func(l *Logger) { l.Info("info message") },
			shouldLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			cfg := Config{
				Level:       tt.logLevel,
				ConsoleMode: false,
				Output:      buf,
			}

			logger := New(cfg)
			tt.logFunc(logger)

			output := buf.String()

			if tt.shouldLog && len(output) == 0 {
				t.Error("Expected log output but got none")
			}

			if !tt.shouldLog && len(output) > 0 {
				t.Errorf("Expected no log output but got: %s", output)
			}
		})
	}
}

func TestLogger_AllFieldsPresent(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	logger.WithFields(logrus.Fields{
		"component": "test-component",
	}).Info("test message")

	output := buf.String()

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	// Verify all required fields are present
	requiredFields := []string{"timestamp", "level", "message", "component"}
	for _, field := range requiredFields {
		if _, ok := logEntry[field]; !ok {
			t.Errorf("Expected field '%s' to be present in log entry", field)
		}
	}
}

func TestDefault(t *testing.T) {
	logger := Default()

	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	if logger.GetLevel() != logrus.InfoLevel {
		t.Errorf("Expected default level to be info, got %v", logger.GetLevel())
	}

	// Check that it uses JSON formatter (daemon mode)
	if _, ok := logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Error("Expected default logger to use JSON formatter")
	}
}

func TestConsole(t *testing.T) {
	logger := Console()

	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	if logger.GetLevel() != logrus.InfoLevel {
		t.Errorf("Expected console level to be info, got %v", logger.GetLevel())
	}

	// Check that it uses Text formatter (console mode)
	if _, ok := logger.Formatter.(*logrus.TextFormatter); !ok {
		t.Error("Expected console logger to use Text formatter")
	}
}

func TestLogger_ErrorWithFullDetails(t *testing.T) {
	buf := &bytes.Buffer{}
	cfg := Config{
		Level:       "error",
		ConsoleMode: false,
		Output:      buf,
	}

	logger := New(cfg)
	testErr := errors.New("database connection failed")

	logger.WithFields(logrus.Fields{
		"component": "database",
		"host":      "localhost",
		"port":      5432,
	}).WithError(testErr).Error("Failed to connect to database")

	output := buf.String()

	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("Expected valid JSON output, got error: %v", err)
	}

	// Verify error details are included
	if logEntry["error"] != "database connection failed" {
		t.Errorf("Expected error message, got %v", logEntry["error"])
	}

	if logEntry["component"] != "database" {
		t.Errorf("Expected component 'database', got %v", logEntry["component"])
	}

	if logEntry["host"] != "localhost" {
		t.Errorf("Expected host 'localhost', got %v", logEntry["host"])
	}

	if logEntry["level"] != "error" {
		t.Errorf("Expected level 'error', got %v", logEntry["level"])
	}
}
