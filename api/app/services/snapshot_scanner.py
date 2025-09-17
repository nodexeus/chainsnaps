import asyncio
import json
import logging
from datetime import datetime
from typing import Dict, List, Optional, Any
from sqlalchemy.orm import Session
from app.database import get_db
from app.db_models import Snapshot, ScanHistory
from app.s3_client import get_s3_client
from app.config import get_settings

logger = logging.getLogger(__name__)


class SnapshotScannerService:
    def __init__(self):
        self.is_running = False
        self.task: Optional[asyncio.Task] = None
        self.settings = get_settings()

    def parse_snapshot_path(self, protocol_dir: str) -> Dict[str, str]:
        """Parse snapshot directory name to extract chain, client, network, and type

        Pattern detection works backwards from the end:
        - Last part before version: type (archive, full, light)
        - Second to last: network (mainnet, testnet, etc)
        - Third to last: client (reth, lighthouse, nitro, etc)
        - Everything else: chain (can be multi-word like arbitrum-one)

        Examples:
        - arbitrum-one-nitro-mainnet-full-v1 -> {chain: arbitrum-one, client: nitro, network: mainnet, type: full}
        - ethereum-lighthouse-mainnet-archive-v1 -> {chain: ethereum, client: lighthouse, network: mainnet, type: archive}
        - ethereum-reth-mainnet-archive-v1 -> {chain: ethereum, client: reth, network: mainnet, type: archive}
        """
        # Remove trailing slash and version suffix
        clean_path = protocol_dir.rstrip('/')
        # Remove version suffix (e.g., -v1, -v2)
        import re
        clean_path = re.sub(r'-v\d+$', '', clean_path)

        parts = clean_path.split('-')

        # Need at least 4 parts: chain-client-network-type
        if len(parts) >= 4:
            # Work backwards from the end
            snapshot_type = parts[-1]  # last part is type
            network = parts[-2]        # second to last is network
            client = parts[-3]         # third to last is client
            chain = '-'.join(parts[:-3])  # everything else is the chain name

            return {
                'chain': chain,
                'client': client,
                'network': network,
                'type': snapshot_type
            }

        # Fallback for incomplete patterns
        return {
            'chain': parts[0] if parts else 'unknown',
            'client': parts[1] if len(parts) > 1 else 'unknown',
            'network': parts[2] if len(parts) > 2 else 'mainnet',
            'type': parts[3] if len(parts) > 3 else 'archive'
        }

    async def start(self) -> Dict[str, Any]:
        """Start the background scanner"""
        logger.info("Starting snapshot scanner service")

        if self.is_running:
            logger.info("Scanner already running")
            return {"status": "already_running", "message": "Scanner is already running"}

        if not self.settings.scan_on_startup:
            logger.info("Scan on startup disabled")
            return {"status": "disabled", "message": "Scan on startup is disabled"}

        # Start the background task
        try:
            self.is_running = True
            self.task = asyncio.create_task(self._scanning_loop())
            logger.info("Background scanner task created")

            await asyncio.sleep(0.1)  # Give task a moment to start

            if self.task.done():
                # Task failed immediately
                try:
                    await self.task
                except Exception as e:
                    logger.error(f"Scanner task failed immediately: {e}")
                    self.is_running = False
                    return {"status": "error", "message": f"Scanner failed to start: {str(e)}"}

            return {"status": "started", "message": "Snapshot scanner started successfully"}

        except Exception as e:
            logger.error(f"Error starting scanner: {e}")
            self.is_running = False
            return {"status": "error", "message": f"Failed to start scanner: {str(e)}"}

    async def stop(self) -> Dict[str, Any]:
        """Stop the background scanner"""
        if not self.is_running:
            return {"status": "already_stopped", "message": "Scanner is not running"}

        self.is_running = False
        if self.task:
            self.task.cancel()
            try:
                await self.task
            except asyncio.CancelledError:
                pass

        logger.info("Snapshot scanner stopped")
        return {"status": "stopped", "message": "Scanner stopped successfully"}

    async def scan_now(self) -> Dict[str, Any]:
        """Run a manual scan immediately"""
        logger.info("Running manual snapshot scan")
        db = next(get_db())
        try:
            result = await self._run_single_scan(db, scan_type="manual")
            return result
        finally:
            db.close()

    async def _scanning_loop(self):
        """Main scanning loop that runs in the background"""
        logger.info("Scanner loop started")

        # Run initial scan
        db = next(get_db())
        try:
            await self._run_single_scan(db, scan_type="scheduled")
        finally:
            db.close()

        # Continue with scheduled scans
        while self.is_running:
            try:
                # Wait for the scanning interval
                interval_seconds = self.settings.scan_interval_hours * 3600
                logger.info(f"Waiting {interval_seconds} seconds until next scan")
                await asyncio.sleep(interval_seconds)

                if not self.is_running:
                    break

                # Run scheduled scan
                db = next(get_db())
                try:
                    await self._run_single_scan(db, scan_type="scheduled")
                finally:
                    db.close()

            except asyncio.CancelledError:
                logger.info("Scanning loop cancelled")
                break
            except Exception as e:
                logger.error(f"Error in scanning loop: {e}", exc_info=True)
                # Wait before retrying on error
                await asyncio.sleep(300)  # 5 minutes

        logger.info("Scanner loop ended")

    async def _run_single_scan(self, db: Session, scan_type: str = "manual") -> Dict[str, Any]:
        """Run a single scanning cycle"""
        start_time = datetime.utcnow()
        scan_history = ScanHistory(
            started_at=start_time,
            scan_type=scan_type,
            snapshots_found=0,
            new_snapshots=0,
            updated_snapshots=0
        )
        db.add(scan_history)
        db.commit()

        logger.info(f"Starting {scan_type} snapshot scan")

        errors = []
        prefixes_scanned = []
        total_snapshots_found = 0
        new_snapshots = 0
        updated_snapshots = 0

        try:
            s3_client = get_s3_client()

            # Scan all top-level directories in the bucket
            logger.info(f"Scanning bucket: {s3_client.bucket_name}")
            result = await self._scan_all_snapshots(db)

            total_snapshots_found += result["snapshots_found"]
            new_snapshots += result["new_snapshots"]
            updated_snapshots += result["updated_snapshots"]
            prefixes_scanned = result.get("prefixes_scanned", [])

            if result.get("errors"):
                errors.extend(result["errors"])

            # Update scan history
            end_time = datetime.utcnow()
            scan_history.completed_at = end_time
            scan_history.snapshots_found = total_snapshots_found
            scan_history.new_snapshots = new_snapshots
            scan_history.updated_snapshots = updated_snapshots
            scan_history.errors = errors if errors else None
            scan_history.prefixes_scanned = prefixes_scanned
            scan_history.duration_seconds = (end_time - start_time).total_seconds()
            db.commit()

            logger.info(
                f"Scan completed: {total_snapshots_found} snapshots found, "
                f"{new_snapshots} new, {updated_snapshots} updated, {len(errors)} errors"
            )

            return {
                "status": "completed",
                "scan_type": scan_type,
                "snapshots_found": total_snapshots_found,
                "new_snapshots": new_snapshots,
                "updated_snapshots": updated_snapshots,
                "chains_scanned": len(prefixes_scanned),
                "errors": errors,
                "duration_seconds": scan_history.duration_seconds,
                "timestamp": end_time.isoformat()
            }

        except Exception as e:
            logger.error(f"Fatal error in scan: {e}", exc_info=True)
            scan_history.completed_at = datetime.utcnow()
            scan_history.errors = [f"Fatal error: {str(e)}"]
            db.commit()
            return {
                "status": "error",
                "message": str(e),
                "timestamp": datetime.utcnow().isoformat()
            }

    async def _scan_all_snapshots(self, db: Session) -> Dict[str, Any]:
        """Scan all snapshots in the bucket"""
        s3_client = get_s3_client()
        client = s3_client.client

        snapshots_found = 0
        new_snapshots = 0
        updated_snapshots = 0
        errors = []
        prefixes_scanned = []

        try:
            # List all top-level directories in the bucket
            paginator = client.get_paginator("list_objects_v2")

            # List all top-level directories
            for page in paginator.paginate(
                Bucket=s3_client.bucket_name,
                Delimiter="/"
            ):
                if "CommonPrefixes" not in page:
                    continue

                # For each top-level directory, list its version subdirectories
                for prefix_obj in page["CommonPrefixes"]:
                    protocol_dir = prefix_obj.get("Prefix", "")
                    if not protocol_dir:
                        continue

                    logger.info(f"Checking protocol directory: {protocol_dir}")
                    prefixes_scanned.append(protocol_dir.rstrip('/'))

                    # List version subdirectories (e.g., "1/", "2/", etc.)
                    for version_page in paginator.paginate(
                        Bucket=s3_client.bucket_name,
                        Prefix=protocol_dir,
                        Delimiter="/"
                    ):
                        if "CommonPrefixes" not in version_page:
                            continue

                        for version_prefix in version_page["CommonPrefixes"]:
                            version_dir = version_prefix.get("Prefix", "")
                            if not version_dir:
                                continue

                            # Check for manifest files
                            manifest_body_path = f"{version_dir}manifest-body.json"
                            manifest_header_path = f"{version_dir}manifest-header.json"

                            try:
                                # Try to get manifest-body.json (required)
                                body_response = client.head_object(
                                    Bucket=s3_client.bucket_name,
                                    Key=manifest_body_path
                                )

                                # Extract snapshot ID from path
                                path_parts = version_dir.rstrip('/').split('/')
                                snapshot_id = path_parts[-1]

                                # Parse the protocol directory to get chain, client, network, type
                                parsed_info = self.parse_snapshot_path(protocol_dir)

                                # Check if snapshot already exists
                                existing_snapshot = db.query(Snapshot).filter(
                                    Snapshot.snapshot_path == version_dir.rstrip('/')
                                ).first()

                                # Try to get manifest-header.json (optional)
                                header_data = None
                                total_size = None
                                total_chunks = None
                                compression_type = None

                                try:
                                    header_response = client.get_object(
                                        Bucket=s3_client.bucket_name,
                                        Key=manifest_header_path
                                    )
                                    header_content = header_response["Body"].read()
                                    header_data = json.loads(header_content)

                                    total_size = header_data.get("total_size", 0)
                                    total_chunks = header_data.get("chunks", 0)
                                    compression_info = header_data.get("compression", {})
                                    if isinstance(compression_info, dict):
                                        compression_type = compression_info.get("algorithm")

                                    logger.debug(f"Found header manifest: {manifest_header_path}")
                                except client.exceptions.NoSuchKey:
                                    logger.debug(f"No header manifest at {manifest_header_path}")
                                except Exception as e:
                                    logger.warning(f"Error reading header manifest: {e}")

                                if existing_snapshot:
                                    # Update existing snapshot if header data changed
                                    updated = False
                                    if total_size and existing_snapshot.total_size_bytes != total_size:
                                        existing_snapshot.total_size_bytes = total_size
                                        updated = True
                                    if total_chunks and existing_snapshot.total_chunks != total_chunks:
                                        existing_snapshot.total_chunks = total_chunks
                                        updated = True
                                    if compression_type and existing_snapshot.compression_type != compression_type:
                                        existing_snapshot.compression_type = compression_type
                                        updated = True

                                    if updated:
                                        existing_snapshot.last_updated_at = datetime.utcnow()
                                        if header_data:
                                            existing_snapshot.snapshot_metadata = header_data
                                        db.commit()
                                        updated_snapshots += 1
                                        logger.info(f"Updated snapshot: {version_dir}")
                                else:
                                    # Create new snapshot record
                                    new_snapshot = Snapshot(
                                        chain=parsed_info['chain'],
                                        client=parsed_info['client'],
                                        network=parsed_info['network'],
                                        type=parsed_info['type'],
                                        snapshot_path=version_dir.rstrip('/'),
                                        snapshot_id=snapshot_id,
                                        manifest_body_path=manifest_body_path,
                                        manifest_header_path=manifest_header_path if header_data else None,
                                        total_size_bytes=total_size,
                                        total_chunks=total_chunks,
                                        compression_type=compression_type,
                                        snapshot_metadata=header_data,
                                        indexed_at=datetime.utcnow()
                                    )
                                    db.add(new_snapshot)
                                    db.commit()
                                    new_snapshots += 1
                                    logger.info(f"Found new snapshot: {version_dir}")

                                snapshots_found += 1

                            except client.exceptions.NoSuchKey:
                                # No manifest-body.json, skip this directory
                                logger.debug(f"No manifest-body.json at {manifest_body_path}")
                                continue
                            except Exception as e:
                                error_msg = f"Error processing {version_dir}: {str(e)}"
                                logger.error(error_msg)
                                errors.append(error_msg)

        except Exception as e:
            error_msg = f"Error scanning bucket: {str(e)}"
            logger.error(error_msg)
            errors.append(error_msg)

        return {
            "snapshots_found": snapshots_found,
            "new_snapshots": new_snapshots,
            "updated_snapshots": updated_snapshots,
            "prefixes_scanned": prefixes_scanned,
            "errors": errors
        }


# Global instance
snapshot_scanner = SnapshotScannerService()