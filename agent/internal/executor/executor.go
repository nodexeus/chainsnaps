package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// CommandExecutor handles external command execution
type CommandExecutor interface {
	// Execute runs a command and returns stdout, stderr, and error
	Execute(ctx context.Context, command string, args ...string) (stdout, stderr string, err error)
}

// DefaultExecutor is the standard implementation of CommandExecutor
type DefaultExecutor struct {
	logger *logrus.Logger
	bvMu   sync.Mutex // Mutex to serialize bv CLI commands
}

// NewDefaultExecutor creates a new DefaultExecutor with the provided logger
func NewDefaultExecutor(logger *logrus.Logger) *DefaultExecutor {
	if logger == nil {
		logger = logrus.New()
	}
	return &DefaultExecutor{
		logger: logger,
	}
}

// Execute runs a command with context support and captures stdout and stderr separately
func (e *DefaultExecutor) Execute(ctx context.Context, command string, args ...string) (stdout, stderr string, err error) {
	// Serialize bv CLI commands to prevent race conditions
	// The bv CLI rewrites /etc/blockvisor.json on every run, causing race conditions in parallel execution
	isBvCommand := command == "bv" || strings.HasSuffix(command, "/bv")
	if isBvCommand {
		e.bvMu.Lock()
		defer e.bvMu.Unlock()
	}

	// Log the command being executed
	e.logger.WithFields(logrus.Fields{
		"component": "executor",
		"command":   command,
		"args":      args,
	}).Debug("Executing command")

	// Create the command with context
	cmd := exec.CommandContext(ctx, command, args...)

	// Create buffers to capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Execute the command
	startTime := time.Now()
	execErr := cmd.Run()
	duration := time.Since(startTime)

	// Get the output strings
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	// Log the result
	logFields := logrus.Fields{
		"component": "executor",
		"command":   command,
		"args":      args,
		"duration":  duration,
	}

	if execErr != nil {
		// Check if the error is due to context cancellation or timeout
		if ctx.Err() == context.DeadlineExceeded {
			e.logger.WithFields(logFields).Error("Command execution timed out")
			return stdout, stderr, fmt.Errorf("command timed out: %w", execErr)
		} else if ctx.Err() == context.Canceled {
			e.logger.WithFields(logFields).Error("Command execution canceled")
			return stdout, stderr, fmt.Errorf("command canceled: %w", execErr)
		}

		// Log the error with full details
		logFields["error"] = execErr.Error()
		logFields["stderr"] = stderr
		e.logger.WithFields(logFields).Error("Command execution failed")
		return stdout, stderr, fmt.Errorf("command failed: %w", execErr)
	}

	e.logger.WithFields(logFields).Info("Command executed successfully")
	return stdout, stderr, nil
}
