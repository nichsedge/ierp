package events_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/events"
)

func TestEventsCRUDAndFTS5(t *testing.T) {
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

	// Insert contact first
	cID, err := contacts.InsertContact(database, contacts.Contact{
		Name: "Alice Wonderland",
		Org:  "Wonderland Inc",
		Tier: 1,
	})
	if err != nil {
		t.Fatalf("InsertContact failed: %v", err)
	}

	// Insert event linking Alice
	ev := events.Event{
		Title:     "Quarterly Strategy Session",
		Place:     "Jakarta",
		StartDate: "2026-10-01",
		Tags:      []string{"strategy", "planning"},
		Notes:     "Discussed the Go migration and architectural roadmap.",
	}

	evID, linked, err := events.InsertEvent(database, ev, []string{"Alice Wonderland"})
	if err != nil {
		t.Fatalf("InsertEvent failed: %v", err)
	}
	if evID <= 0 {
		t.Errorf("expected valid event ID, got %d", evID)
	}
	if len(linked) != 1 || linked[0] != "Alice Wonderland" {
		t.Errorf("expected Alice Wonderland linked, got %v", linked)
	}

	// Fetch event
	fetched, err := events.GetEvent(database, evID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if fetched == nil || fetched.Title != ev.Title {
		t.Errorf("unexpected fetched event: %+v", fetched)
	}
	if len(fetched.Contacts) != 1 || fetched.Contacts[0].ID != cID {
		t.Errorf("expected 1 linked contact with ID %d, got %+v", cID, fetched.Contacts)
	}

	// FTS5 Full-Text Search
	results, err := events.SearchEvents(database, "migration", 10)
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected FTS5 match for 'migration', got none")
	}
	if results[0].ID != evID {
		t.Errorf("expected event ID %d, got %d", evID, results[0].ID)
	}

	// Delete event
	ok, err := events.DeleteEvent(database, evID)
	if err != nil || !ok {
		t.Errorf("DeleteEvent failed: ok=%v, err=%v", ok, err)
	}

	// Verify FTS trigger removed from FTS5
	afterDelete, err := events.SearchEvents(database, "migration", 10)
	if err != nil {
		t.Fatalf("SearchEvents after delete failed: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Errorf("expected 0 matches after delete, got %d", len(afterDelete))
	}
}
