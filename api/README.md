# ChainSnaps API

FastAPI-based REST API for discovering, indexing, and managing blockchain snapshots stored in S3-compatible storage.

## Features

- **Automatic Snapshot Discovery**: Scans S3 buckets for snapshots using manifest files
- **Database Storage**: Stores snapshot metadata in PostgreSQL for fast querying
- **API Key Authentication**: All endpoints are secured with API key authentication
- **S3 Integration**: Connect to any S3-compatible storage (AWS S3, MinIO, etc.)
- **Advanced Filtering**: Filter snapshots by chain, block height, blob availability, and more
- **External Metadata Updates**: Allows external processes to update snapshot metadata
- **Background Scanning**: Periodically scans for new snapshots
- **Health Checks**: Monitor API and S3 connection status

## Setup

1. Install dependencies:
```bash
pip install -r requirements.txt
```

2. Configure environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. Run the API:
```bash
python run.py
```

Or with uvicorn directly:
```bash
uvicorn app.main:app --reload
```

## Database Setup

1. Create a PostgreSQL database:
```sql
CREATE DATABASE chainsnaps;
```

2. Run migrations:
```bash
cd api
alembic upgrade head
```

## API Endpoints

### Health Check
- `GET /health` - Public health check endpoint
- `GET /health/protected` - Protected health check (requires API key)

### Snapshots
- `GET /api/v1/snapshots/` - List all snapshots
  - Query params:
    - `chain`: Filter by blockchain name
    - `block_height_min`: Minimum block height
    - `block_height_max`: Maximum block height
    - `has_blobs`: Filter by blob availability
    - `is_complete`: Filter by completion status
    - `is_active`: Filter by active status (default: true)
    - `limit`: Maximum results (1-1000)
    - `offset`: Result offset for pagination

- `GET /api/v1/snapshots/{snapshot_id}` - Get snapshot by ID
  - Path param: `snapshot_id` - Database snapshot ID

- `GET /api/v1/snapshots/by-path/{chain}/{snapshot_id}` - Get snapshot by chain and version
  - Path params:
    - `chain` - Blockchain name
    - `snapshot_id` - Snapshot version/identifier

- `PATCH /api/v1/snapshots/{snapshot_id}` - Update snapshot metadata
  - Body: JSON with optional fields:
    - `block_height`: Block height at snapshot time
    - `has_blobs`: Whether snapshot includes blobs
    - `blob_start_block`: Earliest block with blob data
    - `blob_end_block`: Latest block with blob data
    - `is_complete`: Whether snapshot is complete
    - `external_metadata`: Additional metadata to merge

- `POST /api/v1/snapshots/scan` - Manually trigger snapshot scan

### Authentication

All `/api/v1/*` endpoints require an API key header:
```
X-API-Key: your-api-key-here
```

## Configuration

Environment variables (`.env` file):

- `DATABASE_URL`: PostgreSQL connection string
- `S3_ENDPOINT_URL`: S3 endpoint URL
- `S3_ACCESS_KEY_ID`: S3 access key
- `S3_SECRET_ACCESS_KEY`: S3 secret key
- `S3_BUCKET_NAME`: S3 bucket name
- `S3_REGION`: AWS region (default: us-east-1)
- `API_KEYS`: Comma-separated list of valid API keys
- `HOST`: Server host (default: 0.0.0.0)
- `PORT`: Server port (default: 8000)
- `SCAN_ON_STARTUP`: Enable automatic scanning on startup (default: true)
- `SCAN_INTERVAL_HOURS`: Hours between automatic scans (default: 6)

## Documentation

When running, interactive API documentation is available at:
- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc