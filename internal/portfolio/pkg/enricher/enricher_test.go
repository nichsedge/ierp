package enricher

import (
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestEnrichHoldings(t *testing.T) {
	holdings := []models.Holding{
		{
			Asset:      "ST013T2",
			Category:   "SBN",
			AssetClass: "Fixed Income",
		},
		{
			Asset:      "SOL",
			Category:   "Staked",
			AssetClass: "Crypto",
		},
		{
			Asset:      "Krom",
			Category:   "Bank Account",
			AssetClass: "Cash & Equivalents",
		},
		{
			Asset:      "Unknown Asset",
			Category:   "Other",
			AssetClass: "Other",
		},
		{
			Asset:      "SUKUK TABUNGAN SERI ST014T4",
			Category:   "SBN",
			AssetClass: "Fixed Income",
		},
	}

	enriched := EnrichHoldings(holdings)

	// ST013T2 should have 0.0640 (6.4%)
	if enriched[0].YieldRate == nil || *enriched[0].YieldRate != 0.0640 {
		t.Fatalf("expected ST013T2 yield rate 0.0640, got %v", enriched[0].YieldRate)
	}

	// SOL Staked should have 0.0680 (6.8%)
	if enriched[1].YieldRate == nil || *enriched[1].YieldRate != 0.0680 {
		t.Fatalf("expected SOL staked yield rate 0.0680, got %v", enriched[1].YieldRate)
	}

	// Krom should have 0.060 (6.0%)
	if enriched[2].YieldRate == nil || *enriched[2].YieldRate != 0.060 {
		t.Fatalf("expected Krom yield rate 0.060, got %v", enriched[2].YieldRate)
	}

	// Unknown should have nil
	if enriched[3].YieldRate != nil {
		t.Fatalf("expected unknown asset yield rate nil, got %v", enriched[3].YieldRate)
	}

	// ST014T4 should have 0.0650 (6.5%)
	if enriched[4].YieldRate == nil || *enriched[4].YieldRate != 0.0650 {
		t.Fatalf("expected ST014T4 yield rate 0.0650, got %v", enriched[4].YieldRate)
	}
}
