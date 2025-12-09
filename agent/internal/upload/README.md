# Upload Management Module

The upload module provides functionality for managing snapshot uploads for blockchain nodes. It handles checking upload status, initiating uploads, and monitoring upload progress.

## Overview

This module implements the upload management logic as specified in the requirements:
- Checks if an upload is currently running using `bv n j <node> info upload`
- Initiates uploads using `bv n run upload <node>`
- Monitors upload progress at regular intervals
- Persists upload records and progress to the database
- Implements skip logic to prevent concurrent uploads

## Components

### Manager

The `Manager` type is the main interface for upload operations. It coordinates between the command executor and database.

```go
manager := upload.NewManager(executor, db, logger)
```

### Key Methods

#### CheckUploadStatus

Checks if an upload is currently running for a node by executing the status check command.

```go
status, err := manager.CheckUploadStatus(ctx, "ethereum-mainnet")
if err != nil {
    // Handle error
}
if status.IsRunning {
    // Upload is running
}
```

#### ShouldSkipUpload

Determines if an upload should be skipped because one is already running. Checks both the database and the actual command output.

```go
shouldSkip, err := manager.ShouldSkipUpload(ctx, "ethereum-mainnet")
if err != nil {
    // Handle error
}
if shouldSkip {
    // Skip this upload cycle
}
```

#### InitiateUpload

Starts a new upload for a node and creates a database record.

```go
uploadID, err := manager.InitiateUpload(ctx, "ethereum-mainnet", "scheduled")
if err != nil {
    // Handle error
}
// Upload started with ID: uploadID
```

#### MonitorUploadProgress

Checks the current progress of an upload and updates the database. If the upload has completed, it updates the completion timestamp.

```go
err := manager.MonitorUploadProgress(ctx, uploadID, "ethereum-mainnet")
if err != nil {
    // Handle error
}
```

## Upload Status Parsing

The module parses the output from `bv n j <node> info upload` which returns a key-value format:

### Running Upload Example
```
status:           2025-12-09 18:08:56 UTC| Running
progress:         0.18% (6/3252 multi-client upload (in progress clients))
restart_count:    0
upgrade_blocking: true
logs:             <empty>
```

### Completed Upload Example
```
status:           2025-12-07 13:41:43 UTC| Finished with exit code 0 and message 'Multi-client upload completed successfully'
progress:         100.00% (3248/3248 multi-client upload completed)
restart_count:    0
upgrade_blocking: true
logs:             <empty>
```

### Parsed Fields

The parser extracts the following information:
- **status**: Full status line (determines if upload is running)
- **progress**: Full progress line
- **progress_percent**: Extracted percentage (e.g., "75.50")
- **chunks_completed**: Number of completed chunks (e.g., "3100")
- **chunks_total**: Total number of chunks (e.g., "3248")
- **restart_count**: Number of job restarts
- **upgrade_blocking**: Whether the job blocks upgrades
- **logs**: Log output from the job
- **raw_output**: Complete original output for debugging

### Status Detection

The upload is considered **running** if the status line contains "Running".

The upload is considered **not running** if the status line contains:
- "Finished"
- "Completed"
- "Failed"
- "exit code"

Empty output or "no job" messages also indicate no running upload.

## Database Integration

The module persists upload information to three tables:

### uploads table
- Stores upload records with start/completion times
- Tracks upload status and trigger type (scheduled/manual)
- Stores progress data as JSONB

### upload_progress table
- Records periodic progress checks
- Links to parent upload via upload_id
- Stores progress snapshots as JSONB

## Command Construction

The module constructs commands according to the specification:

**Status Check**: `bv n j <node_name> info upload`
**Initiate Upload**: `bv n run upload <node_name>`

These commands are protocol-agnostic and work the same way for all node types (Ethereum, Arbitrum, etc.).

## Error Handling

The module implements robust error handling:
- Command execution failures are logged and returned as errors
- Database failures are handled by the database layer's retry logic
- Failed uploads don't affect other nodes (isolation)
- Errors include context about the node and operation

## Usage Example

```go
// Create manager
manager := upload.NewManager(executor, db, logger)

// Check if we should skip
shouldSkip, err := manager.ShouldSkipUpload(ctx, nodeName)
if err != nil {
    return err
}

if shouldSkip {
    logger.Info("Upload already running, skipping")
    return nil
}

// Initiate upload
uploadID, err := manager.InitiateUpload(ctx, nodeName, "scheduled")
if err != nil {
    return err
}

// Monitor progress (typically called on a schedule)
ticker := time.NewTicker(1 * time.Minute)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        err := manager.MonitorUploadProgress(ctx, uploadID, nodeName)
        if err != nil {
            logger.WithError(err).Error("Failed to monitor progress")
        }
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

## Testing

The module includes comprehensive unit tests covering:
- Command construction for different node names
- Status parsing for JSON and text formats
- Database persistence of uploads and progress
- Skip logic for running uploads
- Error handling for command and database failures
- Progress monitoring and completion detection

Run tests with:
```bash
go test ./internal/upload/
```

## Requirements Validation

This implementation satisfies the following requirements:

- **2.1**: Executes `bv n j <node> info upload` command
- **2.2**: Skips upload when one is already running
- **2.3**: Proceeds with upload when none is running
- **5.1**: Executes `bv n run upload <node>` command
- **5.2**: Records upload start timestamp
- **5.3**: Stores node name and initial status
- **5.4**: Begins monitoring upload progress
- **5.5**: Uses same command format regardless of protocol
- **6.2**: Executes status check command with node name
- **6.3**: Parses progress information
- **6.4**: Updates database with current progress
- **6.5**: Records completion timestamp
- **6.6**: Uses same command format regardless of protocol
