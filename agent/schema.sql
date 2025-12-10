-- Database schema for snapd (Snapshot Daemon)
-- This file can be used to manually create the database schema
-- Alternatively, the daemon will automatically run migrations on startup

-- Node Metrics Table
-- Stores collected metrics from blockchain nodes
CREATE TABLE IF NOT EXISTS node_metrics (
    id BIGSERIAL PRIMARY KEY,
    node_name VARCHAR(255) NOT NULL,
    protocol VARCHAR(50) NOT NULL,
    node_type VARCHAR(50),
    collected_at TIMESTAMP NOT NULL DEFAULT NOW(),
    metrics JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_node_time 
ON node_metrics (node_name, collected_at DESC);

-- Uploads Table
-- Tracks snapshot upload operations
CREATE TABLE IF NOT EXISTS uploads (
    id BIGSERIAL PRIMARY KEY,
    node_name VARCHAR(255) NOT NULL,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    status VARCHAR(50) NOT NULL,
    progress JSONB,
    progress_percent DECIMAL(5,2),  -- e.g., 8.96, 100.00
    chunks_completed INTEGER,       -- e.g., 284
    chunks_total INTEGER,          -- e.g., 3170
    trigger_type VARCHAR(20) NOT NULL,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_uploads_node_status 
ON uploads (node_name, status);

CREATE INDEX IF NOT EXISTS idx_uploads_started 
ON uploads (started_at DESC);

-- Upload Progress Table
-- Records progress checks during upload operations
CREATE TABLE IF NOT EXISTS upload_progress (
    id BIGSERIAL PRIMARY KEY,
    upload_id BIGINT NOT NULL REFERENCES uploads(id),
    checked_at TIMESTAMP NOT NULL DEFAULT NOW(),
    progress_data JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_upload_progress_upload 
ON upload_progress (upload_id, checked_at DESC);
