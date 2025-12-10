# Snapshot Daemon Installation Guide

This guide provides step-by-step instructions for installing and configuring the Snapshot Daemon (snapd) on a Linux system.

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Pre-Installation Checklist](#pre-installation-checklist)
3. [Installation Steps](#installation-steps)
4. [Configuration](#configuration)
5. [Database Setup](#database-setup)
6. [Service Installation](#service-installation)
7. [Verification](#verification)
8. [Post-Installation](#post-installation)
9. [Upgrading](#upgrading)
10. [Uninstallation](#uninstallation)

## System Requirements

### Minimum Requirements

- **Operating System**: Linux (Ubuntu 20.04+, Debian 11+, RHEL 8+, or equivalent)
- **CPU**: 2 cores
- **RAM**: 2 GB
- **Disk Space**: 1 GB for application and logs
- **Go**: 1.21+ (for building from source)
- **PostgreSQL**: 12+
- **Network**: Outbound HTTPS access for notifications

### Software Dependencies

- **bv CLI**: Blockchain validator CLI tool (must be in PATH)
- **PostgreSQL client**: For database connectivity
- **systemd**: For service management (optional but recommended)

## Pre-Installation Checklist

Before installing, ensure you have:

- [ ] Root or sudo access to the system
- [ ] PostgreSQL database server running and accessible
- [ ] Database credentials (username, password, database name)
- [ ] Node RPC endpoints accessible (Ethereum, Arbitrum, etc.)
- [ ] Notification webhook URLs (Discord, Slack, etc.) if using notifications
- [ ] `bv` CLI installed and functional

## Installation Steps

### Step 1: Download or Build the Binary

#### Option A: Build from Source

```bash
# Clone the repository
git clone https://github.com/your-org/chainsnaps.git
cd chainsnaps

# Build the daemon
make build-agent

# Verify the build
./agent/bin/snapd version
```

#### Option B: Download Pre-built Binary

```bash
# Download the latest release
wget https://github.com/your-org/chainsnaps/releases/latest/download/snapd

# Make it executable
chmod +x snapd

# Verify the binary
./snapd version
```

### Step 2: Install the Binary

```bash
# Copy to system binary directory
sudo cp snapd /usr/local/bin/
# or if built from source:
# sudo cp agent/bin/snapd /usr/local/bin/

# Verify installation
which snapd
snapd version
```

### Step 3: Create System User

Create a dedicated user for running the daemon:

```bash
# Create system user (no login shell, no home directory)
sudo useradd -r -s /bin/false snapd

# Create data directory
sudo mkdir -p /var/lib/snapperd
sudo chown snapd:snapd /var/lib/snapperd
sudo chmod 755 /var/lib/snapperd

# Create log directory
sudo mkdir -p /var/log/snapperd
sudo chown snapd:snapd /var/log/snapperd
sudo chmod 755 /var/log/snapperd
```

### Step 4: Create Configuration Directory

```bash
# Create configuration directory
sudo mkdir -p /etc/snapperd

# Set permissions
sudo chmod 755 /etc/snapd
```

## Configuration

### Step 1: Copy Configuration Files

```bash
# Copy example configuration
sudo cp agent/config.example.yaml /etc/snapperd/config.yaml

# Copy environment file
sudo cp agent/environment.example /etc/snapperd/environment
```

### Step 2: Edit Configuration

Edit the main configuration file:

```bash
sudo nano /etc/snapperd/config.yaml
```

**Minimum required changes**:

1. **Database settings**: Update host, port, database name, and user
2. **Node definitions**: Add your blockchain nodes with base URLs
3. **Node schedules**: Each node MUST have a schedule (no default) - set appropriate cron schedules for uploads
4. **Notifications**: Configure webhook URLs (optional)

**Important**: 
- Uses 6-field cron format: `"second minute hour day month weekday"`
- Global schedule is for status checks only (keep at `"0 * * * * *"`)
- Each node requires its own upload schedule (hours/days, never minutes)

Example minimal configuration:

```yaml
schedule: "0 * * * * *"

database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: ${DB_PASSWORD}
  ssl_mode: require

nodes:
  ethereum-mainnet:
    protocol: ethereum
    type: archive
    url: http://localhost:8545
    schedule: "0 0 */6 * * *"
```

### Step 3: Set Environment Variables

Edit the environment file:

```bash
sudo nano /etc/snapperd/environment
```

**Required**:
```bash
DB_PASSWORD=your_secure_database_password
```

**Optional**:
```bash
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
LOG_LEVEL=info
```

### Step 4: Secure Configuration Files

Set appropriate permissions to protect sensitive data:

```bash
# Restrict access to configuration files
sudo chmod 600 /etc/snapperd/config.yaml
sudo chmod 600 /etc/snapperd/environment

# Set ownership
sudo chown snapd:snapd /etc/snapperd/config.yaml
sudo chown snapd:snapd /etc/snapperd/environment
```

## Database Setup

### Step 1: Create Database and User

Connect to PostgreSQL as a superuser:

```bash
sudo -u postgres psql
```

Execute the following SQL commands:

```sql
-- Create database
CREATE DATABASE snapd;

-- Create user with password
CREATE USER snapd WITH PASSWORD 'your_secure_password';

-- Grant database privileges
GRANT ALL PRIVILEGES ON DATABASE snapd TO snapd;

-- Connect to the snapd database to set schema permissions
\c snapd

-- Grant schema permissions (required for table creation)
GRANT ALL ON SCHEMA public TO snapd;
GRANT CREATE ON SCHEMA public TO snapd;

-- Grant privileges on existing tables and sequences
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO snapd;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO snapd;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO snapd;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO snapd;

-- Exit
\q
```

### Step 2: Create Database Schema

**Option A: Automatic Migration (Recommended)**

The daemon will automatically create the required tables on first run via the built-in migration system. No manual action needed - just start the daemon and it will handle schema creation.

**Option B: Manual Schema Creation**

If you prefer to create the schema manually before starting the daemon:

```bash
# Apply the schema from the SQL file
psql -h localhost -U snapd -d snapd -f agent/schema.sql
```

The schema includes:

- `node_metrics`: Stores RPC query results and collected metrics
- `uploads`: Tracks upload operations (status, timing, errors)
- `upload_progress`: Monitors upload progress over time

To verify the schema was created:

```bash
# Connect to the database
psql -h localhost -U snapd -d snapd

# List tables
\dt

# Should show: node_metrics, uploads, upload_progress

# Exit
\q
```

### Step 3: Test Database Connection

```bash
# Test connection with environment variable
export DB_PASSWORD=your_secure_password
psql -h localhost -U snapd -d snapd -c "SELECT 1;"
```

Expected output: `1` (one row)

## Service Installation

### Step 1: Copy Systemd Unit File

```bash
# Copy the service file
sudo cp agent/snapperd.service /etc/systemd/system/

# Verify the file
cat /etc/systemd/system/snapperd.service
```

### Step 2: Reload Systemd

```bash
# Reload systemd to recognize the new service
sudo systemctl daemon-reload
```

### Step 3: Enable the Service

```bash
# Enable service to start on boot
sudo systemctl enable snapperd

# Verify it's enabled
sudo systemctl is-enabled snapd
```

### Step 4: Start the Service

```bash
# Start the service
sudo systemctl start snapperd

# Check status
sudo systemctl status snapperd
```

Expected output should show "active (running)".

## Verification

### Step 1: Check Service Status

```bash
# Check if service is running
sudo systemctl status snapperd

# Should show:
# Active: active (running)
```

### Step 2: View Logs

```bash
# View recent logs
sudo journalctl -u snapperd -n 50

# Follow logs in real-time
sudo journalctl -u snapperd -f
```

Look for:
- Successful startup messages
- Database connection confirmation
- Scheduler initialization
- No error messages

### Step 3: Test CLI Commands

```bash
# Check version
snapperd version

# Check status
snapperd status --config /etc/snapperd/config.yaml

# Should show either running uploads or "No active uploads"
```

### Step 4: Verify Database Connectivity

```bash
# Check if daemon can connect to database
sudo journalctl -u snapperd | grep -i database

# Should show successful connection messages
```

### Step 5: Test Manual Upload (Optional)

```bash
# Trigger a manual upload for a configured node
snapperd upload ethereum-mainnet --config /etc/snapperd/config.yaml

# Check status
snapperd status --config /etc/snapperd/config.yaml
```

## Post-Installation

### Configure Log Rotation

Create a logrotate configuration:

```bash
sudo nano /etc/logrotate.d/snapperd
```

Add:

```
/var/log/snapd/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 snapd snapd
    sharedscripts
    postrotate
        systemctl reload snapd > /dev/null 2>&1 || true
    endscript
}
```

### Set Up Monitoring

Consider setting up monitoring for:

- Service health: `systemctl status snapd`
- Database connectivity
- Upload success/failure rates
- Disk space in `/var/lib/snapperd`
- Log file sizes

### Configure Firewall (if applicable)

If using a firewall, ensure outbound connections are allowed:

```bash
# Allow outbound HTTPS for notifications
sudo ufw allow out 443/tcp

# Allow PostgreSQL if on remote host
sudo ufw allow out 5432/tcp
```

### Backup Configuration

Create backups of your configuration:

```bash
# Create backup directory
sudo mkdir -p /var/backups/snapd

# Backup configuration
sudo cp /etc/snapperd/config.yaml /var/backups/snapd/config.yaml.backup
sudo cp /etc/snapperd/environment /var/backups/snapd/environment.backup

# Set permissions
sudo chmod 600 /var/backups/snapd/*
```

## Upgrading

### Upgrade Process

1. **Stop the service**:
   ```bash
   sudo systemctl stop snapperd
   ```

2. **Backup current installation**:
   ```bash
   sudo cp /usr/local/bin/snapd /usr/local/bin/snapd.backup
   sudo cp /etc/snapperd/config.yaml /etc/snapperd/config.yaml.backup
   ```

3. **Install new binary**:
   ```bash
   # Download or build new version
   sudo cp snapd /usr/local/bin/
   sudo chmod +x /usr/local/bin/snapd
   ```

4. **Check for configuration changes**:
   ```bash
   # Compare with new example config
   diff /etc/snapperd/config.yaml agent/config.example.yaml
   ```

5. **Update configuration if needed**:
   ```bash
   sudo nano /etc/snapperd/config.yaml
   ```

6. **Restart the service**:
   ```bash
   sudo systemctl start snapperd
   sudo systemctl status snapperd
   ```

7. **Verify upgrade**:
   ```bash
   snapd version
   sudo journalctl -u snapperd -n 50
   ```

### Rollback (if needed)

If the upgrade fails:

```bash
# Stop the service
sudo systemctl stop snapperd

# Restore backup
sudo cp /usr/local/bin/snapd.backup /usr/local/bin/snapd
sudo cp /etc/snapperd/config.yaml.backup /etc/snapperd/config.yaml

# Restart service
sudo systemctl start snapperd
```

## Uninstallation

### Complete Removal

1. **Stop and disable the service**:
   ```bash
   sudo systemctl stop snapperd
   sudo systemctl disable snapd
   ```

2. **Remove systemd unit file**:
   ```bash
   sudo rm /etc/systemd/system/snapperd.service
   sudo systemctl daemon-reload
   ```

3. **Remove binary**:
   ```bash
   sudo rm /usr/local/bin/snapd
   ```

4. **Remove configuration** (optional):
   ```bash
   sudo rm -rf /etc/snapd
   ```

5. **Remove data directory** (optional):
   ```bash
   sudo rm -rf /var/lib/snapperd
   ```

6. **Remove system user** (optional):
   ```bash
   sudo userdel snapd
   ```

7. **Remove database** (optional):
   ```bash
   sudo -u postgres psql -c "DROP DATABASE snapd;"
   sudo -u postgres psql -c "DROP USER snapd;"
   ```

### Partial Removal (Keep Data)

To remove the daemon but keep configuration and data:

```bash
# Stop and disable service
sudo systemctl stop snapperd
sudo systemctl disable snapd

# Remove binary only
sudo rm /usr/local/bin/snapd

# Keep /etc/snapd and /var/lib/snapperd for future reinstallation
```

## Troubleshooting Installation

### Issue: Binary not found after installation

**Solution**:
```bash
# Verify binary location
ls -la /usr/local/bin/snapd

# Check PATH
echo $PATH

# Ensure /usr/local/bin is in PATH
export PATH=$PATH:/usr/local/bin
```

### Issue: Permission denied when starting service

**Solution**:
```bash
# Check binary permissions
ls -la /usr/local/bin/snapd

# Should be executable
sudo chmod +x /usr/local/bin/snapd

# Check service file permissions
ls -la /etc/systemd/system/snapperd.service
```

### Issue: Database connection failed

**Solution**:
```bash
# Test database connection manually
psql -h localhost -U snapd -d snapd

# Check PostgreSQL is running
sudo systemctl status postgresql

# Verify credentials in /etc/snapperd/environment
sudo cat /etc/snapperd/environment | grep DB_PASSWORD
```

### Issue: "permission denied for schema public" during migration

This error occurs when the snapd user doesn't have permission to create tables.

**Solution**:

**Option 1: Use the provided script (easiest)**:
```bash
# Run the permission fix script
./agent/fix-db-permissions.sh

# Restart the daemon
sudo systemctl restart snapperd
```

### Issue: "Failed to set up mount namespacing" or "No such file or directory"

This error occurs when required directories don't exist for the systemd service.

**Solution**:

**Option 1: Use the provided script (easiest)**:
```bash
# Run the directory fix script
./agent/fix-systemd-directories.sh

# Restart the daemon
sudo systemctl restart snapperd
```

**Option 2: Manual fix**:
```bash
# Create required directories
sudo mkdir -p /var/lib/snapperd /var/log/snapperd /etc/snapperd
sudo chown snapd:snapd /var/lib/snapperd /var/log/snapperd
sudo chmod 755 /var/lib/snapperd /var/log/snapperd /etc/snapperd

# Restart the daemon
sudo systemctl restart snapperd
```

**Option 2: Manual fix**:
```bash
# Quick fix - run as postgres superuser
sudo -u postgres psql -d snapd -c "
GRANT ALL ON SCHEMA public TO snapd;
GRANT CREATE ON SCHEMA public TO snapd;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO snapd;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO snapd;"

# Restart the daemon
sudo systemctl restart snapperd
```

**Alternative solution** - Make snapd the database owner:
```bash
sudo -u postgres psql -c "ALTER DATABASE snapd OWNER TO snapd;"
```

### Issue: Configuration file not found

**Solution**:
```bash
# Check file exists
ls -la /etc/snapperd/config.yaml

# Check permissions
sudo chmod 600 /etc/snapperd/config.yaml
sudo chown snapd:snapd /etc/snapperd/config.yaml

# Verify path in systemd unit
grep ExecStart /etc/systemd/system/snapperd.service
```

## Next Steps

After successful installation:

1. **Monitor the logs** for the first few hours to ensure everything is working
2. **Test manual uploads** to verify the workflow
3. **Set up monitoring** and alerting for production use
4. **Configure backups** for the database
5. **Review security settings** and harden as needed
6. **Document your specific configuration** for your team

For more information, see:
- [README.md](README.md) - General usage and architecture
- [config.example.yaml](config.example.yaml) - Configuration reference
- [.kiro/specs/snapshot-daemon/](../.kiro/specs/snapshot-daemon/) - Detailed specifications

## Support

If you encounter issues not covered in this guide:

1. Check the logs: `sudo journalctl -u snapperd -f`
2. Review the troubleshooting section in README.md
3. Verify your configuration against config.example.yaml
4. Report issues on the GitHub issue tracker
