package binance

import (
	"testing"
)

func TestStandardize(t *testing.T) {
	raw := &RawBinanceData{
		Timestamp: "2026-10-14T00:00:00Z",
		TotalUSD:  1050.0,
		Assets: []BinanceAssetEntry{
			{Symbol: "BTC", Quantity: 0.01, PriceUSD: 65000.0, ValueUSD: 650.0},
			{Symbol: "USDT", Quantity: 400.0, PriceUSD: 1.0, ValueUSD: 400.0},
			{Symbol: "DUST_TOKEN", Quantity: 10.0, PriceUSD: 0.1, ValueUSD: 1.0}, // < $5 threshold
		},
	}

	holdings := Standardize(raw)
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings after dust filter, got %d", len(holdings))
	}

	for _, h := range holdings {
		switch h.Asset {
		case "BTC":
			if h.Category != "Spot" || h.AssetClass != "Crypto" {
				t.Errorf("unexpected BTC classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if *h.ValueUSD != 650.0 {
				t.Errorf("unexpected BTC value: %f", *h.ValueUSD)
			}
		case "USDT":
			if h.Category != "Stablecoin" || h.AssetClass != "Cash & Equivalents" {
				t.Errorf("unexpected USDT classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if *h.ValueUSD != 400.0 {
				t.Errorf("unexpected USDT value: %f", *h.ValueUSD)
			}
		default:
			t.Errorf("unexpected asset: %s", h.Asset)
		}
	}
}
