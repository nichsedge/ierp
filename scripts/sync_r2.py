#!/usr/bin/env python3
"""Bidirectional Cloudflare R2 Synchronizer for iERP SQLite database.

Enables seamless multi-device synchronization between workstation (laptop)
and mobile (Xiaomi 14T Pro / PRoot Debian) using Cloudflare R2 bucket as SSOT.

Usage:
  uv run scripts/sync_r2.py status   # Show sync status (local vs remote)
  uv run scripts/sync_r2.py push     # Force upload local events.db -> R2
  uv run scripts/sync_r2.py pull     # Force download R2 -> local events.db
  uv run scripts/sync_r2.py auto     # Auto-sync (pull if remote is newer, push if local is newer)
"""

from __future__ import annotations

import argparse
import datetime
import hashlib
import hmac
import json
import os
import shutil
import sqlite3
import sys
import urllib.error
import urllib.request
from pathlib import Path

R2_BUCKET = "ichsanul-dev"
R2_KEY_LATEST = "db/ierp_latest.sqlite"
DB_SOURCE = Path.home() / "Projects" / "ierp" / "ierp" / "events.db"
LOCAL_BACKUPS_DIR = Path.home() / "Projects" / "ierp" / "backups"

CREDS_CANDIDATES = [
    Path.home() / "Projects" / "creds" / "cloudflare" / "r2_cred.json",
    Path.home() / "Projects" / "sansfinance" / "app" / "src" / "main" / "assets" / "r2_cred.json",
    Path.home() / "Projects" / "fitly" / "android" / "app" / "src" / "main" / "assets" / "r2_cred.json",
]


STATE_FILE = Path.home() / ".ierp_sync_state.json"


def load_credentials() -> tuple[str, str, str, str]:
    account_id = os.getenv("R2_ACCOUNT_ID") or os.getenv("CLOUDFLARE_ACCOUNT_ID")
    access_key = os.getenv("R2_ACCESS_KEY_ID") or os.getenv("AWS_ACCESS_KEY_ID")
    secret_key = os.getenv("R2_SECRET_ACCESS_KEY") or os.getenv("AWS_SECRET_ACCESS_KEY")
    bucket = os.getenv("R2_BUCKET_NAME") or R2_BUCKET

    if not (account_id and access_key and secret_key):
        for candidate in CREDS_CANDIDATES:
            if candidate.exists():
                try:
                    data = json.loads(candidate.read_text())
                    account_id = account_id or data.get("account_id")
                    access_key = access_key or data.get("access_key_id")
                    secret_key = secret_key or data.get("secret_access_key")
                    bucket = data.get("bucket_name") or bucket
                    break
                except Exception:
                    continue

    if not (account_id and access_key and secret_key):
        raise RuntimeError(
            "Cloudflare R2 credentials not found. "
            "Export R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY or place r2_cred.json in ~/Projects/creds/cloudflare/"
        )

    return account_id, access_key, secret_key, bucket


