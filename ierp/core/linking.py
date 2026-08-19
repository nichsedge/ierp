"""
Relationship resolution engine.
Discovers and links mentions of CRM contacts across journal event texts and location history.
"""

import json
import re
import sqlite3
from typing import Optional
from .config import DB_PATH, C_BOLD, C_GREEN, C_YELLOW, C_RED, C_RESET
from .db import get_db, init_db


def link_events_and_contacts(conn: Optional[sqlite3.Connection] = None) -> int:
    """
    Scans events for mentions of contact names (whole word matching)
    and populates the event_contacts junction table.
    """
    close_at_end = False
    if conn is None:
        init_db()
        conn = get_db()
        close_at_end = True

    cursor = conn.cursor()
    cursor.execute("SELECT id, name FROM contacts WHERE name IS NOT NULL AND length(name) >= 3")
    contacts = cursor.fetchall()

    cursor.execute("SELECT id, title, place, tags, notes FROM events")
    events = cursor.fetchall()

    links_to_insert = []
    for c_id, name in contacts:
        escaped_name = re.escape(name.strip())
        pattern = re.compile(rf"\b{escaped_name}\b", re.IGNORECASE)

        for e_id, title, place, tags_json, notes in events:
            text_to_search = f"{title or ''} {place or ''} {notes or ''}"
            try:
                tags = json.loads(tags_json) if tags_json else []
                text_to_search += " " + " ".join(tags)
            except Exception:
                pass

            if pattern.search(text_to_search):
                links_to_insert.append((e_id, c_id))

    cursor.executemany("""
    INSERT OR IGNORE INTO event_contacts (event_id, contact_id)
    VALUES (?, ?)
    """, links_to_insert)

    conn.commit()
    inserted_count = len(links_to_insert)

    if close_at_end:
        conn.close()

    print(f"{C_GREEN}Linked {inserted_count} event-contact pairs in junction table.{C_RESET}")
    return inserted_count


def run_manual_link() -> None:
    """CLI trigger for manual linking execution."""
    if not DB_PATH.exists():
        print(f"{C_RED}Database does not exist.{C_RESET}")
        return
    conn = get_db()
    link_events_and_contacts(conn)
    conn.close()
