package integrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestIntegrate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "portfolio_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	val1 := 50000000.0
	val2 := 50000000.0
	rate := 16000.0

	holdings := []models.Holding{
		{
			Source:     "ksei",
			Category:   "SBN",
			Asset:      "ST013T2",
			Currency:   "IDR",
			Quantity:   50.0,
			ValueIDR:   &val1,
			AssetClass: "Fixed Income",
		},
		{
			Source:     "sansfinance",
			Category:   "Bank Account",
			Asset:      "Krom",
			Currency:   "IDR",
			Quantity:   1.0,
			ValueIDR:   &val2,
			AssetClass: "Cash & Equivalents",
		},
	}

	snap, err := Integrate("2026-10-01", holdings, rate, tempDir, nil)
	if err != nil {
		t.Fatalf("Integrate failed: %v", err)
	}

	if snap.Totals.NetWorthIDR != 100000000.0 {
		t.Fatalf("expected NetWorthIDR 100,000,000, got %f", snap.Totals.NetWorthIDR)
	}
	if snap.Totals.BankCashIDR != 50000000.0 {
		t.Fatalf("expected BankCashIDR 50,000,000, got %f", snap.Totals.BankCashIDR)
	}
	if snap.Totals.InvestmentsIDR != 50000000.0 {
		t.Fatalf("expected InvestmentsIDR 50,000,000, got %f", snap.Totals.InvestmentsIDR)
	}

	// Verify latest_snapshot.json was written
	latestFile := filepath.Join(tempDir, "latest_snapshot.json")
	if _, err := os.Stat(latestFile); os.IsNotExist(err) {
		t.Fatalf("expected latest_snapshot.json to exist")
	}
}
