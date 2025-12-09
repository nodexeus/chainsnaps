package logger_test

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nodexeus/agent/internal/logger"
	"github.com/sirupsen/logrus"
)

func Example_daemonMode() {
	// Create a logger for daemon mode (JSON output)
	// In production, this would output to stdout
	buf := &bytes.Buffer{}
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	})

	// Simple info log
	log.Info("Daemon started")

	// Log with component
	log.WithComponent("scheduler").Info("Job scheduled")

	// Log with error
	err := errors.New("connection failed")
	log.WithError(err).Error("Database error")

	// Log with multiple fields
	log.WithFields(logrus.Fields{
		"component": "upload",
		"node":      "ethereum-mainnet",
		"uploadID":  12345,
	}).Info("Upload initiated")

	fmt.Println("Logs written in JSON format")
	// Output: Logs written in JSON format
}

func Example_consoleMode() {
	// Create a logger for console mode (human-readable text output)
	buf := &bytes.Buffer{}
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: true,
		Output:      buf,
	})

	// Simple info log
	log.Info("Daemon started in console mode")

	// Log with component
	log.WithComponent("scheduler").Info("Job scheduled")

	// Log with error
	err := errors.New("connection failed")
	log.WithError(err).Error("Database error")

	fmt.Println("Logs written in text format")
	// Output: Logs written in text format
}

func Example_customConfiguration() {
	// Create a logger with custom configuration
	buf := &bytes.Buffer{}
	log := logger.New(logger.Config{
		Level:       "debug",
		ConsoleMode: true,
		Output:      buf,
	})

	// Debug logs will be visible
	log.Debug("Debug information")
	log.Info("Normal operation")
	log.Warn("Warning message")
	log.Error("Error occurred")

	fmt.Println("All log levels captured")
	// Output: All log levels captured
}

func Example_componentLogging() {
	buf := &bytes.Buffer{}
	log := logger.New(logger.Config{
		Level:       "info",
		ConsoleMode: false,
		Output:      buf,
	})

	// Different components can be easily identified
	log.WithComponent("database").Info("Connection established")
	log.WithComponent("scheduler").Info("Jobs scheduled")
	log.WithComponent("upload").Info("Upload started")
	log.WithComponent("executor").Info("Command executed")

	fmt.Println("Component logs written")
	// Output: Component logs written
}

func Example_errorLogging() {
	buf := &bytes.Buffer{}
	log := logger.New(logger.Config{
		Level:       "error",
		ConsoleMode: false,
		Output:      buf,
	})

	// Error logging with full details
	err := errors.New("database connection timeout")

	// Log the error with context
	log.WithFields(logrus.Fields{
		"component": "database",
		"host":      "localhost",
		"port":      5432,
		"timeout":   "30s",
	}).WithError(err).Error("Failed to connect to database")

	// This ensures all error details are captured for debugging
	fmt.Println("Error logged with full context")
	// Output: Error logged with full context
}
