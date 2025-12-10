# Database Setup Guide

This guide covers database setup and management for the Snapshot Daemon.

## Quick Setup

### 1. Create Database and User

```bash
sudo -u postgres psql
```

```sql
CREATE DATABASE snapd;
CREATE USER snapd WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE snapd TO snapd;
\q
```

### 2. Create Schema

**Option A: Automatic (Recommended)**

The daemon automatically runs migrations on startup. Just start the daemon:

```bash
snapd --config /etc/snapd/config.yaml
```

**Option B: Manual**

Apply the schema manually before starting the daemon:

```bash
psql -h localhost -U snapd -d snapd -f agent/schema.sql
```

## Schema Overview

### Tables

#### `node_metrics`
Stores collected metrics from blockchain nodes.

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| node_name | VARCHAR(255) | Node identifier from config |
| protocol | VARCHAR(50) | Protocol type (ethereum, arbitrum) |
| node_type | VARCHAR(50) | Node type (archive, full) |
| collected_at | TIMESTAMP | When metrics were collected |
| metrics | JSONB | JSON object with metric data |

**Indexes:**
- `idx_node_metrics_node_time` on `(node_name, collected_at DESC)`

#### `uploads`
Tracks snapshot upload operations.

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| node_name | VARCHAR(255) | Node identifier |
| started_at | TIMESTAMP | Upload start time |
| completed_at | TIMESTAMP | Upload completion time (NULL if running) |
| status | VARCHAR(50) | Status: running, completed, failed |
| progress | JSONB | Progress information |
| trigger_type | VARCHAR(20) | How upload was triggered: scheduled, manual |
| error_message | TEXT | Error details if failed |

**Indexes:**
- `idx_uploads_node_status` on `(node_name, status)`
- `idx_uploads_started` on `(started_at DESC)`

#### `upload_progress`
Records progress checks during upload operations.

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| upload_id | BIGINT | Foreign key to uploads.id |
| checked_at | TIMESTAMP | When progress was checked |
| progress_data | JSONB | Progress snapshot |

**Indexes:**
- `idx_upload_progress_upload` on `(upload_id, checked_at DESC)`

## Common Queries

### Check Recent Metrics

```sql
SELECT node_name, protocol, collected_at, metrics
FROM node_metrics
ORDER BY collected_at DESC
LIMIT 10;
```

### View Running Uploads

```sql
SELECT node_name, started_at, status, progress
FROM uploads
WHERE status = 'running'
ORDER BY started_at DESC;
```

### Upload History for a Node

```sql
SELECT started_at, completed_at, status, trigger_type, error_message
FROM uploads
WHERE node_name = 'ethereum-mainnet'
ORDER BY started_at DESC
LIMIT 20;
```

### Upload Success Rate

```sql
SELECT 
    node_name,
    COUNT(*) as total_uploads,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as successful,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
    ROUND(100.0 * SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) / COUNT(*), 2) as success_rate
FROM uploads
WHERE completed_at IS NOT NULL
GROUP BY node_name;
```

### Recent Upload Progress

```sql
SELECT 
    u.node_name,
    u.started_at,
    up.checked_at,
    up.progress_data
FROM upload_progress up
JOIN uploads u ON up.upload_id = u.id
WHERE u.node_name = 'ethereum-mainnet'
ORDER BY up.checked_at DESC
LIMIT 10;
```

## Maintenance

### Cleanup Old Metrics

Keep only the last 30 days of metrics:

```sql
DELETE FROM node_metrics
WHERE collected_at < NOW() - INTERVAL '30 days';
```

### Cleanup Old Upload Progress

Keep only the last 90 days of progress records:

```sql
DELETE FROM upload_progress
WHERE checked_at < NOW() - INTERVAL '90 days';
```

### Vacuum Database

Reclaim space after deletions:

```sql
VACUUM ANALYZE node_metrics;
VACUUM ANALYZE uploads;
VACUUM ANALYZE upload_progress;
```

## Backup and Restore

### Backup Database

```bash
# Full database backup
pg_dump -h localhost -U snapd snapd > snapd_backup_$(date +%Y%m%d).sql

# Compressed backup
pg_dump -h localhost -U snapd snapd | gzip > snapd_backup_$(date +%Y%m%d).sql.gz
```

### Restore Database

```bash
# Restore from backup
psql -h localhost -U snapd snapd < snapd_backup_20241209.sql

# Restore from compressed backup
gunzip -c snapd_backup_20241209.sql.gz | psql -h localhost -U snapd snapd
```

### Automated Backups

Add to crontab for daily backups:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * pg_dump -h localhost -U snapd snapd | gzip > /var/backups/snapd/snapd_$(date +\%Y\%m\%d).sql.gz
```

## Monitoring

### Database Size

```sql
SELECT pg_size_pretty(pg_database_size('snapd')) as database_size;
```

### Table Sizes

```sql
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

### Connection Count

```sql
SELECT count(*) as connections
FROM pg_stat_activity
WHERE datname = 'snapd';
```

### Active Queries

```sql
SELECT pid, usename, state, query, query_start
FROM pg_stat_activity
WHERE datname = 'snapd' AND state != 'idle'
ORDER BY query_start;
```

## Troubleshooting

### Connection Issues

```bash
# Test connection
psql -h localhost -U snapd -d snapd -c "SELECT 1;"

# Check PostgreSQL is running
sudo systemctl status postgresql

# Check PostgreSQL logs
sudo journalctl -u postgresql -n 50
```

### Permission Issues

```sql
-- Grant all privileges on database
GRANT ALL PRIVILEGES ON DATABASE snapd TO snapd;

-- Grant privileges on all tables
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO snapd;

-- Grant privileges on sequences (for SERIAL columns)
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO snapd;
```

### Migration Failures

If automatic migrations fail:

1. Check daemon logs:
   ```bash
   sudo journalctl -u snapd | grep -i migration
   ```

2. Manually apply schema:
   ```bash
   psql -h localhost -U snapd -d snapd -f agent/schema.sql
   ```

3. Verify tables exist:
   ```bash
   psql -h localhost -U snapd -d snapd -c "\dt"
   ```

## Security Best Practices

1. **Use strong passwords** for database user
2. **Restrict network access** to PostgreSQL (use `pg_hba.conf`)
3. **Enable SSL/TLS** for database connections
4. **Regular backups** with encryption
5. **Monitor access logs** for suspicious activity
6. **Rotate credentials** periodically
7. **Use connection pooling** (already configured in daemon)

## Performance Tuning

### PostgreSQL Configuration

Edit `/etc/postgresql/*/main/postgresql.conf`:

```ini
# Increase shared buffers for better performance
shared_buffers = 256MB

# Increase work memory for complex queries
work_mem = 16MB

# Increase maintenance work memory for VACUUM
maintenance_work_mem = 128MB

# Enable query planning statistics
track_activities = on
track_counts = on
```

Restart PostgreSQL after changes:

```bash
sudo systemctl restart postgresql
```

### Index Maintenance

Rebuild indexes periodically:

```sql
REINDEX TABLE node_metrics;
REINDEX TABLE uploads;
REINDEX TABLE upload_progress;
```

## Additional Resources

- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [INSTALL.md](INSTALL.md) - Full installation guide
- [README.md](README.md) - General usage documentation
