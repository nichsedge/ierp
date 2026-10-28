package alchemy

import (
	"testing"
)

func TestStandardize(t *testing.T) {
	curated := &CuratedAlchemyData{
		Timestamp: "2026-10-14T00:00:00Z",
		Assets: []AssetEntry{
			{Symbol: "UNKNOWN", Name: "UNKNOWN", TokenAddress: "", Network: "solana-mainnet", Quantity: 5.0, PriceUSD: 150.0, ValueUSD: 750.0},
			{Symbol: "JLP", Name: "Jupiter Perps LP", TokenAddress: "27G8MtK7VtTcCHkpASjSDdkWWYfoqT6ggEuKidVJidD4", Network: "solana-mainnet", Quantity: 8.15, PriceUSD: 4.50, ValueUSD: 36.67},
			{Symbol: "GRASS", Name: "Grass (Staked)", TokenAddress: GrassTokenMint, Network: "solana-mainnet", Quantity: 42.32, PriceUSD: 0.65, ValueUSD: 27.50, Category: "Staked", IsStaked: true},
			{Symbol: "USDC", Name: "USD Coin", Network: "solana-mainnet", Quantity: 200.0, PriceUSD: 1.0, ValueUSD: 200.0},
			{Symbol: "MEME", Name: "Meme Token", Network: "solana-mainnet", Quantity: 1000.0, PriceUSD: 0.001, ValueUSD: 1.0}, // < $5 dust
		},
	}

	holdings := Standardize(curated)
	if len(holdings) != 4 {
		t.Fatalf("expected 4 holdings after dust filter, got %d", len(holdings))
	}

	for _, h := range holdings {
		if h.Source != "alchemy" {
			t.Errorf("expected source alchemy, got %s", h.Source)
		}
		switch h.Asset {
		case "SOL":
			if h.Category != "Spot" || h.AssetClass != "Crypto" {
				t.Errorf("unexpected SOL classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if *h.ValueUSD != 750.0 {
				t.Errorf("unexpected SOL value: %f", *h.ValueUSD)
			}
			if h.Name != "Solana" {
				t.Errorf("expected SOL name to be Solana, got %s", h.Name)
			}
		case "JLP":
			if h.Category != "Yield / LP" || h.AssetClass != "Crypto" {
				t.Errorf("unexpected JLP classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
		case "GRASS":
			if h.Category != "Staked" || h.AssetClass != "Crypto" {
				t.Errorf("unexpected GRASS classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if h.Account != "Grass Staking" {
				t.Errorf("expected account Grass Staking, got %s", h.Account)
			}
		case "USDC":
			if h.Category != "Stablecoin" || h.AssetClass != "Cash & Equivalents" {
				t.Errorf("unexpected USDC classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if *h.ValueUSD != 200.0 {
				t.Errorf("unexpected USDC value: %f", *h.ValueUSD)
			}
		default:
			t.Errorf("unexpected asset: %s", h.Asset)
		}
	}
}

func TestDeriveGrassStakePDA(t *testing.T) {
	wallet := "6VZMafUCnkGQ8EeYgMDFsNFwEwJPkerVtP2DzFmokhus"
	expectedPDA := "5Yr1bNnWgfcTbV7g68bw75jRyxERjhtBzWX47mKdN3LX"

	pda, err := deriveGrassStakePDA(wallet)
	if err != nil {
		t.Fatalf("deriveGrassStakePDA failed: %v", err)
	}

	if pda != expectedPDA {
		t.Errorf("expected PDA %s, got %s", expectedPDA, pda)
	}
}
