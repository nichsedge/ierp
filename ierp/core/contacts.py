"""
Contacts domain service for iERP (Individual Enterprise Resource Planning).
Provides contact management, deduplication, lookup, and queries with zero external dependencies.
"""

import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Any

from .db import get_db, init_db


def resolve_contact(cursor_or_conn: sqlite3.Cursor | sqlite3.Connection, ref: str | int) -> tuple[int | None, str | None]:
    """
    Resolves a contact by numeric ID or exact/case-insensitive/substring name.
    Returns (id, name) or (None, None).
    """
    cursor = cursor_or_conn.cursor() if isinstance(cursor_or_conn, sqlite3.Connection) else cursor_or_conn
    ref_str = str(ref).strip()
    if ref_str.isdigit():
        row = cursor.execute("SELECT id, name FROM contacts WHERE id = ?", (int(ref_str),)).fetchone()
        if row:
            return int(row[0]), row[1]
    row = cursor.execute(
        "SELECT id, name FROM contacts WHERE LOWER(name) = LOWER(?)", (ref_str,)
    ).fetchone()
    if row:
        return int(row[0]), row[1]
    # Fallback: unique substring match
    rows = cursor.execute(
        "SELECT id, name FROM contacts WHERE name LIKE ?", (f"%{ref_str}%",)
    ).fetchall()
    if len(rows) == 1:
        return int(rows[0][0]), rows[0][1]
    return None, None


def insert_contact(
    name: str,
    org: str | None = None,
    client: str | None = None,
    location: str | None = None,
    email: str | None = None,
    phone: str | None = None,
    notes: str | None = None,
    google_id: str | None = None,
    source: str = "manual",
    date_val: str | None = None,
    tier: int = 3,
    cadence_days: int | None = None,
    db_path: Path | None = None,
) -> int:
    """Inserts a structured contact record directly into SQLite. Returns contacts.id."""
    init_db(db_path)
    clean_tier = max(1, min(3, tier))
    days = cadence_days if cadence_days and cadence_days > 0 else (14 if clean_tier == 1 else (60 if clean_tier == 2 else 180))

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute(
            """
            INSERT INTO contacts (name, org, client, location, email, phone, notes, google_id, source, date, tier, cadence_days)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (name, org, client, location, email, phone, notes, google_id, source, date_val, clean_tier, days),
        )
        conn.commit()
        return int(cursor.lastrowid or 0)


def list_contacts(
    source_filter: str | None = None,
    q: str | None = None,
    tier: int | None = None,
    limit: int = 50,
    offset: int = 0,
    sort_col: str = "c.name",
    sort_dir: str = "ASC",
    db_path: Path | None = None,
) -> tuple[list[dict[str, Any]], int]:
    """
    Lists contacts with optional source filtering, search, and pagination.
    Returns (contacts_list, total_count).
    """
    where_clauses = ["1=1"]
    params: list[Any] = []

    if source_filter and source_filter.lower() != "all":
        where_clauses.append("LOWER(c.source) = ?")
        params.append(source_filter.lower())

    if tier is not None:
        where_clauses.append("c.tier = ?")
        params.append(tier)

    if q:
        where_clauses.append(
            "(c.name LIKE ? OR c.org LIKE ? OR c.client LIKE ? OR c.email LIKE ? OR c.phone LIKE ? OR c.notes LIKE ? OR c.location LIKE ?)"
        )
        like_q = f"%{q}%"
        params.extend([like_q, like_q, like_q, like_q, like_q, like_q, like_q])

    where_sql = " AND ".join(where_clauses)
    direction = "DESC" if str(sort_dir).upper() == "DESC" else "ASC"

    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        total = cursor.execute(f"SELECT COUNT(*) FROM contacts c WHERE {where_sql}", params).fetchone()[0]

        fetch_sql = f"""
            SELECT c.id, c.name, c.org, c.client, c.location, c.notes, c.email, c.phone, c.source, c.google_id, c.created_at,
                   c.tier, c.cadence_days,
                   (SELECT COUNT(*) FROM event_contacts ec WHERE ec.contact_id = c.id) as event_count
            FROM contacts c
            WHERE {where_sql}
            ORDER BY {sort_col} {direction}, c.name ASC
            LIMIT ? OFFSET ?
        """
        rows = cursor.execute(fetch_sql, [*params, limit, offset]).fetchall()

        contacts = [{
            "id": r[0],
            "name": r[1],
            "org": r[2],
            "client": r[3],
            "location": r[4],
            "notes": r[5],
            "email": r[6],
            "phone": r[7],
            "source": r[8] or ("google" if r[9] else "manual"),
            "is_google_linked": bool(r[9]),
            "google_id": r[9],
            "created_at": r[10],
            "tier": r[11] or 3,
            "cadence_days": r[12] or 180,
            "event_count": r[13],
        } for r in rows]

    return contacts, total


def get_contact(contact_id: int, db_path: Path | None = None) -> dict[str, Any] | None:
    """Fetches full contact details and linked events."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        row = cursor.execute(
            """
            SELECT name, client, date, location, org, notes, email, phone, google_id, source, created_at, tier, cadence_days
            FROM contacts WHERE id = ?
            """,
            (contact_id,),
        ).fetchone()

        if not row:
            return None

        name, client, date_val, location, org, notes, email, phone, google_id, source, created_at, tier_val, cadence_val = row
        event_rows = cursor.execute(
            """
            SELECT e.id, e.title, e.start_date FROM events e
            JOIN event_contacts ec ON e.id = ec.event_id
            WHERE ec.contact_id = ?
            ORDER BY e.start_date DESC
            """,
            (contact_id,),
        ).fetchall()

        return {
            "id": contact_id,
            "name": name,
            "client": client,
            "date": date_val,
            "location": location,
            "org": org,
            "notes": notes,
            "email": email,
            "phone": phone,
            "google_id": google_id,
            "source": source or ("google" if google_id else "manual"),
            "is_google_linked": bool(google_id),
            "created_at": created_at,
            "tier": tier_val or 3,
            "cadence_days": cadence_val or 180,
            "events": [{"id": eid, "title": etitle, "start_date": estart} for eid, etitle, estart in event_rows],
        }


