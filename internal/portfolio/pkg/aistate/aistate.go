package aistate

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

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

// Default target asset allocation
var DefaultTargetAllocation = map[string]float64{
	"Fixed Income":       50.0,
	"Equities":           25.0,
	"Cash & Equivalents": 10.0,
	"Crypto":             10.0,
	"Commodities":        5.0,
}

var AssetClassYieldBenchmarks = map[string]float64{
	"Fixed Income":       0.065,
	"Equities":           0.035,
	"Crypto":             0.045,
	"Cash & Equivalents": 0.040,
	"Commodities":        0.0,
}

type MacroMetrics struct {
	NetWorthIDR         float64 `json:"net_worth_idr"`
	NetWorthUSD         float64 `json:"net_worth_usd"`
	TotalAssetsIDR      float64 `json:"total_assets_idr"`
	TotalLiabilitiesIDR float64 `json:"total_liabilities_idr"`
	MoMGrowthPct        float64 `json:"mom_growth_pct"`
	MoMGrowthIDR        float64 `json:"mom_growth_idr"`
	ComparisonPeriod    string  `json:"comparison_period"`
}

type AssetAllocationItem struct {
	AssetClass string  `json:"asset_class"`
	WeightPct  float64 `json:"weight_pct"`
	TargetPct  float64 `json:"target_pct"`
	DriftPct   float64 `json:"drift_pct"`
	ValueIDR   float64 `json:"value_idr"`
	ValueUSD   float64 `json:"value_usd"`
	Count      int     `json:"count"`
}

type CurrencyExposure struct {
	IDRPct    float64 `json:"idr_pct"`
	USDPct    float64 `json:"usd_pct"`
	CryptoPct float64 `json:"crypto_pct"`
	IDRVal    float64 `json:"idr_val"`
	USDVal    float64 `json:"usd_val"`
	CryptoVal float64 `json:"crypto_val"`
}

type Liquidity struct {
	LiquidCashIDR float64 `json:"liquid_cash_idr"`
	LiquidCashUSD float64 `json:"liquid_cash_usd"`
	LiquidCashPct float64 `json:"liquid_cash_pct"`
}

type PassiveIncomeBreakdown struct {
	AssetClass        string  `json:"asset_class"`
	ValueIDR          float64 `json:"value_idr"`
	AnnualIncomeIDR   float64 `json:"annual_income_idr"`
	MonthlyIncomeIDR  float64 `json:"monthly_income_idr"`
	EstimatedYieldPct float64 `json:"estimated_yield_pct"`
}

type PassiveIncome struct {
	ProjectedAnnualPassiveIncomeIDR  float64                  `json:"projected_annual_passive_income_idr"`
	ProjectedMonthlyPassiveIncomeIDR float64                  `json:"projected_monthly_passive_income_idr"`
	ProjectedMonthlyPassiveIncomeUSD float64                  `json:"projected_monthly_passive_income_usd"`
	FICoveragePct                    float64                  `json:"fi_coverage_pct"`
	FIStatus                         string                   `json:"fi_status"`
	Breakdown                        []PassiveIncomeBreakdown `json:"breakdown"`
}

type SukukItem struct {
	Asset            string  `json:"asset"`
	BondType         string  `json:"bond_type"`
	Category         string  `json:"category"`
	Frequency        string  `json:"frequency"`
	PayoutDay        int     `json:"payout_day"`
	AnnualCouponPct  float64 `json:"annual_coupon_pct"`
	PrincipalIDR     float64 `json:"principal_idr"`
	GrossMonthlyIDR  float64 `json:"gross_monthly_idr"`
	NetMonthlyIDR    float64 `json:"net_monthly_idr"`
	NetMonthlyUSD    float64 `json:"net_monthly_usd"`
	MaturityDate     string  `json:"maturity_date"`
	DaysToMaturity   int     `json:"days_to_maturity"`
	MonthsToMaturity float64 `json:"months_to_maturity"`
}

type MaturityHorizonItem struct {
	Asset               string  `json:"asset"`
	MaturityDate        string  `json:"maturity_date"`
	PrincipalReturnIDR  float64 `json:"principal_return_idr"`
	MonthsLeft          float64 `json:"months_left"`
	Year                string  `json:"year"`
}

type SukukSchedule struct {
	TotalPrincipalIDR     float64             `json:"total_principal_idr"`
	TotalGrossMonthlyIDR  float64             `json:"total_gross_monthly_idr"`
	TotalNetMonthlyIDR    float64             `json:"total_net_monthly_idr"`
	TotalNetMonthlyUSD    float64             `json:"total_net_monthly_usd"`
	Schedule              []SukukItem         `json:"schedule"`
	MaturityHorizon       []MaturityHorizonItem `json:"maturity_horizon"`
	PrincipalReturnByYear map[string]float64  `json:"principal_return_by_year"`
}

