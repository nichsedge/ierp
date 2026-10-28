package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestServerToolsList(t *testing.T) {
	if len(serverTools) == 0 {
		t.Fatal("expected serverTools to have definitions")
	}

	foundOverview := false
	for _, tool := range serverTools {
		if tool.Name == "get_portfolio_overview" {
			foundOverview = true
			break
		}
	}

	if !foundOverview {
		t.Error("expected to find get_portfolio_overview in serverTools")
	}
}

func TestCallToolWithSnapshot(t *testing.T) {
	tmpDir := t.TempDir()

	valIDR := 10000000.0
	valUSD := 650.0
	testSnap := models.Snapshot{
		Metadata: models.Metadata{
			Date:         "2026-10-14",
			ExchangeRate: 15500.0,
		},
		Totals: models.Totals{
			NetWorthIDR:    valIDR,
			NetWorthUSD:    valUSD,
			TotalAssetsIDR: valIDR,
			BankCashIDR:    5000000.0,
			InvestmentsIDR: 5000000.0,
		},
		AllHoldings: []models.Holding{
			{
				Source:     "debank",
				Category:   "Spot",
				Asset:      "ETH",
				Currency:   "USD",
				ValueUSD:   &valUSD,
				AssetClass: "Crypto",
			},
			{
				Source:     "alchemy",
				Category:   "Spot",
				Asset:      "SOL",
				Currency:   "USD",
				ValueUSD:   &valUSD,
				AssetClass: "Crypto",
			},
			{
				Source:     "sansfinance",
				Category:   "Bank Account",
				Asset:      "BCA",
				Currency:   "IDR",
				ValueIDR:   &valIDR,
				AssetClass: "Cash & Equivalents",
			},
		},
	}

	snapData, err := json.Marshal(testSnap)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "latest_snapshot.json"), snapData, 0644); err != nil {
		t.Fatalf("failed to write test latest_snapshot.json: %v", err)
	}

	server := NewServer(tmpDir, nil)

	// 1. Test get_portfolio_overview
	resOverview, err := server.callTool("get_portfolio_overview", nil)
	if err != nil {
		t.Fatalf("get_portfolio_overview failed: %v", err)
	}
	if len(resOverview) == 0 {
		t.Error("expected non-empty overview result")
	}

	// 2. Test get_holdings_breakdown with debank filter (and alias)
	resDebank, err := server.callTool("get_holdings_breakdown", map[string]interface{}{"source": "debank"})
	if err != nil {
		t.Fatalf("get_holdings_breakdown failed: %v", err)
	}
	var debankHoldings []models.Holding
	if err := json.Unmarshal([]byte(resDebank), &debankHoldings); err != nil {
		t.Fatalf("failed to unmarshal holdings breakdown: %v", err)
	}
	if len(debankHoldings) != 1 || debankHoldings[0].Asset != "ETH" {
		t.Errorf("expected 1 ETH holding for debank filter, got %d", len(debankHoldings))
	}

	// Test EVM alias
	resEVM, err := server.callTool("get_holdings_breakdown", map[string]interface{}{"source": "evm"})
	if err != nil {
		t.Fatalf("get_holdings_breakdown (evm) failed: %v", err)
	}
	var evmHoldings []models.Holding
	_ = json.Unmarshal([]byte(resEVM), &evmHoldings)
	if len(evmHoldings) != 1 {
		t.Errorf("expected 1 holding for evm alias filter, got %d", len(evmHoldings))
	}

	// 3. Test get_holdings_breakdown with alchemy filter
	resAlchemy, err := server.callTool("get_holdings_breakdown", map[string]interface{}{"source": "alchemy"})
	if err != nil {
		t.Fatalf("get_holdings_breakdown (alchemy) failed: %v", err)
	}
	var alchemyHoldings []models.Holding
	_ = json.Unmarshal([]byte(resAlchemy), &alchemyHoldings)
	if len(alchemyHoldings) != 1 || alchemyHoldings[0].Asset != "SOL" {
		t.Errorf("expected 1 SOL holding for alchemy filter, got %d", len(alchemyHoldings))
	}
}
