package debank

import (
	"encoding/json"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/integrator"
)

func TestStandardizeNilData(t *testing.T) {
	holdings := Standardize(nil)
	if holdings != nil {
		t.Fatalf("expected nil holdings for nil input, got %v", holdings)
	}
}

func TestStandardizeTokens(t *testing.T) {
	raw := &RawDebankData{
		Timestamp: "2026-10-06T00:00:00Z",
		Tokens: []DebankToken{
			{
				Symbol:   "USDT",
				Quantity: "50.0",
				Price:    "$1.00",
				Value:    "$50.00",
			},
			{
				Symbol:   "USDT",
				Quantity: 25.5,
				Price:    1.0,
				Value:    25.5,
			},
			{
				Symbol:   "ETH",
				Quantity: "1.5",
				Price:    "$3,200.00",
				Value:    "$4,800.00",
			},
			{
				Symbol:   "SPAM_DUST",
				Quantity: "1000000",
				Price:    "<$0.00001",
				Value:    "$0.05", // Dust below $5 threshold
			},
		},
	}

	holdings := Standardize(raw)

	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings (dust filtered), got %d", len(holdings))
	}

	var foundUSDT, foundETH bool
	for _, h := range holdings {
		if h.Asset == "USDT" {
			foundUSDT = true
			if h.Category != "Stablecoin" {
				t.Errorf("expected USDT category Stablecoin, got %s", h.Category)
			}
			if h.AssetClass != "Cash & Equivalents" {
				t.Errorf("expected USDT asset class Cash & Equivalents, got %s", h.AssetClass)
			}
			if h.Quantity != 75.5 {
				t.Errorf("expected aggregated USDT quantity 75.5, got %f", h.Quantity)
			}
			if *h.ValueUSD != 75.5 {
				t.Errorf("expected aggregated USDT value $75.50, got %f", *h.ValueUSD)
			}
			if *h.Price != 1.0 {
				t.Errorf("expected USDT price 1.0, got %f", *h.Price)
			}
			if h.Source != "debank" {
				t.Errorf("expected source debank, got %s", h.Source)
			}
		}
		if h.Asset == "ETH" {
			foundETH = true
			if h.Category != "Spot" {
				t.Errorf("expected ETH category Spot, got %s", h.Category)
			}
			if h.AssetClass != "Crypto" {
				t.Errorf("expected ETH asset class Crypto, got %s", h.AssetClass)
			}
			if h.Quantity != 1.5 {
				t.Errorf("expected ETH quantity 1.5, got %f", h.Quantity)
			}
			if *h.ValueUSD != 4800.0 {
				t.Errorf("expected ETH value $4800.00, got %f", *h.ValueUSD)
			}
			if *h.Price != 3200.0 {
				t.Errorf("expected ETH price 3200.0, got %f", *h.Price)
			}
		}
	}

	if !foundUSDT || !foundETH {
		t.Errorf("did not find expected tokens: foundUSDT=%v, foundETH=%v", foundUSDT, foundETH)
	}
}