type AttributionAnalysis struct {
	HasHistory              bool    `json:"has_history"`
	PeriodStart             string  `json:"period_start,omitempty"`
	PeriodEnd               string  `json:"period_end,omitempty"`
	NetWorthStartIDR        float64 `json:"net_worth_start_idr,omitempty"`
	NetWorthEndIDR          float64 `json:"net_worth_end_idr,omitempty"`
	NetWorthDeltaIDR        float64 `json:"net_worth_delta_idr,omitempty"`
	NetWorthDeltaPct        float64 `json:"net_worth_delta_pct,omitempty"`
	InvestmentsDeltaIDR     float64 `json:"investments_delta_idr,omitempty"`
	BankCashDeltaIDR        float64 `json:"bank_cash_delta_idr,omitempty"`
	MarketVsCashCommentary  string  `json:"market_vs_cash_commentary,omitempty"`
}

type RebalanceItem struct {
	AssetClass             string  `json:"asset_class"`
	CurrentWeightPct       float64 `json:"current_weight_pct"`
	TargetWeightPct        float64 `json:"target_weight_pct"`
	DriftPct               float64 `json:"drift_pct"`
	DepositAllocationIDR   float64 `json:"deposit_allocation_idr"`
	DepositAllocationUSD   float64 `json:"deposit_allocation_usd"`
	AllocationPctOfDeposit float64 `json:"allocation_pct_of_deposit"`
	ActionDirective        string  `json:"action_directive"`
}

type RebalancingPlan struct {
	DepositAmountIDR    float64         `json:"deposit_amount_idr"`
	DepositAmountUSD    float64         `json:"deposit_amount_usd"`
	PostDepositTotalIDR float64         `json:"post_deposit_total_idr"`
	Recommendations     []RebalanceItem `json:"recommendations"`
}

type DustHolding struct {
	Asset     string  `json:"asset"`
	Category  string  `json:"category"`
	Source    string  `json:"source"`
	ValueIDR  float64 `json:"value_idr"`
	WeightPct float64 `json:"weight_pct"`
}

type ClutterAudit struct {
	ClutterScore        float64       `json:"clutter_score"`
	ClutterRating       string        `json:"clutter_rating"`
	FragmentedCount     int           `json:"fragmented_count"`
	FragmentedTotalIDR  float64       `json:"fragmented_total_idr"`
	FragmentedWeightPct float64       `json:"fragmented_weight_pct"`
	DustHoldings        []DustHolding `json:"dust_holdings"`
	Directives          []string      `json:"directives"`
}

type SBNReinvestmentPlaybook struct {
	HasUpcomingMaturity bool     `json:"has_upcoming_maturity"`
	MaturingAsset       string   `json:"maturing_asset,omitempty"`
	MaturityDate        string   `json:"maturity_date,omitempty"`
	DaysLeft            int      `json:"days_left,omitempty"`
	PrincipalIDR        float64  `json:"principal_idr,omitempty"`
	LostMonthlyIDR      float64  `json:"lost_monthly_idr,omitempty"`
	ActionPlan          []string `json:"action_plan,omitempty"`
}

type VaultSummary struct {
	LockedVaultIDR       float64 `json:"locked_vault_idr"`
	LockedVaultPct       float64 `json:"locked_vault_pct"`
	ActionableCapitalIDR float64 `json:"actionable_capital_idr"`
	ActionableCapitalPct float64 `json:"actionable_capital_pct"`
	CalmnessDirective    string  `json:"calmness_directive"`
}

type DepositRouter struct {
	OneStepAction      string  `json:"one_step_action"`
	PriorityAssetClass string  `json:"priority_asset_class"`
	DepositAmountIDR   float64 `json:"deposit_amount_idr"`
}

type ADHDFocusMetrics struct {
	ClutterAudit            ClutterAudit            `json:"clutter_audit"`
	SBNReinvestmentPlaybook SBNReinvestmentPlaybook `json:"sbn_reinvestment_playbook"`
	VaultSummary            VaultSummary            `json:"vault_summary"`
	DepositRouter           DepositRouter           `json:"deposit_router"`
}

type AIState struct {
	StateDate              string              `json:"state_date"`
	ExchangeRate           float64             `json:"exchange_rate"`
	MacroMetrics           MacroMetrics        `json:"macro_metrics"`
	AssetAllocation        []AssetAllocationItem `json:"asset_allocation"`
	CurrencyExposure       CurrencyExposure    `json:"currency_exposure"`
	Liquidity              Liquidity           `json:"liquidity"`
	PassiveIncome          PassiveIncome       `json:"passive_income"`
	SukukAndCouponSchedule SukukSchedule       `json:"sukuk_and_coupon_schedule"`
	AttributionAnalysis    AttributionAnalysis `json:"attribution_analysis"`
	RebalancingPlan        RebalancingPlan     `json:"rebalancing_plan"`
	ADHDFocusMetrics       ADHDFocusMetrics    `json:"adhd_focus_metrics"`
}

