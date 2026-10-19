package transparency_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/finance"
	"github.com/nichsedge/ierp/internal/transparency"
)

func TestBuildReportPercentagesOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_transparency.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	if _, err := finance.InsertSnapshot(database, finance.Snapshot{
		SnapshotDate: "2026-10-18",
		LiquidCash:   100000000,
		Investments:  50000000,
		Liabilities:  0,
	}); err != nil {
		t.Fatalf("InsertSnapshot failed: %v", err)
	}

	if _, err := finance.InsertCommitment(database, finance.Commitment{
		Name:      "Living Expenses",
		Category:  "lifestyle",
		Amount:    4000000,
		Frequency: "monthly",
	}); err != nil {
		t.Fatalf("InsertCommitment failed: %v", err)
	}
	if _, err := finance.InsertCommitment(database, finance.Commitment{
		Name:      "Housing",
		Category:  "housing",
		Amount:    1000000,
		Frequency: "monthly",
	}); err != nil {
		t.Fatalf("InsertCommitment 2 failed: %v", err)
	}

	report, err := transparency.BuildReport(database)
	if err != nil {
		t.Fatalf("BuildReport failed: %v", err)
	}

	if report.RunwayMonths != 20 {
		t.Errorf("expected 20 runway months, got %.1f", report.RunwayMonths)
	}
	if report.StatusLabel != "SOVEREIGN (>12m)" {
		t.Errorf("expected SOVEREIGN (>12m), got '%s'", report.StatusLabel)
	}
	if got := report.AllocationPct["liquid_cash"]; got < 66.6 || got > 66.7 {
		t.Errorf("expected liquid_cash ~66.67%%, got %.2f", got)
	}
	if got := report.AllocationPct["investments"]; got < 33.3 || got > 33.4 {
		t.Errorf("expected investments ~33.33%%, got %.2f", got)
	}
	if got := report.BurnPct["lifestyle"]; got < 79.9 || got > 80.1 {
		t.Errorf("expected lifestyle burn ~80%%, got %.2f", got)
	}
	if got := report.BurnPct["housing"]; got < 19.9 || got > 20.1 {
		t.Errorf("expected housing burn ~20%%, got %.2f", got)
	}
}

func TestExportWritesPrivacySafeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_transparency_export.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	if _, err := finance.InsertSnapshot(database, finance.Snapshot{
		SnapshotDate: "2026-10-18",
		LiquidCash:   123828497,
		Investments:  49466866.89,
		Liabilities:  833850,
	}); err != nil {
		t.Fatalf("InsertSnapshot failed: %v", err)
	}

	outDir := filepath.Join(tmpDir, "data")
	outPath, err := transparency.Export(database, outDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if filepath.Base(outPath) != "transparency.json" {
		t.Errorf("expected transparency.json, got %s", outPath)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded transparency.Report
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded.SnapshotDate != "2026-10-18" {
		t.Errorf("expected snapshot date 2026-10-18, got %s", decoded.SnapshotDate)
	}

	// Privacy guard: no absolute currency amounts may leak into the public payload.
	for _, secret := range []string{"123828497", "49466866", "833850"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("public payload leaks absolute amount %s", secret)
		}
	}
}
