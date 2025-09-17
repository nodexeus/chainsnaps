#!/usr/bin/env python
"""
CLI tool for managing ChainSnaps API
"""
import argparse
import sys
from datetime import datetime, timedelta
from sqlalchemy.orm import Session
from app.database import SessionLocal, engine, Base
from app.db_models import APIKey
from app.config import get_settings
import getpass


def create_tables():
    """Create all database tables"""
    print("Creating database tables...")
    Base.metadata.create_all(bind=engine)
    print("Database tables created successfully!")


def create_api_key(name: str, description: str = None, scopes: list = None,
                   is_admin: bool = False, expires_days: int = None):
    """Create a new API key"""
    db = SessionLocal()
    try:
        # Check if name already exists
        existing = db.query(APIKey).filter(APIKey.name == name).first()
        if existing:
            print(f"Error: API key with name '{name}' already exists")
            return None

        # Generate new key
        api_key = APIKey.generate_api_key()
        key_hash = APIKey.hash_key(api_key)
        key_prefix = APIKey.get_key_prefix(api_key)

        # Calculate expiration
        expires_at = None
        if expires_days:
            expires_at = datetime.utcnow() + timedelta(days=expires_days)

        # Default scopes
        if scopes is None:
            scopes = ["admin"] if is_admin else ["snapshots:read"]

        # Create database record
        db_key = APIKey(
            name=name,
            description=description,
            key_hash=key_hash,
            key_prefix=key_prefix,
            scopes=scopes,
            is_admin=is_admin,
            expires_at=expires_at,
            created_by="CLI"
        )

        db.add(db_key)
        db.commit()

        print(f"\n✅ API Key created successfully!")
        print(f"Name: {name}")
        print(f"Prefix: {key_prefix}")
        print(f"Scopes: {', '.join(scopes)}")
        print(f"Admin: {is_admin}")
        if expires_at:
            print(f"Expires: {expires_at.isoformat()}")

        print(f"\n🔑 API Key (save this - it won't be shown again):")
        print(f"{api_key}\n")

        return api_key

    except Exception as e:
        print(f"Error creating API key: {e}")
        db.rollback()
        return None
    finally:
        db.close()


def list_api_keys(include_inactive: bool = False):
    """List all API keys"""
    db = SessionLocal()
    try:
        query = db.query(APIKey)
        if not include_inactive:
            query = query.filter(APIKey.is_active == True)

        keys = query.order_by(APIKey.created_at.desc()).all()

        if not keys:
            print("No API keys found")
            return

        print(f"\n{'='*80}")
        print(f"{'ID':<5} {'Name':<20} {'Prefix':<15} {'Admin':<7} {'Active':<7} {'Created':<20}")
        print(f"{'-'*80}")

        for key in keys:
            created = key.created_at.strftime("%Y-%m-%d %H:%M")
            print(f"{key.id:<5} {key.name:<20} {key.key_prefix:<15} "
                  f"{'Yes' if key.is_admin else 'No':<7} "
                  f"{'Yes' if key.is_active else 'No':<7} {created:<20}")

        print(f"{'='*80}\n")
        print(f"Total: {len(keys)} keys")

    except Exception as e:
        print(f"Error listing API keys: {e}")
    finally:
        db.close()


def revoke_api_key(key_id: int = None, key_name: str = None):
    """Revoke an API key"""
    db = SessionLocal()
    try:
        if key_id:
            key = db.query(APIKey).filter(APIKey.id == key_id).first()
        elif key_name:
            key = db.query(APIKey).filter(APIKey.name == key_name).first()
        else:
            print("Error: Must specify either --id or --name")
            return

        if not key:
            print(f"API key not found")
            return

        if not key.is_active:
            print(f"API key '{key.name}' is already inactive")
            return

        key.is_active = False
        key.revoked_at = datetime.utcnow()
        key.revoked_by = "CLI"
        db.commit()

        print(f"✅ API key '{key.name}' has been revoked")

    except Exception as e:
        print(f"Error revoking API key: {e}")
        db.rollback()
    finally:
        db.close()


def create_initial_setup():
    """Create initial admin key for setup"""
    print("\n🚀 ChainSnaps Initial Setup")
    print("="*50)

    db = SessionLocal()
    try:
        # Check if admin already exists
        existing_admin = db.query(APIKey).filter(APIKey.is_admin == True).first()
        if existing_admin:
            print(f"❌ Admin API key already exists: '{existing_admin.name}'")
            print("Only one admin is allowed in the system.")
            print("\nUse the existing admin key to manage other API keys.")
            return

        # Check if any keys exist
        existing_count = db.query(APIKey).count()
        if existing_count > 0:
            print(f"⚠️  {existing_count} non-admin API keys already exist")
            print("But no admin key found. Creating admin key...")
    finally:
        db.close()

    print("\nCreating the system admin API key:")
    print("This will be the only admin key in the system.")

    print("\nCreating admin API key...")
    api_key = create_api_key(
        name="System Admin",
        description="Primary system administrator API key",
        scopes=["admin", "snapshots:read", "snapshots:write"],
        is_admin=True
    )

    if api_key:
        print("\n✨ Setup complete!")
        print("\n📝 Next steps:")
        print("1. Save the API key shown above - it won't be displayed again")
        print("2. Use this admin key to create additional non-admin keys via the API:")
        print(f"   POST /api/v1/auth/keys - Create new API keys")
        print(f"   GET /api/v1/auth/keys - List all API keys")
        print(f"   DELETE /api/v1/auth/keys/{{id}} - Revoke keys")
        print("\n⚠️  Important: This is the only admin key allowed in the system.")


def main():
    parser = argparse.ArgumentParser(description="ChainSnaps API CLI")
    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Setup command
    setup_parser = subparsers.add_parser("setup", help="Initial setup with admin key creation")

    # Create tables command
    db_parser = subparsers.add_parser("create-tables", help="Create database tables")

    # Create key command
    create_parser = subparsers.add_parser("create-key", help="Create a new API key")
    create_parser.add_argument("name", help="Unique name for the API key")
    create_parser.add_argument("--description", help="Description of key purpose")
    create_parser.add_argument("--admin", action="store_true", help="Create admin key")
    create_parser.add_argument("--scopes", nargs="+",
                              help="Space-separated list of scopes (default: snapshots:read)")
    create_parser.add_argument("--expires", type=int,
                              help="Days until expiration")

    # List keys command
    list_parser = subparsers.add_parser("list-keys", help="List API keys")
    list_parser.add_argument("--all", action="store_true",
                            help="Include inactive keys")

    # Revoke key command
    revoke_parser = subparsers.add_parser("revoke-key", help="Revoke an API key")
    revoke_parser.add_argument("--id", type=int, help="Key ID")
    revoke_parser.add_argument("--name", help="Key name")

    args = parser.parse_args()

    if args.command == "setup":
        create_tables()
        create_initial_setup()
    elif args.command == "create-tables":
        create_tables()
    elif args.command == "create-key":
        create_api_key(
            name=args.name,
            description=args.description,
            scopes=args.scopes,
            is_admin=args.admin,
            expires_days=args.expires
        )
    elif args.command == "list-keys":
        list_api_keys(include_inactive=args.all)
    elif args.command == "revoke-key":
        revoke_api_key(key_id=args.id, key_name=args.name)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()