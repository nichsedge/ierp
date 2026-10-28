package models

import (
	"fmt"
	"strings"
)

// Standard category and asset class definitions
var StableCoins = map[string]bool{
	"USDT": true, "USDC": true, "DAI": true, "FDUSD": true,
	"TUSD": true, "BUSD": true, "PYUSD": true, "USDP": true,
}

var GoldAssets = map[string]bool{
	"PAXG": true, "XAUT": true,
}

var ValidCategories = map[string]bool{
	"Bank Account": true, "Digital Bank": true, "Stablecoin": true, "Money Market Fund": true,
	"SBN": true, "Corporate Bond": true, "P2P Lending": true,
	"US Stocks": true, "Indo Stocks": true, "Equity Fund": true,
	"Spot": true, "Staked": true, "Yield / LP": true,
	"Gold": true, "Silver": true, "Liabilities": true, "Other": true,
}

var FilterThresholds = map[string]float64{
	"IDR": 50000.0,
	"USD": 5.0,
}

// Holding represents a unified portfolio holding record.
type Holding struct {
	Source               string   `json:"source"`
	Category             string   `json:"category"`
	Asset                string   `json:"asset"`
	Currency             string   `json:"currency"`
	Quantity             float64  `json:"quantity"`
	Price                *float64 `json:"price,omitempty"`
	PriceIDR             *float64 `json:"price_idr,omitempty"`
	ValueIDR             *float64 `json:"value_idr,omitempty"`
	ValueUSD             *float64 `json:"value_usd,omitempty"`
	Account              string   `json:"account,omitempty"`
	Details              string   `json:"details,omitempty"`
	AssetClass           string   `json:"asset_class"`
	YieldRate            *float64 `json:"yield_rate,omitempty"`
	AllocationPercentage *float64 `json:"allocation_percentage,omitempty"`
	Ticker               string   `json:"ticker,omitempty"`
	Name                 string   `json:"name,omitempty"`
}

// Metadata holds snapshot meta info.
type Metadata struct {
	Date         string  `json:"date"`
	ExchangeRate float64 `json:"exchange_rate"`
	TotalItems   int     `json:"total_items"`
	GeneratedAt  string  `json:"generated_at"`
}

// Totals holds net worth and balance aggregates.
type Totals struct {
	NetWorthIDR           float64 `json:"net_worth_idr"`
	NetWorthUSD           float64 `json:"net_worth_usd"`
	TotalAssetsIDR        float64 `json:"total_assets_idr"`
	TotalLiabilitiesIDR   float64 `json:"total_liabilities_idr"`
	InvestmentsIDR        float64 `json:"investments_idr"`
	InvestmentsUSD        float64 `json:"investments_usd"`
	BankCashIDR           float64 `json:"bank_cash_idr"`
	BankCashUSD           float64 `json:"bank_cash_usd"`
	WeightedYieldPct      float64 `json:"weighted_yield_pct"`
	AnnualYieldIncomeIDR  float64 `json:"annual_yield_income_idr"`
	MonthlyYieldIncomeIDR float64 `json:"monthly_yield_income_idr"`
}

// CategoryAllocation represents aggregate stats for one category.
type CategoryAllocation struct {
	Category   string  `json:"category"`
	ValueIDR   float64 `json:"value_idr"`
	Percentage float64 `json:"percentage"`
	Count      int     `json:"count"`
}

// AssetClassAllocation represents aggregate stats for one asset class.
type AssetClassAllocation struct {
	AssetClass string  `json:"asset_class"`
	ValueIDR   float64 `json:"value_idr"`
	Percentage float64 `json:"percentage"`
	Count      int     `json:"count"`
}

// AllocationBreakdown contains category and asset class breakdowns.
type AllocationBreakdown struct {
	ByCategory   []CategoryAllocation   `json:"by_category"`
	ByAssetClass []AssetClassAllocation `json:"by_asset_class"`
}

// Snapshot represents a full daily portfolio snapshot.
type Snapshot struct {
	Metadata           Metadata            `json:"metadata"`
	Totals             Totals              `json:"totals"`
	Allocation         AllocationBreakdown `json:"allocation"`
	Holdings           []Holding           `json:"holdings"`
	LiquidCashAccounts []Holding           `json:"liquid_cash_accounts"`
	AllHoldings        []Holding           `json:"all_holdings"`
}

// GetStandardCategory returns the canonical category name.
func GetStandardCategory(category, asset string) string {
	if StableCoins[asset] {
		return "Stablecoin"
	}
	if GoldAssets[asset] {
		return "Gold"
	}
	if ValidCategories[category] {
		return category
	}
	if category == "Cryptocurrency" {
		return "Spot"
	}
	if strings.Contains(category, "Staked") {
		return "Staked"
	}
	for _, term := range []string{"Yield", "LP", "Protocol", "Vault", "Rewards"} {
		if strings.Contains(category, term) {
			return "Yield / LP"
		}
	}
	if strings.Contains(category, "Lending") && !strings.Contains(category, "P2P") {
		return "Yield / LP"
	}
	return category
}

// GetAssetClass maps category to broad asset class.
func GetAssetClass(category string) string {
	mapping := map[string]string{
		"Bank Account":      "Cash & Equivalents",
		"Digital Bank":      "Cash & Equivalents",
		"Stablecoin":        "Cash & Equivalents",
		"Money Market Fund": "Cash & Equivalents",
		"SBN":               "Fixed Income",
		"Corporate Bond":    "Fixed Income",
		"P2P Lending":       "Fixed Income",
		"US Stocks":         "Equities",
		"Indo Stocks":       "Equities",
		"Equity Fund":       "Equities",
		"Spot":              "Crypto",
		"Staked":            "Crypto",
		"Yield / LP":        "Crypto",
		"Gold":              "Commodities",
		"Silver":            "Commodities",
		"Liabilities":       "Other",
	}

	if val, ok := mapping[category]; ok {
		return val
	}

	switch category {
	case "Cash", "Deposit":
		return "Cash & Equivalents"
	case "Equity":
		return "Equities"
	case "Mutual Fund":
		return "Equities"
	case "Bond":
		return "Fixed Income"
	case "P2P Syariah":
		return "Fixed Income"
	case "Cryptocurrency":
		return "Crypto"
	case "DeFi Protocol", "DeFi Yield":
		return "Crypto"
	case "DeFi Staked":
		return "Crypto"
	}

	return "Other"
}

// FormatNumber formats a floating point amount with comma thousands separators.
func FormatNumber(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		parts = append([]string{s}, parts...)
	}
	return strings.Join(parts, ",")
}
