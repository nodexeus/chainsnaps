# Command Executor

The executor package provides a simple interface for executing external system commands with proper error handling, logging, and output capture.

## Interface

```go
type CommandExecutor interface {
    Execute(ctx context.Context, command string, args ...string) (stdout, stderr string, err error)
}
```

## Features

- **Context Support**: All command executions support context for timeout and cancellation
- **Separate Output Capture**: Stdout and stderr are captured separately
- **Comprehensive Logging**: All command executions are logged with structured fields
- **Error Handling**: Distinguishes between timeout, cancellation, and execution errors

## Usage

```go
import (
    "context"
    "time"
    
    "github.com/sirupsen/logrus"
    "github.com/nodexeus/agent/internal/executor"
)

// Create an executor with a logger
logger := logrus.New()
exec := executor.NewDefaultExecutor(logger)

// Execute a command with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

stdout, stderr, err := exec.Execute(ctx, "bv", "n", "j", "ethereum-mainnet", "info", "upload")
if err != nil {
    // Handle error - check stdout/stderr for details
    log.Printf("Command failed: %v\nStderr: %s", err, stderr)
    return
}

// Process stdout
log.Printf("Command output: %s", stdout)
```

## Error Types

The executor returns different error messages based on the failure type:

- **Timeout**: `"command timed out: ..."` - Context deadline exceeded
- **Cancellation**: `"command canceled: ..."` - Context was canceled
- **Execution Failure**: `"command failed: ..."` - Command returned non-zero exit code
- **Not Found**: Standard exec error - Command not found in PATH

## Logging

All command executions are logged with the following fields:

- `component`: Always "executor"
- `command`: The command being executed
- `args`: Command arguments
- `duration`: Execution time
- `stdout`: Command stdout (on completion)
- `stderr`: Command stderr (on completion)
- `error`: Error message (on failure)

Log levels:
- **DEBUG**: Command start
- **INFO**: Successful completion
- **ERROR**: Failures (timeout, cancellation, execution error)

## Testing

The package includes comprehensive unit tests covering:

- Successful command execution
- Stdout and stderr capture (separately and together)
- Command failures
- Timeouts and cancellation
- Large output handling
- Missing commands
- Nil logger handling

Run tests:
```bash
go test ./internal/executor/...
```
