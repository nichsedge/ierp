package models

import "testing"

func TestGetAssetClass(t *testing.T) {
	tests := []struct {
		category string
		expected string
	}{
		{"Bank Account", "Cash & Equivalents"},
		{"Digital Bank", "Cash & Equivalents"},
		{"Stablecoin", "Cash & Equivalents"},
		{"SBN", "Fixed Income"},
		{"P2P Lending", "Fixed Income"},
		{"Indo Stocks", "Equities"},
		{"US Stocks", "Equities"},
		{"Spot", "Crypto"},
		{"Staked", "Crypto"},
		{"Gold", "Commodities"},
		{"Unknown Category", "Other"},
	}

	for _, tc := range tests {
		actual := GetAssetClass(tc.category)
		if actual != tc.expected {
			t.Errorf("GetAssetClass(%q) = %q; expected %q", tc.category, actual, tc.expected)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1500000, "1,500,000"},
		{1234567890, "1,234,567,890"},
	}

	for _, tc := range tests {
		actual := FormatNumber(tc.input)
		if actual != tc.expected {
			t.Errorf("FormatNumber(%f) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}
