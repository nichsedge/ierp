package portfolio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/finance"
)

func TestSyncPortfolio(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ierp-portfolio-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "events.db")
	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	portfolioDataDir := filepath.Join(tempDir, "portfolio_data")
	if err := os.MkdirAll(portfolioDataDir, 0755); err != nil {
		t.Fatalf("failed to create portfolio data dir: %v", err)
	}

	// 1. Create a valid mock snapshot payload
	type catAlloc struct {
		Category string  `json:"category"`
		ValueIDR float64 `json:"value_idr"`
	}
	type mockPayload struct {
		Metadata struct {
			Date string `json:"date"`
		} `json:"metadata"`
		Totals struct {
			TotalLiabilitiesIDR float64 `json:"total_liabilities_idr"`
		} `json:"totals"`
		Allocation struct {
			ByCategory []catAlloc `json:"by_category"`
		} `json:"allocation"`
	}

	snap1 := mockPayload{}
	snap1.Metadata.Date = "2026-10-01"
	snap1.Totals.TotalLiabilitiesIDR = 500000
	snap1.Allocation.ByCategory = []catAlloc{
		{Category: "Bank Account", ValueIDR: 10000000},
		{Category: "Stablecoin", ValueIDR: 5000000},
		{Category: "Equities", ValueIDR: 20000000},
		{Category: "Crypto", ValueIDR: 15000000},
	}

	data1, _ := json.MarshalIndent(snap1, "", "  ")
	snap1Path := filepath.Join(portfolioDataDir, "latest_snapshot.json")
	if err := os.WriteFile(snap1Path, data1, 0644); err != nil {
		t.Fatalf("failed to write snap1: %v", err)
	}

	// First sync should succeed and seed baseline commitments
	if err := SyncPortfolio(database, portfolioDataDir, false); err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	latest, err := finance.GetLatestSnapshot(database)
	if err != nil || latest == nil {
		t.Fatalf("failed to get latest snapshot: %v", err)
	}
	if latest.SnapshotDate != "2026-10-01" {
		t.Errorf("expected date 2026-10-01, got %s", latest.SnapshotDate)
	}
	if latest.LiquidCash != 15000000 {
		t.Errorf("expected liquid cash 15,000,000, got %f", latest.LiquidCash)
	}
	if latest.Investments != 35000000 {
		t.Errorf("expected investments 35,000,000, got %f", latest.Investments)
	}

	// Commitments should be seeded
	commitments, err := finance.ListCommitments(database, "active", "")
	if err != nil || len(commitments) == 0 {
		t.Fatalf("expected seeded commitments, got %v", err)
	}

	// 2. Duplicate sync: same date should not create a new record
	if err := SyncPortfolio(database, portfolioDataDir, false); err != nil {
		t.Fatalf("subsequent sync failed: %v", err)
	}
	snaps, err := finance.ListSnapshots(database, 10)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("expected exactly 1 snapshot, got %d", len(snaps))
	}

	// 3. Deviation Guard test: incoming snapshot has 300% liquid cash swing
	snap2 := mockPayload{}
	snap2.Metadata.Date = "2026-10-02"
	snap2.Allocation.ByCategory = []catAlloc{
		{Category: "Bank Account", ValueIDR: 45000000}, // 15M -> 45M (200% swing)
	}
	data2, _ := json.MarshalIndent(snap2, "", "  ")
	if err := os.WriteFile(snap1Path, data2, 0644); err != nil {
		t.Fatalf("failed to write snap2: %v", err)
	}

	// Without force -> should fail
	err = SyncPortfolio(database, portfolioDataDir, false)
	if err == nil {
		t.Fatalf("expected error from deviation guard, got nil")
	}

	// With force -> should succeed
	err = SyncPortfolio(database, portfolioDataDir, true)
	if err != nil {
		t.Fatalf("expected forced sync to succeed, got %v", err)
	}

	latest, err = finance.GetLatestSnapshot(database)
	if err != nil || latest.SnapshotDate != "2026-10-02" {
		t.Fatalf("expected latest date 2026-10-02, got %v", latest)
	}
}
