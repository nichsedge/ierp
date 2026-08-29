"""
Events domain service for iERP (Individual Enterprise Resource Planning).
Provides event logging, searching, querying, and linking with zero external dependencies.
"""

import json
import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Any

from .db import get_db, init_db
from .importers import parse_date_to_iso
from .linking import link_events_and_contacts


def insert_event(
    title: str,
    place: str | None = None,
    start_date: str | None = None,
    end_date: str | None = None,
    tags: str | list[str] | None = None,
    url: str | None = None,
    notes: str | None = None,
    contacts: list[str | int] | None = None,
    db_path: Path | None = None,
) -> tuple[int, list[str]]:
    """
    Inserts a structured event record directly into SQLite.
    Returns (event_id, list_of_explicitly_linked_contact_names).
    """
    init_db(db_path)
    parsed_start, _ = parse_date_to_iso(start_date)
    parsed_end, _ = parse_date_to_iso(end_date)

    if isinstance(tags, list):
        tag_list = [str(t).strip() for t in tags if str(t).strip()]
    elif isinstance(tags, str):
        tag_list = [t.strip() for t in tags.split(",") if t.strip()]
    else:
        tag_list = []

    linked_names: list[str] = []
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute(
            """
            INSERT INTO events (title, place, start_date, end_date, raw_date, tags, url, notes)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (title, place, parsed_start, parsed_end, start_date, json.dumps(tag_list), url, notes),
        )
        ev_id = int(cursor.lastrowid or 0)

        for ref in contacts or []:
            from .contacts import resolve_contact
            cid, cname = resolve_contact(cursor, ref)
            if cid is not None:
                cursor.execute(
                    "INSERT OR IGNORE INTO event_contacts (event_id, contact_id) VALUES (?, ?)",
                    (ev_id, cid),
                )
                if cname:
                    linked_names.append(cname)

        conn.commit()
        link_events_and_contacts(conn)

    return ev_id, linked_names


def list_events(
    limit: int = 20,
    offset: int = 0,
    q: str | None = None,
    tag: str | None = None,
    from_date: str | None = None,
    to_date: str | None = None,
    sort_col: str = "e.start_date",
    sort_dir: str = "DESC",
    db_path: Path | None = None,
) -> tuple[list[dict[str, Any]], int]:
    """
    Lists events with optional filtering, search, and pagination.
    Returns (events_list, total_count).
    """
    where_clauses = ["1=1"]
    params: list[Any] = []

    if q:
        where_clauses.append("(e.title LIKE ? OR e.place LIKE ? OR e.notes LIKE ?)")
        like_q = f"%{q}%"
        params.extend([like_q, like_q, like_q])

    if tag:
        where_clauses.append("e.tags LIKE ?")
        params.append(f"%{tag}%")

    if from_date:
        where_clauses.append("(e.start_date >= ? OR (e.start_date IS NULL AND e.raw_date >= ?))")
        params.extend([from_date, from_date])

    if to_date:
        to_bound = to_date + " 23:59:59" if len(to_date) == 10 else to_date
        where_clauses.append("(e.start_date <= ? OR (e.start_date IS NULL AND e.raw_date <= ?))")
        params.extend([to_bound, to_bound])

    where_sql = " AND ".join(where_clauses)
    direction = "ASC" if str(sort_dir).upper() == "ASC" else "DESC"

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        total = cursor.execute(f"SELECT COUNT(*) FROM events e WHERE {where_sql}", params).fetchone()[0]

        fetch_sql = f"""
            SELECT e.id, e.title, e.place, e.start_date, e.end_date, e.raw_date, e.tags, e.notes, e.created_at
            FROM events e
            WHERE {where_sql}
            ORDER BY {sort_col} {direction}, e.id DESC
            LIMIT ? OFFSET ?
        """
        rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

        events = []
        for r in rows:
            ev_id, title, place, start, end, raw_d, tags_json, notes, created_at = r
            try:
                tags_data = json.loads(tags_json) if tags_json else []
            except Exception:
                tags_data = []

            linked_contacts = cursor.execute(
                """
                SELECT c.id, c.name FROM contacts c
                JOIN event_contacts ec ON c.id = ec.contact_id
                WHERE ec.event_id = ?
                """,
                (ev_id,),
            ).fetchall()

            events.append({
                "id": ev_id,
                "title": title,
                "place": place,
                "start_date": start,
                "end_date": end,
                "raw_date": raw_d,
                "tags": tags_data,
                "notes": notes,
                "created_at": created_at,
                "linked_contacts": [{"id": cid, "name": cname} for cid, cname in linked_contacts],
            })

    return events, total


def get_event(event_id: int, db_path: Path | None = None) -> dict[str, Any] | None:
    """Fetches a single event with linked contacts and media attachments."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        row = cursor.execute(
            """
            SELECT id, title, place, start_date, end_date, raw_date, tags, url, notes, created_at
            FROM events WHERE id = ?
            """,
            (event_id,),
        ).fetchone()

        if not row:
            return None

        eid, title, place, start, end, raw_d, tags_json, url, notes, created_at = row
        try:
            tags_data = json.loads(tags_json) if tags_json else []
        except Exception:
            tags_data = []

        contacts = cursor.execute(
            """
            SELECT c.id, c.name, c.org FROM contacts c
            JOIN event_contacts ec ON c.id = ec.contact_id
            WHERE ec.event_id = ?
            """,
            (eid,),
        ).fetchall()

        media = cursor.execute(
            "SELECT id, original_filename, stored_path FROM event_media WHERE event_id = ?",
            (eid,),
        ).fetchall()

        return {
            "id": eid,
            "title": title,
            "place": place,
            "start_date": start,
            "end_date": end,
            "raw_date": raw_d,
            "tags": tags_data,
            "url": url,
            "notes": notes,
            "created_at": created_at,
            "contacts": [{"id": c[0], "name": c[1], "org": c[2]} for c in contacts],
            "media": [{"id": m[0], "original_filename": m[1], "stored_path": m[2]} for m in media],
        }


def search_events(query: str, limit: int = 50, db_path: Path | None = None) -> list[dict[str, Any]]:
    """Searches for events matching a query keyword across title, place, tags, and notes."""
    like_query = f"%{query}%"
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        rows = cursor.execute(
            """
            SELECT id, title, place, start_date, end_date, raw_date, tags, notes
            FROM events
            WHERE title LIKE ? OR place LIKE ? OR tags LIKE ? OR notes LIKE ?
            ORDER BY start_date DESC LIMIT ?
            """,
            (like_query, like_query, like_query, like_query, limit),
        ).fetchall()

        results = []
        for r in rows:
            eid, title, place, start, end, raw_d, tags_json, notes = r
            try:
                tags_data = json.loads(tags_json) if tags_json else []
            except Exception:
                tags_data = []
            results.append({
                "id": eid,
                "title": title,
                "place": place,
                "start_date": start,
                "end_date": end,
                "raw_date": raw_d,
                "tags": tags_data,
                "notes": notes,
            })
        return results


def delete_event(event_id: int, db_path: Path | None = None) -> bool:
    """Deletes an event by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM events WHERE id = ?", (event_id,))
        conn.commit()
        return cursor.rowcount > 0
