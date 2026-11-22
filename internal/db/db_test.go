package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
)

func TestInitDBAndWAL(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_events.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	var journalMode string
	if err := database.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("QueryRow journal_mode failed: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode wal, got %s", journalMode)
	}

	// Verify tables exist
	tables := []string{
		"events", "event_media", "contacts", "vendors", "media_items",
		"media_logs", "links", "payment_accounts", "referrals", "event_contacts",
		"projects", "decisions", "networth_snapshots", "recurring_commitments",
		"maintenance_items", "retrospectives", "gadgets", "github_repositories",
	}

	for _, tbl := range tables {
		var name string
		row := database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %s was not created: %v", tbl, err)
		}
	}

	// Verify FTS5 virtual table
	var ftsName string
	row := database.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='events_fts'")
	if err := row.Scan(&ftsName); err != nil {
		t.Errorf("events_fts virtual table was not created: %v", err)
	}

	// Verify idempotent re-run
	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("Second InitDB failed: %v", err)
	}

	_ = os.Remove(dbPath)
}

func TestVersionedMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_migrations.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// 1. CurrentSchemaVersion should be 4
	ver, err := db.CurrentSchemaVersion(database)
	if err != nil {
		t.Fatalf("CurrentSchemaVersion failed: %v", err)
	}
	if ver != 4 {
		t.Fatalf("expected schema version 4, got %d", ver)
	}

	// 2. Query schema_migrations rows
	rows, err := database.Query("SELECT version, description FROM schema_migrations ORDER BY version ASC;")
	if err != nil {
		t.Fatalf("Query schema_migrations failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var v int
		var desc string
		if err := rows.Scan(&v, &desc); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		count++
		if v != count {
			t.Errorf("expected version %d, got %d", count, v)
		}
		if desc == "" {
			t.Errorf("expected non-empty description for version %d", v)
		}
	}
	if count != 4 {
		t.Errorf("expected 4 applied migrations, found %d", count)
	}

	// 3. Re-running Migrate should be idempotent and succeed
	if err := db.Migrate(database); err != nil {
		t.Fatalf("re-running Migrate failed: %v", err)
	}
}
