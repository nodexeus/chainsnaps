#!/bin/bash

# Fix database permissions for snapd user
# This script resolves the "permission denied for schema public" error

set -e

DB_NAME="${1:-snapd}"
DB_USER="${2:-snapd}"

echo "Fixing database permissions for user '$DB_USER' on database '$DB_NAME'..."

# Run as postgres superuser to grant permissions
sudo -u postgres psql -d "$DB_NAME" << EOF
-- Grant schema permissions
GRANT ALL ON SCHEMA public TO $DB_USER;
GRANT CREATE ON SCHEMA public TO $DB_USER;
GRANT USAGE ON SCHEMA public TO $DB_USER;

-- Grant privileges on all existing tables
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO $DB_USER;

-- Grant privileges on all existing sequences (for SERIAL columns)
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO $DB_USER;

-- Set default privileges for future objects created by any user
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO $DB_USER;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO $DB_USER;

-- Show current permissions
\dp
EOF

echo "Database permissions fixed successfully!"
echo "You can now restart the snapd service:"
echo "  sudo systemctl restart snapd"