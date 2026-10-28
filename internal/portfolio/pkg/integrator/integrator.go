package integrator

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/db"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

// Integrate compiles raw standardized holdings into a consolidated snapshot.
func Integrate(date string, allHoldings []models.Holding, exchangeRate float64, dataDir string, dbConn *sql.DB) (*models.Snapshot, error) {
	if exchangeRate <= 0 {
		return nil, fmt.Errorf("exchange rate must be positive, got %f", exchangeRate)
	}

	// 1. Fill in missing currency values
	for i := range allHoldings {
		h := &allHoldings[i]
		if h.ValueIDR != nil && h.ValueUSD == nil {
			usd := math.Round((*h.ValueIDR/exchangeRate)*100) / 100
			h.ValueUSD = &usd
		} else if h.ValueUSD != nil && h.ValueIDR == nil {
			idr := math.Round((*h.ValueUSD*exchangeRate)*100) / 100
			h.ValueIDR = &idr
		} else if h.ValueIDR == nil && h.ValueUSD == nil {
			zero := 0.0
			h.ValueIDR = &zero
			h.ValueUSD = &zero
		}
		// Standardize source taxonomy
		if h.Source == "EVM Wallet" || strings.ToLower(h.Source) == "evm" {
			h.Source = "debank"
		} else if h.Source == "SOL Wallet" || strings.ToLower(h.Source) == "sol" {
			h.Source = "alchemy"
		}

		if h.AssetClass == "" {
			h.AssetClass = models.GetAssetClass(h.Category)
		}
		if (h.Asset == "UNKNOWN" || h.Asset == "") && (strings.Contains(strings.ToLower(h.Source), "sol") || strings.Contains(strings.ToLower(h.Source), "alchemy")) {
			h.Asset = "SOL"
			h.Ticker = "SOL"
			if h.Name == "" || h.Name == "UNKNOWN" {
				h.Name = "Solana"
			}
		}
	}

	// Sort holdings consistently
	sort.Slice(allHoldings, func(i, j int) bool {
		if allHoldings[i].Category != allHoldings[j].Category {
			return allHoldings[i].Category < allHoldings[j].Category
		}
		if allHoldings[i].Source != allHoldings[j].Source {
			return allHoldings[i].Source < allHoldings[j].Source
		}
		return allHoldings[i].Asset < allHoldings[j].Asset
	})

	// 2. Separate investments vs liquid cash accounts
	var investmentsData []models.Holding
	var liquidCashAccounts []models.Holding
	for _, h := range allHoldings {
		if strings.ToLower(h.Source) == "sansfinance" && h.Category != "P2P Lending" {
			liquidCashAccounts = append(liquidCashAccounts, h)
		} else {
			investmentsData = append(investmentsData, h)
		}
	}

	// 3. Compute totals and breakdowns
	var totalAssetsIDR, totalLiabilitiesIDR float64
	catMap := make(map[string]*models.CategoryAllocation)
	acMap := make(map[string]*models.AssetClassAllocation)

	for _, h := range allHoldings {
		valIDR := 0.0
		if h.ValueIDR != nil {
			valIDR = *h.ValueIDR
		}

		cat := h.Category
		if strings.Contains(cat, "Debt") || strings.Contains(cat, "Borrow") || strings.Contains(cat, "Liabilities") {
			totalLiabilitiesIDR += valIDR
		} else {
			totalAssetsIDR += valIDR
		}

		// Category breakdown
		if entry, ok := catMap[cat]; ok {
			entry.ValueIDR += valIDR
			entry.Count++
		} else {
			catMap[cat] = &models.CategoryAllocation{
				Category: cat,
				ValueIDR: valIDR,
				Count:    1,
			}
		}

		// Asset class breakdown
		ac := h.AssetClass
		if entry, ok := acMap[ac]; ok {
			entry.ValueIDR += valIDR
			entry.Count++
		} else {
			acMap[ac] = &models.AssetClassAllocation{
				AssetClass: ac,
				ValueIDR:   valIDR,
				Count:      1,
			}
		}
	}

	netWorthIDR := totalAssetsIDR - totalLiabilitiesIDR
	var investmentsIDR, bankCashIDR float64
	for _, h := range investmentsData {
		if h.ValueIDR != nil {
			investmentsIDR += *h.ValueIDR
		}
	}
	for _, h := range liquidCashAccounts {
		if h.ValueIDR != nil {
			bankCashIDR += *h.ValueIDR
		}
	}

	var annualYieldIncomeIDR float64
	for _, h := range allHoldings {
		if h.ValueIDR != nil && h.YieldRate != nil {
			annualYieldIncomeIDR += *h.ValueIDR * *h.YieldRate
		}
	}
	var weightedYieldPct float64
	if totalAssetsIDR > 0 {
		weightedYieldPct = (annualYieldIncomeIDR / totalAssetsIDR) * 100.0
	}

	// Calculate allocation percentages
	for i := range allHoldings {
		h := &allHoldings[i]
		if h.ValueIDR != nil && totalAssetsIDR > 0 {
			pct := math.Round((*h.ValueIDR/totalAssetsIDR*100.0)*100) / 100
			h.AllocationPercentage = &pct
		}
	}

	// Sort category breakdown
	var byCategory []models.CategoryAllocation
	for _, c := range catMap {
		c.ValueIDR = math.Round(c.ValueIDR*100) / 100
		if totalAssetsIDR > 0 {
			c.Percentage = math.Round((c.ValueIDR/totalAssetsIDR*100.0)*100) / 100
		}
		byCategory = append(byCategory, *c)
	}
	sort.Slice(byCategory, func(i, j int) bool {
		return byCategory[i].ValueIDR > byCategory[j].ValueIDR
	})

	// Sort asset class breakdown
	var byAssetClass []models.AssetClassAllocation
	for _, ac := range acMap {
		ac.ValueIDR = math.Round(ac.ValueIDR*100) / 100
		if totalAssetsIDR > 0 {
			ac.Percentage = math.Round((ac.ValueIDR/totalAssetsIDR*100.0)*100) / 100
		}
		byAssetClass = append(byAssetClass, *ac)
	}
	sort.Slice(byAssetClass, func(i, j int) bool {
		return byAssetClass[i].ValueIDR > byAssetClass[j].ValueIDR
	})

	snapshot := &models.Snapshot{
		Metadata: models.Metadata{
			Date:         date,
			ExchangeRate: exchangeRate,
			TotalItems:   len(allHoldings),
			GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		},
		Totals: models.Totals{
			NetWorthIDR:           math.Round(netWorthIDR*100) / 100,
			NetWorthUSD:           math.Round((netWorthIDR/exchangeRate)*100) / 100,
			TotalAssetsIDR:        math.Round(totalAssetsIDR*100) / 100,
			TotalLiabilitiesIDR:   math.Round(totalLiabilitiesIDR*100) / 100,
			InvestmentsIDR:        math.Round(investmentsIDR*100) / 100,
			InvestmentsUSD:        math.Round((investmentsIDR/exchangeRate)*100) / 100,
			BankCashIDR:           math.Round(bankCashIDR*100) / 100,
			BankCashUSD:           math.Round((bankCashIDR/exchangeRate)*100) / 100,
			WeightedYieldPct:      math.Round(weightedYieldPct*100) / 100,
			AnnualYieldIncomeIDR:  math.Round(annualYieldIncomeIDR*100) / 100,
			MonthlyYieldIncomeIDR: math.Round((annualYieldIncomeIDR/12.0)*100) / 100,
		},
		Allocation: models.AllocationBreakdown{
			ByCategory:   byCategory,
			ByAssetClass: byAssetClass,
		},
		Holdings:           investmentsData,
		LiquidCashAccounts: liquidCashAccounts,
		AllHoldings:        allHoldings,
	}

	// 4. Save to JSON files
	_ = os.MkdirAll(dataDir, 0755)
	dateSnapshotPath := filepath.Join(dataDir, fmt.Sprintf("%s_snapshot.json", date))
	latestSnapshotPath := filepath.Join(dataDir, "latest_snapshot.json")

	snapBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(dateSnapshotPath, snapBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write date snapshot: %w", err)
	}
	if err := os.WriteFile(latestSnapshotPath, snapBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write latest snapshot: %w", err)
	}

	// 5. Upsert to SQLite
	if dbConn != nil {
		if err := db.UpsertSnapshot(dbConn, snapshot); err != nil {
			fmt.Printf("⚠️ Warning: Failed to upsert snapshot into SQLite: %v\n", err)
		}
	}

	return snapshot, nil
}