func TestStandardizeProtocolsAndLiabilities(t *testing.T) {
	raw := &RawDebankData{
		Timestamp: "2026-10-06T00:00:00Z",
		Protocols: []DebankProtocol{
			{
				Name:  "Aave V3",
				Chain: "arbitrum",
				Positions: []DebankPosition{
					{
						Pool:  "Supply Pool",
						Type:  "Supplied",
						Value: "$1,500.00",
						Tokens: []struct {
							Balance string `json:"balance"`
						}{
							{Balance: "1,500 USDC"},
						},
					},
					{
						Pool:  "Debt Pool",
						Type:  "Borrowed", // Critical: Liability
						Value: "$450.00",
						Tokens: []struct {
							Balance string `json:"balance"`
						}{
							{Balance: "450 USDT"},
						},
					},
				},
			},
			{
				Name:  "Lido",
				Chain: "eth",
				Positions: []DebankPosition{
					{
						Pool:  "stETH",
						Type:  "Staked",
						Value: "$3,000.00",
						Tokens: []struct {
							Balance string `json:"balance"`
						}{
							{Balance: "1.0 stETH"},
						},
					},
				},
			},
			{
				Name:  "Uniswap V3",
				Value: "$200.00", // Summary only
			},
		},
	}

	holdings := Standardize(raw)
	if len(holdings) != 4 {
		t.Fatalf("expected 4 protocol holdings, got %d", len(holdings))
	}

	var foundSupplied, foundBorrowed, foundStaked, foundSummary bool
	for _, h := range holdings {
		switch {
		case h.Category == "Liabilities":
			foundBorrowed = true
			if h.AssetClass != "Other" {
				t.Errorf("expected Liabilities asset class Other, got %s", h.AssetClass)
			}
			if *h.ValueUSD != 450.0 {
				t.Errorf("expected borrowed value 450, got %f", *h.ValueUSD)
			}
		case h.Category == "Staked":
			foundStaked = true
			if h.AssetClass != "Crypto" {
				t.Errorf("expected Staked asset class Crypto, got %s", h.AssetClass)
			}
			if *h.ValueUSD != 3000.0 {
				t.Errorf("expected staked value 3000, got %f", *h.ValueUSD)
			}
		case h.Asset == "Aave V3 (Supplied) - Supply Pool":
			foundSupplied = true
			if h.Category != "Yield / LP" {
				t.Errorf("expected supplied category Yield / LP, got %s", h.Category)
			}
			if *h.ValueUSD != 1500.0 {
				t.Errorf("expected supplied value 1500, got %f", *h.ValueUSD)
			}
		case h.Asset == "Uniswap V3":
			foundSummary = true
			if h.Category != "Yield / LP" {
				t.Errorf("expected summary category Yield / LP, got %s", h.Category)
			}
			if *h.ValueUSD != 200.0 {
				t.Errorf("expected summary value 200, got %f", *h.ValueUSD)
			}
		}
	}

	if !foundSupplied || !foundBorrowed || !foundStaked || !foundSummary {
		t.Errorf("missing protocol position: supplied=%v, borrowed=%v, staked=%v, summary=%v",
			foundSupplied, foundBorrowed, foundStaked, foundSummary)
	}

	// Verify integration net worth calculation:
	// Total Assets: 1500 (Supply) + 3000 (Lido) + 200 (Uni) = 4700 USD
	// Total Liabilities: 450 USD
	// Net Worth: 4700 - 450 = 4250 USD
	rate := 16000.0
	snap, err := integrator.Integrate("2026-10-06", holdings, rate, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("integration error: %v", err)
	}

	expectedNetWorthUSD := 4250.0
	if snap.Totals.NetWorthUSD != expectedNetWorthUSD {
		t.Errorf("expected net worth $%.2f, got $%.2f", expectedNetWorthUSD, snap.Totals.NetWorthUSD)
	}
	expectedLiabilitiesIDR := 450.0 * rate
	if snap.Totals.TotalLiabilitiesIDR != expectedLiabilitiesIDR {
		t.Errorf("expected liabilities IDR %.2f, got %.2f", expectedLiabilitiesIDR, snap.Totals.TotalLiabilitiesIDR)
	}
}

func TestStandardizeNFTs(t *testing.T) {
	raw := &RawDebankData{
		NFTs: []DebankNFT{
			{
				Collection: "Pudgy Penguins",
				AvgPrice:   "$35,000.00",
				Amount:     1,
			},
			{
				Collection: "Spam NFT",
				AvgPrice:   "$0.01",
				Amount:     "100", // Value $1.00 < $5 threshold
			},
		},
	}

	holdings := Standardize(raw)
	if len(holdings) != 1 {
		t.Fatalf("expected 1 NFT holding after dust filter, got %d", len(holdings))
	}

	nft := holdings[0]
	if nft.Asset != "Pudgy Penguins" {
		t.Errorf("expected asset Pudgy Penguins, got %s", nft.Asset)
	}
	if *nft.ValueUSD != 35000.0 {
		t.Errorf("expected value $35000.00, got %f", *nft.ValueUSD)
	}
	if nft.Category != "Spot" {
		t.Errorf("expected category Spot, got %s", nft.Category)
	}
	if nft.AssetClass != "Crypto" {
		t.Errorf("expected asset class Crypto, got %s", nft.AssetClass)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	rawJSON := `{
		"timestamp": "2026-10-06T12:00:00Z",
		"tokens": [
			{"symbol": "SOL", "amount": "10.5", "price": "$150.00", "value": "$1,575.00"}
		],
		"protocols": [],
		"nfts": []
	}`

	var raw RawDebankData
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	holdings := Standardize(&raw)
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if holdings[0].Asset != "SOL" || *holdings[0].ValueUSD != 1575.0 {
		t.Errorf("unexpected holding: %+v", holdings[0])
	}
}
