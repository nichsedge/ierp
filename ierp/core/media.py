"""
Media consumption log engine for iERP.
Stores structured media items (books, films, anime, manga, dramas) and
per-user logs (status, rating, dates, review) in pure stdlib SQLite.
Also manages the `links` table for profile/social/reference URLs.
"""

import json
import sqlite3
from contextlib import closing
from datetime import datetime
from pathlib import Path
from typing import Optional

from .db import get_db

# Canonical upsert key for media items.
MEDIA_ITEM_KEY = ("media_type", "source", "title")

_ITEM_UPSERT_FIELDS = ("original_title", "year", "author", "extra_json")
_LOG_UPSERT_FIELDS = (
    "status", "rating", "progress", "started_at", "finished_at",
    "date_logged", "review", "raw_json",
)


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _dump_json(value) -> Optional[str]:
    if value is None:
        return None
    return json.dumps(value, ensure_ascii=False, default=str)


def _coalesce_update(table: str, fields: tuple, key_where: str, key_params: tuple,
                     values: dict, cursor: sqlite3.Cursor) -> None:
    """
    Builds and runs an idempotent UPDATE: each field only overwrites when the
    new value is non-NULL (COALESCE), so partial records never erase good data.
    """
    assignments = ", ".join(f"{f} = COALESCE(?, {f})" for f in fields)
    params = [values[f] for f in fields] + [_now()] + list(key_params)
    cursor.execute(f"UPDATE {table} SET {assignments}, updated_at = ? WHERE {key_where}", params)


def upsert_media_item(
    cursor: sqlite3.Cursor,
    media_type: str,
    title: str,
    original_title: Optional[str] = None,
    year: Optional[int] = None,
    author: Optional[str] = None,
    source: Optional[str] = None,
    extra: Optional[dict] = None,
) -> int:
    """
    Idempotently upserts a media item keyed by (media_type, source, title).
    Returns the media_items.id.
    """
    title = (title or "").strip() or "Untitled"
    values = {
        "original_title": original_title,
        "year": int(year) if year is not None and str(year).strip() else None,
        "author": author,
        "extra_json": _dump_json(extra),
    }

    cursor.execute(
        "SELECT id FROM media_items WHERE media_type = ? AND source IS ? AND title = ?",
        (media_type, source, title),
    )
    row = cursor.fetchone()
    if row:
        item_id = int(row[0])
        _coalesce_update("media_items", _ITEM_UPSERT_FIELDS,
                         "id = ?", (item_id,), values, cursor)
        return item_id

    cursor.execute("""
        INSERT INTO media_items (media_type, title, original_title, year, author, source, extra_json)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    """, (media_type, title, values["original_title"], values["year"],
          values["author"], source, values["extra_json"]))
    return int(cursor.lastrowid or 0)


def upsert_media_log(
    cursor: sqlite3.Cursor,
    media_item_id: int,
    status: Optional[str] = None,
    rating: Optional[float] = None,
    progress: Optional[str] = None,
    started_at: Optional[str] = None,
    finished_at: Optional[str] = None,
    date_logged: Optional[str] = None,
    review: Optional[str] = None,
    raw: Optional[dict] = None,
    source: Optional[str] = None,
) -> int:
    """
    Idempotently upserts the user's log for a media item keyed by (media_item_id, source).
    Returns the media_logs.id.
    """
    values = {
        "status": status, "rating": rating, "progress": progress,
        "started_at": started_at, "finished_at": finished_at,
        "date_logged": date_logged, "review": review,
        "raw_json": _dump_json(raw),
    }

    cursor.execute(
        "SELECT id FROM media_logs WHERE media_item_id = ? AND source IS ?",
        (media_item_id, source),
    )
    row = cursor.fetchone()
    if row:
        log_id = int(row[0])
        _coalesce_update("media_logs", _LOG_UPSERT_FIELDS,
                         "id = ?", (log_id,), values, cursor)
        return log_id

    cursor.execute("""
        INSERT INTO media_logs (media_item_id, status, rating, progress, started_at,
                                finished_at, date_logged, review, raw_json, source)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    """, (media_item_id, *values.values(), source))
    return int(cursor.lastrowid or 0)


def ingest_media_records(records: list, db_path: Optional[Path] = None) -> dict:
    """
    Bulk-ingests normalized media records from get-data in a single transaction.
    Each record dict supports:
      media_type, title, original_title, year, author, source,
      extra (dict), status, rating, progress, started_at, finished_at, date_logged, review, raw (dict)
    Returns {"items": n, "logs": n}.
    """
    if not records:
        return {"items": 0, "logs": 0}

    with closing(get_db(db_path)) as conn:
        try:
            cursor = conn.cursor()
            items = logs = 0
            for rec in records:
                item_id = upsert_media_item(
                    cursor,
                    media_type=rec.get("media_type") or "unknown",
                    title=rec.get("title") or "Untitled",
                    original_title=rec.get("original_title"),
                    year=rec.get("year"),
                    author=rec.get("author"),
                    source=rec.get("source"),
                    extra=rec.get("extra"),
                )
                items += 1
                upsert_media_log(
                    cursor,
                    media_item_id=item_id,
                    status=rec.get("status"),
                    rating=rec.get("rating"),
                    progress=rec.get("progress"),
                    started_at=rec.get("started_at"),
                    finished_at=rec.get("finished_at"),
                    date_logged=rec.get("date_logged"),
                    review=rec.get("review"),
                    raw=rec.get("raw"),
                    source=rec.get("source"),
                )
                logs += 1
            conn.commit()
        except Exception:
            conn.rollback()
            raise
    return {"items": items, "logs": logs}


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
    """Returns recent media logs joined with items, newest first."""
    sql = """
    SELECT l.id, i.media_type, i.title, l.status, l.rating, l.date_logged, l.finished_at
    FROM media_logs l JOIN media_items i ON i.id = l.media_item_id
    """
    params: list = []
    if media_type:
        sql += " WHERE i.media_type = ?"
        params.append(media_type)
    sql += " ORDER BY COALESCE(l.date_logged, l.finished_at, l.started_at) DESC LIMIT ?"
    params.append(limit)
    return _query(db_path, sql, params)


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
