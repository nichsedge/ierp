package lifeops_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/lifeops"
)

func TestLifeOpsAndAutoReschedule(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_lifeops.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	interval := 90
	id, err := lifeops.InsertMaintenance(database, lifeops.MaintenanceItem{
		Name:         "Motorbike Oil Change",
		DueDate:      "2026-10-01",
		Category:     "vehicle",
		IntervalDays: &interval,
		Cost:         150000,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertMaintenance failed: id=%d, err=%v", id, err)
	}

	// Complete task and verify auto-rescheduling
	cost := 160000.0
	res, err := lifeops.CompleteMaintenance(database, id, "2026-10-01", &cost)
	if err != nil {
		t.Fatalf("CompleteMaintenance failed: %v", err)
	}
	if success, _ := res["success"].(bool); !success {
		t.Fatalf("expected success, got %v", res)
	}
	if rescheduled, _ := res["rescheduled"].(bool); !rescheduled {
		t.Errorf("expected auto-reschedule to be true, got %v", rescheduled)
	}
	nextDue, _ := res["next_due_date"].(string)
	if nextDue != "2026-12-30" {
		t.Errorf("expected next due date 2026-12-30, got %s", nextDue)
	}

	// Verify items count: 1 completed, 1 pending
	items, err := lifeops.ListMaintenance(database, "all", "", false)
	if err != nil {
		t.Fatalf("ListMaintenance failed: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}
