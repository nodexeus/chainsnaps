from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.security import HTTPBasic, HTTPBasicCredentials
from typing import List, Optional
from sqlalchemy.orm import Session
from datetime import datetime, timedelta
from app.models import (
    UserRegister,
    UserLogin,
    UserResponse,
    APIKeyCreate,
    APIKeyResponse,
    APIKeyCreateResponse,
    APIKeyListResponse,
    ErrorResponse
)
from app.database import get_db
from app.db_models import User, APIKey
import secrets
import logging

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/auth", tags=["authentication"])

# HTTP Basic Auth for admin endpoints
security = HTTPBasic()


def verify_admin_user(
    credentials: HTTPBasicCredentials = Depends(security),
    db: Session = Depends(get_db)
) -> User:
    """Verify admin username and password"""
    # Find user
    user = db.query(User).filter(User.username == credentials.username).first()

    if not user or not user.verify_password(credentials.password):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid username or password",
            headers={"WWW-Authenticate": "Basic"},
        )

    if not user.is_admin:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Admin access required"
        )

    # Update last login
    user.last_login = datetime.utcnow()
    db.commit()

    return user


@router.post(
    "/register",
    response_model=UserResponse,
    responses={
        409: {"model": ErrorResponse, "description": "Registration closed - user already exists"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def register_user(
    user_data: UserRegister,
    db: Session = Depends(get_db)
):
    """
    Register a user. Only the first user can register and automatically becomes admin.
    After the first user, registration is permanently closed.
    """
    try:
        # Check if ANY user exists - if so, registration is closed forever
        existing_user_count = db.query(User).count()
        if existing_user_count > 0:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="Registration is closed. A user already exists in the system."
            )

        # Create the first and only user as admin
        password_hash = User.hash_password(user_data.password)
        admin_user = User(
            username=user_data.username,
            password_hash=password_hash,
            is_admin=True  # First user is always admin
        )

        db.add(admin_user)
        db.commit()
        db.refresh(admin_user)

        logger.info(f"First user '{user_data.username}' registered as admin")

        return UserResponse.from_orm(admin_user)

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error during registration: {e}")
        db.rollback()
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to register: {str(e)}"
        )


@router.post(
    "/keys",
    response_model=APIKeyCreateResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        403: {"model": ErrorResponse, "description": "Forbidden - admin access required"},
        409: {"model": ErrorResponse, "description": "Key name already exists"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def create_api_key(
    key_data: APIKeyCreate,
    db: Session = Depends(get_db),
    admin_user: User = Depends(verify_admin_user)
):
    """Create a new API key (requires admin authentication)"""
    try:
        # Check if name already exists
        existing = db.query(APIKey).filter(APIKey.name == key_data.name).first()
        if existing:
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail=f"API key with name '{key_data.name}' already exists"
            )

        # Generate new key
        api_key = APIKey.generate_api_key()
        key_hash = APIKey.hash_key(api_key)
        key_prefix = APIKey.get_key_prefix(api_key)

        # Calculate expiration
        expires_at = None
        if key_data.expires_in_days:
            expires_at = datetime.utcnow() + timedelta(days=key_data.expires_in_days)

        # Determine if admin key
        is_admin = "admin" in key_data.scopes

        # Create database record
        db_key = APIKey(
            name=key_data.name,
            description=key_data.description,
            api_key=api_key,
            key_hash=key_hash,
            key_prefix=key_prefix,
            user_id=admin_user.id,  # Link to admin user
            scopes=key_data.scopes,
            is_admin=is_admin,
            expires_at=expires_at,
            rate_limit=key_data.rate_limit,
            created_by=admin_user.username
        )

        db.add(db_key)
        db.commit()
        db.refresh(db_key)

        logger.info(f"API key '{key_data.name}' created by {admin_user.username}")

        return APIKeyCreateResponse(
            id=db_key.id,
            name=db_key.name,
            description=db_key.description,
            key_prefix=db_key.key_prefix,
            scopes=db_key.scopes,
            is_admin=db_key.is_admin,
            is_active=db_key.is_active,
            expires_at=db_key.expires_at,
            created_at=db_key.created_at,
            last_used_at=db_key.last_used_at,
            usage_count=db_key.usage_count,
            rate_limit=db_key.rate_limit,
            api_key=api_key  # Return the actual key
        )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error creating API key: {e}")
        db.rollback()
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to create API key: {str(e)}"
        )


@router.get(
    "/keys",
    response_model=APIKeyListResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        403: {"model": ErrorResponse, "description": "Forbidden - admin access required"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def list_api_keys(
    include_inactive: bool = False,
    db: Session = Depends(get_db),
    admin_user: User = Depends(verify_admin_user)
):
    """List all API keys (requires admin authentication)

    Note: Full API keys cannot be retrieved after creation as they are hashed.
    Only the key prefix (first 8 characters) is shown for identification."""
    try:
        query = db.query(APIKey)

        if not include_inactive:
            query = query.filter(APIKey.is_active == True)

        keys = query.order_by(APIKey.created_at.desc()).all()

        return APIKeyListResponse(
            keys=[APIKeyResponse.from_orm(key) for key in keys],
            count=len(keys)
        )

    except Exception as e:
        logger.error(f"Error listing API keys: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to list API keys: {str(e)}"
        )


@router.get(
    "/keys/{key_id}",
    response_model=APIKeyResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        403: {"model": ErrorResponse, "description": "Forbidden - admin access required"},
        404: {"model": ErrorResponse, "description": "Key not found"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def get_api_key(
    key_id: int,
    db: Session = Depends(get_db),
    admin_user: User = Depends(verify_admin_user)
):
    """Get details of a specific API key (requires admin authentication)"""
    try:
        api_key = db.query(APIKey).filter(APIKey.id == key_id).first()

        if not api_key:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"API key with ID {key_id} not found"
            )

        return APIKeyResponse.from_orm(api_key)

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting API key: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get API key: {str(e)}"
        )


@router.delete(
    "/keys/{key_id}",
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        403: {"model": ErrorResponse, "description": "Forbidden - admin access required"},
        404: {"model": ErrorResponse, "description": "Key not found"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def revoke_api_key(
    key_id: int,
    db: Session = Depends(get_db),
    admin_user: User = Depends(verify_admin_user)
):
    """Revoke an API key (requires admin authentication)"""
    try:
        api_key = db.query(APIKey).filter(APIKey.id == key_id).first()

        if not api_key:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"API key with ID {key_id} not found"
            )

        api_key.is_active = False
        api_key.revoked_at = datetime.utcnow()
        api_key.revoked_by = admin_user.username

        db.commit()

        logger.info(f"API key '{api_key.name}' (ID: {key_id}) revoked by {admin_user.username}")

        return {"message": f"API key '{api_key.name}' has been revoked"}

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error revoking API key: {e}")
        db.rollback()
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to revoke API key: {str(e)}"
        )


@router.get(
    "/status",
    responses={
        200: {"description": "System status"}
    }
)
async def get_auth_status(db: Session = Depends(get_db)):
    """Check if system has been initialized (first user registered)"""
    try:
        user_exists = db.query(User).first() is not None
        total_keys = db.query(APIKey).count()
        active_keys = db.query(APIKey).filter(APIKey.is_active == True).count()

        return {
            "initialized": user_exists,
            "registration_open": not user_exists,  # Registration only open if no users exist
            "total_keys": total_keys,
            "active_keys": active_keys
        }

    except Exception as e:
        logger.error(f"Error getting auth status: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get auth status: {str(e)}"
        )