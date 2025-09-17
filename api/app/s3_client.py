import boto3
from botocore.exceptions import ClientError, NoCredentialsError
from typing import List, Dict, Optional
from datetime import datetime
from app.config import get_settings
import logging

logger = logging.getLogger(__name__)


class S3Client:
    def __init__(self):
        settings = get_settings()
        self.bucket_name = settings.s3_bucket_name

        self.client = boto3.client(
            's3',
            endpoint_url=settings.s3_endpoint_url,
            aws_access_key_id=settings.s3_access_key_id,
            aws_secret_access_key=settings.s3_secret_access_key,
            region_name=settings.s3_region
        )

    def test_connection(self) -> bool:
        try:
            self.client.head_bucket(Bucket=self.bucket_name)
            return True
        except ClientError as e:
            error_code = e.response['Error']['Code']
            if error_code == '404':
                logger.error(f"Bucket {self.bucket_name} not found")
            else:
                logger.error(f"Error connecting to S3: {e}")
            return False
        except NoCredentialsError:
            logger.error("No S3 credentials available")
            return False

    def list_snapshots(
        self,
        prefix: Optional[str] = None,
        file_extensions: List[str] = None,
        max_keys: int = 1000
    ) -> List[Dict]:
        if file_extensions is None:
            file_extensions = ['.tar.gz', '.tar.zst', '.tar.lz4', '.snapshot']

        try:
            paginator = self.client.get_paginator('list_objects_v2')
            page_iterator = paginator.paginate(
                Bucket=self.bucket_name,
                Prefix=prefix or '',
                PaginationConfig={'MaxItems': max_keys, 'PageSize': 100}
            )

            snapshots = []
            for page in page_iterator:
                if 'Contents' not in page:
                    continue

                for obj in page['Contents']:
                    key = obj['Key']

                    # Check if file has one of the specified extensions
                    if any(key.endswith(ext) for ext in file_extensions):
                        snapshot_info = {
                            'key': key,
                            'size': obj['Size'],
                            'size_mb': round(obj['Size'] / (1024 * 1024), 2),
                            'size_gb': round(obj['Size'] / (1024 * 1024 * 1024), 2),
                            'last_modified': obj['LastModified'].isoformat(),
                            'etag': obj.get('ETag', '').strip('"'),
                            'storage_class': obj.get('StorageClass', 'STANDARD')
                        }

                        # Extract blockchain info from path if possible
                        parts = key.split('/')
                        if len(parts) > 0:
                            snapshot_info['filename'] = parts[-1]
                            if len(parts) > 1:
                                snapshot_info['chain'] = parts[0]

                        snapshots.append(snapshot_info)

            return sorted(snapshots, key=lambda x: x['last_modified'], reverse=True)

        except ClientError as e:
            logger.error(f"Error listing snapshots: {e}")
            raise

    def get_snapshot_metadata(self, key: str) -> Dict:
        try:
            response = self.client.head_object(Bucket=self.bucket_name, Key=key)

            return {
                'key': key,
                'size': response['ContentLength'],
                'size_mb': round(response['ContentLength'] / (1024 * 1024), 2),
                'size_gb': round(response['ContentLength'] / (1024 * 1024 * 1024), 2),
                'last_modified': response['LastModified'].isoformat(),
                'etag': response.get('ETag', '').strip('"'),
                'content_type': response.get('ContentType', ''),
                'metadata': response.get('Metadata', {}),
                'storage_class': response.get('StorageClass', 'STANDARD')
            }
        except ClientError as e:
            if e.response['Error']['Code'] == '404':
                logger.error(f"Snapshot {key} not found")
                raise ValueError(f"Snapshot {key} not found")
            else:
                logger.error(f"Error getting snapshot metadata: {e}")
                raise


# Singleton instance
_s3_client: Optional[S3Client] = None


def get_s3_client() -> S3Client:
    global _s3_client
    if _s3_client is None:
        _s3_client = S3Client()
    return _s3_client