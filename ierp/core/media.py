"""
Media consumption engine for iERP (stdlib only).
Single lean `media_items` table: (media_type, source, title) key + a `data_json`
blob holding everything else (author, year, status, rating, dates, review, and
any raw source fields). Idempotent upserts; JSON values merge key-wise, so a
partial record never erases existing data.
Also manages the `links` table (profile/social/reference URLs).
"""

import json
import sqlite3
from contextlib import closing
from datetime import datetime
from pathlib import Path
from typing import Optional

from .db import get_db

MEDIA_ITEM_KEY = ("media_type", "source", "title")


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def upsert_media_item(cursor: sqlite3.Cursor, rec: dict) -> int:
    """
    Idempotently upserts one media record into media_items, keyed by
    (media_type, source, title). All non-key fields go into data_json,
    merged key-wise with existing JSON (new non-null values win).
    Returns the row id.
    """
    media_type = rec.get("media_type") or "unknown"
    title = (rec.get("title") or "").strip() or "Untitled"
    source = rec.get("source")

    new_data = {k: v for k, v in rec.items()
                if k not in ("media_type", "title", "source") and v is not None}
    new_json = json.dumps(new_data, ensure_ascii=False, default=str)

    cursor.execute(
        "SELECT id, data_json FROM media_items WHERE media_type = ? AND source IS ? AND title = ?",
        (media_type, source, title),
    )
    row = cursor.fetchone()
    if row:
        item_id, existing_json = int(row[0]), row[1]
        try:
            merged = json.loads(existing_json or "{}")
        except (json.JSONDecodeError, TypeError):
            merged = {}
        merged.update(new_data)  # key-wise merge: non-null new values win
        cursor.execute(
            "UPDATE media_items SET data_json = ?, updated_at = ? WHERE id = ?",
            (json.dumps(merged, ensure_ascii=False, default=str), _now(), item_id),
        )
        return item_id

    cursor.execute(
        "INSERT INTO media_items (media_type, title, source, data_json) VALUES (?, ?, ?, ?)",
        (media_type, title, source, new_json),
    )
    return int(cursor.lastrowid or 0)


def ingest_media_records(records: list, db_path: Optional[Path] = None) -> dict:
    """
    Bulk-ingests normalized media records in a single transaction.
    Returns {"items": n}.
    """
    if not records:
        return {"items": 0}

    with closing(get_db(db_path)) as conn:
        try:
            cursor = conn.cursor()
            items = 0
            for rec in records:
                upsert_media_item(cursor, rec)
                items += 1
            conn.commit()
        except Exception:
            conn.rollback()
            raise
    return {"items": items}


def get_media_data(cursor: sqlite3.Cursor, row: tuple, keys: tuple = ("media_type", "title", "source", "data_json")) -> dict:
    """Helper: converts a media_items row tuple into a flat dict (JSON merged in)."""
    d = dict(zip(keys, row))
    try:
        data = json.loads(d.pop("data_json") or "{}")
    except (json.JSONDecodeError, TypeError):
        data = {}
    d.update(data)
    return d


def upsert_link(label: str, url: str, category: Optional[str] = None,
                is_public: bool = True, notes: Optional[str] = None,
                db_path: Optional[Path] = None) -> int:
    """Idempotently upserts a link keyed by (label, url). Returns links.id."""
    with closing(get_db(db_path)) as conn:
        try:
            cursor = conn.cursor()
            cursor.execute("SELECT id FROM links WHERE label = ? AND url = ?", (label, url))
            row = cursor.fetchone()
            if row:
                link_id = int(row[0])
                cursor.execute("""
                    UPDATE links SET category = COALESCE(?, category), is_public = ?,
                                     notes = COALESCE(?, notes)
                    WHERE id = ?
                """, (category, 1 if is_public else 0, notes, link_id))
            else:
                cursor.execute("""
                    INSERT INTO links (label, url, category, is_public, notes) VALUES (?, ?, ?, ?, ?)
                """, (label, url, category, 1 if is_public else 0, notes))
                link_id = int(cursor.lastrowid or 0)
            conn.commit()
            return link_id
        except Exception:
            conn.rollback()
            raise


def _query(db_path: Optional[Path], sql: str, params: list) -> list:
    """Runs a read-only query on a short-lived connection."""
    with closing(get_db(db_path)) as conn:
        return conn.execute(sql, params).fetchall()


def list_media(media_type: Optional[str] = None, limit: int = 20,
               db_path: Optional[Path] = None) -> list:
    """Returns recent media items (id, media_type, title, rating, date), newest first."""
    sql = ("SELECT id, media_type, title, data_json FROM media_items")
    params: list = []
    if media_type:
        sql += " WHERE media_type = ?"
        params.append(media_type)
    sql += " ORDER BY id DESC LIMIT ?"
    params.append(limit)
    rows = _query(db_path, sql, params)
    out = []
    for rid, mtype, title, dj in rows:
        try:
            data = json.loads(dj or "{}")
        except (json.JSONDecodeError, TypeError):
            data = {}
        out.append((rid, mtype, title, data.get("rating"), data.get("date_logged")))
    return out


def list_links(category: Optional[str] = None, public_only: bool = False,
               db_path: Optional[Path] = None) -> list:
    """Returns links rows with optional filters."""
    sql = "SELECT id, label, url, category, is_public FROM links WHERE 1=1"
    params: list = []
    if category:
        sql += " AND category = ?"
        params.append(category)
    if public_only:
        sql += " AND is_public = 1"
    sql += " ORDER BY category, label"
    return _query(db_path, sql, params)
