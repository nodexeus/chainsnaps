from sqlalchemy import Column, Integer, String, DateTime, BigInteger, JSON, Boolean, Float, Index, Text, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from datetime import datetime
from app.database import Base
import secrets
import hashlib
import bcrypt


class User(Base):
    __tablename__ = "users"

    id = Column(Integer, primary_key=True, index=True)
    username = Column(String, unique=True, nullable=False, index=True)
    password_hash = Column(String, nullable=False)
    is_admin = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime, nullable=False, default=func.now())
    last_login = Column(DateTime, nullable=True)

    # Relationship to API keys
    api_keys = relationship("APIKey", back_populates="user")

    @staticmethod
    def hash_password(password: str) -> str:
        """Hash a password for storage"""
        salt = bcrypt.gensalt()
        return bcrypt.hashpw(password.encode('utf-8'), salt).decode('utf-8')

    def verify_password(self, password: str) -> bool:
        """Verify a password against the hash"""
        return bcrypt.checkpw(password.encode('utf-8'), self.password_hash.encode('utf-8'))


class Snapshot(Base):
    __tablename__ = "snapshots"

    id = Column(Integer, primary_key=True, index=True)

    # Basic identification
    chain = Column(String, nullable=False, index=True)  # e.g., "ethereum", "arbitrum-one"
    client = Column(String, nullable=False, index=True)  # e.g., "reth", "lighthouse", "nitro"
    network = Column(String, nullable=False, index=True)  # e.g., "mainnet", "testnet"
    type = Column(String, nullable=False, index=True)  # e.g., "archive", "full", "light"
    snapshot_path = Column(String, nullable=False, unique=True)  # Full S3 path to snapshot directory
    snapshot_id = Column(String, nullable=False)  # Version or identifier (e.g., "1", "2", etc.)

    # Manifest file paths
    manifest_body_path = Column(String, nullable=False)  # Path to manifest-body.json
    manifest_header_path = Column(String, nullable=True)  # Path to manifest-header.json (optional)

    # Snapshot metadata from manifests
    total_size_bytes = Column(BigInteger, nullable=True)  # Total size in bytes
    total_chunks = Column(Integer, nullable=True)  # Number of chunks
    compression_type = Column(String, nullable=True)  # Compression algorithm used

    # Additional metadata that can be updated externally
    block_height = Column(BigInteger, nullable=True, index=True)  # Block height at snapshot time
    has_blobs = Column(Boolean, nullable=True)  # Whether snapshot includes blobs
    blob_start_block = Column(BigInteger, nullable=True)  # Earliest block with blob data
    blob_end_block = Column(BigInteger, nullable=True)  # Latest block with blob data

    # Custom metadata (JSON field for flexibility)
    snapshot_metadata = Column(JSON, nullable=True)  # Additional metadata from manifest or external sources
    external_metadata = Column(JSON, nullable=True)  # Metadata provided by external processes

    # Timestamps
    snapshot_created_at = Column(DateTime, nullable=True)  # When snapshot was created (from manifest)
    indexed_at = Column(DateTime, nullable=False, default=func.now())  # When we discovered/indexed it
    last_updated_at = Column(DateTime, nullable=False, default=func.now(), onupdate=func.now())

    # Status
    is_active = Column(Boolean, nullable=False, default=True)  # Whether snapshot is still available
    is_complete = Column(Boolean, nullable=True)  # Whether snapshot is complete (from external validation)

    # Create indexes for common queries
    __table_args__ = (
        Index('idx_chain_block_height', 'chain', 'block_height'),
        Index('idx_chain_active', 'chain', 'is_active'),
        Index('idx_snapshot_path', 'snapshot_path'),
    )


class ScanHistory(Base):
    __tablename__ = "scan_history"

    id = Column(Integer, primary_key=True, index=True)
    started_at = Column(DateTime, nullable=False, default=func.now())
    completed_at = Column(DateTime, nullable=True)
    scan_type = Column(String, nullable=False)  # 'manual' or 'scheduled'

    # Scan results
    snapshots_found = Column(Integer, nullable=False, default=0)
    new_snapshots = Column(Integer, nullable=False, default=0)
    updated_snapshots = Column(Integer, nullable=False, default=0)
    errors = Column(JSON, nullable=True)  # List of any errors encountered

    # Scan metadata
    prefixes_scanned = Column(JSON, nullable=True)  # List of S3 prefixes that were scanned
    duration_seconds = Column(Float, nullable=True)

    __table_args__ = (
        Index('idx_scan_started_at', 'started_at'),
    )


class APIKey(Base):
    __tablename__ = "api_keys"

    id = Column(Integer, primary_key=True, index=True)

    # Key identification
    name = Column(String, nullable=False, unique=True)  # e.g., "Web App", "Metadata Updater"
    description = Column(Text, nullable=True)  # Additional description of key purpose

    # Key value
    api_key = Column(String, nullable=False, unique=True, index=True)  # The actual API key
    key_hash = Column(String, nullable=False, unique=True, index=True)  # SHA256 hash of the actual key
    key_prefix = Column(String, nullable=False)  # First 8 characters for identification (e.g., "csnp_abc...")

    # Ownership
    user_id = Column(Integer, ForeignKey('users.id', ondelete='CASCADE'), nullable=False)
    user = relationship("User", back_populates="api_keys")

    # Permissions
    scopes = Column(JSON, nullable=False, default=list)  # List of allowed scopes/permissions
    is_admin = Column(Boolean, nullable=False, default=False)  # Admin keys can manage other keys

    # Usage tracking
    last_used_at = Column(DateTime, nullable=True)
    usage_count = Column(Integer, nullable=False, default=0)

    # Status
    is_active = Column(Boolean, nullable=False, default=True)
    expires_at = Column(DateTime, nullable=True)  # Optional expiration

    # Metadata
    created_at = Column(DateTime, nullable=False, default=func.now())
    created_by = Column(String, nullable=True)  # Who created this key
    revoked_at = Column(DateTime, nullable=True)
    revoked_by = Column(String, nullable=True)

    # Rate limiting (optional)
    rate_limit = Column(Integer, nullable=True)  # Requests per hour, null = unlimited

    @staticmethod
    def generate_api_key() -> str:
        """Generate a new API key with prefix"""
        random_bytes = secrets.token_urlsafe(32)
        return f"csnp_{random_bytes}"

    @staticmethod
    def hash_key(api_key: str) -> str:
        """Hash an API key for storage"""
        return hashlib.sha256(api_key.encode()).hexdigest()

    @staticmethod
    def get_key_prefix(api_key: str) -> str:
        """Get the prefix for display purposes"""
        return api_key[:12] if len(api_key) > 12 else api_key