package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DB wraps the database connection with retry logic
type DB struct {
	conn           *sqlx.DB
	maxRetries     int
	retryBaseDelay time.Duration
}

// Config holds database connection configuration
type Config struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

// NodeMetrics represents collected metrics for storage
type NodeMetrics struct {
	ID          int64     `db:"id"`
	NodeName    string    `db:"node_name"`
	Protocol    string    `db:"protocol"`
	NodeType    string    `db:"node_type"`
	CollectedAt time.Time `db:"collected_at"`
	Metrics     JSONB     `db:"metrics"`
}

// Upload represents an upload operation
type Upload struct {
	ID           int64      `db:"id"`
	NodeName     string     `db:"node_name"`
	StartedAt    time.Time  `db:"started_at"`
	CompletedAt  *time.Time `db:"completed_at"`
	Status       string     `db:"status"`
	Progress     JSONB      `db:"progress"`
	TriggerType  string     `db:"trigger_type"`
	ErrorMessage *string    `db:"error_message"`
}

// UploadProgress represents a progress check for an upload
type UploadProgress struct {
	ID           int64     `db:"id"`
	UploadID     int64     `db:"upload_id"`
	CheckedAt    time.Time `db:"checked_at"`
	ProgressData JSONB     `db:"progress_data"`
}

// New creates a new database connection with connection pooling
func New(ctx context.Context, cfg Config) (*DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	conn, err := sqlx.ConnectContext(ctx, "postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	db := &DB{
		conn:           conn,
		maxRetries:     3,
		retryBaseDelay: 100 * time.Millisecond,
	}

	return db, nil
}

// Close closes the database connection gracefully
func (db *DB) Close() error {
	return db.conn.Close()
}

// Migrate runs database migrations to create required tables
func (db *DB) Migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS node_metrics (
			id BIGSERIAL PRIMARY KEY,
			node_name VARCHAR(255) NOT NULL,
			protocol VARCHAR(50) NOT NULL,
			node_type VARCHAR(50),
			collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
			metrics JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_node_metrics_node_time 
		 ON node_metrics (node_name, collected_at DESC)`,
		`CREATE TABLE IF NOT EXISTS uploads (
			id BIGSERIAL PRIMARY KEY,
			node_name VARCHAR(255) NOT NULL,
			started_at TIMESTAMP NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMP,
			status VARCHAR(50) NOT NULL,
			progress JSONB,
			trigger_type VARCHAR(20) NOT NULL,
			error_message TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_node_status 
		 ON uploads (node_name, status)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_started 
		 ON uploads (started_at DESC)`,
		`CREATE TABLE IF NOT EXISTS upload_progress (
			id BIGSERIAL PRIMARY KEY,
			upload_id BIGINT NOT NULL REFERENCES uploads(id),
			checked_at TIMESTAMP NOT NULL DEFAULT NOW(),
			progress_data JSONB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_upload_progress_upload 
		 ON upload_progress (upload_id, checked_at DESC)`,
	}

	for _, migration := range migrations {
		if err := db.execWithRetry(ctx, migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// StoreNodeMetrics persists node metrics to the database
func (db *DB) StoreNodeMetrics(ctx context.Context, metrics NodeMetrics) error {
	query := `INSERT INTO node_metrics (node_name, protocol, node_type, collected_at, metrics)
	          VALUES ($1, $2, $3, $4, $5)`

	return db.execWithRetry(ctx, query, metrics.NodeName, metrics.Protocol, metrics.NodeType, metrics.CollectedAt, metrics.Metrics)
}

// CreateUpload creates a new upload record
func (db *DB) CreateUpload(ctx context.Context, upload Upload) (int64, error) {
	query := `INSERT INTO uploads (node_name, started_at, status, progress, trigger_type)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING id`

	var id int64
	err := db.queryRowWithRetry(ctx, query, &id, upload.NodeName, upload.StartedAt, upload.Status, upload.Progress, upload.TriggerType)
	if err != nil {
		return 0, fmt.Errorf("failed to create upload: %w", err)
	}

	return id, nil
}

// UpdateUpload updates an existing upload record
func (db *DB) UpdateUpload(ctx context.Context, upload Upload) error {
	query := `UPDATE uploads 
	          SET completed_at = $1, status = $2, progress = $3, error_message = $4
	          WHERE id = $5`

	return db.execWithRetry(ctx, query, upload.CompletedAt, upload.Status, upload.Progress, upload.ErrorMessage, upload.ID)
}

// GetRunningUploads retrieves all currently running uploads
func (db *DB) GetRunningUploads(ctx context.Context) ([]Upload, error) {
	query := `SELECT id, node_name, started_at, completed_at, status, progress, trigger_type, error_message
	          FROM uploads
	          WHERE status = 'running'
	          ORDER BY started_at DESC`

	var uploads []Upload
	err := db.queryWithRetry(ctx, &uploads, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get running uploads: %w", err)
	}

	return uploads, nil
}

// GetRunningUploadForNode retrieves a running upload for a specific node
func (db *DB) GetRunningUploadForNode(ctx context.Context, nodeName string) (*Upload, error) {
	query := `SELECT id, node_name, started_at, completed_at, status, progress, trigger_type, error_message
	          FROM uploads
	          WHERE node_name = $1 AND status = 'running'
	          ORDER BY started_at DESC
	          LIMIT 1`

	var upload Upload
	err := db.getWithRetry(ctx, &upload, query, nodeName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get running upload for node: %w", err)
	}

	return &upload, nil
}

// StoreUploadProgress records a progress check for an upload
func (db *DB) StoreUploadProgress(ctx context.Context, progress UploadProgress) error {
	query := `INSERT INTO upload_progress (upload_id, checked_at, progress_data)
	          VALUES ($1, $2, $3)`

	return db.execWithRetry(ctx, query, progress.UploadID, progress.CheckedAt, progress.ProgressData)
}

// execWithRetry executes a query with exponential backoff retry logic
func (db *DB) execWithRetry(ctx context.Context, query string, args ...interface{}) error {
	var lastErr error
	delay := db.retryBaseDelay

	for attempt := 0; attempt <= db.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2 // Exponential backoff
			}
		}

		_, err := db.conn.ExecContext(ctx, query, args...)
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("operation failed after %d retries: %w", db.maxRetries, lastErr)
}

// queryRowWithRetry executes a query that returns a single row with retry logic
func (db *DB) queryRowWithRetry(ctx context.Context, query string, dest interface{}, args ...interface{}) error {
	var lastErr error
	delay := db.retryBaseDelay

	for attempt := 0; attempt <= db.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2
			}
		}

		err := db.conn.QueryRowContext(ctx, query, args...).Scan(dest)
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("query failed after %d retries: %w", db.maxRetries, lastErr)
}

// queryWithRetry executes a query that returns multiple rows with retry logic
func (db *DB) queryWithRetry(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	var lastErr error
	delay := db.retryBaseDelay

	for attempt := 0; attempt <= db.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2
			}
		}

		err := db.conn.SelectContext(ctx, dest, query, args...)
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("query failed after %d retries: %w", db.maxRetries, lastErr)
}

// getWithRetry executes a query that returns a single struct with retry logic
func (db *DB) getWithRetry(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	var lastErr error
	delay := db.retryBaseDelay

	for attempt := 0; attempt <= db.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2
			}
		}

		err := db.conn.GetContext(ctx, dest, query, args...)
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("query failed after %d retries: %w", db.maxRetries, lastErr)
}
