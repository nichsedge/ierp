package finance_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/finance"
)

func TestFinanceSnapshotsCommitmentsAndRunway(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_finance.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Insert Snapshot
	snapID, err := finance.InsertSnapshot(database, finance.Snapshot{
		SnapshotDate: "2026-10-01",
		LiquidCash:   120000000, // 120M IDR
		Investments:  300000000,
		HardAssets:   50000000,
		Liabilities:  0,
	})
	if err != nil || snapID <= 0 {
		t.Fatalf("InsertSnapshot failed: id=%d, err=%v", snapID, err)
	}

	// Insert Recurring Commitments
	_, err = finance.InsertCommitment(database, finance.Commitment{
		Name:      "Cloud Server & Hosting",
		Category:  "saas",
		Amount:    2000000, // 2M monthly
		Frequency: "monthly",
	})
	if err != nil {
		t.Fatalf("InsertCommitment failed: %v", err)
	}

	_, err = finance.InsertCommitment(database, finance.Commitment{
		Name:      "Domain renewals",
		Category:  "saas",
		Amount:    12000000, // 12M yearly = 1M monthly
		Frequency: "yearly",
	})
	if err != nil {
		t.Fatalf("InsertCommitment 2 failed: %v", err)
	}

	// Monthly Burn
	burn, err := finance.ComputeMonthlyBurn(database, "active")
	if err != nil {
		t.Fatalf("ComputeMonthlyBurn failed: %v", err)
	}
	expectedBurn := 3000000.0 // 2M + 1M
	if burn.TotalMonthlyBurn != expectedBurn {
		t.Errorf("expected monthly burn %.2f, got %.2f", expectedBurn, burn.TotalMonthlyBurn)
	}

	// Sovereign Runway
	runway, err := finance.ComputeRunway(database)
	if err != nil {
		t.Fatalf("ComputeRunway failed: %v", err)
	}
	expectedRunwayMonths := 40.0 // 120M / 3M = 40 months
	if runway.RunwayMonths != expectedRunwayMonths {
		t.Errorf("expected runway %.1f months, got %.1f", expectedRunwayMonths, runway.RunwayMonths)
	}
	if runway.StatusLabel != "FORTRESS (>24m)" {
		t.Errorf("expected status 'FORTRESS (>24m)', got '%s'", runway.StatusLabel)
	}
}
