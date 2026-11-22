package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

// Open returns an optimized SQLite database connection with WAL mode, foreign keys, and busy timeout.
// If dbPath is a remote libSQL/Turso URL (libsql:// or https://), it uses the pure Go libSQL driver.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = config.DBPath()
	}

	if config.IsRemoteDB(dbPath) {
		token := config.TursoAuthToken()
		connector, err := libsql.NewConnector(dbPath, libsql.WithAuthToken(token))
		if err != nil {
			return nil, fmt.Errorf("failed creating libsql connector for %s: %w", dbPath, err)
		}

		database := sql.OpenDB(connector)
		database.SetMaxOpenConns(10)
		database.SetMaxIdleConns(0)
		return database, nil
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA cache_size = -20000;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}

	for _, p := range pragmas {
		if _, err := database.Exec(p); err != nil {
			database.Close()
			return nil, fmt.Errorf("failed executing %s: %w", p, err)
		}
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

    CREATE TABLE IF NOT EXISTS places_cache (
        place TEXT PRIMARY KEY,
        display_name TEXT,
        latitude REAL NOT NULL,
        longitude REAL NOT NULL,
        city TEXT,
        country TEXT,
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

    CREATE TABLE IF NOT EXISTS antivision_items (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        description TEXT NOT NULL,
        domains TEXT NOT NULL,
        status TEXT DEFAULT 'active',
        severity INTEGER DEFAULT 3,
        date_logged TEXT NOT NULL,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    CREATE TABLE IF NOT EXISTS role_explorations (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        role TEXT NOT NULL,
        category TEXT,
        status TEXT DEFAULT 'explored',
        date_logged TEXT NOT NULL,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    CREATE TABLE IF NOT EXISTS skills_inventory (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        skill TEXT NOT NULL,
        skill_type TEXT NOT NULL,
        status TEXT DEFAULT 'active',
        date_logged TEXT NOT NULL,
        notes TEXT,
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    CREATE TABLE IF NOT EXISTS expenses (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        date TEXT NOT NULL,
        title TEXT NOT NULL,
        amount REAL NOT NULL,
        currency TEXT NOT NULL DEFAULT 'IDR',
        category TEXT NOT NULL DEFAULT 'general',
        account TEXT,
        notes TEXT,
        tags TEXT,
        type TEXT NOT NULL DEFAULT 'EXPENSE',
        source TEXT DEFAULT 'manual',
        created_at TEXT DEFAULT (datetime('now', 'localtime')),
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    CREATE TABLE IF NOT EXISTS accounts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL UNIQUE,
        type TEXT NOT NULL DEFAULT 'Bank Account',
        balance REAL NOT NULL DEFAULT 0,
        currency TEXT NOT NULL DEFAULT 'IDR',
        updated_at TEXT DEFAULT (datetime('now', 'localtime'))
    );

    DROP TABLE IF EXISTS receipts;
    `

	if _, err := database.Exec(schema); err != nil {
		return fmt.Errorf("failed executing initial schema: %w", err)
	}

	// Versioned transactional schema migrations
	if err := Migrate(database); err != nil {
		return fmt.Errorf("failed running database migrations: %w", err)
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
    CREATE INDEX IF NOT EXISTS idx_event_contacts_contact ON event_contacts(contact_id);
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
    CREATE INDEX IF NOT EXISTS idx_antivision_status ON antivision_items(status);
    CREATE INDEX IF NOT EXISTS idx_antivision_date ON antivision_items(date_logged);
    CREATE INDEX IF NOT EXISTS idx_role_explorations_status ON role_explorations(status);
    CREATE INDEX IF NOT EXISTS idx_skills_type ON skills_inventory(skill_type);
    CREATE INDEX IF NOT EXISTS idx_expenses_date ON expenses(date);
    CREATE INDEX IF NOT EXISTS idx_expenses_category ON expenses(category);
    CREATE INDEX IF NOT EXISTS idx_expenses_tags ON expenses(tags);
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

type migration struct {
	version     int
	description string
	apply       func(tx *sql.Tx) error
}

var schemaMigrations = []migration{
	{
		version:     1,
		description: "Add contacts CRM communication, source, and tier columns",
		apply: func(tx *sql.Tx) error {
			cols := []struct {
				name string
				def  string
			}{
				{"email", "TEXT"},
				{"phone", "TEXT"},
				{"google_id", "TEXT"},
				{"source", "TEXT DEFAULT 'manual'"},
				{"tier", "INTEGER DEFAULT 0"},
				{"cadence_days", "INTEGER DEFAULT 0"},
			}
			for _, c := range cols {
				if err := addColumnIfNotExists(tx, "contacts", c.name, c.def); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     2,
		description: "Add events project association foreign key",
		apply: func(tx *sql.Tx) error {
			return addColumnIfNotExists(tx, "events", "project_id", "INTEGER REFERENCES projects(id) ON DELETE SET NULL")
		},
	},
	{
		version:     3,
		description: "Add vendors profile, categorization, and metadata columns",
		apply: func(tx *sql.Tx) error {
			cols := []struct {
				name string
				def  string
			}{
				{"category", "TEXT"},
				{"phone", "TEXT"},
				{"email", "TEXT"},
				{"url", "TEXT"},
				{"notes", "TEXT"},
				{"favorite", "INTEGER DEFAULT 0"},
				{"source", "TEXT DEFAULT 'manual'"},
			}
			for _, c := range cols {
				if err := addColumnIfNotExists(tx, "vendors", c.name, c.def); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		version:     4,
		description: "Add expenses transaction classification type column",
		apply: func(tx *sql.Tx) error {
			return addColumnIfNotExists(tx, "expenses", "type", "TEXT NOT NULL DEFAULT 'EXPENSE'")
		},
	},
}

// Migrate applies all unapplied versioned schema migrations atomically in transactions.
// If any migration fails, it aborts immediately, rolls back the transaction, and returns the error.
func Migrate(database *sql.DB) error {
	createVersionTable := `
    CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        description TEXT NOT NULL,
        applied_at TEXT DEFAULT (datetime('now', 'localtime'))
    );`
	if _, err := database.Exec(createVersionTable); err != nil {
		return fmt.Errorf("failed creating schema_migrations table: %w", err)
	}

	rows, err := database.Query("SELECT version FROM schema_migrations ORDER BY version ASC;")
	if err != nil {
		return fmt.Errorf("failed querying schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("failed scanning migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed reading migration versions: %w", err)
	}

	for _, m := range schemaMigrations {
		if applied[m.version] {
			continue
		}

		tx, err := database.Begin()
		if err != nil {
			return fmt.Errorf("failed beginning transaction for migration %d (%s): %w", m.version, m.description, err)
		}

		if err := m.apply(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s) failed: %w", m.version, m.description, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, description) VALUES (?, ?);", m.version, m.description); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed recording migration %d (%s): %w", m.version, m.description, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed committing transaction for migration %d (%s): %w", m.version, m.description, err)
		}
	}

	return nil
}

// CurrentSchemaVersion returns the highest applied schema migration version.
func CurrentSchemaVersion(database *sql.DB) (int, error) {
	var version int
	err := database.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations;").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func getTxTableColumns(tx *sql.Tx, tableName string) (map[string]bool, error) {
	rows, err := tx.Query(fmt.Sprintf("PRAGMA table_info(%s);", tableName))
	if err != nil {
		return nil, fmt.Errorf("failed getting table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("failed scanning column info for %s: %w", tableName, err)
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

func addColumnIfNotExists(tx *sql.Tx, tableName, columnName, columnDef string) error {
	cols, err := getTxTableColumns(tx, tableName)
	if err != nil {
		return err
	}
	if cols[strings.ToLower(columnName)] {
		return nil
	}
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", tableName, columnName, columnDef)
	if _, err := tx.Exec(query); err != nil {
		return fmt.Errorf("failed to add column %s to %s: %w", columnName, tableName, err)
	}
	return nil
}
