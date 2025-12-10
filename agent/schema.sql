-- Database schema for snapd (Snapshot Daemon)
-- This file can be used to manually create the database schema
-- Alternatively, the daemon will automatically run migrations on startup

-- Uploads Table
-- Tracks snapshot upload operations and the blockchain state they contain
CREATE TABLE IF NOT EXISTS uploads (
    id BIGSERIAL PRIMARY KEY,
    node_name VARCHAR(255) NOT NULL,
    protocol VARCHAR(50) NOT NULL,
    node_type VARCHAR(50),
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP,
    status VARCHAR(50) NOT NULL,
    trigger_type VARCHAR(20) NOT NULL,
    error_message TEXT,
    -- Blockchain state data captured before upload (what the snapshot contains)
    protocol_data JSONB NOT NULL,  -- Full protocol metrics (latest_block, latest_slot, earliest_blob, etc)
    -- Current progress data (updated during upload)
    progress_percent DECIMAL(5,2),  -- e.g., 8.96, 100.00
    chunks_completed INTEGER,       -- e.g., 284
    chunks_total INTEGER,          -- e.g., 3170
    last_progress_check TIMESTAMP, -- When progress was last updated
    -- Completion metadata
    total_chunks INTEGER,          -- Total chunks in completed upload (final count)
    completion_message TEXT        -- Success/completion message from upload
);

CREATE INDEX IF NOT EXISTS idx_uploads_node_status 
ON uploads (node_name, status);

CREATE INDEX IF NOT EXISTS idx_uploads_started 
ON uploads (started_at DESC);

CREATE INDEX IF NOT EXISTS idx_uploads_completed 
ON uploads (node_name, completed_at DESC) WHERE completed_at IS NOT NULL;
