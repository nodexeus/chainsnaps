from fastapi import APIRouter, Depends, HTTPException, Query, Path, status
from typing import List, Optional
from sqlalchemy.orm import Session
from sqlalchemy import desc
from app.models import (
    SnapshotResponse,
    SnapshotListResponse,
    SnapshotUpdateRequest,
    ErrorResponse
)
from app.auth_middleware import require_snapshots_read, require_snapshots_write
from app.database import get_db
from app.db_models import Snapshot, APIKey
from app.services.snapshot_scanner import snapshot_scanner
from datetime import datetime
import logging

logger = logging.getLogger(__name__)
router = APIRouter(prefix="/snapshots", tags=["snapshots"])


@router.get(
    "/",
    response_model=SnapshotListResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def list_snapshots(
    chain: Optional[str] = Query(None, description="Filter by blockchain name"),
    block_height_min: Optional[int] = Query(None, description="Minimum block height"),
    block_height_max: Optional[int] = Query(None, description="Maximum block height"),
    has_blobs: Optional[bool] = Query(None, description="Filter by blob availability"),
    is_complete: Optional[bool] = Query(None, description="Filter by completion status"),
    is_active: bool = Query(True, description="Filter by active status"),
    limit: int = Query(default=100, ge=1, le=1000, description="Maximum results"),
    offset: int = Query(default=0, ge=0, description="Result offset"),
    api_key: APIKey = Depends(require_snapshots_read),
    db: Session = Depends(get_db)
):
    try:
        query = db.query(Snapshot)

        # Apply filters
        if chain:
            query = query.filter(Snapshot.chain == chain)
        if block_height_min is not None:
            query = query.filter(Snapshot.block_height >= block_height_min)
        if block_height_max is not None:
            query = query.filter(Snapshot.block_height <= block_height_max)
        if has_blobs is not None:
            query = query.filter(Snapshot.has_blobs == has_blobs)
        if is_complete is not None:
            query = query.filter(Snapshot.is_complete == is_complete)

        query = query.filter(Snapshot.is_active == is_active)

        # Order by block height (newest first), then by indexed_at
        query = query.order_by(
            desc(Snapshot.block_height),
            desc(Snapshot.indexed_at)
        )

        # Get total count
        total_count = query.count()

        # Apply pagination
        snapshots = query.offset(offset).limit(limit).all()

        # Calculate total size
        total_size_bytes = sum(s.total_size_bytes or 0 for s in snapshots)
        total_size_tb = round(total_size_bytes / (1024 ** 4), 2)

        return SnapshotListResponse(
            snapshots=[SnapshotResponse.from_orm(s) for s in snapshots],
            count=total_count,
            total_size_tb=total_size_tb
        )

    except Exception as e:
        logger.error(f"Error listing snapshots: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to list snapshots: {str(e)}"
        )


@router.get(
    "/{snapshot_id:int}",
    response_model=SnapshotResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        404: {"model": ErrorResponse, "description": "Snapshot not found"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def get_snapshot(
    snapshot_id: int = Path(..., description="Snapshot ID"),
    api_key: APIKey = Depends(require_snapshots_read),
    db: Session = Depends(get_db)
):
    try:
        snapshot = db.query(Snapshot).filter(Snapshot.id == snapshot_id).first()

        if not snapshot:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Snapshot with ID {snapshot_id} not found"
            )

        return SnapshotResponse.from_orm(snapshot)

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting snapshot: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get snapshot: {str(e)}"
        )


@router.patch(
    "/{snapshot_id:int}",
    response_model=SnapshotResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        404: {"model": ErrorResponse, "description": "Snapshot not found"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def update_snapshot(
    snapshot_id: int = Path(..., description="Snapshot ID"),
    update_data: SnapshotUpdateRequest = None,
    api_key: APIKey = Depends(require_snapshots_write),
    db: Session = Depends(get_db)
):
    try:
        snapshot = db.query(Snapshot).filter(Snapshot.id == snapshot_id).first()

        if not snapshot:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Snapshot with ID {snapshot_id} not found"
            )

        # Update fields if provided
        if update_data.block_height is not None:
            snapshot.block_height = update_data.block_height
        if update_data.has_blobs is not None:
            snapshot.has_blobs = update_data.has_blobs
        if update_data.blob_start_block is not None:
            snapshot.blob_start_block = update_data.blob_start_block
        if update_data.blob_end_block is not None:
            snapshot.blob_end_block = update_data.blob_end_block
        if update_data.is_complete is not None:
            snapshot.is_complete = update_data.is_complete

        # Merge external metadata
        if update_data.external_metadata:
            if snapshot.external_metadata:
                snapshot.external_metadata.update(update_data.external_metadata)
            else:
                snapshot.external_metadata = update_data.external_metadata

        snapshot.last_updated_at = datetime.utcnow()
        db.commit()
        db.refresh(snapshot)

        logger.info(f"Updated snapshot {snapshot_id}")
        return SnapshotResponse.from_orm(snapshot)

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error updating snapshot: {e}")
        db.rollback()
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to update snapshot: {str(e)}"
        )


@router.post(
    "/scan",
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def trigger_scan(
    api_key: APIKey = Depends(require_snapshots_write)
):
    """Manually trigger a snapshot scan"""
    try:
        result = await snapshot_scanner.scan_now()
        return result
    except Exception as e:
        logger.error(f"Error triggering scan: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to trigger scan: {str(e)}"
        )


@router.get(
    "/by-path/{chain}/{snapshot_id}",
    response_model=SnapshotResponse,
    responses={
        401: {"model": ErrorResponse, "description": "Unauthorized"},
        404: {"model": ErrorResponse, "description": "Snapshot not found"},
        500: {"model": ErrorResponse, "description": "Internal server error"}
    }
)
async def get_snapshot_by_path(
    chain: str = Path(..., description="Blockchain name"),
    snapshot_id: str = Path(..., description="Snapshot identifier"),
    api_key: APIKey = Depends(require_snapshots_write),
    db: Session = Depends(get_db)
):
    try:
        # Look for snapshot with matching chain and snapshot_id
        snapshot = db.query(Snapshot).filter(
            Snapshot.chain == chain,
            Snapshot.snapshot_id == snapshot_id
        ).first()

        if not snapshot:
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail=f"Snapshot not found for chain={chain}, id={snapshot_id}"
            )

        return SnapshotResponse.from_orm(snapshot)

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting snapshot by path: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Failed to get snapshot: {str(e)}"
        )