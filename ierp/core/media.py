"""
Media consumption log engine for iERP.
Stores structured media items (books, films, anime, manga, dramas) and
per-user logs (status, rating, dates, review) in pure stdlib SQLite.
Also manages the `links` table for profile/social/reference URLs.
"""

import json
import sqlite3
from datetime import datetime
from pathlib import Path
from typing import Optional
from .config import DB_PATH, C_GREEN, C_RED, C_RESET


def _now() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def upsert_media_item(
    cursor: sqlite3.Cursor,
    media_type: str,
    title: str,
    original_title: Optional[str] = None,
    year: Optional[int] = None,
    author: Optional[str] = None,
    country: Optional[str] = None,
    external_id: Optional[str] = None,
    source: Optional[str] = None,
    url: Optional[str] = None,
    extra: Optional[dict] = None,
) -> int:
    """
    Idempotently upserts a media item keyed by (media_type, source, external_id).
    Falls back to (media_type, title) matching when external_id is missing.
    Returns the media_items.id.
    """
    extra_json = json.dumps(extra, ensure_ascii=False, default=str) if extra else None

    if external_id:
        cursor.execute(
            "SELECT id FROM media_items WHERE media_type = ? AND source = ? AND external_id = ?",
            (media_type, source, str(external_id)),
        )
    else:
        cursor.execute(
            "SELECT id FROM media_items WHERE media_type = ? AND title = ? AND (external_id IS NULL OR external_id = '')",
            (media_type, title),
        )

    row = cursor.fetchone()
    if row:
        item_id = row[0]
        cursor.execute("""
            UPDATE media_items
            SET title = ?, original_title = COALESCE(?, original_title), year = COALESCE(?, year),
                author = COALESCE(?, author), country = COALESCE(?, country), url = COALESCE(?, url),
                extra_json = COALESCE(?, extra_json), updated_at = ?
            WHERE id = ?
        """, (title, original_title, year, author, country, url, extra_json, _now(), item_id))
        return item_id

    cursor.execute("""
        INSERT INTO media_items (media_type, title, original_title, year, author, country, external_id, source, url, extra_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    """, (media_type, title, original_title, year, author, country,
          str(external_id) if external_id else None, source, url, extra_json))
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
    raw_json = json.dumps(raw, ensure_ascii=False, default=str) if raw else None

    cursor.execute(
        "SELECT id FROM media_logs WHERE media_item_id = ? AND source IS ?",
        (media_item_id, source),
    )
    row = cursor.fetchone()
    if row:
        log_id = row[0]
        cursor.execute("""
            UPDATE media_logs
            SET status = COALESCE(?, status), rating = COALESCE(?, rating), progress = COALESCE(?, progress),
                started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at),
                date_logged = COALESCE(?, date_logged), review = COALESCE(?, review),
                raw_json = COALESCE(?, raw_json), updated_at = ?
            WHERE id = ?
        """, (status, rating, progress, started_at, finished_at, date_logged, review, raw_json, _now(), log_id))
        return log_id

    cursor.execute("""
        INSERT INTO media_logs (media_item_id, status, rating, progress, started_at, finished_at, date_logged, review, raw_json, source)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    """, (media_item_id, status, rating, progress, started_at, finished_at, date_logged, review, raw_json, source))
    return int(cursor.lastrowid or 0)


def ingest_media_records(records: list, db_path: Optional[Path] = None) -> dict:
    """
    Bulk-ingests normalized media records from get-data.
    Each record dict supports:
      media_type, title, original_title, year, author, country, external_id, source, url,
      extra (dict), status, rating, progress, started_at, finished_at, date_logged, review, raw (dict)
    Returns {"items": n, "logs": n}.
    """
    from .db import get_db

    conn = get_db(db_path)
    cursor = conn.cursor()
    items = logs = 0
    try:
        for rec in records:
            item_id = upsert_media_item(
                cursor,
                media_type=rec.get("media_type") or "unknown",
                title=rec.get("title") or "Untitled",
                original_title=rec.get("original_title"),
                year=rec.get("year"),
                author=rec.get("author"),
                country=rec.get("country"),
                external_id=rec.get("external_id"),
                source=rec.get("source"),
                url=rec.get("url"),
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
    finally:
        conn.close()
    return {"items": items, "logs": logs}


def upsert_link(label: str, url: str, category: Optional[str] = None,
                is_public: bool = True, notes: Optional[str] = None,
                db_path: Optional[Path] = None) -> int:
    """Idempotently upserts a link keyed by (label, url). Returns links.id."""
    from .db import get_db

    conn = get_db(db_path)
    cursor = conn.cursor()
    try:
        cursor.execute("SELECT id FROM links WHERE label = ? AND url = ?", (label, url))
        row = cursor.fetchone()
        if row:
            link_id = int(row[0])
            cursor.execute("""
                UPDATE links SET category = COALESCE(?, category), is_public = ?, notes = COALESCE(?, notes)
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
    finally:
        conn.close()


def list_media(media_type: Optional[str] = None, limit: int = 20,
               db_path: Optional[Path] = None) -> list:
    """Returns recent media logs joined with items, newest first."""
    from .db import get_db

    conn = get_db(db_path)
    cursor = conn.cursor()
    sql = """
    SELECT l.id, i.media_type, i.title, l.status, l.rating, l.date_logged, l.finished_at
    FROM media_logs l JOIN media_items i ON i.id = l.media_item_id
    """
    params = []
    if media_type:
        sql += " WHERE i.media_type = ?"
        params.append(media_type)
    sql += " ORDER BY COALESCE(l.date_logged, l.finished_at, l.started_at) DESC LIMIT ?"
    params.append(limit)
    rows = cursor.execute(sql, params).fetchall()
    conn.close()
    return rows


def list_links(category: Optional[str] = None, public_only: bool = False,
               db_path: Optional[Path] = None) -> list:
    """Returns links rows with optional filters."""
    from .db import get_db

    conn = get_db(db_path)
    cursor = conn.cursor()
    sql = "SELECT id, label, url, category, is_public FROM links WHERE 1=1"
    params = []
    if category:
        sql += " AND category = ?"
        params.append(category)
    if public_only:
        sql += " AND is_public = 1"
    sql += " ORDER BY category, label"
    rows = cursor.execute(sql, params).fetchall()
    conn.close()
    return rows
