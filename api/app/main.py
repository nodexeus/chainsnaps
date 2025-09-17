from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from contextlib import asynccontextmanager
import logging
from app.config import get_settings
from app.routers import snapshots, health, auth
from app.s3_client import get_s3_client
from app.database import engine, Base
from app.services.snapshot_scanner import snapshot_scanner

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup
    logger.info("Starting ChainSnaps API...")
    settings = get_settings()

    # Create database tables
    logger.info("Creating database tables...")
    Base.metadata.create_all(bind=engine)

    # Test S3 connection (if configured)
    if settings.s3_bucket_name:
        try:
            s3_client = get_s3_client()
            if s3_client.test_connection():
                logger.info(f"Successfully connected to S3 bucket: {settings.s3_bucket_name}")
            else:
                logger.warning(f"Failed to connect to S3 bucket: {settings.s3_bucket_name}")
        except Exception as e:
            logger.warning(f"S3 connection test failed: {e}")
            logger.warning("S3 features will be unavailable until properly configured")
    else:
        logger.warning("S3 bucket not configured. S3 features will be unavailable.")

    # Start background scanner if enabled and S3 is configured
    if settings.scan_on_startup and settings.s3_bucket_name:
        logger.info("Starting background snapshot scanner...")
        await snapshot_scanner.start()
    elif settings.scan_on_startup:
        logger.warning("Scanner disabled: S3 bucket not configured")

    yield

    # Shutdown
    logger.info("Shutting down ChainSnaps API...")
    await snapshot_scanner.stop()


def create_app() -> FastAPI:
    settings = get_settings()

    app = FastAPI(
        title=settings.api_title,
        description=settings.api_description,
        version=settings.api_version,
        lifespan=lifespan
    )

    # Configure CORS
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],  # Configure this based on your needs
        allow_credentials=True,
        allow_methods=["GET"],
        allow_headers=["*"],
    )

    # Include routers
    app.include_router(health.router)
    app.include_router(auth.router, prefix="/api/v1")
    app.include_router(snapshots.router, prefix="/api/v1")

    @app.get("/")
    async def root():
        return {
            "message": "ChainSnaps API",
            "version": settings.api_version,
            "docs": "/docs",
            "health": "/health"
        }

    return app


app = create_app()