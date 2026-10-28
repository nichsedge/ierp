package db

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestOpenDB_And_InitDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_portfolio.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Verify table existence
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('snapshots', 'holdings', 'categories', 'asset_classes', 'ai_states')").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 tables created, found %d", count)
	}
}

func TestUpsertSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_portfolio.db")

	database, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	valIDR := 15000000.0
	valUSD := 1000.0
	yieldRate := 0.065
	allocPct := 100.0

	snapshot := &models.Snapshot{
		Metadata: models.Metadata{
			Date:         "2026-10-13",
			ExchangeRate: 15000.0,
			TotalItems:   1,
			GeneratedAt:  "2026-10-13T10:00:00Z",
		},
		Totals: models.Totals{
			NetWorthIDR:         valIDR,
			NetWorthUSD:         valUSD,
			TotalAssetsIDR:      valIDR,
			TotalLiabilitiesIDR: 0,
			InvestmentsIDR:      valIDR,
			InvestmentsUSD:      valUSD,
		},
		Allocation: models.AllocationBreakdown{
			ByCategory: []models.CategoryAllocation{
				{Category: "SBN", ValueIDR: valIDR, Percentage: 100.0, Count: 1},
			},
			ByAssetClass: []models.AssetClassAllocation{
				{AssetClass: "Fixed Income", ValueIDR: valIDR, Percentage: 100.0, Count: 1},
			},
		},
		Holdings: []models.Holding{
			{
				Source:               "ksei",
				Category:             "SBN",
				AssetClass:           "Fixed Income",
				Ticker:               "SR021",
				Name:                 "Sukuk Ritel SR021",
				Account:              "BCA",
				ValueIDR:             &valIDR,
				ValueUSD:             &valUSD,
				AllocationPercentage: &allocPct,
				YieldRate:            &yieldRate,
			},
		},
	}

	if err := UpsertSnapshot(database, snapshot); err != nil {
		t.Fatalf("failed to upsert snapshot: %v", err)
	}

	// Verify snapshot row
	var netWorthIDR float64
	var totalItems int
	err = database.QueryRow("SELECT net_worth_idr, total_items FROM snapshots WHERE date = ?", "2026-10-13").Scan(&netWorthIDR, &totalItems)
	if err != nil {
		t.Fatalf("failed to query snapshot: %v", err)
	}
	if netWorthIDR != valIDR {
		t.Errorf("expected net_worth_idr %f, got %f", valIDR, netWorthIDR)
	}
	if totalItems != 1 {
		t.Errorf("expected total_items 1, got %d", totalItems)
	}

	// Verify holding row
	var ticker string
	var storedYield *float64
	err = database.QueryRow("SELECT ticker, yield_rate FROM holdings WHERE snapshot_date = ?", "2026-10-13").Scan(&ticker, &storedYield)
	if err != nil {
		t.Fatalf("failed to query holding: %v", err)
	}
	if ticker != "SR021" {
		t.Errorf("expected ticker SR021, got %s", ticker)
	}
	if storedYield == nil || *storedYield != yieldRate {
		t.Errorf("expected yield_rate %f, got %v", yieldRate, storedYield)
	}

	// Verify idempotency (re-inserting the same snapshot should update cleanly)
	if err := UpsertSnapshot(database, snapshot); err != nil {
		t.Fatalf("failed to upsert snapshot idempotently: %v", err)
	}

	var countHoldings int
	_ = database.QueryRow("SELECT COUNT(*) FROM holdings WHERE snapshot_date = ?", "2026-10-13").Scan(&countHoldings)
	if countHoldings != 1 {
		t.Errorf("expected exactly 1 holding after re-upsert, got %d", countHoldings)
	}
}