def _s3_request(
    method: str,
    key: str,
    account_id: str,
    access_key: str,
    secret_key: str,
    bucket: str,
    data_bytes: bytes | None = None,
) -> urllib.request.Request:
    host = f"{account_id}.r2.cloudflarestorage.com"
    url = f"https://{host}/{bucket}/{key}"

    now = datetime.datetime.now(datetime.timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    date_stamp = now.strftime("%Y%m%d")

    payload_hash = hashlib.sha256(data_bytes or b"").hexdigest()
    canonical_uri = f"/{bucket}/{key}"

    if method == "PUT":
        canonical_headers = (
            f"content-type:application/x-sqlite3\n"
            f"host:{host}\n"
            f"x-amz-content-sha256:{payload_hash}\n"
            f"x-amz-date:{amz_date}\n"
        )
        signed_headers = "content-type;host;x-amz-content-sha256;x-amz-date"
    else:
        canonical_headers = f"host:{host}\nx-amz-content-sha256:{payload_hash}\nx-amz-date:{amz_date}\n"
        signed_headers = "host;x-amz-content-sha256;x-amz-date"

    canonical_request = f"{method}\n{canonical_uri}\n\n{canonical_headers}\n{signed_headers}\n{payload_hash}"
    algorithm = "AWS4-HMAC-SHA256"
    credential_scope = f"{date_stamp}/auto/s3/aws4_request"
    string_to_sign = f"{algorithm}\n{amz_date}\n{credential_scope}\n{hashlib.sha256(canonical_request.encode()).hexdigest()}"

    def sign(key_bytes: bytes, msg: str) -> bytes:
        return hmac.new(key_bytes, msg.encode(), hashlib.sha256).digest()

    k_date = sign(("AWS4" + secret_key).encode(), date_stamp)
    k_region = sign(k_date, "auto")
    k_service = sign(k_region, "s3")
    k_signing = sign(k_service, "aws4_request")
    signature = hmac.new(k_signing, string_to_sign.encode(), hashlib.sha256).hexdigest()

    auth_header = f"{algorithm} Credential={access_key}/{credential_scope}, SignedHeaders={signed_headers}, Signature={signature}"

    req = urllib.request.Request(url, data=data_bytes, method=method)
    if method == "PUT":
        req.add_header("content-type", "application/x-sqlite3")
    req.add_header("x-amz-content-sha256", payload_hash)
    req.add_header("x-amz-date", amz_date)
    req.add_header("Authorization", auth_header)
    return req


def get_remote_metadata(key: str, account_id: str, access_key: str, secret_key: str, bucket: str) -> dict | None:
    try:
        req = _s3_request("HEAD", key, account_id, access_key, secret_key, bucket)
        with urllib.request.urlopen(req) as resp:
            headers = dict(resp.headers)
            last_modified_str = headers.get("Last-Modified")
            last_modified_dt = None
            if last_modified_str:
                try:
                    last_modified_dt = datetime.datetime.strptime(last_modified_str, "%a, %d %b %Y %H:%M:%S %Z").replace(tzinfo=datetime.timezone.utc)
                except Exception:
                    pass
            return {
                "size": int(headers.get("Content-Length", 0)),
                "etag": headers.get("ETag", "").strip('"'),
                "last_modified": last_modified_dt,
            }
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        raise


def checkpoint_db() -> None:
    """Flush SQLite WAL journal into main DB file atomically."""
    if not DB_SOURCE.exists():
        return
    try:
        conn = sqlite3.connect(DB_SOURCE)
        conn.execute("PRAGMA wal_checkpoint(TRUNCATE);")
        conn.close()
    except Exception:
        pass


def get_local_metadata() -> dict:
    if not DB_SOURCE.exists():
        return {"exists": False, "size": 0, "mtime": None, "md5": None, "last_event_dt": None}

    checkpoint_db()

    size = DB_SOURCE.stat().st_size
    mtime = datetime.datetime.fromtimestamp(DB_SOURCE.stat().st_mtime, tz=datetime.timezone.utc)
    md5_hex = hashlib.md5(DB_SOURCE.read_bytes()).hexdigest()

    last_event_dt = None
    try:
        conn = sqlite3.connect(f"file:{DB_SOURCE}?mode=ro", uri=True)
        cur = conn.cursor()
        cur.execute("SELECT max(created_at) FROM events")
        row = cur.fetchone()
        if row and row[0]:
            try:
                last_event_dt = datetime.datetime.fromisoformat(row[0]).astimezone(datetime.timezone.utc)
            except Exception:
                pass
        conn.close()
    except Exception:
        pass

    return {
        "exists": True,
        "size": size,
        "mtime": mtime,
        "md5": md5_hex,
        "last_event_dt": last_event_dt,
    }


def load_sync_state() -> dict:
    if STATE_FILE.exists():
        try:
            return json.loads(STATE_FILE.read_text())
        except Exception:
            pass
    return {}


def save_sync_state(etag: str, mtime_iso: str) -> None:
    try:
        STATE_FILE.write_text(json.dumps({
            "last_synced_etag": etag,
            "last_synced_time": mtime_iso,
            "updated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        }, indent=2))
    except Exception:
        pass


def push_to_r2() -> None:
    if not DB_SOURCE.exists():
        raise FileNotFoundError(f"Database not found at {DB_SOURCE}")

    checkpoint_db()

    LOCAL_BACKUPS_DIR.mkdir(parents=True, exist_ok=True)
    latest_file = LOCAL_BACKUPS_DIR / "events_backup_latest.sqlite"
    previous_file = LOCAL_BACKUPS_DIR / "events_backup_previous.sqlite"

    if latest_file.exists():
        shutil.copy2(latest_file, previous_file)

    shutil.copy2(DB_SOURCE, latest_file)

    backup_bytes = DB_SOURCE.read_bytes()
    backup_size = len(backup_bytes)
    local_md5 = hashlib.md5(backup_bytes).hexdigest()

    account_id, access_key, secret_key, bucket = load_credentials()
    req = _s3_request("PUT", R2_KEY_LATEST, account_id, access_key, secret_key, bucket, backup_bytes)
    with urllib.request.urlopen(req) as resp:
        if resp.status != 200:
            raise RuntimeError(f"Failed to upload to R2 (HTTP {resp.status})")

    # Fetch remote meta to ensure exact ETag & timestamp alignment
    meta = get_remote_metadata(R2_KEY_LATEST, account_id, access_key, secret_key, bucket)
    if meta and meta["last_modified"]:
        epoch = meta["last_modified"].timestamp()
        os.utime(DB_SOURCE, (epoch, epoch))
        save_sync_state(meta["etag"], meta["last_modified"].isoformat())
    else:
        save_sync_state(local_md5, datetime.datetime.now(datetime.timezone.utc).isoformat())

    print(f"🚀 PUSH SUCCESS: Local events.db ({backup_size:,} bytes | MD5: {local_md5[:8]}) uploaded to Cloudflare R2 ({R2_KEY_LATEST}).")


def pull_from_r2() -> None:
    account_id, access_key, secret_key, bucket = load_credentials()
    meta = get_remote_metadata(R2_KEY_LATEST, account_id, access_key, secret_key, bucket)
    if not meta:
        raise RuntimeError(f"Remote R2 object {R2_KEY_LATEST} does not exist in bucket {bucket}")

    LOCAL_BACKUPS_DIR.mkdir(parents=True, exist_ok=True)
    if DB_SOURCE.exists():
        backup_before = LOCAL_BACKUPS_DIR / "events_backup_before_pull.sqlite"
        shutil.copy2(DB_SOURCE, backup_before)

    req = _s3_request("GET", R2_KEY_LATEST, account_id, access_key, secret_key, bucket)
    with urllib.request.urlopen(req) as resp:
        if resp.status != 200:
            raise RuntimeError(f"Failed to download from R2 (HTTP {resp.status})")
        remote_bytes = resp.read()

    DB_SOURCE.parent.mkdir(parents=True, exist_ok=True)
    tmp_target = DB_SOURCE.with_suffix(".tmp")
    tmp_target.write_bytes(remote_bytes)

    # Verify SQLite integrity
    conn = sqlite3.connect(tmp_target)
    integrity = conn.execute("PRAGMA integrity_check;").fetchone()[0]
    conn.close()
    if integrity != "ok":
        tmp_target.unlink(missing_ok=True)
        raise RuntimeError(f"Integrity check failed for downloaded DB: {integrity}")

    tmp_target.replace(DB_SOURCE)

    # Set local DB mtime to match remote Last-Modified timestamp exactly
    if meta["last_modified"]:
        epoch = meta["last_modified"].timestamp()
        os.utime(DB_SOURCE, (epoch, epoch))
        save_sync_state(meta["etag"], meta["last_modified"].isoformat())

    # Ensure root symlink exists
    root_symlink = DB_SOURCE.parent.parent / "events.db"
    if not root_symlink.exists() or root_symlink.is_symlink():
        root_symlink.unlink(missing_ok=True)
        try:
            root_symlink.symlink_to(Path("ierp/events.db"))
        except Exception:
            pass

    print(f"📥 PULL SUCCESS: Cloudflare R2 snapshot ({len(remote_bytes):,} bytes | ETag: {meta['etag'][:8]}) downloaded to {DB_SOURCE}.")


def show_status() -> None:
    account_id, access_key, secret_key, bucket = load_credentials()
    remote = get_remote_metadata(R2_KEY_LATEST, account_id, access_key, secret_key, bucket)
    local = get_local_metadata()
    state = load_sync_state()

    print("═════════════════════════════════════════════════════════════════")
    print("📊 iERP Cloudflare R2 Synchronization Status")
    print("═════════════════════════════════════════════════════════════════")
    print(f"Bucket:  {bucket}")
    print(f"Object:  {R2_KEY_LATEST}")
    print("─────────────────────────────────────────────────────────────────")
    if local["exists"]:
        local_time = local["mtime"].strftime("%Y-%m-%d %H:%M:%S UTC") if local["mtime"] else "N/A"
        md5_short = local["md5"][:8] if local["md5"] else "N/A"
        print(f"Local DB:   {local['size']:,} bytes | MD5: {md5_short} | Modified: {local_time}")
    else:
        print("Local DB:   (does not exist)")

    if remote:
        remote_time = remote["last_modified"].strftime("%Y-%m-%d %H:%M:%S UTC") if remote["last_modified"] else "N/A"
        etag_short = remote["etag"][:8] if remote["etag"] else "N/A"
        print(f"Remote R2:  {remote['size']:,} bytes | ETag: {etag_short} | Modified: {remote_time}")
    else:
        print("Remote R2:  (not found)")
    print("═════════════════════════════════════════════════════════════════")

    if not remote and local["exists"]:
        print("Status: 🚀 LOCAL ONLY (Remote missing, run 'push' to seed R2)")
    elif remote and not local["exists"]:
        print("Status: 📥 REMOTE ONLY (Local missing, run 'pull' to restore)")
    elif remote and local["exists"]:
        if local["md5"] and remote["etag"] and local["md5"].lower() == remote["etag"].lower():
            print("Status: ✅ IN SYNC (Local database is byte-for-byte identical to Cloudflare R2)")
        else:
            last_synced_etag = state.get("last_synced_etag")
            if last_synced_etag and last_synced_etag.lower() == remote["etag"].lower():
                print("Status: ⬆️ LOCAL NEWER (You made changes locally since last sync)")
            elif local["mtime"] and remote["last_modified"]:
                diff = (local["mtime"] - remote["last_modified"]).total_seconds()
                if diff > 60:
                    print("Status: ⬆️ LOCAL NEWER (Local timestamp is newer than R2)")
                elif diff < -60:
                    print("Status: ⬇️ REMOTE NEWER (Cloudflare R2 has newer data from another device)")
                else:
                    print("Status: 🔄 DIVERGED (Different content with similar timestamp, check status)")
            else:
                print("Status: 🔄 DIVERGED (Hashes differ)")


def auto_sync() -> None:
    account_id, access_key, secret_key, bucket = load_credentials()
    remote = get_remote_metadata(R2_KEY_LATEST, account_id, access_key, secret_key, bucket)
    local = get_local_metadata()
    state = load_sync_state()

    if not remote:
        if local["exists"]:
            print("Remote R2 snapshot does not exist. Pushing local DB...")
            push_to_r2()
        else:
            print("Neither local nor remote database exists. Nothing to sync.")
        return

    if not local["exists"]:
        print("Local database does not exist. Pulling from R2...")
        pull_from_r2()
        return

    # Check byte-for-byte match
    if local["md5"] and remote["etag"] and local["md5"].lower() == remote["etag"].lower():
        print("✅ Already in sync with Cloudflare R2 (MD5 matches ETag).")
        # Ensure mtime and state are recorded
        if remote["last_modified"]:
            save_sync_state(remote["etag"], remote["last_modified"].isoformat())
        return

    last_synced_etag = state.get("last_synced_etag")
    if last_synced_etag and last_synced_etag.lower() == remote["etag"].lower():
        print("Local changes detected since last sync. Pushing to Cloudflare R2...")
        push_to_r2()
        return

    if local["mtime"] and remote["last_modified"]:
        diff = (local["mtime"] - remote["last_modified"]).total_seconds()
        if diff > 60:
            print(f"Local is newer by {int(diff)}s. Pushing to R2...")
            push_to_r2()
        else:
            print(f"Remote R2 is newer. Pulling from R2...")
            pull_from_r2()
    else:
        print("Defaulting to pulling latest snapshot from Cloudflare R2...")
        pull_from_r2()


def main() -> None:
    parser = argparse.ArgumentParser(description="iERP Cloudflare R2 Sync Tool")
    parser.add_argument(
        "action",
        choices=["status", "push", "pull", "auto"],
        nargs="?",
        default="auto",
        help="Action to perform: status, push, pull, or auto (default: auto)",
    )
    args = parser.parse_args()

    try:
        if args.action == "status":
            show_status()
        elif args.action == "push":
            push_to_r2()
        elif args.action == "pull":
            pull_from_r2()
        elif args.action == "auto":
            auto_sync()
    except Exception as e:
        print(f"❌ Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
