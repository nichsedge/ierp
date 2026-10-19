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

func TestSnapshotDeviationGuard(t *testing.T) {
	cases := []struct {
		name   string
		prev   float64
		curr   float64
		expect float64
	}{
		{"identical", 100000000, 100000000, 0},
		{"half scale under", 123828497, 2500000, 0.9798},
		{"modest swing", 120000000, 130000000, 0.0833},
		{"double", 50000000, 100000000, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dev := finance.Deviation(tc.prev, tc.curr)
			if dev < tc.expect-0.01 || dev > tc.expect+0.01 {
				t.Errorf("expected deviation ~%.4f, got %.4f", tc.expect, dev)
			}
		})
	}

	// A 50x under-scale must trip the guard; a normal quarterly move must not.
	if dev := finance.Deviation(123828497, 2500000); dev <= finance.MaxSnapshotDeviation {
		t.Errorf("expected under-scaled snapshot to exceed guard, got %.4f", dev)
	}
	if dev := finance.Deviation(123828497, 130000000); dev > finance.MaxSnapshotDeviation {
		t.Errorf("expected normal swing to pass guard, got %.4f", dev)
	}
}

func TestUpdateSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_update_snapshot.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	snapID, err := finance.InsertSnapshot(database, finance.Snapshot{
		SnapshotDate: "2026-10-18",
		LiquidCash:   2500000,
		Investments:  1200000,
		HardAssets:   800000,
		Liabilities:  400000,
		Notes:        "Quarterly financial snapshot after Q4 2026",
	})
	if err != nil {
		t.Fatalf("InsertSnapshot failed: %v", err)
	}

	ok, err := finance.UpdateSnapshot(database, snapID, finance.Snapshot{
		SnapshotDate: "2026-10-18",
		LiquidCash:   123828497,
		Investments:  49466866.89,
		HardAssets:   0,
		Liabilities:  833850,
		Currency:     "IDR",
		Notes:        "Corrected: values were previously under-scaled ~50x",
	})
	if err != nil {
		t.Fatalf("UpdateSnapshot failed: %v", err)
	}
	if !ok {
		t.Fatal("expected snapshot to be updated")
	}

	latest, err := finance.GetLatestSnapshot(database)
	if err != nil || latest == nil {
		t.Fatalf("GetLatestSnapshot failed: %v", err)
	}
	if latest.LiquidCash != 123828497 {
		t.Errorf("expected liquid 123828497, got %.2f", latest.LiquidCash)
	}
	if latest.Investments != 49466866.89 {
		t.Errorf("expected investments 49466866.89, got %.2f", latest.Investments)
	}
	expectedNetWorth := 123828497.0 + 49466866.89 - 833850.0
	if latest.NetWorth != expectedNetWorth {
		t.Errorf("expected net worth %.2f, got %.2f", expectedNetWorth, latest.NetWorth)
	}

	// Updating a non-existent snapshot must report no rows affected.
	ok, err = finance.UpdateSnapshot(database, 9999, finance.Snapshot{SnapshotDate: "2026-10-18"})
	if err != nil {
		t.Fatalf("UpdateSnapshot on missing id failed: %v", err)
	}
	if ok {
		t.Error("expected no rows affected for missing snapshot id")
	}
}
