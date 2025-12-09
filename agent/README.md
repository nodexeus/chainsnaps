# Snapshot Daemon (snapd)

A Go-based monitoring and backup orchestration service for blockchain nodes. The daemon automatically collects metrics, manages snapshot uploads, and sends notifications for important events.

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Systemd Integration](#systemd-integration)
- [Building from Source](#building-from-source)
- [Architecture](#architecture)
- [Development](#development)

## Features

- **Automated Monitoring**: Periodic status checks and metric collection via cron schedules
- **Protocol Modules**: Pluggable support for different blockchain types (Ethereum, Arbitrum, etc.)
- **Upload Management**: Automatic snapshot upload initiation and progress tracking
- **Notification System**: Configurable alerts for failures, skips, and completions
- **Multiple Notification Types**: Support for Discord, Slack, and other notification services
- **Database Persistence**: All metrics and upload status stored in PostgreSQL
- **Graceful Shutdown**: Clean handling of SIGTERM/SIGINT with in-progress operation completion
- **CLI Subcommands**: Manual upload triggering, status checking, and version display
- **Flexible Configuration**: YAML-based config with environment variable support

## Prerequisites

- **Go 1.21+** (for building from source)
- **PostgreSQL 12+** (for data persistence)
- **bv CLI** (blockchain validator CLI tool for executing uploads)
- **Linux system** (recommended for production deployment)

## Installation

### 1. Install the Binary

#### Option A: Build from Source

```bash
# Clone the repository
git clone <repository-url>
cd chainsnaps

# Build the daemon
make build-agent

# Install to system path
sudo cp agent/bin/snapd /usr/local/bin/
sudo chmod +x /usr/local/bin/snapd
```

#### Option B: Download Pre-built Binary

```bash
# Download the latest release
wget https://github.com/your-org/chainsnaps/releases/latest/download/snapd

# Install to system path
sudo mv snapd /usr/local/bin/
sudo chmod +x /usr/local/bin/snapd
```

### 2. Create System User

```bash
# Create a dedicated user for the daemon
sudo useradd -r -s /bin/false -d /var/lib/snapd snapd

# Create necessary directories
sudo mkdir -p /etc/snapd
sudo mkdir -p /var/lib/snapd
sudo chown snapd:snapd /var/lib/snapd
```

### 3. Set Up Database

```bash
# Connect to PostgreSQL as admin
sudo -u postgres psql

# Create database and user
CREATE DATABASE snapd;
CREATE USER snapd WITH PASSWORD 'your_secure_password';
GRANT ALL PRIVILEGES ON DATABASE snapd TO snapd;
\q
```

### 4. Configure the Daemon

```bash
# Copy example configuration
sudo cp agent/config.example.yaml /etc/snapd/config.yaml

# Copy environment file
sudo cp agent/environment.example /etc/snapd/environment

# Edit configuration
sudo nano /etc/snapd/config.yaml

# Edit environment variables (set DB_PASSWORD)
sudo nano /etc/snapd/environment

# Set secure permissions
sudo chmod 600 /etc/snapd/config.yaml
sudo chmod 600 /etc/snapd/environment
sudo chown snapd:snapd /etc/snapd/config.yaml
sudo chown snapd:snapd /etc/snapd/environment
```

### 5. Install Systemd Service

```bash
# Copy systemd unit file
sudo cp agent/snapd.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable the service
sudo systemctl enable snapd

# Start the service
sudo systemctl start snapd

# Check status
sudo systemctl status snapd
```

## Configuration

The daemon is configured via a YAML file (default: `/etc/snapd/config.yaml`). See `config.example.yaml` for a fully documented example.

### Key Configuration Sections

#### Global Schedule

```yaml
# Cron expression for status checks (default: every minute)
# This controls how often the daemon checks upload status and progress
# NOT when uploads are initiated (that's per-node)
schedule: "* * * * *"
```

**Important**: The global schedule is for monitoring only. Each node must have its own upload schedule.

#### Global Notifications

```yaml
notifications:
  failure: true      # Notify on upload failures
  skip: false        # Notify when uploads are skipped
  complete: true     # Notify on successful completion
  
  # Multiple notification types supported
  discord:
    url: https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_TOKEN
  slack:
    url: https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK
```

#### Database Connection

```yaml
database:
  host: localhost
  port: 5432
  database: snapd
  user: snapd
  password: ${DB_PASSWORD}  # Use environment variable
  ssl_mode: require
```

#### Node Definitions

```yaml
nodes:
  ethereum-mainnet:
    protocol: ethereum           # Protocol type (REQUIRED)
    type: archive               # Node type for metadata (optional)
    url: http://localhost:8545  # Base URL (REQUIRED)
    schedule: "0 */6 * * *"     # Upload schedule (REQUIRED)
    
    # Optional: Per-node notification override
    notifications:
      failure: true
      skip: true
      complete: true
      discord:
        url: https://discord.com/api/webhooks/NODE_SPECIFIC_WEBHOOK
```

**Key Points**:
- `url`: Base URL for the node. Protocol modules build specific endpoints from this.
  - Ethereum: Uses `url` for RPC, appends `/beacon` for consensus layer
  - Arbitrum: Uses `url` directly
- `schedule`: **REQUIRED** - Controls when uploads are initiated for this node
  - Must be less frequent than global schedule (hours/days, not minutes)
  - Never use `"* * * * *"` for node schedules

### Environment Variables

Environment variables can be referenced in the configuration using `${VAR_NAME}` syntax:

```yaml
database:
  password: ${DB_PASSWORD}

notifications:
  discord:
    url: ${DISCORD_WEBHOOK_URL}
```

Set environment variables in `/etc/snapd/environment`:

```bash
DB_PASSWORD=your_secure_password
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
```

## Building from Source

Build the daemon binary:

```bash
make build-agent
```

This will:
1. Run `go generate` to create build metadata (build date and git commit)
2. Build the binary at `agent/bin/snapd`

You can also build directly with Go:

```bash
cd agent
go generate ./cmd/snapd  # Generate build metadata
go build -o bin/snapd ./cmd/snapd
```

The version is defined in `agent/cmd/snapd/version.go` and the build metadata is auto-generated.

## Usage

### Running as a Service (Recommended)

Once installed as a systemd service:

```bash
# Start the daemon
sudo systemctl start snapd

# Stop the daemon
sudo systemctl stop snapd

# Restart the daemon
sudo systemctl restart snapd

# Check status
sudo systemctl status snapd

# View logs
sudo journalctl -u snapd -f

# View recent logs
sudo journalctl -u snapd -n 100
```

### Daemon Mode (Manual)

Run the daemon in background mode with structured JSON logging (suitable for systemd):

```bash
snapd --config /path/to/config.yaml
```

If `--config` is omitted, it defaults to `/etc/snapd/config.yaml`.

### Console Mode (Debugging)

Run the daemon in foreground mode with human-readable logs:

```bash
snapd --console --config /path/to/config.yaml
```

In console mode:
- Logs are output to stdout/stderr in text format
- The daemon runs in the foreground
- Press Ctrl+C (SIGINT) to gracefully shutdown
- Useful for debugging configuration issues

### CLI Subcommands

#### Version

Display version information including build date and commit hash:

```bash
snapd version
# or
snapd --version
```

Example output:
```
snapd version 1.0.0
Build Date: 2024-12-09T10:30:00Z
Commit: a1b2c3d4
```

#### Status

Check currently running uploads across all nodes:

```bash
snapd status --config /path/to/config.yaml
```

Example output:
```
Currently running uploads:
  ethereum-mainnet: Started 2024-12-09 10:15:00, Progress: 45%
  arbitrum-one: Started 2024-12-09 09:30:00, Progress: 78%

No active uploads.
```

#### Manual Upload

Trigger a manual upload for a specific node:

```bash
snapd upload <node-name> --config /path/to/config.yaml
```

Example:
```bash
# Trigger upload for ethereum-mainnet node
snapd upload ethereum-mainnet

# With custom config path
snapd upload arbitrum-one --config /etc/snapd/config.yaml
```

This will:
1. Check if an upload is already running (exits with error code 1 if so)
2. Collect metrics via the protocol module
3. Initiate the upload via `bv n run upload <node-name>`
4. Record it in the database with `trigger_type="manual"`
5. Exit with code 0 on success

**Note**: Manual uploads follow the same workflow as scheduled uploads and will appear in the status output.

## Systemd Integration

The daemon is designed to run as a systemd service for production deployments.

### Service File

A complete systemd unit file is provided at `agent/snapd.service`. Key features:

- **Automatic restart** on failure with 10-second delay
- **Dependency management** ensures PostgreSQL is running first
- **Security hardening** with restricted permissions
- **Environment file support** for sensitive credentials
- **Graceful shutdown** with 30-second timeout
- **Resource limits** to prevent runaway processes

### Installation Steps

```bash
# 1. Copy the service file
sudo cp agent/snapd.service /etc/systemd/system/

# 2. Copy the environment file
sudo cp agent/environment.example /etc/snapd/environment
sudo nano /etc/snapd/environment  # Edit with your credentials

# 3. Set secure permissions
sudo chmod 600 /etc/snapd/environment
sudo chown snapd:snapd /etc/snapd/environment

# 4. Reload systemd
sudo systemctl daemon-reload

# 5. Enable and start
sudo systemctl enable snapd
sudo systemctl start snapd
```

### Service Management

```bash
# Check service status
sudo systemctl status snapd

# View logs (follow mode)
sudo journalctl -u snapd -f

# View logs (last 100 lines)
sudo journalctl -u snapd -n 100

# View logs (since boot)
sudo journalctl -u snapd -b

# View logs (specific time range)
sudo journalctl -u snapd --since "2024-12-09 10:00:00" --until "2024-12-09 11:00:00"

# Restart service
sudo systemctl restart snapd

# Stop service
sudo systemctl stop snapd

# Disable service (prevent auto-start)
sudo systemctl disable snapd
```

### Environment File

The systemd service uses `/etc/snapd/environment` for sensitive configuration:

```bash
# /etc/snapd/environment
DB_PASSWORD=your_secure_password
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
```

**Security Best Practices**:
- Set permissions to `600` (owner read/write only)
- Set ownership to `snapd:snapd`
- Never commit to version control
- Rotate credentials regularly

### Troubleshooting Systemd

If the service fails to start:

```bash
# Check service status
sudo systemctl status snapd

# View detailed logs
sudo journalctl -u snapd -n 50 --no-pager

# Check configuration syntax
snapd --console --config /etc/snapd/config.yaml

# Verify file permissions
ls -la /etc/snapd/
ls -la /usr/local/bin/snapd

# Test database connection
sudo -u snapd psql -h localhost -U snapd -d snapd -c "SELECT 1;"
```

## Graceful Shutdown

The daemon handles SIGTERM and SIGINT signals for graceful shutdown:

1. **Stop accepting new jobs**: Scheduler stops scheduling new tasks
2. **Wait for in-progress operations**: Allows up to 30 seconds for completion
3. **Close database connections**: Ensures all transactions are committed
4. **Exit cleanly**: Returns appropriate exit code

**Shutdown Behavior**:
- In-progress uploads are allowed to complete
- Database writes are flushed
- Notification deliveries are attempted
- If operations don't complete within 30 seconds, they are forcefully terminated

**Triggering Shutdown**:
```bash
# Via systemd
sudo systemctl stop snapd

# Via signal (if running in console mode)
kill -TERM <pid>
# or press Ctrl+C in console mode
```

## Architecture

The daemon uses a modular, pluggable architecture with clear separation of concerns.

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                            │
│  (Argument parsing, subcommand routing, mode selection)     │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                      Core Daemon                             │
│  (Lifecycle management, component coordination)             │
└─────┬──────────────┬──────────────┬────────────────┬────────┘
      │              │              │                │
┌─────▼─────┐  ┌────▼─────┐  ┌─────▼──────┐  ┌─────▼────────┐
│ Scheduler │  │  Config  │  │  Database  │  │   Logger     │
│           │  │  Manager │  │   Layer    │  │              │
└─────┬─────┘  └──────────┘  └────────────┘  └──────────────┘
      │
┌─────▼──────────────────────────────────────────────────────┐
│                    Scheduled Jobs                           │
│  ┌──────────────────┐      ┌──────────────────┐           │
│  │ Upload Monitor   │      │ Metrics Collector│           │
│  └────────┬─────────┘      └────────┬─────────┘           │
└───────────┼──────────────────────────┼─────────────────────┘
            │                          │
    ┌───────▼────────┐        ┌────────▼────────┐
    │    Command     │        │    Protocol     │
    │    Executor    │        │    Registry     │
    └───────┬────────┘        └────────┬────────┘
            │                          │
    ┌───────▼────────┐        ┌────────▼────────────────────┐
    │   bv CLI       │        │  Protocol Modules           │
    │   (external)   │        │  ┌──────────┐ ┌──────────┐ │
    └────────────────┘        │  │ Ethereum │ │ Arbitrum │ │
                              │  └──────────┘ └──────────┘ │
                              └─────────────────────────────┘
```

### Component Descriptions

**CLI Layer**: Parses command-line arguments and routes to appropriate handlers (daemon mode, status, version, manual upload)

**Core Daemon**: Orchestrates all daemon operations, manages lifecycle, and coordinates between subsystems

**Config Manager**: Loads and validates YAML configuration, merges global and per-node settings, supports environment variable expansion

**Scheduler**: Manages cron-based job scheduling for both global status updates and per-node upload schedules

**Upload Monitor**: Checks upload status via `bv n j <node> info upload`, initiates uploads when appropriate, tracks progress

**Metrics Collector**: Invokes protocol modules to gather node-specific metrics before uploads

**Command Executor**: Executes external system commands (`bv` CLI) with timeout support and captures stdout/stderr

**Protocol Registry**: Maintains registered protocol modules and routes queries to appropriate implementations

**Notification Registry**: Maintains registered notification modules and dispatches alerts based on configuration

**Database Layer**: Handles all PostgreSQL interactions with connection pooling, retry logic, and graceful shutdown

### Data Flow

1. **Scheduled Monitoring**:
   - Scheduler triggers job based on cron expression
   - Upload Monitor checks if upload is running via `bv n j <node> info upload`
   - If not running, Metrics Collector invokes protocol module
   - Protocol module executes RPC queries and returns metrics
   - Metrics are persisted to database
   - Upload is initiated via `bv n run upload <node>`
   - Progress monitoring begins with 1-minute interval

2. **Manual Upload**:
   - User executes `snapd upload <node>`
   - System checks for running upload
   - If not running, follows same workflow as scheduled upload
   - Records trigger_type="manual" in database

3. **Notifications**:
   - Events (failure, skip, complete) trigger notification checks
   - System evaluates node-specific or global notification config
   - Notification modules are invoked for each configured type
   - Webhooks are called with formatted payloads

### Extension Points

**Protocol Modules**: Implement the `ProtocolModule` interface to add support for new blockchain types:

```go
type ProtocolModule interface {
    Name() string
    CollectMetrics(ctx context.Context, config NodeConfig) (map[string]interface{}, error)
}
```

**Notification Modules**: Implement the `NotificationModule` interface to add new notification types:

```go
type NotificationModule interface {
    Name() string
    Send(ctx context.Context, url string, payload NotificationPayload) error
}
```

See `.kiro/specs/snapshot-daemon/design.md` for detailed architecture documentation.

## Development

### Running Tests

```bash
# Run all tests
make test-agent

# Run tests with coverage
cd agent
go test -v -cover ./...

# Run specific package tests
go test -v ./internal/config
go test -v ./internal/protocol
go test -v ./internal/notification

# Run with race detection
go test -race ./...
```

### Development Workflow

```bash
# 1. Make code changes

# 2. Run tests
make test-agent

# 3. Build binary
make build-agent

# 4. Test in console mode
./agent/bin/snapd --console --config agent/config.example.yaml

# 5. Check for issues
go vet ./...
go fmt ./...
```

### Adding a New Protocol Module

1. Create a new file in `agent/internal/protocol/`:
   ```go
   package protocol
   
   type MyProtocolModule struct{}
   
   func (m *MyProtocolModule) Name() string {
       return "myprotocol"
   }
   
   func (m *MyProtocolModule) CollectMetrics(ctx context.Context, config NodeConfig) (map[string]interface{}, error) {
       // Implement RPC queries
       return metrics, nil
   }
   ```

2. Register in `agent/internal/protocol/protocol.go`:
   ```go
   func init() {
       RegisterProtocol(&MyProtocolModule{})
   }
   ```

3. Add tests in `agent/internal/protocol/myprotocol_test.go`

4. Update documentation

### Adding a New Notification Module

1. Create a new file in `agent/internal/notification/`:
   ```go
   package notification
   
   type MyNotificationModule struct{}
   
   func (m *MyNotificationModule) Name() string {
       return "myservice"
   }
   
   func (m *MyNotificationModule) Send(ctx context.Context, url string, payload NotificationPayload) error {
       // Implement notification delivery
       return nil
   }
   ```

2. Register in `agent/internal/notification/notification.go`:
   ```go
   func init() {
       RegisterNotification(&MyNotificationModule{})
   }
   ```

3. Add tests in `agent/internal/notification/myservice_test.go`

4. Update documentation

### Debugging Tips

**Enable verbose logging**:
```bash
snapd --console --config config.yaml
```

**Test configuration parsing**:
```bash
# The daemon will validate config on startup
snapd --console --config /path/to/config.yaml
# Press Ctrl+C immediately after startup
```

**Test database connection**:
```bash
# Set environment variable
export DB_PASSWORD=your_password

# Run in console mode
snapd --console --config config.yaml
```

**Test protocol modules**:
```bash
cd agent/internal/protocol
go test -v -run TestEthereumModule
```

**Test notification delivery**:
```bash
cd agent/internal/notification
go test -v -run TestDiscordModule
```

## Troubleshooting

### Common Issues

**Issue**: Daemon fails to start with "database connection failed"

**Solution**:
- Verify PostgreSQL is running: `sudo systemctl status postgresql`
- Check database credentials in `/etc/snapd/environment`
- Test connection: `psql -h localhost -U snapd -d snapd`
- Check firewall rules if using remote database

---

**Issue**: Uploads not starting on schedule

**Solution**:
- Check cron expression syntax: https://crontab.guru/
- Verify node configuration in config.yaml
- Check logs: `sudo journalctl -u snapd -f`
- Ensure `bv` CLI is installed and accessible

---

**Issue**: Notifications not being sent

**Solution**:
- Verify webhook URL is correct
- Check notification flags (failure, skip, complete)
- Test webhook manually with curl
- Check logs for notification errors
- Verify notification module is registered

---

**Issue**: "Protocol module not found" error

**Solution**:
- Check protocol name in node configuration
- Verify protocol module is registered
- Check available protocols: see `agent/internal/protocol/`
- Ensure protocol name matches exactly (case-sensitive)

---

**Issue**: High memory usage

**Solution**:
- Check number of concurrent uploads
- Review database connection pool settings
- Check for memory leaks in custom modules
- Monitor with: `systemctl status snapd`

---

**Issue**: Graceful shutdown timeout

**Solution**:
- Increase TimeoutStopSec in systemd unit file
- Check for long-running database operations
- Review upload command timeouts
- Check logs for stuck operations

### Getting Help

- **Documentation**: See `.kiro/specs/snapshot-daemon/` for detailed specs
- **Logs**: Check `sudo journalctl -u snapd -f` for runtime issues
- **Issues**: Report bugs on GitHub issue tracker
- **Configuration**: Review `agent/config.example.yaml` for all options

## License

[Your License Here]
