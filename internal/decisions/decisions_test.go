package decisions_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/decisions"
)

func TestDecisionsCRUDAndReview(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_decisions.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	id, err := decisions.InsertDecision(database, decisions.Decision{
		Title:           "Choose modernc.org/sqlite over mattn/go-sqlite3",
		Choice:          "modernc pure Go driver",
		Context:         "Avoid cgo and gcc toolchain dependencies",
		ExpectedOutcome: "Zero-cgo portable compilation with full WAL and FTS5 support",
		Confidence:      9,
		ReviewDate:      "2026-10-01", // in past to test overdue alerts
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertDecision failed: id=%d, err=%v", id, err)
	}

	// Verify alert detects it
	overdue, _, err := decisions.GetDecisionAlerts(database, 14)
	if err != nil {
		t.Fatalf("GetDecisionAlerts failed: %v", err)
	}
	if len(overdue) != 1 || overdue[0].ID != id {
		t.Errorf("expected 1 overdue decision with ID %d, got %+v", id, overdue)
	}

	// Review decision
	ok, err := decisions.ReviewDecision(database, id, "Pure Go modernc compiles in milliseconds and passes all FTS5 tests without cgo.", "reviewed")
	if err != nil || !ok {
		t.Fatalf("ReviewDecision failed: ok=%v, err=%v", ok, err)
	}

	d, err := decisions.GetDecision(database, id)
	if err != nil || d == nil {
		t.Fatalf("GetDecision failed: %+v, err=%v", d, err)
	}
	if d.Status != "reviewed" || d.ActualOutcome == "" {
		t.Errorf("unexpected decision state: %+v", d)
	}
}
