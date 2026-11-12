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

func TestListEventsExcludeTags(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_events_filter.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// 1. Life event
	_, _, err = events.InsertEvent(database, events.Event{
		Title:     "Dinner with Friends",
		StartDate: "2026-11-01",
		Tags:      []string{"social", "dinner"},
	}, nil)
	if err != nil {
		t.Fatalf("InsertEvent life failed: %v", err)
	}

	// 2. Google Timeline event
	_, _, err = events.InsertEvent(database, events.Event{
		Title:     "Visit to Coffee Shop",
		StartDate: "2026-11-02",
		Tags:      []string{"timeline", "visit", "jakarta"},
	}, nil)
	if err != nil {
		t.Fatalf("InsertEvent timeline failed: %v", err)
	}

	// 3. Atracker deepwork event
	_, _, err = events.InsertEvent(database, events.Event{
		Title:     "Deep Work: Coding",
		StartDate: "2026-11-03",
		Tags:      []string{"deepwork", "focus", "atracker"},
	}, nil)
	if err != nil {
		t.Fatalf("InsertEvent atracker failed: %v", err)
	}

	// All events
	allEvs, totalAll, err := events.ListEvents(database, events.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents all failed: %v", err)
	}
	if totalAll != 3 || len(allEvs) != 3 {
		t.Fatalf("expected 3 total events, got %d", totalAll)
	}

	// Exclude timeline & atracker (Life filter)
	lifeEvs, totalLife, err := events.ListEvents(database, events.ListOptions{
		Limit:       10,
		ExcludeTags: []string{"timeline", "atracker", "deepwork"},
	})
	if err != nil {
		t.Fatalf("ListEvents life filter failed: %v", err)
	}
	if totalLife != 1 || len(lifeEvs) != 1 {
		t.Fatalf("expected 1 life event, got total=%d len=%d", totalLife, len(lifeEvs))
	}
	if lifeEvs[0].Title != "Dinner with Friends" {
		t.Errorf("expected 'Dinner with Friends', got %q", lifeEvs[0].Title)
	}
}
