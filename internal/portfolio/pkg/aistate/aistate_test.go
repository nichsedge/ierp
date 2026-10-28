package aistate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestGenerateAIState(t *testing.T) {
	val1 := 90000000.0
	val2 := 10000000.0
	val3 := 50000.0 // Dust < 1%
	total := val1 + val2 + val3

	snap := &models.Snapshot{
		Metadata: models.Metadata{
			Date:         "2026-10-01",
			ExchangeRate: 16000.0,
			TotalItems:   3,
		},
		Totals: models.Totals{
			NetWorthIDR:    total,
			NetWorthUSD:    total / 16000.0,
			TotalAssetsIDR: total,
			BankCashIDR:    val2,
		},
		Allocation: models.AllocationBreakdown{
			ByAssetClass: []models.AssetClassAllocation{
				{AssetClass: "Fixed Income", ValueIDR: val1, Percentage: 89.9, Count: 1},
				{AssetClass: "Cash & Equivalents", ValueIDR: val2, Percentage: 10.0, Count: 1},
				{AssetClass: "Other", ValueIDR: val3, Percentage: 0.1, Count: 1},
			},
		},
		AllHoldings: []models.Holding{
			{
				Asset:      "ST012T4 - SUKUK TABUNGAN SERI ST012T4",
				Category:   "SBN",
				AssetClass: "Fixed Income",
				ValueIDR:   &val1,
			},
			{
				Asset:      "Krom",
				Category:   "Bank Account",
				AssetClass: "Cash & Equivalents",
				ValueIDR:   &val2,
			},
			{
				Asset:      "Dust Coin",
				Category:   "Spot",
				AssetClass: "Other",
				ValueIDR:   &val3,
			},
		},
	}

	state, err := GenerateAIState(snap, nil)
	if err != nil {
		t.Fatalf("GenerateAIState failed: %v", err)
	}

	if state.MacroMetrics.NetWorthIDR != total {
		t.Fatalf("expected NetWorthIDR %f, got %f", total, state.MacroMetrics.NetWorthIDR)
	}

	// SBN schedule should have 1 item
	if len(state.SukukAndCouponSchedule.Schedule) != 1 {
		t.Fatalf("expected 1 Sukuk item, got %d", len(state.SukukAndCouponSchedule.Schedule))
	}

	// Clutter audit should have identified 1 dust holding
	if state.ADHDFocusMetrics.ClutterAudit.FragmentedCount != 1 {
		t.Fatalf("expected 1 dust holding, got %d", state.ADHDFocusMetrics.ClutterAudit.FragmentedCount)
	}

	// Markdown generation
	md := GenerateAIDigestMarkdown(state)
	if !strings.Contains(md, "Executive Macro Overview") {
		t.Fatalf("markdown missing executive overview section")
	}

	tempDir, err := os.MkdirTemp("", "aistate_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	if err := SaveAIState(state, md, tempDir, nil); err != nil {
		t.Fatalf("SaveAIState failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "latest_ai_state.json")); os.IsNotExist(err) {
		t.Fatalf("expected latest_ai_state.json to exist")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "latest_ai_digest.md")); os.IsNotExist(err) {
		t.Fatalf("expected latest_ai_digest.md to exist")
	}
}
