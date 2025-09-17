from pydantic_settings import BaseSettings
from typing import List
from functools import lru_cache


class Settings(BaseSettings):
    # Database Configuration
    database_url: str = "postgresql://user:password@localhost/chainsnaps"

    # S3 Configuration (optional - S3 features disabled if not provided)
    s3_endpoint_url: str = "https://s3.amazonaws.com"
    s3_access_key_id: str = ""
    s3_secret_access_key: str = ""
    s3_bucket_name: str = ""
    s3_region: str = "us-east-1"

    # API Configuration
    api_title: str = "ChainSnaps API"
    api_version: str = "1.0.0"
    api_description: str = "API for managing blockchain snapshots in S3 storage"

    # Server Configuration
    host: str = "0.0.0.0"
    port: int = 8000
    reload: bool = False
    log_level: str = "info"

    # Scanner Configuration
    scan_on_startup: bool = True
    scan_interval_hours: int = 6

    class Config:
        env_file = ".env"
        case_sensitive = False


@lru_cache()
def get_settings() -> Settings:
    return Settings()