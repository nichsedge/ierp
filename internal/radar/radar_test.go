package radar_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/events"
	"github.com/nichsedge/ierp/internal/radar"
)

func TestRadarAndDailyReconnection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_radar.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Insert Tier 1 contact (14 days cadence)
	cID, err := contacts.InsertContact(database, contacts.Contact{
		Name: "Darmawan",
		Tier: 1,
	})
	if err != nil {
		t.Fatalf("InsertContact failed: %v", err)
	}

	// Insert an event from 30 days ago
	oldDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	_, _, err = events.InsertEvent(database, events.Event{
		Title:     "Coffee catchup",
		StartDate: oldDate,
	}, []string{"Darmawan"})
	if err != nil {
		t.Fatalf("InsertEvent failed: %v", err)
	}

	// Compute radar
	items, err := radar.ComputeRadar(database, nil, true, 10, 0)
	if err != nil {
		t.Fatalf("ComputeRadar failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected overdue contact in radar, got 0")
	}

	found := items[0]
	if found.ID != cID || !found.IsOverdue {
		t.Errorf("expected contact %d to be overdue, got %+v", cID, found)
	}
	if found.DaysOverdue < 15 {
		t.Errorf("expected at least 15 days overdue, got %d", found.DaysOverdue)
	}

	// Daily Reconnection
	daily, err := radar.GetDailyReconnection(database)
	if err != nil || daily == nil {
		t.Fatalf("GetDailyReconnection failed: %+v, err=%v", daily, err)
	}
	if daily.ID != cID {
		t.Errorf("expected top priority ID %d, got %d", cID, daily.ID)
	}
}