// GenerateAIState processes a snapshot and returns the full AI State representation.
func GenerateAIState(snap *models.Snapshot, dbConn *sql.DB) (*AIState, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot is nil")
	}

	date := snap.Metadata.Date
	rate := snap.Metadata.ExchangeRate
	if rate <= 0 {
		rate = 17500.0
	}
	totalAssets := snap.Totals.TotalAssetsIDR
	netWorth := snap.Totals.NetWorthIDR

	// 1. Asset Allocation & Drift
	var allocItems []AssetAllocationItem
	for _, ac := range snap.Allocation.ByAssetClass {
		target := DefaultTargetAllocation[ac.AssetClass]
		drift := ac.Percentage - target
		allocItems = append(allocItems, AssetAllocationItem{
			AssetClass: ac.AssetClass,
			WeightPct:  math.Round(ac.Percentage*100) / 100,
			TargetPct:  target,
			DriftPct:   math.Round(drift*10) / 10,
			ValueIDR:   ac.ValueIDR,
			ValueUSD:   math.Round((ac.ValueIDR/rate)*100) / 100,
			Count:      ac.Count,
		})
	}
	// Check for target asset classes not currently held
	for targetClass, targetPct := range DefaultTargetAllocation {
		found := false
		for _, item := range allocItems {
			if item.AssetClass == targetClass {
				found = true
				break
			}
		}
		if !found {
			allocItems = append(allocItems, AssetAllocationItem{
				AssetClass: targetClass,
				WeightPct:  0.0,
				TargetPct:  targetPct,
				DriftPct:   -targetPct,
				ValueIDR:   0.0,
				ValueUSD:   0.0,
				Count:      0,
			})
		}
	}
	sort.Slice(allocItems, func(i, j int) bool {
		return allocItems[i].AssetClass < allocItems[j].AssetClass
	})

	// 2. Currency Exposure
	var idrVal, usdVal, cryptoVal float64
	for _, h := range snap.AllHoldings {
		val := 0.0
		if h.ValueIDR != nil {
			val = *h.ValueIDR
		}
		curr := strings.ToUpper(h.Currency)
		if curr == "IDR" {
			idrVal += val
		} else if models.StableCoins[h.Asset] {
			usdVal += val
		} else if h.AssetClass == "Crypto" {
			cryptoVal += val
		} else if curr == "USD" {
			usdVal += val
		} else {
			idrVal += val
		}
	}
	currExp := CurrencyExposure{
		IDRVal:    math.Round(idrVal*100) / 100,
		USDVal:    math.Round(usdVal*100) / 100,
		CryptoVal: math.Round(cryptoVal*100) / 100,
	}
	if totalAssets > 0 {
		currExp.IDRPct = math.Round((idrVal/totalAssets*100.0)*10) / 10
		currExp.USDPct = math.Round((usdVal/totalAssets*100.0)*10) / 10
		currExp.CryptoPct = math.Round((cryptoVal/totalAssets*100.0)*10) / 10
	}

	// 3. Liquidity
	bankCash := snap.Totals.BankCashIDR
	liqPct := 0.0
	if totalAssets > 0 {
		liqPct = math.Round((bankCash/totalAssets*100.0)*10) / 10
	}
	liquidity := Liquidity{
		LiquidCashIDR: bankCash,
		LiquidCashUSD: math.Round((bankCash/rate)*100) / 100,
		LiquidCashPct: liqPct,
	}

	// 4. Passive Income
	var breakdown []PassiveIncomeBreakdown
	var totalAnnualPassiveIDR float64
	for _, ac := range snap.Allocation.ByAssetClass {
		benchRate := AssetClassYieldBenchmarks[ac.AssetClass]
		annualInc := ac.ValueIDR * benchRate
		totalAnnualPassiveIDR += annualInc
		breakdown = append(breakdown, PassiveIncomeBreakdown{
			AssetClass:        ac.AssetClass,
			ValueIDR:          ac.ValueIDR,
			AnnualIncomeIDR:   math.Round(annualInc),
			MonthlyIncomeIDR:  math.Round(annualInc / 12.0),
			EstimatedYieldPct: math.Round(benchRate * 1000) / 10,
		})
	}
	monthlyPassiveIDR := totalAnnualPassiveIDR / 12.0
	burnIDR := 5000000.0 // Baseline conservative monthly burn
	fiCoverage := math.Round((monthlyPassiveIDR/burnIDR*100.0)*10) / 10
	fiStatus := "Accumulation (<25%)"
	if fiCoverage >= 100.0 {
		fiStatus = "Full Financial Independence (>=100%)"
	} else if fiCoverage >= 75.0 {
		fiStatus = "Near FI (75-99%)"
	} else if fiCoverage >= 50.0 {
		fiStatus = "Coast / Halfway FI (50-74%)"
	} else if fiCoverage >= 25.0 {
		fiStatus = "Emerging FI Buffer (25-49%)"
	}

	passive := PassiveIncome{
		ProjectedAnnualPassiveIncomeIDR:  math.Round(totalAnnualPassiveIDR*100) / 100,
		ProjectedMonthlyPassiveIncomeIDR: math.Round(monthlyPassiveIDR*100) / 100,
		ProjectedMonthlyPassiveIncomeUSD: math.Round((monthlyPassiveIDR/rate)*100) / 100,
		FICoveragePct:                    fiCoverage,
		FIStatus:                         fiStatus,
		Breakdown:                        breakdown,
	}

	// 5. Sukuk Schedule & Maturity Horizon
	var sukukItems []SukukItem
	var maturityHorizon []MaturityHorizonItem
	byYear := make(map[string]float64)
	var totalPrincipal, totalGrossMonthly, totalNetMonthly float64

	now, _ := time.Parse("2006-01-02", date)
	if now.IsZero() {
		now = time.Now()
	}

	sukukMeta := map[string]struct {
		rate     float64
		maturity string
		bType    string
	}{
		"ST010T4":          {0.0640, "2027-05-10", "Sukuk Tabungan 4-Yr"},
		"ST012T4":          {0.0655, "2028-05-10", "Sukuk Tabungan 4-Yr"},
		"ST013T2":          {0.0640, "2026-11-10", "Sukuk Tabungan 2-Yr"},
		"ST014T2":          {0.0640, "2027-05-10", "Sukuk Tabungan 2-Yr"},
		"DX002ETCD0BN0101": {0.0625, "2026-12-15", "Corporate Bond / Sukuk"},
	}

	for _, h := range snap.AllHoldings {
		cat := h.Category
		assetUpper := strings.ToUpper(h.Asset)
		if cat == "SBN" || cat == "Corporate Bond" || strings.Contains(cat, "Bond") || strings.Contains(assetUpper, "SUKUK") {
			val := 0.0
			if h.ValueIDR != nil {
				val = *h.ValueIDR
			}
			matchedRate := 0.0625
			matchedMaturity := "2027-12-31"
			bType := "Fixed Income Bond"

			for code, meta := range sukukMeta {
				if strings.Contains(assetUpper, code) {
					matchedRate = meta.rate
					matchedMaturity = meta.maturity
					bType = meta.bType
					break
				}
			}

			grossMonthly := (val * matchedRate) / 12.0
			netMonthly := grossMonthly * 0.90 // 10% final tax

			matTime, err := time.Parse("2006-01-02", matchedMaturity)
			daysLeft := 365
			monthsLeft := 12.0
			if err == nil {
				daysLeft = int(matTime.Sub(now).Hours() / 24)
				if daysLeft < 0 {
					daysLeft = 0
				}
				monthsLeft = math.Round((float64(daysLeft)/30.44)*10) / 10
			}

			sItem := SukukItem{
				Asset:            h.Asset,
				BondType:         bType,
				Category:         cat,
				Frequency:        "Monthly (Every 10th)",
				PayoutDay:        10,
				AnnualCouponPct:  math.Round(matchedRate*10000) / 100,
				PrincipalIDR:     val,
				GrossMonthlyIDR:  math.Round(grossMonthly),
				NetMonthlyIDR:    math.Round(netMonthly),
				NetMonthlyUSD:    math.Round((netMonthly/rate)*100) / 100,
				MaturityDate:     matchedMaturity,
				DaysToMaturity:   daysLeft,
				MonthsToMaturity: monthsLeft,
			}
			sukukItems = append(sukukItems, sItem)

			yr := strings.Split(matchedMaturity, "-")[0]
			maturityHorizon = append(maturityHorizon, MaturityHorizonItem{
				Asset:              h.Asset,
				MaturityDate:       matchedMaturity,
				PrincipalReturnIDR: val,
				MonthsLeft:         monthsLeft,
				Year:               yr,
			})
			byYear[yr] += val

			totalPrincipal += val
			totalGrossMonthly += grossMonthly
			totalNetMonthly += netMonthly
		}
	}

	sort.Slice(sukukItems, func(i, j int) bool {
		return sukukItems[i].PrincipalIDR > sukukItems[j].PrincipalIDR
	})
	sort.Slice(maturityHorizon, func(i, j int) bool {
		return maturityHorizon[i].MaturityDate < maturityHorizon[j].MaturityDate
	})

	sukukSched := SukukSchedule{
		TotalPrincipalIDR:     totalPrincipal,
		TotalGrossMonthlyIDR:  math.Round(totalGrossMonthly),
		TotalNetMonthlyIDR:    math.Round(totalNetMonthly),
		TotalNetMonthlyUSD:    math.Round((totalNetMonthly/rate)*100) / 100,
		Schedule:              sukukItems,
		MaturityHorizon:       maturityHorizon,
		PrincipalReturnByYear: byYear,
	}

	// 6. Attribution Analysis (from SQLite history if available)
	attribution := AttributionAnalysis{HasHistory: false}
	var momGrowthPct float64
	var momGrowthIDR float64
	var compPeriod string

	if dbConn != nil {
		rows, err := dbConn.Query("SELECT date, net_worth_idr, investments_idr, bank_cash_idr FROM snapshots ORDER BY date ASC")
		if err == nil {
			defer rows.Close()
			type histSnap struct {
				date        string
				netWorthIDR float64
				invIDR      float64
				cashIDR     float64
			}
			var history []histSnap
			for rows.Next() {
				var hs histSnap
				if err := rows.Scan(&hs.date, &hs.netWorthIDR, &hs.invIDR, &hs.cashIDR); err == nil {
					history = append(history, hs)
				}
			}

			if len(history) >= 2 {
				// Oldest and prior
				oldest := history[0]
				prior := history[len(history)-2]
				current := history[len(history)-1]

				deltaNW := current.netWorthIDR - oldest.netWorthIDR
				deltaPct := 0.0
				if oldest.netWorthIDR > 0 {
					deltaPct = math.Round((deltaNW/oldest.netWorthIDR*100.0)*100) / 100
				}
				deltaInv := current.invIDR - oldest.invIDR
				deltaCash := current.cashIDR - oldest.cashIDR

				attribution = AttributionAnalysis{
					HasHistory:             true,
					PeriodStart:            oldest.date,
					PeriodEnd:              current.date,
					NetWorthStartIDR:       oldest.netWorthIDR,
					NetWorthEndIDR:         current.netWorthIDR,
					NetWorthDeltaIDR:       deltaNW,
					NetWorthDeltaPct:       deltaPct,
					InvestmentsDeltaIDR:    deltaInv,
					BankCashDeltaIDR:       deltaCash,
					MarketVsCashCommentary: fmt.Sprintf("Grew by Rp %s (+%.1f%%) over period.", formatNumber(deltaNW), deltaPct),
				}

				// MoM
				momGrowthIDR = current.netWorthIDR - prior.netWorthIDR
				if prior.netWorthIDR > 0 {
					momGrowthPct = math.Round((momGrowthIDR/prior.netWorthIDR*100.0)*100) / 100
				}
				compPeriod = fmt.Sprintf("%s → %s", prior.date, current.date)
			}
		}
	}

	macroMetrics := MacroMetrics{
		NetWorthIDR:         netWorth,
		NetWorthUSD:         math.Round((netWorth/rate)*100) / 100,
		TotalAssetsIDR:      totalAssets,
		TotalLiabilitiesIDR: snap.Totals.TotalLiabilitiesIDR,
		MoMGrowthPct:        momGrowthPct,
		MoMGrowthIDR:        momGrowthIDR,
		ComparisonPeriod:    compPeriod,
	}

	// 7. Rebalancing Plan (Monthly Deposit Simulation e.g. Rp 5,000,000)
	monthlyDeposit := 5000000.0
	postTotal := netWorth + monthlyDeposit
	var underweighted []struct {
		acClass   string
		curWeight float64
		tgtWeight float64
		deficit   float64
	}
	var totalDeficit float64
	for _, item := range allocItems {
		if item.DriftPct < 0 {
			def := math.Abs(item.DriftPct)
			totalDeficit += def
			underweighted = append(underweighted, struct {
				acClass   string
				curWeight float64
				tgtWeight float64
				deficit   float64
			}{item.AssetClass, item.WeightPct, item.TargetPct, def})
		}
	}

	var rebalanceItems []RebalanceItem
	for _, u := range underweighted {
		allocFraction := u.deficit / totalDeficit
		allocAmount := math.Round(monthlyDeposit * allocFraction)
		directive := fmt.Sprintf("DCA into %s to close %.1f%% underweight gap", u.acClass, u.deficit)
		if u.acClass == "Equities" {
			directive = "Accumulate Indo Value/Dividend Stocks or S&P 500 Index Funds"
		} else if u.acClass == "Commodities" {
			directive = "Buy Physical Gold / Gold Stablecoins (PAXG)"
		} else if u.acClass == "Crypto" {
			directive = "DCA into Bluechip Crypto (BTC / ETH / SOL)"
		} else if u.acClass == "Fixed Income" {
			directive = "DCA into Government Bonds (SBN / FR / ORI) or Corporate Bonds"
		}

		rebalanceItems = append(rebalanceItems, RebalanceItem{
			AssetClass:             u.acClass,
			CurrentWeightPct:       u.curWeight,
			TargetWeightPct:        u.tgtWeight,
			DriftPct:               -u.deficit,
			DepositAllocationIDR:   allocAmount,
			DepositAllocationUSD:   math.Round((allocAmount/rate)*100) / 100,
			AllocationPctOfDeposit: math.Round(allocFraction*1000) / 10,
			ActionDirective:        directive,
		})
	}
	sort.Slice(rebalanceItems, func(i, j int) bool {
		return rebalanceItems[i].DepositAllocationIDR > rebalanceItems[j].DepositAllocationIDR
	})

	rebalPlan := RebalancingPlan{
		DepositAmountIDR:    monthlyDeposit,
		DepositAmountUSD:    math.Round((monthlyDeposit/rate)*100) / 100,
		PostDepositTotalIDR: postTotal,
		Recommendations:     rebalanceItems,
	}

	// 8. ADHD Focus Suite
	// A. Clutter & Dust Sweeper (< 1% NW)
	var dustList []DustHolding
	var dustTotalIDR float64
	for _, h := range snap.AllHoldings {
		val := 0.0
		if h.ValueIDR != nil {
			val = *h.ValueIDR
		}
		if val > 0 && netWorth > 0 {
			weight := (val / netWorth) * 100.0
			if weight < 1.0 {
				dustTotalIDR += val
				dustList = append(dustList, DustHolding{
					Asset:     h.Asset,
					Category:  h.Category,
					Source:    h.Source,
					ValueIDR:  val,
					WeightPct: math.Round(weight*100) / 100,
				})
			}
		}
	}
	sort.Slice(dustList, func(i, j int) bool {
		return dustList[i].ValueIDR > dustList[j].ValueIDR
	})

	var directives []string
	clutterScore := math.Min(100.0, float64(len(dustList))*5.0)
	clutterRating := "Low Cognitive Load"
	if clutterScore >= 40 {
		clutterRating = "High Cognitive Load"
	} else if clutterScore >= 20 {
		clutterRating = "Moderate Cognitive Load"
	}

	if len(dustList) > 0 {
		directives = append(directives, fmt.Sprintf("Consolidate %d micro-holdings totaling Rp %s to simplify monitoring.", len(dustList), formatNumber(dustTotalIDR)))
		directives = append(directives, "Sweep small wallet tokens (<1% NW) into primary ETH/BTC or convert to liquid cash.")
		directives = append(directives, "Liquidate micro equity positions under 1% weight into core dividend/index anchors.")
	}

	clutterAudit := ClutterAudit{
		ClutterScore:        clutterScore,
		ClutterRating:       clutterRating,
		FragmentedCount:     len(dustList),
		FragmentedTotalIDR:  dustTotalIDR,
		FragmentedWeightPct: math.Round((dustTotalIDR/netWorth*100.0)*100) / 100,
		DustHoldings:        dustList,
		Directives:          directives,
	}

	// B. SBN Reinvestment Playbook
	var sbnPlaybook SBNReinvestmentPlaybook
	if len(maturityHorizon) > 0 {
		first := maturityHorizon[0]
		if first.MonthsLeft <= 3.0 {
			halfPrinc := math.Round(first.PrincipalReturnIDR / 2.0)
			sbnPlaybook = SBNReinvestmentPlaybook{
				HasUpcomingMaturity: true,
				MaturingAsset:       first.Asset,
				MaturityDate:        first.MaturityDate,
				DaysLeft:            int(first.MonthsLeft * 30.44),
				PrincipalIDR:        first.PrincipalReturnIDR,
				LostMonthlyIDR:      math.Round((first.PrincipalReturnIDR * 0.064 * 0.90) / 12.0),
				ActionPlan: []string{
					fmt.Sprintf("Pre-commit Rollover: On %s, allocate Rp %s into next sovereign Sukuk/SBN (e.g. ST/SR) to lock in ~6.4%%+ yield.", first.MaturityDate, formatNumber(halfPrinc)),
					fmt.Sprintf("Equities Rebalance: Deploy remaining Rp %s into SRI-KEHATI / BBCA to automatically eliminate equity drift in 1 transaction.", formatNumber(halfPrinc)),
					"Automate Calendar Reminder: Set alert for 7 days prior to maturity to prevent cash sitting idle in checking.",
				},
			}
		}
	}

	// C. Autonomous Vault Summary
	lockedVaultIDR := totalPrincipal
	lockedPct := 0.0
	actionableIDR := netWorth - lockedVaultIDR
	actionablePct := 0.0
	if netWorth > 0 {
		lockedPct = math.Round((lockedVaultIDR/netWorth*100.0)*10) / 10
		actionablePct = math.Round((actionableIDR/netWorth*100.0)*10) / 10
	}
	vaultSummary := VaultSummary{
		LockedVaultIDR:       lockedVaultIDR,
		LockedVaultPct:       lockedPct,
		ActionableCapitalIDR: actionableIDR,
		ActionableCapitalPct: actionablePct,
		CalmnessDirective:    fmt.Sprintf("Rp %s (%.1f%%) is locked in sovereign Sukuk/Vaults compounding autonomously. Do not check or touch daily.", formatNumber(lockedVaultIDR), lockedPct),
	}

	// D. Deposit Router
	oneStepAction := "Deposit Rp 5,000,000 into Cash & Equivalents reserve."
	priorityAC := "Cash & Equivalents"
	if len(rebalanceItems) > 0 {
		priorityAC = rebalanceItems[0].AssetClass
		oneStepAction = fmt.Sprintf("Deposit Rp %s into %s (%s). Currently %.1f%% underweight. Zero debate.",
			formatNumber(monthlyDeposit), priorityAC, rebalanceItems[0].ActionDirective, math.Abs(rebalanceItems[0].DriftPct))
	}
	depositRouter := DepositRouter{
		OneStepAction:      oneStepAction,
		PriorityAssetClass: priorityAC,
		DepositAmountIDR:   monthlyDeposit,
	}

	adhdMetrics := ADHDFocusMetrics{
		ClutterAudit:            clutterAudit,
		SBNReinvestmentPlaybook: sbnPlaybook,
		VaultSummary:            vaultSummary,
		DepositRouter:           depositRouter,
	}

	state := &AIState{
		StateDate:              date,
		ExchangeRate:           rate,
		MacroMetrics:           macroMetrics,
		AssetAllocation:        allocItems,
		CurrencyExposure:       currExp,
		Liquidity:              liquidity,
		PassiveIncome:          passive,
		SukukAndCouponSchedule: sukukSched,
		AttributionAnalysis:    attribution,
		RebalancingPlan:        rebalPlan,
		ADHDFocusMetrics:       adhdMetrics,
	}

	return state, nil
}

