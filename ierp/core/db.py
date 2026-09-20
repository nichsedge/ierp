"""
Database engine and connection management for iERP.
Enforces SQLite Write-Ahead Logging (WAL) and creates performance indexes.
"""

from collections.abc import Generator
from contextlib import contextmanager
from pathlib import Path
import sqlite3

from . import config
from .config import C_GREEN, C_RESET


def get_db(db_path: Path | None = None) -> sqlite3.Connection:
    """
    Returns an optimized SQLite database connection with:
      - Write-Ahead Logging (WAL mode) for non-blocking concurrent reads and writes
      - Foreign key enforcement
      - 5000ms busy timeout to prevent 'database is locked' errors under load
    """
    target = db_path or config.DB_PATH
    conn = sqlite3.connect(str(target), timeout=10.0)
    conn.execute("PRAGMA journal_mode = WAL;")
    conn.execute("PRAGMA busy_timeout = 5000;")
    conn.execute("PRAGMA foreign_keys = ON;")
    return conn


@contextmanager
def db_session(db_path: Path | None = None) -> Generator[sqlite3.Cursor, None, None]:
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


def init_db(db_path: Path | None = None, verbose: bool = False) -> None:
    """Initializes tables, creates indexes, and performs idempotent migrations."""
    target = db_path or config.DB_PATH
    config.MEDIA_DIR.mkdir(parents=True, exist_ok=True)

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
        source TEXT,
        data_json TEXT,
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
    CREATE TABLE IF NOT EXISTS payment_accounts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE,
        name TEXT NOT NULL,
        category TEXT,
        number TEXT,
        recipient TEXT,
        details TEXT,
        details_id TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    cursor.execute("""
    CREATE TABLE IF NOT EXISTS referrals (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE,
        name TEXT NOT NULL,
        category TEXT,
        code TEXT,
        link TEXT,
        benefit TEXT,
        status TEXT DEFAULT 'ACTIVE',
        is_public INTEGER DEFAULT 1,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
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
        if "tier" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN tier INTEGER DEFAULT 0")
        if "cadence_days" not in cols:
            cursor.execute("ALTER TABLE contacts ADD COLUMN cadence_days INTEGER DEFAULT 0")
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
        cursor.execute("PRAGMA table_info(events)")
        e_cols = [row[1] for row in cursor.fetchall()]
        if "project_id" not in e_cols:
            cursor.execute("ALTER TABLE events ADD COLUMN project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL")
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
        vendor_entries = [
            ("ASA Tours and Travel", "Motorbike Rental", "Bali, Indonesia", "+62 851-7300-3683", "Favorite motorbike rental in Bali"),
            ("Nasgor Sekayu", "Culinary", "Jakarta, Indonesia", None, None),
            ("Nasgor R3 Gambiran", "Culinary", "Yogyakarta, Indonesia", None, None),
            ("Warteg Gambiran", "Culinary", "Yogyakarta, Indonesia", None, None),
            ("Warung Maya Pancasila", "Culinary", "Tasikmalaya, Indonesia", None, None),
            ("Warung Yulex", "Culinary", "Indonesia", None, None),
            ("Laundry Jagalan", "Services", "Yogyakarta, Indonesia", None, None),
            ("IT Servicedesk Telkomsel", "IT Support", "Indonesia", None, None),
            ("Kost Benhil", "Housing", "Jakarta, Indonesia", None, None),
            ("Klinik UI", "Healthcare", "Depok, Indonesia", None, None),
            ("Pandawa Bpjs", "Services", "Indonesia", None, None),
            ("Grab Supoort", "Transport", "Indonesia", None, None),
            ("Cod Rothko", "Commerce", "Indonesia", None, None),
            ("Data Transaksi", "Utility", "Indonesia", None, None),
            ("Beli", "Utility", "Indonesia", None, None),
            ("Service", "Utility", "Indonesia", None, None),
        ]
        for vname, vcat, vloc, vphone, vnotes in vendor_entries:
            cursor.execute("SELECT id, name, location, notes, source, phone, email FROM contacts WHERE LOWER(name) = LOWER(?)", (vname,))
            rows = cursor.fetchall()
            for cid, cname, cloc, cnotes, csrc, cphone, cemail in rows:
                cursor.execute("SELECT id FROM vendors WHERE LOWER(name) = LOWER(?)", (cname,))
                if not cursor.fetchone():
                    cursor.execute("""
                    INSERT INTO vendors (name, category, location, phone, email, notes, favorite, source)
                    VALUES (?, ?, ?, ?, ?, ?, 0, ?)
                    """, (cname, vcat, cloc or vloc, cphone or vphone, cemail, cnotes or vnotes, csrc or "migrated"))
                cursor.execute("DELETE FROM event_contacts WHERE contact_id = ?", (cid,))
                cursor.execute("DELETE FROM contacts WHERE id = ?", (cid,))

        # Data Migration: Demote unclassified contacts without events from Tier 3 to Tier 0 (Untracked Directory)
        cursor.execute("""
        UPDATE contacts
        SET tier = 0, cadence_days = 0
        WHERE tier = 3
          AND id NOT IN (SELECT DISTINCT contact_id FROM event_contacts)
          AND id NOT IN (1, 8, 11, 14);
        """)

        # Data Migration: Set proper 180-day cadence for remaining Tier 3 contacts with logged interactions
        cursor.execute("""
        UPDATE contacts
        SET cadence_days = 180
        WHERE tier = 3 AND (cadence_days IS NULL OR cadence_days = 60);
        """)
    except Exception:
        pass

    # Projects / Strategic Initiatives
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS projects (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE NOT NULL,
        title TEXT NOT NULL,
        description TEXT,
        status TEXT DEFAULT 'active',
        priority TEXT DEFAULT 'medium',
        start_date TEXT,
        target_date TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Decision Journal
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS decisions (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        context TEXT,
        choice TEXT NOT NULL,
        expected_outcome TEXT,
        confidence INTEGER DEFAULT 7,
        review_date TEXT,
        actual_outcome TEXT,
        status TEXT DEFAULT 'pending',
        project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Net Worth Snapshots
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS networth_snapshots (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        snapshot_date TEXT NOT NULL,
        liquid_cash REAL DEFAULT 0,
        investments REAL DEFAULT 0,
        hard_assets REAL DEFAULT 0,
        liabilities REAL DEFAULT 0,
        currency TEXT DEFAULT 'IDR',
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Recurring Commitments / Fixed Burn Rate
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS recurring_commitments (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        category TEXT DEFAULT 'saas',
        amount REAL NOT NULL,
        currency TEXT DEFAULT 'IDR',
        frequency TEXT DEFAULT 'monthly',
        payment_account_id INTEGER REFERENCES payment_accounts(id) ON DELETE SET NULL,
        status TEXT DEFAULT 'active',
        renewal_date TEXT,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Life Ops & Preventive Maintenance
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS maintenance_items (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        category TEXT DEFAULT 'general',
        due_date TEXT NOT NULL,
        interval_days INTEGER,
        status TEXT DEFAULT 'pending',
        cost REAL DEFAULT 0,
        notes TEXT,
        gadget_id INTEGER REFERENCES gadgets(id) ON DELETE SET NULL,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Sprint Retrospectives / Double-Loop Learning
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS retrospectives (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        period_start TEXT NOT NULL,
        period_end TEXT NOT NULL,
        period_type TEXT DEFAULT 'monthly',
        wins TEXT,
        drains_burnout TEXT,
        lessons TEXT,
        focus_next TEXT,
        rating INTEGER DEFAULT 7,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)

    # Performance Indexes
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_events_start_date ON events(start_date);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_events_place ON events(place);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_google_id ON contacts(google_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_source ON contacts(source);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_contacts_tier ON contacts(tier);")
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
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_payment_accounts_category ON payment_accounts(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_referrals_category ON referrals(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_decisions_status ON decisions(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_decisions_review_date ON decisions(review_date);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_decisions_project ON decisions(project_id);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_snapshots_date ON networth_snapshots(snapshot_date);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_commitments_status ON recurring_commitments(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_commitments_cat ON recurring_commitments(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_maintenance_due ON maintenance_items(due_date);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_maintenance_status ON maintenance_items(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_retrospectives_start ON retrospectives(period_start);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_retrospectives_type ON retrospectives(period_type);")

    # Legacy migration: drop receipts table if exists (Sans Finance is SSOT)
    cursor.execute("DROP TABLE IF EXISTS receipts;")

    # Gadgets / Hardware Assets
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS gadgets (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        slug TEXT UNIQUE NOT NULL,
        name TEXT NOT NULL,
        brand TEXT,
        model TEXT,
        category TEXT,
        status TEXT DEFAULT 'active',
        purchase_date TEXT,
        purchase_price REAL,
        currency TEXT DEFAULT 'IDR',
        specs_json TEXT,
        serial_number TEXT,
        vendor_id INTEGER REFERENCES vendors(id) ON DELETE SET NULL,
        event_id INTEGER REFERENCES events(id) ON DELETE SET NULL,
        notes TEXT,
        is_public INTEGER DEFAULT 1,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );
    """)
    # Migration: Drop legacy receipt_id column from gadgets if present
    cols = [r[1] for r in cursor.execute("PRAGMA table_info(gadgets)").fetchall()]
    if "receipt_id" in cols:
        try:
            cursor.execute("ALTER TABLE gadgets DROP COLUMN receipt_id;")
        except Exception:
            pass

    cursor.execute("CREATE INDEX IF NOT EXISTS idx_gadgets_name ON gadgets(name);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_gadgets_brand ON gadgets(brand);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_gadgets_status ON gadgets(status);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_gadgets_category ON gadgets(category);")
    cursor.execute("CREATE INDEX IF NOT EXISTS idx_gadgets_slug ON gadgets(slug);")

    # SQLite FTS5 Full-Text Search Virtual Table & Real-Time Sync Triggers
    cursor.execute("""
    CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
        title,
        place,
        notes,
        tags,
        content='events',
        content_rowid='id'
    );
    """)

    cursor.execute("""
    CREATE TRIGGER IF NOT EXISTS events_ai AFTER INSERT ON events BEGIN
        INSERT INTO events_fts(rowid, title, place, notes, tags)
        VALUES (new.id, new.title, new.place, new.notes, new.tags);
    END;
    """)

    cursor.execute("""
    CREATE TRIGGER IF NOT EXISTS events_ad AFTER DELETE ON events BEGIN
        INSERT INTO events_fts(events_fts, rowid, title, place, notes, tags)
        VALUES ('delete', old.id, old.title, old.place, old.notes, old.tags);
    END;
    """)

    cursor.execute("""
    CREATE TRIGGER IF NOT EXISTS events_au AFTER UPDATE ON events BEGIN
        INSERT INTO events_fts(events_fts, rowid, title, place, notes, tags)
        VALUES ('delete', old.id, old.title, old.place, old.notes, old.tags);
        INSERT INTO events_fts(rowid, title, place, notes, tags)
        VALUES (new.id, new.title, new.place, new.notes, new.tags);
    END;
    """)

    # Populate/Rebuild FTS index idempotently
    try:
        cursor.execute("INSERT INTO events_fts(events_fts) VALUES('rebuild');")
    except Exception:
        pass

    conn.commit()
    conn.close()

    if verbose:
        print(f"{C_GREEN}Database and media folder initialized successfully.{C_RESET}")
        print(f"  DB Path: {target}")
        print(f"  Media folder: {config.MEDIA_DIR}")
