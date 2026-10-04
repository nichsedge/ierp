package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/nichsedge/ierp/internal/config"
	_ "modernc.org/sqlite"
)

// Open returns an optimized SQLite database connection with WAL mode, foreign keys, and busy timeout.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = config.DBPath()
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	if _, err := database.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to set WAL journal mode: %w", err)
	}
	if _, err := database.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to set busy_timeout: %w", err)
	}
	if _, err := database.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to enable foreign_keys: %w", err)
	}

	database.SetMaxOpenConns(1)

	return database, nil
}

// InitDB initializes tables, performance indexes, and SQLite FTS5 triggers.
func InitDB(dbPath string, verbose bool) error {
	if dbPath == "" {
		dbPath = config.DBPath()
	}

	if err := os.MkdirAll(config.MediaDir(), 0755); err != nil {
		return fmt.Errorf("failed to create media directory: %w", err)
	}

	database, err := Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	schema := `
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

    CREATE TABLE IF NOT EXISTS event_media (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
        original_filename TEXT,
        stored_path TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

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

    CREATE TABLE IF NOT EXISTS links (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        label TEXT NOT NULL,
        url TEXT NOT NULL,
        category TEXT,
        is_public INTEGER DEFAULT 1,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    CREATE TABLE IF NOT EXISTS sync_state (
        key TEXT PRIMARY KEY,
        value TEXT,
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

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

    CREATE TABLE IF NOT EXISTS event_contacts (
        event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
        contact_id INTEGER REFERENCES contacts(id) ON DELETE CASCADE,
        PRIMARY KEY (event_id, contact_id)
    );

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

    CREATE TABLE IF NOT EXISTS github_repositories (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        repo_id TEXT UNIQUE NOT NULL,
        name TEXT NOT NULL,
        full_name TEXT UNIQUE NOT NULL,
        owner_login TEXT,
        owner_url TEXT,
        html_url TEXT,
        homepage TEXT,
        description TEXT,
        topics TEXT,
        language TEXT,
        private INTEGER DEFAULT 0,
        fork INTEGER DEFAULT 0,
        archived INTEGER DEFAULT 0,
        template INTEGER DEFAULT 0,
        disabled INTEGER DEFAULT 0,
        created_at TEXT,
        updated_at TEXT,
        pushed_at TEXT,
        default_branch TEXT,
        default_branch_oid TEXT,
        stargazers_count INTEGER DEFAULT 0,
        watchers_count INTEGER DEFAULT 0,
        forks_count INTEGER DEFAULT 0,
        open_issues_count INTEGER DEFAULT 0,
        open_prs_count INTEGER DEFAULT 0,
        license_spdx TEXT,
        license_name TEXT,
        synced_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    DROP TABLE IF EXISTS receipts;
    `

	if _, err := database.Exec(schema); err != nil {
		return fmt.Errorf("failed executing initial schema: %w", err)
	}

	// Idempotent column migrations
	if err := migrateColumns(database); err != nil {
		return fmt.Errorf("failed running column migrations: %w", err)
	}

	indexes := `
    CREATE INDEX IF NOT EXISTS idx_events_start_date ON events(start_date);
    CREATE INDEX IF NOT EXISTS idx_events_place ON events(place);
    CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_id);
    CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);
    CREATE INDEX IF NOT EXISTS idx_contacts_email ON contacts(email);
    CREATE INDEX IF NOT EXISTS idx_contacts_google_id ON contacts(google_id);
    CREATE INDEX IF NOT EXISTS idx_contacts_source ON contacts(source);
    CREATE INDEX IF NOT EXISTS idx_contacts_tier ON contacts(tier);
    CREATE INDEX IF NOT EXISTS idx_event_contacts_pair ON event_contacts(event_id, contact_id);
    CREATE INDEX IF NOT EXISTS idx_vendors_name ON vendors(name);
    CREATE INDEX IF NOT EXISTS idx_vendors_category ON vendors(category);
    CREATE INDEX IF NOT EXISTS idx_vendors_favorite ON vendors(favorite);
    CREATE INDEX IF NOT EXISTS idx_media_items_type ON media_items(media_type);
    CREATE INDEX IF NOT EXISTS idx_media_items_title ON media_items(title);
    CREATE INDEX IF NOT EXISTS idx_media_items_src ON media_items(source, title);
    CREATE INDEX IF NOT EXISTS idx_media_logs_item ON media_logs(media_item_id);
    CREATE INDEX IF NOT EXISTS idx_media_logs_date ON media_logs(date_logged);
    CREATE INDEX IF NOT EXISTS idx_links_category ON links(category);
    CREATE INDEX IF NOT EXISTS idx_payment_accounts_category ON payment_accounts(category);
    CREATE INDEX IF NOT EXISTS idx_referrals_category ON referrals(category);
    CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals(status);
    CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);
    CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug);
    CREATE INDEX IF NOT EXISTS idx_decisions_status ON decisions(status);
    CREATE INDEX IF NOT EXISTS idx_decisions_review_date ON decisions(review_date);
    CREATE INDEX IF NOT EXISTS idx_decisions_project ON decisions(project_id);
    CREATE INDEX IF NOT EXISTS idx_snapshots_date ON networth_snapshots(snapshot_date);
    CREATE INDEX IF NOT EXISTS idx_commitments_status ON recurring_commitments(status);
    CREATE INDEX IF NOT EXISTS idx_commitments_cat ON recurring_commitments(category);
    CREATE INDEX IF NOT EXISTS idx_maintenance_due ON maintenance_items(due_date);
    CREATE INDEX IF NOT EXISTS idx_maintenance_status ON maintenance_items(status);
    CREATE INDEX IF NOT EXISTS idx_retrospectives_start ON retrospectives(period_start);
    CREATE INDEX IF NOT EXISTS idx_retrospectives_type ON retrospectives(period_type);
    CREATE INDEX IF NOT EXISTS idx_gadgets_name ON gadgets(name);
    CREATE INDEX IF NOT EXISTS idx_gadgets_brand ON gadgets(brand);
    CREATE INDEX IF NOT EXISTS idx_gadgets_status ON gadgets(status);
    CREATE INDEX IF NOT EXISTS idx_gadgets_category ON gadgets(category);
    CREATE INDEX IF NOT EXISTS idx_gadgets_slug ON gadgets(slug);
    CREATE INDEX IF NOT EXISTS idx_gh_repos_name ON github_repositories(name);
    CREATE INDEX IF NOT EXISTS idx_gh_repos_full_name ON github_repositories(full_name);
    CREATE INDEX IF NOT EXISTS idx_gh_repos_updated_at ON github_repositories(updated_at);
    `
	if _, err := database.Exec(indexes); err != nil {
		return fmt.Errorf("failed creating indexes: %w", err)
	}

	ftsSchema := `
    CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
        title,
        place,
        notes,
        tags,
        content='events',
        content_rowid='id'
    );

    CREATE TRIGGER IF NOT EXISTS events_ai AFTER INSERT ON events BEGIN
        INSERT INTO events_fts(rowid, title, place, notes, tags)
        VALUES (new.id, new.title, new.place, new.notes, new.tags);
    END;

    CREATE TRIGGER IF NOT EXISTS events_ad AFTER DELETE ON events BEGIN
        INSERT INTO events_fts(events_fts, rowid, title, place, notes, tags)
        VALUES ('delete', old.id, old.title, old.place, old.notes, old.tags);
    END;

    CREATE TRIGGER IF NOT EXISTS events_au AFTER UPDATE ON events BEGIN
        INSERT INTO events_fts(events_fts, rowid, title, place, notes, tags)
        VALUES ('delete', old.id, old.title, old.place, old.notes, old.tags);
        INSERT INTO events_fts(rowid, title, place, notes, tags)
        VALUES (new.id, new.title, new.place, new.notes, new.tags);
    END;
    `
	if _, err := database.Exec(ftsSchema); err != nil {
		return fmt.Errorf("failed setting up FTS5 tables and triggers: %w", err)
	}

	_, _ = database.Exec("INSERT INTO events_fts(events_fts) VALUES('rebuild');")

	if verbose {
		fmt.Printf("%sDatabase and media folder initialized successfully.%s\n", config.Green, config.Reset)
		fmt.Printf("  DB Path: %s\n", dbPath)
		fmt.Printf("  Media folder: %s\n", config.MediaDir())
	}

	return nil
}

