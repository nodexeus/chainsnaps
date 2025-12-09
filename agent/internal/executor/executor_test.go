package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestDefaultExecutor_Execute_Success(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	stdout, stderr, err := executor.Execute(ctx, "echo", "hello world")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(stdout, "hello world") {
		t.Errorf("Expected stdout to contain 'hello world', got: %s", stdout)
	}

	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestDefaultExecutor_Execute_StderrCapture(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	// Use a command that writes to stderr (ls with invalid option)
	stdout, stderr, err := executor.Execute(ctx, "sh", "-c", "echo 'error message' >&2")

	// This command should succeed (exit 0) but write to stderr
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if stdout != "" {
		t.Errorf("Expected empty stdout, got: %s", stdout)
	}

	if !strings.Contains(stderr, "error message") {
		t.Errorf("Expected stderr to contain 'error message', got: %s", stderr)
	}
}

func TestDefaultExecutor_Execute_BothOutputs(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	// Command that writes to both stdout and stderr
	stdout, stderr, err := executor.Execute(ctx, "sh", "-c", "echo 'stdout message'; echo 'stderr message' >&2")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(stdout, "stdout message") {
		t.Errorf("Expected stdout to contain 'stdout message', got: %s", stdout)
	}

	if !strings.Contains(stderr, "stderr message") {
		t.Errorf("Expected stderr to contain 'stderr message', got: %s", stderr)
	}
}

func TestDefaultExecutor_Execute_CommandFailure(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	// Use a command that will fail (exit non-zero)
	_, _, err := executor.Execute(ctx, "sh", "-c", "exit 1")

	if err == nil {
		t.Fatal("Expected error for failed command, got nil")
	}

	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("Expected error message to contain 'command failed', got: %v", err)
	}
}

func TestDefaultExecutor_Execute_CommandNotFound(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	// Use a command that doesn't exist
	_, _, err := executor.Execute(ctx, "nonexistent-command-xyz")

	if err == nil {
		t.Fatal("Expected error for nonexistent command, got nil")
	}
}

func TestDefaultExecutor_Execute_Timeout(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run a command that will take longer than the timeout
	_, _, err := executor.Execute(ctx, "sleep", "5")

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("Expected timeout or canceled error, got: %v", err)
	}
}

func TestDefaultExecutor_Execute_ContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	cancel()

	// Try to execute a command with canceled context
	_, _, err := executor.Execute(ctx, "echo", "test")

	if err == nil {
		t.Fatal("Expected error for canceled context, got nil")
	}
}

func TestDefaultExecutor_Execute_WithArguments(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	stdout, stderr, err := executor.Execute(ctx, "echo", "arg1", "arg2", "arg3")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(stdout, "arg1") || !strings.Contains(stdout, "arg2") || !strings.Contains(stdout, "arg3") {
		t.Errorf("Expected stdout to contain all arguments, got: %s", stdout)
	}

	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}

func TestNewDefaultExecutor_NilLogger(t *testing.T) {
	// Test that NewDefaultExecutor handles nil logger gracefully
	executor := NewDefaultExecutor(nil)

	if executor == nil {
		t.Fatal("Expected non-nil executor")
	}

	if executor.logger == nil {
		t.Fatal("Expected executor to have a logger")
	}

	// Verify it still works
	ctx := context.Background()
	stdout, _, err := executor.Execute(ctx, "echo", "test")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(stdout, "test") {
		t.Errorf("Expected stdout to contain 'test', got: %s", stdout)
	}
}

func TestDefaultExecutor_Execute_LargeOutput(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	executor := NewDefaultExecutor(logger)

	ctx := context.Background()
	// Generate a large output
	stdout, stderr, err := executor.Execute(ctx, "sh", "-c", "for i in $(seq 1 1000); do echo line$i; done")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that we captured all the output
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1000 {
		t.Errorf("Expected 1000 lines of output, got: %d", len(lines))
	}

	if stderr != "" {
		t.Errorf("Expected empty stderr, got: %s", stderr)
	}
}