// GenerateAIDigestMarkdown formats the AIState into a token-optimized markdown document.
func GenerateAIDigestMarkdown(state *AIState) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Financial Portfolio AI State Brief — %s\n", state.StateDate))
	sb.WriteString(fmt.Sprintf("**Exchange Rate**: 1 USD = Rp %s | **Generated for**: Autonomous AI Advisor Review\n\n", formatNumber(state.ExchangeRate)))

	// 1. Executive Macro Overview
	sb.WriteString("## 1. Executive Macro Overview\n")
	sb.WriteString(fmt.Sprintf("- **Net Worth**: **Rp %s** ($%.2f)\n", formatNumber(state.MacroMetrics.NetWorthIDR), state.MacroMetrics.NetWorthUSD))
	if state.MacroMetrics.ComparisonPeriod != "" {
		sign := "+"
		if state.MacroMetrics.MoMGrowthIDR < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("- **Period Change**: **%s%.2f%% (Rp %s%s)** (%s)\n",
			sign, state.MacroMetrics.MoMGrowthPct, sign, formatNumber(state.MacroMetrics.MoMGrowthIDR), state.MacroMetrics.ComparisonPeriod))
	}
	sb.WriteString(fmt.Sprintf("- **Total Assets**: Rp %s\n", formatNumber(state.MacroMetrics.TotalAssetsIDR)))
	sb.WriteString(fmt.Sprintf("- **Liquid Cash Reserve**: Rp %s (%.1f%% of portfolio)\n", formatNumber(state.Liquidity.LiquidCashIDR), state.Liquidity.LiquidCashPct))
	sb.WriteString(fmt.Sprintf("- **Autonomous Vault (Locked)**: Rp %s (%.1f%%) | **Actionable Liquid**: Rp %s (%.1f%%)\n",
		formatNumber(state.ADHDFocusMetrics.VaultSummary.LockedVaultIDR), state.ADHDFocusMetrics.VaultSummary.LockedVaultPct,
		formatNumber(state.ADHDFocusMetrics.VaultSummary.ActionableCapitalIDR), state.ADHDFocusMetrics.VaultSummary.ActionableCapitalPct))
	sb.WriteString(fmt.Sprintf("- 🧘 **Cognitive Calmer**: %s\n", state.ADHDFocusMetrics.VaultSummary.CalmnessDirective))
	if state.AttributionAnalysis.HasHistory {
		sb.WriteString(fmt.Sprintf("- **Period Attribution (%s → %s)**: %s\n",
			state.AttributionAnalysis.PeriodStart, state.AttributionAnalysis.PeriodEnd, state.AttributionAnalysis.MarketVsCashCommentary))
	}
	sb.WriteString("\n")

	// 2. Asset Allocation & Target Drift
	sb.WriteString("## 2. Asset Allocation & Target Drift\n")
	sb.WriteString("| Asset Class | Current Value (IDR) | Weight (%) | Target (%) | Drift (%) |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	for _, ac := range state.AssetAllocation {
		driftSign := "+"
		if ac.DriftPct < 0 {
			driftSign = ""
		}
		sb.WriteString(fmt.Sprintf("| **%s** | Rp %s | %.1f%% | %.1f%% | `%s%.1f%%` |\n",
			ac.AssetClass, formatNumber(ac.ValueIDR), ac.WeightPct, ac.TargetPct, driftSign, ac.DriftPct))
	}
	sb.WriteString("\n")

	// 3. Projected Passive Cashflow & FI Status
	sb.WriteString("## 3. Projected Passive Cashflow & FI Status\n")
	sb.WriteString(fmt.Sprintf("- **Projected Annual Yield**: **Rp %s**\n", formatNumber(state.PassiveIncome.ProjectedAnnualPassiveIncomeIDR)))
	sb.WriteString(fmt.Sprintf("- **Projected Monthly Passive Cashflow**: **Rp %s** ($%.2f/mo)\n",
		formatNumber(state.PassiveIncome.ProjectedMonthlyPassiveIncomeIDR), state.PassiveIncome.ProjectedMonthlyPassiveIncomeUSD))
	sb.WriteString(fmt.Sprintf("- **Financial Independence Coverage**: **%.1f%%** of living burn covered (*%s*)\n\n",
		state.PassiveIncome.FICoveragePct, state.PassiveIncome.FIStatus))

	if len(state.SukukAndCouponSchedule.Schedule) > 0 {
		sb.WriteString("### 3.5 Guaranteed SBN Sukuk Monthly Coupons (Payout: Every 10th)\n")
		sb.WriteString(fmt.Sprintf("- **Total SBN Principal**: Rp %s\n", formatNumber(state.SukukAndCouponSchedule.TotalPrincipalIDR)))
		sb.WriteString(fmt.Sprintf("- **Net Monthly Coupon Inflow**: **Rp %s** ($%.2f/mo) *(After 10%% PPh Final)*\n\n",
			formatNumber(state.SukukAndCouponSchedule.TotalNetMonthlyIDR), state.SukukAndCouponSchedule.TotalNetMonthlyUSD))

		sb.WriteString("| Sukuk Seri | Principal | Annual Coupon | Net Monthly Payout | Maturity Date | Horizon |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, s := range state.SukukAndCouponSchedule.Schedule {
			sb.WriteString(fmt.Sprintf("| **%s** | Rp %s | %.2f%% | **Rp %s**/mo | %s | %.1f mo |\n",
				s.Asset, formatNumber(s.PrincipalIDR), s.AnnualCouponPct, formatNumber(s.NetMonthlyIDR), s.MaturityDate, s.MonthsToMaturity))
		}
		sb.WriteString("\n")
	}

	if state.ADHDFocusMetrics.SBNReinvestmentPlaybook.HasUpcomingMaturity {
		play := state.ADHDFocusMetrics.SBNReinvestmentPlaybook
		sb.WriteString("### 3.6 🎯 Autopilot SBN Reinvestment Playbook (Decision Fatigue Shield)\n")
		sb.WriteString(fmt.Sprintf("> ⚠️ **Maturing Asset**: **%s** (Rp %s) matures on **%s** (%d days left).\n",
			play.MaturingAsset, formatNumber(play.PrincipalIDR), play.MaturityDate, play.DaysLeft))
		sb.WriteString(fmt.Sprintf("> 📉 **Lost Cashflow upon Maturity**: -Rp %s/mo net coupon.\n", formatNumber(play.LostMonthlyIDR)))
		sb.WriteString("> **Pre-Committed Action Plan:**\n")
		for _, step := range play.ActionPlan {
			sb.WriteString(fmt.Sprintf("> • %s\n", step))
		}
		sb.WriteString("\n")
	}

	// 4. Currency Exposure
	sb.WriteString("## 4. Currency Exposure\n")
	sb.WriteString(fmt.Sprintf("- **IDR Assets**: %.1f%% (Rp %s)\n", state.CurrencyExposure.IDRPct, formatNumber(state.CurrencyExposure.IDRVal)))
	sb.WriteString(fmt.Sprintf("- **USD / Stablecoins**: %.1f%% (Rp %s)\n", state.CurrencyExposure.USDPct, formatNumber(state.CurrencyExposure.USDVal)))
	sb.WriteString(fmt.Sprintf("- **Native Crypto**: %.1f%% (Rp %s)\n\n", state.CurrencyExposure.CryptoPct, formatNumber(state.CurrencyExposure.CryptoVal)))

	// 5. Deposit-Only Rebalancing Guideline
	sb.WriteString(fmt.Sprintf("## 5. Deposit-Only Rebalancing Guideline (Based on Rp %s DCA)\n", formatNumber(state.RebalancingPlan.DepositAmountIDR)))
	sb.WriteString(fmt.Sprintf("> ⚡ **Zero-Brain Next Action**: %s\n\n", state.ADHDFocusMetrics.DepositRouter.OneStepAction))

	sb.WriteString("| Asset Class | Current Weight | Target Weight | Drift | Suggested Allocation | Action |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, r := range state.RebalancingPlan.Recommendations {
		sb.WriteString(fmt.Sprintf("| **%s** | %.1f%% | %.1f%% | `%.1f%%` | **Rp %s** (%.1f%%) | %s |\n",
			r.AssetClass, r.CurrentWeightPct, r.TargetWeightPct, r.DriftPct, formatNumber(r.DepositAllocationIDR), r.AllocationPctOfDeposit, r.ActionDirective))
	}
	sb.WriteString("\n")

	// 6. ADHD Clutter & Dust Sweeper Audit
	sb.WriteString("## 6. 🧹 ADHD Asset Clutter & Dust Sweeper Audit\n")
	sb.WriteString(fmt.Sprintf("- **Portfolio Clutter Score**: **%.1f/100** (%s)\n",
		state.ADHDFocusMetrics.ClutterAudit.ClutterScore, state.ADHDFocusMetrics.ClutterAudit.ClutterRating))
	sb.WriteString(fmt.Sprintf("- **Fragmented Micro-Holdings**: **%d assets** totaling Rp %s (%.2f%% of portfolio)\n\n",
		state.ADHDFocusMetrics.ClutterAudit.FragmentedCount, formatNumber(state.ADHDFocusMetrics.ClutterAudit.FragmentedTotalIDR), state.ADHDFocusMetrics.ClutterAudit.FragmentedWeightPct))
	if len(state.ADHDFocusMetrics.ClutterAudit.Directives) > 0 {
		sb.WriteString("**Actionable Consolidation Directives:**\n")
		for _, d := range state.ADHDFocusMetrics.ClutterAudit.Directives {
			sb.WriteString(fmt.Sprintf("  • %s\n", d))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SaveAIState writes latest_ai_state.json, latest_ai_digest.md and saves to SQLite.
func SaveAIState(state *AIState, digestMD string, dataDir string, dbConn *sql.DB) error {
	_ = os.MkdirAll(dataDir, 0755)

	stateBytes, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ai state: %w", err)
	}

	stateFile := filepath.Join(dataDir, "latest_ai_state.json")
	if err := os.WriteFile(stateFile, stateBytes, 0644); err != nil {
		return fmt.Errorf("failed to write latest_ai_state.json: %w", err)
	}

	digestFile := filepath.Join(dataDir, "latest_ai_digest.md")
	if err := os.WriteFile(digestFile, []byte(digestMD), 0644); err != nil {
		return fmt.Errorf("failed to write latest_ai_digest.md: %w", err)
	}

	// Also write dated files
	dateStateFile := filepath.Join(dataDir, fmt.Sprintf("%s_ai_state.json", state.StateDate))
	_ = os.WriteFile(dateStateFile, stateBytes, 0644)

	// Save to SQLite
	if dbConn != nil {
		query := `
		INSERT INTO ai_states (snapshot_date, state_json, digest_md, generated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(snapshot_date) DO UPDATE SET
			state_json=excluded.state_json,
			digest_md=excluded.digest_md,
			generated_at=excluded.generated_at;
		`
		_, err := dbConn.Exec(query, state.StateDate, string(stateBytes), digestMD, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			fmt.Printf("⚠️ Warning: Failed to upsert ai_state into SQLite: %v\n", err)
		}
	}

	return nil
}

func formatNumber(v float64) string {
	return models.FormatNumber(v)
}
