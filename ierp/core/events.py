"""
Events domain service for iERP (Individual Enterprise Resource Planning).
Provides event logging, searching, querying, and linking with zero external dependencies.
"""

import json
import re
import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Any

from .db import get_db, init_db
from .importers import parse_date_to_iso
from .linking import link_events_and_contacts


def sanitize_fts5_query(query: str) -> str:
    """
    Cleans raw user search input into a safe FTS5 MATCH query with prefix matching.
    Strips dangerous punctuation while preserving words and numbers.
    """
    if not query:
        return ""
    cleaned = re.sub(r'["\'*():^~+\-{}[\]]', ' ', query).strip()
    words = cleaned.split()
    if not words:
        return ""
    return " ".join(f'"{w}"*' for w in words)


def insert_event(
    title: str,
    place: str | None = None,
    start_date: str | None = None,
    end_date: str | None = None,
    tags: str | list[str] | None = None,
    url: str | None = None,
    notes: str | None = None,
    contacts: list[str | int] | None = None,
    project_id: int | None = None,
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
            INSERT INTO events (title, place, start_date, end_date, raw_date, tags, url, notes, project_id)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (title, place, parsed_start, parsed_end, start_date, json.dumps(tag_list), url, notes, project_id),
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


def update_event(
    event_id: int,
    title: str | None = None,
    place: str | None = None,
    start_date: str | None = None,
    end_date: str | None = None,
    tags: str | list[str] | None = None,
    url: str | None = None,
    notes: str | None = None,
    contacts: list[str | int] | None = None,
    project_id: int | None = None,
    db_path: Path | None = None,
) -> tuple[bool, list[str]]:
    """
    Updates an existing event record in SQLite.
    Returns (success_boolean, list_of_explicitly_linked_contact_names).
    """
    init_db(db_path)
    linked_names: list[str] = []

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        existing = cursor.execute("SELECT id FROM events WHERE id = ?", (event_id,)).fetchone()
        if not existing:
            return False, []

        updates: list[str] = []
        params: list[Any] = []

        if title is not None:
            updates.append("title = ?")
            params.append(title)
        if place is not None:
            updates.append("place = ?")
            params.append(place)
        if start_date is not None:
            parsed_start, _ = parse_date_to_iso(start_date)
            updates.append("start_date = ?")
            params.append(parsed_start)
            updates.append("raw_date = ?")
            params.append(start_date)
        if end_date is not None:
            parsed_end, _ = parse_date_to_iso(end_date)
            updates.append("end_date = ?")
            params.append(parsed_end)
        if tags is not None:
            if isinstance(tags, list):
                tag_list = [str(t).strip() for t in tags if str(t).strip()]
            elif isinstance(tags, str):
                tag_list = [t.strip() for t in tags.split(",") if t.strip()]
            else:
                tag_list = []
            updates.append("tags = ?")
            params.append(json.dumps(tag_list))
        if url is not None:
            updates.append("url = ?")
            params.append(url)
        if notes is not None:
            updates.append("notes = ?")
            params.append(notes)
        if project_id is not None:
            updates.append("project_id = ?")
            params.append(project_id)

        if updates:
            params.append(event_id)
            cursor.execute(f"UPDATE events SET {', '.join(updates)} WHERE id = ?", params)

        if contacts is not None:
            from .contacts import resolve_contact
            for ref in contacts:
                cid, cname = resolve_contact(cursor, ref)
                if cid is not None:
                    cursor.execute(
                        "INSERT OR IGNORE INTO event_contacts (event_id, contact_id) VALUES (?, ?)",
                        (event_id, cid),
                    )
                    if cname:
                        linked_names.append(cname)

        conn.commit()
        link_events_and_contacts(conn)

    return True, linked_names


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
        fts_q = sanitize_fts5_query(q)
        if fts_q:
            where_clauses.append("e.id IN (SELECT rowid FROM events_fts WHERE events_fts MATCH ?)")
            params.append(fts_q)
        else:
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
            SELECT e.id, e.title, e.place, e.start_date, e.end_date, e.raw_date, e.tags, e.url, e.notes, e.created_at, e.project_id, p.title
            FROM events e
            LEFT JOIN projects p ON p.id = e.project_id
            WHERE e.id = ?
            """,
            (event_id,),
        ).fetchone()

        if not row:
            return None

        eid, title, place, start, end, raw_d, tags_json, url, notes, created_at, project_id, project_title = row
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
            "project_id": project_id,
            "project_title": project_title,
            "contacts": [{"id": c[0], "name": c[1], "org": c[2]} for c in contacts],
            "media": [{"id": m[0], "original_filename": m[1], "stored_path": m[2]} for m in media],
        }


def search_events(query: str, limit: int = 50, db_path: Path | None = None) -> list[dict[str, Any]]:
    """
    Full-text search across events and notes.
    Uses SQLite FTS5 with BM25 relevance ranking and snippet extraction.
    Falls back gracefully to SQL LIKE matching if FTS5 query encounters an error.
    """
    if not query or not query.strip():
        return []

    fts_query = sanitize_fts5_query(query)

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()

        if fts_query:
            try:
                sql = """
                    SELECT e.id, e.title, e.place, e.start_date, e.end_date, e.raw_date, e.tags, e.notes,
                           bm25(events_fts) as rank,
                           snippet(events_fts, 0, '[MATCH]', '[/MATCH]', '...', 12) as title_snip,
                           snippet(events_fts, 2, '[MATCH]', '[/MATCH]', '...', 16) as notes_snip
                    FROM events_fts
                    JOIN events e ON e.id = events_fts.rowid
                    WHERE events_fts MATCH ?
                    ORDER BY rank ASC, e.start_date DESC
                    LIMIT ?
                """
                rows = cursor.execute(sql, (fts_query, limit)).fetchall()
                results = []
                for r in rows:
                    eid, title, place, start, end, raw_d, tags_json, notes, rank, t_snip, n_snip = r
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
                        "rank": round(float(rank), 4) if rank is not None else 0.0,
                        "title_snippet": t_snip or title,
                        "notes_snippet": n_snip or notes,
                    })
                return results
            except Exception:
                pass

        # Fallback to standard LIKE search
        like_query = f"%{query}%"
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
                "rank": 0.0,
                "title_snippet": title,
                "notes_snippet": notes,
            })
        return results


def delete_event(event_id: int, db_path: Path | None = None) -> bool:
    """Deletes an event by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM events WHERE id = ?", (event_id,))
        conn.commit()
        return cursor.rowcount > 0
