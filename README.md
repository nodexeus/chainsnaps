# ChainSnaps

A blockchain snapshot discovery and management system with S3 storage integration.

## Quick Start with Docker

### Prerequisites
- Docker and Docker Compose installed
- S3-compatible storage credentials
- API keys for authentication

### Setup

1. **Clone the repository**
```bash
git clone <repository-url>
cd chainsnaps
```

2. **Configure environment**
```bash
# Copy the example environment file
cp .env.docker.example .env

# Edit .env with your S3 credentials
```

3. **Start the services**
```bash
# Build and start all services
docker-compose up -d

# Or use Make commands
make up
```

4. **Initialize the system**
```bash
# Check if registration is open
curl http://localhost:8000/api/v1/auth/status

# Register the first (and only) user - becomes admin automatically
curl -X POST http://localhost:8000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "your_secure_password"}'

# After this, no other users can register
# Use these credentials to create and manage API keys
```

The API will be available at `http://localhost:8000`

### Management Commands

Using Make:
```bash
make build       # Build Docker images
make up          # Start all services
make down        # Stop all services
make restart     # Restart all services
make logs        # View logs from all services
make api-logs    # View API logs only
make shell       # Open shell in API container
make db-shell    # Open PostgreSQL shell
make migrate     # Run database migrations manually
make clean       # Remove all containers and volumes
make scan        # Trigger manual snapshot scan
```

Using Docker Compose directly:
```bash
docker-compose up -d                # Start services
docker-compose down                 # Stop services
docker-compose logs -f api          # View API logs
docker-compose exec api /bin/bash   # API shell
docker-compose exec api alembic upgrade head  # Run migrations
```

### Development Mode

For local development with hot reloading:

1. Copy the override file:
```bash
cp docker-compose.override.yaml.example docker-compose.override.yaml
```

2. Start services (will mount local code):
```bash
docker-compose up
```

## Architecture

### Components

1. **API Service** (FastAPI)
   - REST API for snapshot management
   - Automatic S3 scanning for snapshots
   - API key authentication
   - Background scanning service

2. **PostgreSQL Database**
   - Stores snapshot metadata
   - Tracks scan history
   - Persistent volume for data

### How It Works

1. **Automatic Discovery**: On startup, the scanner searches S3 for snapshot directories containing `manifest-body.json` and `manifest-header.json` files

2. **Database Storage**: Snapshot metadata is extracted and stored in PostgreSQL for fast querying

3. **External Updates**: Other processes can update snapshot metadata (block heights, blob data) via the API

4. **Periodic Scanning**: Background service rescans S3 periodically (default: every 6 hours)

## Authentication Flow

### 1. Initial Setup (One-time only)
```bash
# Register the admin user (only the first user can register)
curl -X POST http://localhost:8000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "SecurePassword123"}'
```

### 2. Create API Keys
```bash
# Create an API key for the web app
curl -X POST http://localhost:8000/api/v1/auth/keys \
  -u admin:SecurePassword123 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Web App",
    "description": "Frontend application key",
    "scopes": ["snapshots:read"]
  }'

# Create an API key for the metadata updater (with write permissions)
curl -X POST http://localhost:8000/api/v1/auth/keys \
  -u admin:SecurePassword123 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Metadata Updater",
    "description": "External service for updating snapshot metadata",
    "scopes": ["snapshots:read", "snapshots:write"]
  }'

# Create an admin API key (can manage other keys)
curl -X POST http://localhost:8000/api/v1/auth/keys \
  -u admin:SecurePassword123 \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Admin CLI",
    "description": "Administrative access",
    "scopes": ["admin", "snapshots:read", "snapshots:write"]
  }'
```

### 3. Use API Keys
```bash
# Use an API key to access endpoints
curl http://localhost:8000/api/v1/snapshots \
  -H "X-API-Key: csnp_your_api_key_here"
```

## API Documentation

When running, interactive API documentation is available at:
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc

### Key Endpoints

- `GET /health` - Health check (no auth required)
- `GET /api/v1/snapshots/` - List snapshots with filtering
- `GET /api/v1/snapshots/{id}` - Get snapshot by ID
- `PATCH /api/v1/snapshots/{id}` - Update snapshot metadata
- `POST /api/v1/snapshots/scan` - Trigger manual scan

All API endpoints (except /health) require an `X-API-Key` header.

## Configuration

Key environment variables:

- `S3_ACCESS_KEY_ID` - S3 access key (required)
- `S3_SECRET_ACCESS_KEY` - S3 secret key (required)
- `S3_BUCKET_NAME` - S3 bucket name (required)
- `API_KEYS` - Comma-separated API keys (required)
- `SCAN_ON_STARTUP` - Auto-scan on startup (default: true)
- `SCAN_INTERVAL_HOURS` - Hours between scans (default: 6)

See `.env.docker.example` for all available options.

## Database Migrations

Migrations run automatically on container startup. To run manually:

```bash
# Using Make
make migrate

# Using Docker Compose
docker-compose exec api alembic upgrade head

# Create a new migration
docker-compose exec api alembic revision --autogenerate -m "Description"
```

## Monitoring

Check service health:
```bash
# Health endpoint (no auth)
curl http://localhost:8000/health

# With authentication
curl -H "X-API-Key: your_api_key" http://localhost:8000/api/v1/snapshots/
```

View logs:
```bash
# All services
docker-compose logs -f

# API only
docker-compose logs -f api

# Database only
docker-compose logs -f postgres
```

## Production Deployment

For production:

1. Use strong passwords and API keys
2. Set `RELOAD=false` in environment
3. Consider using external PostgreSQL
4. Add SSL/TLS termination (nginx/traefik)
5. Set appropriate resource limits in docker-compose.yaml
6. Use volume backups for PostgreSQL data

## Troubleshooting

### API won't start
- Check PostgreSQL is healthy: `docker-compose ps`
- Check logs: `docker-compose logs api`
- Ensure S3 credentials are correct

### Migrations fail
- Check database connection: `docker-compose logs api`
- Manually connect: `make db-shell`
- Reset if needed: `make clean && make up`

### Snapshots not found
- Verify S3 bucket name and credentials
- Check scanner logs: `docker-compose logs api | grep scanner`
- Manually trigger scan: `make scan`

## License

[Your License Here]