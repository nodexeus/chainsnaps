# Database Package

This package provides database connectivity and persistence for the Snapshot Daemon.

## Features

- **Connection Pooling**: Configured with 25 max open connections, 5 max idle connections, and 5-minute connection lifetime
- **Automatic Migrations**: Creates required tables and indexes on startup
- **Retry Logic**: Exponential backoff with 3 retries for transient failures
- **JSONB Support**: Custom type for PostgreSQL JSONB columns
- **Context Support**: All operations support context cancellation

## Usage

### Creating a Connection

```go
cfg := database.Config{
    Host:     "localhost",
    Port:     5432,
    Database: "snapd",
    User:     "snapd",
    Password: "password",
    SSLMode:  "require",
}

db, err := database.New(context.Background(), cfg)
if err != nil {
    log.Fatal(err)
}
defer db.Close()
```

### Running Migrations

```go
if err := db.Migrate(context.Background()); err != nil {
    log.Fatal(err)
}
```

### Storing Node Metrics

```go
metrics := database.NodeMetrics{
    NodeName:    "ethereum-mainnet",
    Protocol:    "ethereum",
    NodeType:    "archive",
    CollectedAt: time.Now(),
    Metrics: database.JSONB{
        "latest_block": 12345,
        "latest_slot":  67890,
    },
}

if err := db.StoreNodeMetrics(ctx, metrics); err != nil {
    log.Printf("failed to store metrics: %v", err)
}
```

### Creating an Upload

```go
upload := database.Upload{
    NodeName:    "ethereum-mainnet",
    StartedAt:   time.Now(),
    Status:      "running",
    TriggerType: "scheduled",
    Progress: database.JSONB{
        "percent": 0,
    },
}

id, err := db.CreateUpload(ctx, upload)
if err != nil {
    log.Printf("failed to create upload: %v", err)
}
```

### Updating an Upload

```go
upload.ID = id
upload.Status = "completed"
completedAt := time.Now()
upload.CompletedAt = &completedAt
upload.Progress = database.JSONB{
    "percent": 100,
}

if err := db.UpdateUpload(ctx, upload); err != nil {
    log.Printf("failed to update upload: %v", err)
}
```

### Checking for Running Uploads

```go
// Get all running uploads
uploads, err := db.GetRunningUploads(ctx)
if err != nil {
    log.Printf("failed to get running uploads: %v", err)
}

// Get running upload for specific node
upload, err := db.GetRunningUploadForNode(ctx, "ethereum-mainnet")
if err != nil {
    log.Printf("failed to get running upload: %v", err)
}
if upload == nil {
    log.Println("no running upload for node")
}
```

### Storing Upload Progress

```go
progress := database.UploadProgress{
    UploadID:  uploadID,
    CheckedAt: time.Now(),
    ProgressData: database.JSONB{
        "percent":       50,
        "bytes_uploaded": 1024000,
    },
}

if err := db.StoreUploadProgress(ctx, progress); err != nil {
    log.Printf("failed to store progress: %v", err)
}
```

## Database Schema

### node_metrics

Stores metrics collected from blockchain nodes.

- `id`: Auto-incrementing primary key
- `node_name`: Name of the node
- `protocol`: Protocol type (ethereum, arbitrum, etc.)
- `node_type`: Node type (archive, full, etc.)
- `collected_at`: Timestamp when metrics were collected
- `metrics`: JSONB column containing metric data

### uploads

Tracks snapshot upload operations.

- `id`: Auto-incrementing primary key
- `node_name`: Name of the node
- `started_at`: When the upload started
- `completed_at`: When the upload completed (nullable)
- `status`: Current status (running, completed, failed)
- `progress`: JSONB column containing progress data
- `trigger_type`: How the upload was triggered (scheduled, manual)
- `error_message`: Error details if upload failed (nullable)

### upload_progress

Records progress checks for uploads.

- `id`: Auto-incrementing primary key
- `upload_id`: Foreign key to uploads table
- `checked_at`: When the progress was checked
- `progress_data`: JSONB column containing progress details

## Retry Logic

All database operations automatically retry on failure with exponential backoff:

- **Max Retries**: 3
- **Base Delay**: 100ms
- **Backoff**: Doubles on each retry (100ms, 200ms, 400ms)
- **Context Aware**: Respects context cancellation

## Error Handling

The package returns wrapped errors with context:

```go
if err := db.StoreNodeMetrics(ctx, metrics); err != nil {
    // Error will be wrapped with context like:
    // "operation failed after 3 retries: <original error>"
    log.Printf("database error: %v", err)
}
```

## Connection Pooling

The database connection pool is configured for optimal performance:

- **MaxOpenConns**: 25 - Maximum number of open connections
- **MaxIdleConns**: 5 - Maximum number of idle connections
- **ConnMaxLifetime**: 5 minutes - Maximum connection lifetime

These settings balance performance with resource usage for typical daemon workloads.
