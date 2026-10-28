package sansfinance

import (
	"testing"
)

func TestStandardize(t *testing.T) {
	raw := &RawSansfinanceData{
		Accounts: []AccountEntry{
			{Name: "BCA Primary", Type: "Bank Account", Balance: 15000000.0, Currency: "IDR"},
			{Name: "Jago Pocket", Type: "Cash", Balance: 5000000.0, Currency: "IDR"},
			{Name: "KoinWorks", Type: "P2P Lending", Balance: 10000000.0, Currency: "IDR"},
			{Name: "Dust Account", Type: "Cash", Balance: 0.5, Currency: "IDR"}, // Should be filtered out (< 1.0)
			{Name: "Credit Card", Type: "Bank Account", Balance: -2500000.0, Currency: "IDR"},
		},
	}

	holdings := Standardize(raw)
	if len(holdings) != 4 {
		t.Fatalf("expected 4 holdings after dust filter, got %d", len(holdings))
	}

	for _, h := range holdings {
		switch h.Asset {
		case "BCA Primary":
			if h.Category != "Bank Account" || h.AssetClass != "Cash & Equivalents" {
				t.Errorf("unexpected BCA classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
			if *h.ValueIDR != 15000000.0 {
				t.Errorf("unexpected BCA balance: %f", *h.ValueIDR)
			}
		case "Jago Pocket":
			if h.Category != "Digital Bank" || h.AssetClass != "Cash & Equivalents" {
				t.Errorf("unexpected Jago classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
		case "KoinWorks":
			if h.Category != "P2P Lending" || h.AssetClass != "Fixed Income" {
				t.Errorf("unexpected KoinWorks classification: cat=%s, ac=%s", h.Category, h.AssetClass)
			}
		case "Credit Card":
			if h.Category != "Liabilities" {
				t.Errorf("unexpected Credit Card category: %s", h.Category)
			}
			if *h.ValueIDR != 2500000.0 {
				t.Errorf("unexpected Credit Card positive liability value: %f", *h.ValueIDR)
			}
		default:
			t.Errorf("unexpected asset: %s", h.Asset)
		}
	}
}
