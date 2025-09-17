from fastapi import APIRouter, Depends
from datetime import datetime
from app.models import HealthCheckResponse
from app.s3_client import get_s3_client
from app.config import get_settings
from app.auth_middleware import validate_api_key
from app.database import get_db
from app.db_models import APIKey
from sqlalchemy.orm import Session
from sqlalchemy import text
import logging

logger = logging.getLogger(__name__)
router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthCheckResponse)
async def health_check(db: Session = Depends(get_db)):
    settings = get_settings()
    s3_client = get_s3_client()

    # Test database connection
    db_connected = False
    try:
        db.execute(text("SELECT 1"))
        db_connected = True
    except Exception as e:
        logger.error(f"Database health check failed: {e}")

    # Test S3 connection (if configured)
    s3_connected = False
    if settings.s3_bucket_name:
        try:
            s3_connected = s3_client.test_connection()
        except Exception as e:
            logger.debug(f"S3 connection test failed: {e}")
            s3_connected = False
    else:
        logger.debug("S3 not configured")

    # Determine overall status
    if db_connected and s3_connected:
        status = "healthy"
    elif db_connected or s3_connected:
        status = "degraded"
    else:
        status = "unhealthy"

    return HealthCheckResponse(
        status=status,
        s3_connected=s3_connected,
        db_connected=db_connected,
        timestamp=datetime.utcnow().isoformat(),
        version=settings.api_version
    )


@router.get("/health/protected", response_model=HealthCheckResponse)
async def protected_health_check(api_key: APIKey = Depends(validate_api_key), db: Session = Depends(get_db)):
    return await health_check(db)