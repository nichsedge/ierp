"""
Database engine and connection management for iERP.
Enforces SQLite Write-Ahead Logging (WAL) and creates performance indexes.
"""

import sqlite3
from contextlib import contextmanager
from pathlib import Path
from typing import Optional, Generator
from .config import DB_PATH, MEDIA_DIR, C_GREEN, C_RESET


def get_db(db_path: Optional[Path] = None) -> sqlite3.Connection:
    """
    Returns an optimized SQLite database connection with:
      - Write-Ahead Logging (WAL mode) for non-blocking concurrent reads and writes
      - Foreign key enforcement
      - 5000ms busy timeout to prevent 'database is locked' errors under load
    """
    target = db_path or DB_PATH
    conn = sqlite3.connect(str(target), timeout=10.0)
    conn.execute("PRAGMA journal_mode = WAL;")
    conn.execute("PRAGMA busy_timeout = 5000;")
    conn.execute("PRAGMA foreign_keys = ON;")
    return conn


@contextmanager
def db_session(db_path: Optional[Path] = None) -> Generator[sqlite3.Cursor, None, None]:
    """Context manager for transactional database operations."""
    conn = get_db(db_path)
    cursor = conn.cursor()
    try:
        yield cursor
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def init_db(db_path: Optional[Path] = None, verbose: bool = False) -> None:
    """Initializes tables, creates indexes, and performs idempotent migrations."""
    target = db_path or DB_PATH
    MEDIA_DIR.mkdir(parents=True, exist_ok=True)

    conn = get_db(target)
    cursor = conn.cursor()

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS events (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        place TEXT,
        start_date TEXT,
        end_date TEXT,
        raw_date TEXT,
        tags TEXT,
        url TEXT,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS event_media (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
        original_filename TEXT,
        stored_path TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS contacts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        client TEXT,
        date TEXT,
        location TEXT,
        org TEXT,
        notes TEXT,
        email TEXT,
        phone TEXT,
        google_id TEXT,
        source TEXT DEFAULT 'manual',
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS vendors (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        category TEXT,
        location TEXT,
        phone TEXT,
        email TEXT,
        url TEXT,
        notes TEXT,
        favorite INTEGER DEFAULT 0,
        source TEXT DEFAULT 'manual',
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS media_items (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        media_type TEXT NOT NULL,
        title TEXT NOT NULL,
        original_title TEXT,
        year INTEGER,
        author TEXT,
        source TEXT,
        extra_json TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime')),
        UNIQUE(media_type, source, title)
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS media_logs (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        media_item_id INTEGER NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
        status TEXT,
        rating REAL,
        progress TEXT,
        started_at TEXT,
        finished_at TEXT,
        date_logged TEXT,
        review TEXT,
        raw_json TEXT,
        source TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime')),
        UNIQUE(media_item_id, source)
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS links (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        label TEXT NOT NULL,
        url TEXT NOT NULL,
        category TEXT,
        is_public INTEGER DEFAULT 1,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS sync_state (
        key TEXT PRIMARY KEY,
        value TEXT,
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS event_contacts (
        event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
        contact_id INTEGER REFERENCES contacts(id) ON DELETE CASCADE,
        PRIMARY KEY (event_id, contact_id)
    );
    """)

    # Column Migrations
    try:
        cursor.execute("PRAGMA table_info(contacts)")
        cols = [row[1] for row in cursor.fetchall()]
        if "email" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN email TEXT")
        if "phone" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN phone TEXT")
        if "google_id" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN google_id TEXT")
        if "source" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN source TEXT DEFAULT 'manual'")
        cursor.execute("""
        UPDATE contacts
        SET source = CASE
            WHEN google_id IS NOT NULL AND (client IS NOT NULL AND client != '') THEN 'merged'
            WHEN google_id IS NOT NULL THEN 'google'
            ELSE 'manual'
        END
        WHERE source IS NULL OR source = 'manual' AND google_id IS NOT NULL;
        """)
    except Exception:
        pass

    try:
        cursor.execute("PRAGMA table_info(vendors)")
        v_cols = [row[1] for row in cursor.fetchall()]
        if "category" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN category TEXT")
        if "phone" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN phone TEXT")
        if "email" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN email TEXT")
        if "url" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN url TEXT")
        if "notes" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN notes TEXT")
        if "favorite" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN favorite INTEGER DEFAULT 0")
        if "source" not in v_cols:
            cursor.execute("ALTER TABLE vendors ADD COLUMN source TEXT DEFAULT 'manual'")
    except Exception:
        pass

    # Data Migration: Move vendor records erroneously stored in contacts
    try:
        cursor.execute("SELECT id, name, location, notes, source FROM contacts WHERE id = 311 OR name = 'ASA Tours and Travel'")
        rows = cursor.fetchall()
        for cid, cname, cloc, cnotes, csrc in rows:
            cursor.execute("SELECT id FROM vendors WHERE name = ?", (cname,))
            if not cursor.fetchone():
                cursor.execute("""
                INSERT INTO vendors (name, category, location, phone, notes, favorite, source)
                VALUES (?, ?, ?, ?, ?, ?, ?)
                """, (
                    cname,
                    "Motorbike Rental",
                    cloc or "Bali, Indonesia",
                    "+62 851-7300-3683",
                    cnotes or "Favorite motorbike rental vendor/seller in Bali",
                    1,
                    csrc or "manual"
                ))
            cursor.execute("DELETE FROM event_contacts WHERE contact_id = ?", (cid,))
            cursor.execute("DELETE FROM contacts WHERE id = ?", (cid,))
    except Exception:
        pass

    # Performance Indexes
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_events_start_date ON events(start_date);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_events_place ON events(place);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_google_id ON contacts(google_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_source ON contacts(source);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_event_contacts_pair ON event_contacts(event_id, contact_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_vendors_name ON vendors(name);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_vendors_category ON vendors(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_vendors_favorite ON vendors(favorite);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_media_items_type ON media_items(media_type);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_media_items_title ON media_items(title);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_media_items_src ON media_items(source, title);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_media_logs_item ON media_logs(media_item_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_media_logs_date ON media_logs(date_logged);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_links_category ON links(category);")

    conn.commit()
    conn.close()

    if verbose:
        print(f"{C_GREEN}Database and media folder initialized successfully.{C_RESET}")
        print(f"  DB Path: {target}")
        print(f"  Media folder: {MEDIA_DIR}")
