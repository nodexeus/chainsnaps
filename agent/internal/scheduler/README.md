# Scheduler Package

The scheduler package provides cron-based job scheduling and orchestration for the snapshot daemon. It coordinates the workflow of metrics collection, upload initiation, and progress monitoring across multiple nodes.

## Overview

The scheduler implements three main components:

1. **CronScheduler**: A cron-based scheduler that manages job execution on configurable schedules
2. **NodeUploadJob**: Orchestrates the complete upload workflow for a single node
3. **UploadMonitorJob**: Monitors all running uploads and updates their progress

## Architecture

### CronScheduler

The `CronScheduler` uses the `robfig/cron` library to execute jobs on cron schedules. It provides:

- Job registration with cron expressions
- Panic recovery for individual jobs
- Graceful shutdown with timeout support
- Concurrent job execution with proper synchronization

### NodeUploadJob

The `NodeUploadJob` implements the complete upload workflow for a node:

1. **Check Upload Status**: Verifies if an upload is already running
2. **Collect Metrics**: Invokes the protocol module to gather node metrics
3. **Store Metrics**: Persists metrics to the database
4. **Initiate Upload**: Starts the snapshot upload process
5. **Send Notifications**: Alerts on failures, skips, and completions

### UploadMonitorJob

The `UploadMonitorJob` monitors all running uploads:

- Queries database for running uploads
- Checks progress for each upload independently
- Updates database with current progress
- Implements node isolation (failures don't affect other nodes)

## Usage

### Creating a Scheduler

```go
logger := logrus.New()
scheduler := scheduler.NewCronScheduler(logger)
```

### Adding Jobs

```go
// Add a node upload job that runs every 6 hours
nodeJob := scheduler.NewNodeUploadJob(
    "ethereum-mainnet",
    nodeConfig,
    protocolRegistry,
    uploadManager,
    db,
    notifyRegistry,
    notifyConfig,
    logger,
)

err := scheduler.AddJob("0 */6 * * *", nodeJob)
if err != nil {
    log.Fatalf("Failed to add job: %v", err)
}

// Add an upload monitor job that runs every minute
monitorJob := scheduler.NewUploadMonitorJob(
    uploadManager,
    db,
    logger,
)

err = scheduler.AddJob("* * * * *", monitorJob)
if err != nil {
    log.Fatalf("Failed to add monitor job: %v", err)
}
```

### Starting and Stopping

```go
// Start the scheduler
scheduler.Start()

// Wait for shutdown signal
<-shutdownChan

// Stop gracefully with timeout
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := scheduler.Stop(ctx); err != nil {
    log.Printf("Scheduler stop timeout: %v", err)
}
```

## Node Isolation

The scheduler implements node isolation to ensure that failures in one node don't affect others:

- Each `NodeUploadJob` runs independently
- Errors are logged but don't stop the scheduler
- The `UploadMonitorJob` monitors uploads concurrently
- Failed uploads are tracked but don't block other uploads

## Workflow Coordination

The scheduler coordinates the complete upload workflow:

```
NodeUploadJob (per-node schedule):
  1. Check if upload is running → Skip if yes
  2. Collect metrics via protocol module
  3. Store metrics in database
  4. Initiate upload command
  5. Send notifications based on result

UploadMonitorJob (every minute):
  1. Query database for running uploads
  2. Check progress for each upload
  3. Update database with current progress
  4. Mark uploads as complete when finished
```

## Error Handling

The scheduler implements robust error handling:

- **Panic Recovery**: Jobs that panic are recovered and logged
- **Node Isolation**: Errors in one node don't affect others
- **Retry Logic**: Database operations use exponential backoff
- **Graceful Shutdown**: In-progress jobs are allowed to complete

## Testing

The package includes comprehensive tests:

- Scheduler lifecycle (start/stop)
- Job execution and scheduling
- Panic recovery
- Node isolation
- Upload workflow
- Notification sending
- Upload monitoring

Run tests with:

```bash
go test ./internal/scheduler/...
```

## Dependencies

- `github.com/robfig/cron/v3`: Cron scheduling
- `github.com/sirupsen/logrus`: Structured logging
- Internal packages: config, protocol, notification, database, upload

## Configuration

Jobs are configured via the daemon's YAML configuration:

```yaml
# Global schedule for status updates
schedule: "* * * * *"

# Node-specific schedules
nodes:
  ethereum-mainnet:
    schedule: "0 */6 * * *"  # Every 6 hours
    protocol: ethereum
    # ... other config
```

The scheduler uses:
- Global schedule for the upload monitor job
- Per-node schedules for node upload jobs
- Defaults to every minute if not specified
