from fastapi import HTTPException, Security, status, Depends
from fastapi.security import APIKeyHeader
from typing import Optional
from sqlalchemy.orm import Session
from datetime import datetime
from app.database import get_db
from app.db_models import APIKey
import logging

logger = logging.getLogger(__name__)

api_key_header = APIKeyHeader(name="X-API-Key", auto_error=False)


async def validate_api_key(
    api_key: Optional[str] = Security(api_key_header),
    db: Session = Depends(get_db)
) -> APIKey:
    """Validate API key and return the key object"""

    if not api_key:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="API key is missing"
        )

    # Hash the provided key
    key_hash = APIKey.hash_key(api_key)

    # Look up key in database
    db_key = db.query(APIKey).filter(
        APIKey.key_hash == key_hash,
        APIKey.is_active == True
    ).first()

    if not db_key:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid API key"
        )

    # Check expiration
    if db_key.expires_at and db_key.expires_at < datetime.utcnow():
        db_key.is_active = False
        db.commit()
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="API key has expired"
        )

    # Update usage statistics
    db_key.last_used_at = datetime.utcnow()
    db_key.usage_count += 1
    db.commit()

    return db_key


def require_scope(scope: str):
    """Dependency to require a specific scope"""
    async def check_scope(api_key: APIKey = Depends(validate_api_key)):
        if scope not in api_key.scopes and "admin" not in api_key.scopes:
            raise HTTPException(
                status_code=status.HTTP_403_FORBIDDEN,
                detail=f"API key does not have required scope: {scope}"
            )
        return api_key
    return check_scope


# Convenience dependencies for common scopes
require_snapshots_read = require_scope("snapshots:read")
require_snapshots_write = require_scope("snapshots:write")
require_admin = require_scope("admin")