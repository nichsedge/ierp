package antivision_test

import (
	"database/sql"
	"testing"

	"github.com/nichsedge/ierp/internal/antivision"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	schema := `
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
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return db
}

func TestAntiVisionLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Insert
	item := antivision.AntiVisionItem{
		Description: "scroll social media impulsively",
		Domains:     "behaviour",
		Severity:    4,
		DateLogged:  "2026-11-08",
		Notes:       "Drains morning focus",
	}
	id, err := antivision.InsertAntiVision(db, item)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive id, got %d", id)
	}

	// 2. List
	items, err := antivision.ListAntiVision(db, "behaviour", "active")
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Description != "scroll social media impulsively" {
		t.Errorf("unexpected description: %s", items[0].Description)
	}

	// 3. Update
	sev := 5
	ok, err := antivision.UpdateAntiVision(db, id, "mitigated", &sev, "Installed app blockers")
	if err != nil || !ok {
		t.Fatalf("failed to update: ok=%v, err=%v", ok, err)
	}

	itemsUpdated, err := antivision.ListAntiVision(db, "", "mitigated")
	if err != nil || len(itemsUpdated) != 1 {
		t.Fatalf("expected 1 mitigated item, got %d", len(itemsUpdated))
	}
	if itemsUpdated[0].Severity != 5 {
		t.Errorf("expected severity 5, got %d", itemsUpdated[0].Severity)
	}

	// 4. Delete
	deleted, err := antivision.DeleteAntiVision(db, id)
	if err != nil || !deleted {
		t.Fatalf("failed to delete: deleted=%v, err=%v", deleted, err)
	}
}