def update_contact(
    contact_id: int,
    name: str | None = None,
    org: str | None = None,
    client: str | None = None,
    location: str | None = None,
    email: str | None = None,
    phone: str | None = None,
    notes: str | None = None,
    tier: int | None = None,
    cadence_days: int | None = None,
    db_path: Path | None = None,
) -> bool:
    """Updates fields of an existing contact. Returns True if contact was found and updated."""
    init_db(db_path)
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        existing = cursor.execute("SELECT id FROM contacts WHERE id = ?", (contact_id,)).fetchone()
        if not existing:
            return False

        updates: list[str] = []
        params: list[Any] = []

        if name is not None:
            updates.append("name = ?")
            params.append(name)
        if org is not None:
            updates.append("org = ?")
            params.append(org)
        if client is not None:
            updates.append("client = ?")
            params.append(client)
        if location is not None:
            updates.append("location = ?")
            params.append(location)
        if email is not None:
            updates.append("email = ?")
            params.append(email)
        if phone is not None:
            updates.append("phone = ?")
            params.append(phone)
        if notes is not None:
            updates.append("notes = ?")
            params.append(notes)
        if tier is not None:
            clean_tier = max(1, min(3, tier))
            updates.append("tier = ?")
            params.append(clean_tier)
            if cadence_days is None:
                updates.append("cadence_days = ?")
                params.append(14 if clean_tier == 1 else (60 if clean_tier == 2 else 180))
        if cadence_days is not None:
            updates.append("cadence_days = ?")
            params.append(cadence_days)

        if updates:
            params.append(contact_id)
            cursor.execute(f"UPDATE contacts SET {', '.join(updates)} WHERE id = ?", params)
            conn.commit()

        return True


def delete_contact(contact_id: int, db_path: Path | None = None) -> bool:
    """Deletes a contact by ID. Returns True if row was deleted."""
    with closing(get_db(db_path)) as conn:
        cursor = conn.cursor()
        cursor.execute("DELETE FROM contacts WHERE id = ?", (contact_id,))
        conn.commit()
        return cursor.rowcount > 0