func migrateColumns(database *sql.DB) error {
	// Contacts columns
	contactsCols, err := getTableColumns(database, "contacts")
	if err == nil {
		if !contactsCols["email"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN email TEXT;")
		}
		if !contactsCols["phone"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN phone TEXT;")
		}
		if !contactsCols["google_id"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN google_id TEXT;")
		}
		if !contactsCols["source"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN source TEXT DEFAULT 'manual';")
		}
		if !contactsCols["tier"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN tier INTEGER DEFAULT 0;")
		}
		if !contactsCols["cadence_days"] {
			_, _ = database.Exec("ALTER TABLE contacts ADD COLUMN cadence_days INTEGER DEFAULT 0;")
		}
	}

	// Events columns
	eventsCols, err := getTableColumns(database, "events")
	if err == nil {
		if !eventsCols["project_id"] {
			_, _ = database.Exec("ALTER TABLE events ADD COLUMN project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL;")
		}
	}

	// Vendors columns
	vendorsCols, err := getTableColumns(database, "vendors")
	if err == nil {
		if !vendorsCols["category"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN category TEXT;")
		}
		if !vendorsCols["phone"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN phone TEXT;")
		}
		if !vendorsCols["email"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN email TEXT;")
		}
		if !vendorsCols["url"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN url TEXT;")
		}
		if !vendorsCols["notes"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN notes TEXT;")
		}
		if !vendorsCols["favorite"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN favorite INTEGER DEFAULT 0;")
		}
		if !vendorsCols["source"] {
			_, _ = database.Exec("ALTER TABLE vendors ADD COLUMN source TEXT DEFAULT 'manual';")
		}
	}

	return nil
}

func getTableColumns(database *sql.DB, tableName string) (map[string]bool, error) {
	rows, err := database.Query(fmt.Sprintf("PRAGMA table_info(%s);", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			cols[strings.ToLower(name)] = true
		}
	}
	return cols, nil
}
