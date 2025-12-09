# Logger Package

The logger package provides structured logging functionality for the Snapshot Daemon using logrus.

## Features

- **Structured Logging**: All log entries include timestamp, level, component, and message
- **Dual Formatting**: JSON format for daemon mode (systemd), human-readable text format for console mode
- **Configurable Log Levels**: Support for debug, info, warn, and error levels
- **Error Logging**: Full error details included in error log entries
- **Component Tagging**: Easy component identification in logs

## Usage

### Creating a Logger

```go
import "github.com/yourusername/snapd/internal/logger"

// Default logger (daemon mode, JSON format, info level)
log := logger.Default()

// Console logger (console mode, text format, info level)
log := logger.Console()

// Custom configuration
log := logger.New(logger.Config{
    Level:       "debug",
    ConsoleMode: true,
    Output:      os.Stdout,
})
```

### Logging Messages

```go
// Simple logging
log.Info("Daemon started")
log.Error("Failed to connect")

// With component
log.WithComponent("scheduler").Info("Job scheduled")

// With error
err := errors.New("connection failed")
log.WithError(err).Error("Database error")

// With multiple fields
log.WithFields(logrus.Fields{
    "component": "upload",
    "node":      "ethereum-mainnet",
    "uploadID":  12345,
}).Info("Upload initiated")
```

### Log Levels

- **Debug**: Detailed debugging information
- **Info**: Normal operational messages
- **Warn**: Warning messages for potentially problematic situations
- **Error**: Error messages with full error details

### Output Formats

#### Daemon Mode (JSON)
```json
{
  "timestamp": "2024-01-15T10:30:45.123Z",
  "level": "info",
  "message": "Upload initiated",
  "component": "upload",
  "node": "ethereum-mainnet",
  "uploadID": 12345
}
```

#### Console Mode (Text)
```
2024-01-15 10:30:45 level=info msg="Upload initiated" component=upload node=ethereum-mainnet uploadID=12345
```

## Requirements Validation

This package satisfies the following requirements:

- **14.1**: Structured log entries for all operations
- **14.2**: All log entries include timestamp, level, component, and message
- **14.3**: Errors logged at ERROR level with full error details
- **14.4**: Normal operations logged at INFO level
- **14.5**: DEBUG log level support via configuration

## Integration

The logger is designed to be used throughout the daemon:

```go
// In main.go
var log *logger.Logger
if consoleMode {
    log = logger.Console()
} else {
    log = logger.Default()
}

// Pass to components
scheduler := scheduler.NewCronScheduler(log.Logger)
executor := executor.NewDefaultExecutor(log.Logger)
uploadManager := upload.NewManager(executor, db, log.Logger)
```

## Testing

Run tests with:
```bash
go test ./internal/logger/...
```

The test suite verifies:
- Log level configuration
- Output formatting (JSON and text)
- Field inclusion (timestamp, level, component, message)
- Error logging with full details
- Custom output writers
