from pydantic import BaseModel, Field
from typing import List, Optional, Dict, Any
from datetime import datetime


class SnapshotBase(BaseModel):
    chain: str = Field(..., description="Blockchain name (e.g., ethereum, arbitrum-one)")
    client: str = Field(..., description="Client name (e.g., reth, lighthouse, nitro)")
    network: str = Field(..., description="Network type (e.g., mainnet, testnet)")
    type: str = Field(..., description="Snapshot type (e.g., archive, full, light)")
    snapshot_path: str = Field(..., description="Full S3 path to snapshot directory")
    snapshot_id: str = Field(..., description="Snapshot version or identifier")
    manifest_body_path: str = Field(..., description="Path to manifest-body.json")
    manifest_header_path: Optional[str] = Field(None, description="Path to manifest-header.json")

    total_size_bytes: Optional[int] = Field(None, description="Total size in bytes")
    total_chunks: Optional[int] = Field(None, description="Number of chunks")
    compression_type: Optional[str] = Field(None, description="Compression algorithm")

    block_height: Optional[int] = Field(None, description="Block height at snapshot time")
    has_blobs: Optional[bool] = Field(None, description="Whether snapshot includes blobs")
    blob_start_block: Optional[int] = Field(None, description="Earliest block with blob data")
    blob_end_block: Optional[int] = Field(None, description="Latest block with blob data")

    snapshot_metadata: Optional[Dict[str, Any]] = Field(default_factory=dict, description="Additional metadata")
    external_metadata: Optional[Dict[str, Any]] = Field(default_factory=dict, description="External metadata")

    is_active: bool = Field(True, description="Whether snapshot is available")
    is_complete: Optional[bool] = Field(None, description="Whether snapshot is complete")


class SnapshotResponse(SnapshotBase):
    id: int = Field(..., description="Snapshot ID")
    indexed_at: datetime = Field(..., description="When snapshot was indexed")
    last_updated_at: datetime = Field(..., description="Last update timestamp")
    snapshot_created_at: Optional[datetime] = Field(None, description="When snapshot was created")

    class Config:
        from_attributes = True


class SnapshotUpdateRequest(BaseModel):
    block_height: Optional[int] = Field(None, description="Block height at snapshot time")
    has_blobs: Optional[bool] = Field(None, description="Whether snapshot includes blobs")
    blob_start_block: Optional[int] = Field(None, description="Earliest block with blob data")
    blob_end_block: Optional[int] = Field(None, description="Latest block with blob data")
    is_complete: Optional[bool] = Field(None, description="Whether snapshot is complete")
    external_metadata: Optional[Dict[str, Any]] = Field(None, description="Additional metadata to merge")


class SnapshotListResponse(BaseModel):
    snapshots: List[SnapshotResponse]
    count: int = Field(..., description="Number of snapshots returned")
    total_size_tb: float = Field(..., description="Total size in TB")


class HealthCheckResponse(BaseModel):
    status: str = Field(..., description="Service health status")
    s3_connected: bool = Field(..., description="S3 connection status")
    db_connected: bool = Field(..., description="Database connection status")
    timestamp: str = Field(..., description="Health check timestamp")
    version: str = Field(..., description="API version")


class ErrorResponse(BaseModel):
    detail: str = Field(..., description="Error message")
    status_code: int = Field(..., description="HTTP status code")
    timestamp: str = Field(default_factory=lambda: datetime.utcnow().isoformat())


class UserRegister(BaseModel):
    username: str = Field(..., description="Admin username", min_length=3, max_length=50)
    password: str = Field(..., description="Admin password", min_length=8)


class UserLogin(BaseModel):
    username: str = Field(..., description="Username")
    password: str = Field(..., description="Password")


class UserResponse(BaseModel):
    id: int = Field(..., description="User ID")
    username: str = Field(..., description="Username")
    is_admin: bool = Field(..., description="Whether user is admin")
    created_at: datetime = Field(..., description="Account creation time")
    last_login: Optional[datetime] = Field(None, description="Last login time")

    class Config:
        from_attributes = True


class APIKeyCreate(BaseModel):
    name: str = Field(..., description="Unique name for the API key", min_length=1, max_length=100)
    description: Optional[str] = Field(None, description="Description of key purpose")
    scopes: List[str] = Field(
        default=["snapshots:read"],
        description="List of permissions (e.g., 'snapshots:read', 'snapshots:write', 'admin')"
    )
    expires_in_days: Optional[int] = Field(None, description="Days until expiration (null for no expiration)", ge=1)
    rate_limit: Optional[int] = Field(None, description="Requests per hour limit (null for unlimited)", ge=1)


class APIKeyResponse(BaseModel):
    id: int = Field(..., description="API key ID")
    name: str = Field(..., description="API key name")
    description: Optional[str] = Field(None, description="Key description")
    key_prefix: str = Field(..., description="Key prefix for identification (first 8 chars of key)")
    api_key: Optional[str] = Field(None, description="Full API key (only shown once during creation)")
    scopes: List[str] = Field(..., description="Granted permissions")
    is_admin: bool = Field(..., description="Whether this is an admin key")
    is_active: bool = Field(..., description="Whether key is active")
    expires_at: Optional[datetime] = Field(None, description="Expiration time")
    created_at: datetime = Field(..., description="Creation time")
    last_used_at: Optional[datetime] = Field(None, description="Last usage time")
    usage_count: int = Field(..., description="Total usage count")
    rate_limit: Optional[int] = Field(None, description="Rate limit (requests/hour)")

    class Config:
        from_attributes = True


class APIKeyCreateResponse(APIKeyResponse):
    api_key: str = Field(..., description="The actual API key (only shown once)")


class APIKeyListResponse(BaseModel):
    keys: List[APIKeyResponse]
    count: int = Field(..., description="Total number of keys")