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

// Upload represents an upload operation and the blockchain state it contains
type Upload struct {
	ID                int64      `db:"id"`
	NodeName          string     `db:"node_name"`
	Protocol          string     `db:"protocol"`
	NodeType          string     `db:"node_type"`
	StartedAt         time.Time  `db:"started_at"`
	CompletedAt       *time.Time `db:"completed_at"`
	Status            string     `db:"status"`
	TriggerType       string     `db:"trigger_type"`
	ErrorMessage      *string    `db:"error_message"`
	ProtocolData      JSONB      `db:"protocol_data"`       // Blockchain state when upload started
	ProgressPercent   *float64   `db:"progress_percent"`    // Current progress percentage
	ChunksCompleted   *int       `db:"chunks_completed"`    // Current chunks completed
	ChunksTotal       *int       `db:"chunks_total"`        // Total chunks in upload
	LastProgressCheck *time.Time `db:"last_progress_check"` // When progress was last updated
	TotalChunks       *int       `db:"total_chunks"`        // Total chunks in completed upload (final count)
	CompletionMessage *string    `db:"completion_message"`  // Success/completion message
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
		// Create new uploads table structure
		`CREATE TABLE IF NOT EXISTS uploads (
			id BIGSERIAL PRIMARY KEY,
			node_name VARCHAR(255) NOT NULL,
			protocol VARCHAR(50) NOT NULL,
			node_type VARCHAR(50),
			started_at TIMESTAMP NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMP,
			status VARCHAR(50) NOT NULL,
			trigger_type VARCHAR(20) NOT NULL,
			error_message TEXT,
			protocol_data JSONB NOT NULL,
			total_chunks INTEGER,
			completion_message TEXT
		)`,
		// Add new columns to existing uploads table
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS protocol VARCHAR(50)`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS node_type VARCHAR(50)`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS protocol_data JSONB`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS total_chunks INTEGER`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS completion_message TEXT`,
		// Add progress columns to uploads table
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS progress_percent DECIMAL(5,2)`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS chunks_completed INTEGER`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS chunks_total INTEGER`,
		`ALTER TABLE uploads ADD COLUMN IF NOT EXISTS last_progress_check TIMESTAMP`,
		// Drop old columns (will be ignored if they don't exist)
		`ALTER TABLE uploads DROP COLUMN IF EXISTS progress`,
		`ALTER TABLE uploads DROP COLUMN IF EXISTS latest_block`,
		`ALTER TABLE uploads DROP COLUMN IF EXISTS latest_slot`,
		`ALTER TABLE uploads DROP COLUMN IF EXISTS data_size_bytes`,
		// Create indexes
		`CREATE INDEX IF NOT EXISTS idx_uploads_node_status 
		 ON uploads (node_name, status)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_started 
		 ON uploads (started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_completed 
		 ON uploads (node_name, completed_at DESC) WHERE completed_at IS NOT NULL`,
		// Drop old tables
		`DROP TABLE IF EXISTS upload_progress`,
		`DROP TABLE IF EXISTS node_metrics`,
	}

	for _, migration := range migrations {
		if err := db.execWithRetry(ctx, migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// CreateUpload creates a new upload record with protocol data
func (db *DB) CreateUpload(ctx context.Context, upload Upload) (int64, error) {
	query := `INSERT INTO uploads (node_name, protocol, node_type, started_at, status, trigger_type, protocol_data, 
	                              progress_percent, chunks_completed, chunks_total, last_progress_check,
	                              total_chunks, completion_message, error_message)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	          RETURNING id`

	var id int64
	err := db.queryRowWithRetry(ctx, query, &id, upload.NodeName, upload.Protocol, upload.NodeType, upload.StartedAt, upload.Status, upload.TriggerType, upload.ProtocolData, upload.ProgressPercent, upload.ChunksCompleted, upload.ChunksTotal, upload.LastProgressCheck, upload.TotalChunks, upload.CompletionMessage, upload.ErrorMessage)
	if err != nil {
		return 0, fmt.Errorf("failed to create upload: %w", err)
	}

	return id, nil
}

// UpdateUpload updates an existing upload record
func (db *DB) UpdateUpload(ctx context.Context, upload Upload) error {
	query := `UPDATE uploads 
	          SET completed_at = $1, status = $2, error_message = $3, 
	              progress_percent = $4, chunks_completed = $5, chunks_total = $6, last_progress_check = $7,
	              total_chunks = $8, completion_message = $9
	          WHERE id = $10`

	return db.execWithRetry(ctx, query, upload.CompletedAt, upload.Status, upload.ErrorMessage, upload.ProgressPercent, upload.ChunksCompleted, upload.ChunksTotal, upload.LastProgressCheck, upload.TotalChunks, upload.CompletionMessage, upload.ID)
}

// GetRunningUploads retrieves all currently running uploads
func (db *DB) GetRunningUploads(ctx context.Context) ([]Upload, error) {
	query := `SELECT id, node_name, protocol, node_type, started_at, completed_at, status, 
	                 trigger_type, error_message, protocol_data, 
	                 progress_percent, chunks_completed, chunks_total, last_progress_check,
	                 total_chunks, completion_message
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
	query := `SELECT id, node_name, protocol, node_type, started_at, completed_at, status, 
	                 trigger_type, error_message, protocol_data,
	                 progress_percent, chunks_completed, chunks_total, last_progress_check,
	                 total_chunks, completion_message
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

// GetLatestCompletedUploadForNode retrieves the most recent completed upload for a node
func (db *DB) GetLatestCompletedUploadForNode(ctx context.Context, nodeName string) (*Upload, error) {
	query := `SELECT id, node_name, protocol, node_type, started_at, completed_at, status, 
	                 trigger_type, error_message, protocol_data,
	                 progress_percent, chunks_completed, chunks_total, last_progress_check,
	                 total_chunks, completion_message
	          FROM uploads
	          WHERE node_name = $1 AND status = 'completed' AND completed_at IS NOT NULL
	          ORDER BY completed_at DESC
	          LIMIT 1`

	var upload Upload
	err := db.getWithRetry(ctx, &upload, query, nodeName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest completed upload for node: %w", err)
	}

	return &upload, nil
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

		// Don't retry on sql.ErrNoRows - it's not a transient error
		if err == sql.ErrNoRows {
			return err
		}

		lastErr = err
	}

	return fmt.Errorf("query failed after %d retries: %w", db.maxRetries, lastErr)
}
