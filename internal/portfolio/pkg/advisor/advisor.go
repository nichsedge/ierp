package advisor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/aistate"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

type RunwayTier struct {
	Name          string  `json:"name"`
	TargetMonths  int     `json:"target_months,omitempty"`
	TargetIDR     float64 `json:"target_idr,omitempty"`
	AllocatedIDR  float64 `json:"allocated_idr"`
	AllocationPct float64 `json:"allocation_pct"`
	Vehicle       string  `json:"vehicle"`
	Mandate       string  `json:"mandate"`
}

type SovereignAllocationPlan struct {
	LiquidCash             float64    `json:"liquid_cash"`
	MonthlyBurn            float64    `json:"monthly_burn"`
	RunwayMonths           float64    `json:"runway_months"`
	Status                 string     `json:"status"`
	Tier1OperatingReserve  RunwayTier `json:"tier1_operating_reserve"`
	Tier2FortressBuffer    RunwayTier `json:"tier2_fortress_buffer"`
	Tier3DeployableSurplus RunwayTier `json:"tier3_deployable_surplus"`
	TacticalAdvice         string     `json:"tactical_advice"`
}

type EquityVerdict struct {
	Ticker          string  `json:"ticker"`
	ValueIDR        float64 `json:"value_idr"`
	WeightPct       float64 `json:"weight_pct"`
	Verdict         string  `json:"verdict"`
	TrendRegime     string  `json:"trend_regime"`
	NFF20D          float64 `json:"nff_20d,omitempty"`
	WyckoffPhase    string  `json:"wyckoff_phase,omitempty"`
	SmartMoneyDelta float64 `json:"smart_money_delta,omitempty"`
	CompounderScore float64 `json:"compounder_score,omitempty"`
}

type AdvisorPayload struct {
	Date                    string                  `json:"date"`
	NetWorthIDR             float64                 `json:"net_worth_idr"`
	DryPowderIDR            float64                 `json:"dry_powder_idr"`
	DryPowderPct            float64                 `json:"dry_powder_pct"`
	RunwayMonths            float64                 `json:"runway_months"`
	RunwayStatus            string                  `json:"runway_status"`
	SovereignAllocationPlan SovereignAllocationPlan `json:"sovereign_allocation_plan"`
	ActionSummary           string                  `json:"action_summary"`
	EquitiesVerdicts        []EquityVerdict         `json:"equities_verdicts"`
	IdxAlphaSignals         []IdxAlphaSignal        `json:"idx_alpha_signals,omitempty"`
	ADHDDirectives          []string                `json:"adhd_directives"`
}

// ComputeSovereignPlan builds the 3-tier sovereign runway matrix.
func ComputeSovereignPlan(liquidCash, monthlyBurn float64) SovereignAllocationPlan {
	if monthlyBurn <= 0 {
		monthlyBurn = 5350000.0
	}
	runwayMonths := math.Round((liquidCash/monthlyBurn)*10) / 10

	status := "FRAGILE (<6m)"
	if runwayMonths >= 36.0 {
		status = "SOVEREIGN (>36m)"
	} else if runwayMonths >= 24.0 {
		status = "FORTRESS (>24m)"
	} else if runwayMonths >= 12.0 {
		status = "STRONG (12-24m)"
	} else if runwayMonths >= 6.0 {
		status = "STABLE (6-12m)"
	}

	tier1Target := 12.0 * monthlyBurn
	tier1Alloc := math.Min(liquidCash, tier1Target)

	rem1 := math.Max(0, liquidCash-tier1Alloc)
	tier2Target := 12.0 * monthlyBurn
	tier2Alloc := math.Min(rem1, tier2Target)

	tier3Alloc := math.Max(0, rem1-tier2Alloc)

	pct1 := 0.0
	pct2 := 0.0
	pct3 := 0.0
	if liquidCash > 0 {
		pct1 = math.Round((tier1Alloc/liquidCash*100.0)*10) / 10
		pct2 = math.Round((tier2Alloc/liquidCash*100.0)*10) / 10
		pct3 = math.Round((tier3Alloc/liquidCash*100.0)*10) / 10
	}

	advice := fmt.Sprintf("Runway is %s (%.1fm). Base reserves secure.", status, runwayMonths)
	if tier3Alloc > 0 {
		advice = fmt.Sprintf("Runway is %s (%.1fm). You have Rp %s in true surplus above 24 months of living expenses. Recommended: 50%% staged DCA into core index/equities, 30%% dip reserve, 20%% crypto barbell.",
			status, runwayMonths, formatNumber(tier3Alloc))
	}

	return SovereignAllocationPlan{
		LiquidCash:   liquidCash,
		MonthlyBurn:  monthlyBurn,
		RunwayMonths: runwayMonths,
		Status:       status,
		Tier1OperatingReserve: RunwayTier{
			Name:          "Tier 1: Base Operating Reserve (0-12m)",
			TargetMonths:  12,
			TargetIDR:     tier1Target,
			AllocatedIDR:  tier1Alloc,
			AllocationPct: pct1,
			Vehicle:       "Ultra-Liquid Yield (Krom, Aladin, Superbank @ 5-7% p.a.)",
			Mandate:       "Non-negotiable survival buffer. Never deployed into volatile assets.",
		},
		Tier2FortressBuffer: RunwayTier{
			Name:          "Tier 2: Fortress Buffer (12-24m)",
			TargetMonths:  12,
			TargetIDR:     tier2Target,
			AllocatedIDR:  tier2Alloc,
			AllocationPct: pct2,
			Vehicle:       "Sovereign Fixed-Income (Sukuk ST013/ST014, SR021 @ 6.4-6.5% p.a.)",
			Mandate:       "Multi-year macro drawdown defense and guaranteed real compounding.",
		},
		Tier3DeployableSurplus: RunwayTier{
			Name:          "Tier 3: Deployable Strategic Dry Powder (>24m)",
			AllocatedIDR:  tier3Alloc,
			AllocationPct: pct3,
			Vehicle:       "Asymmetric Compounders (Top IDX Value / Index) + Barbell Satellite (ETH/BTC)",
			Mandate:       "Aggressive sovereign wealth acceleration with staged multi-cycle DCA.",
		},
		TacticalAdvice: advice,
	}
}

func queryBurn(d *sql.DB) float64 {
	if d == nil {
		return 0
	}
	var exp30d float64
	_ = d.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE (type = 'EXPENSE' OR type IS NULL OR type = '')
		  AND date >= date('now', '-30 days')
	`).Scan(&exp30d)

	var commitmentsBurn float64
	_ = d.QueryRow(`
		SELECT COALESCE(SUM(
			CASE frequency
				WHEN 'yearly' THEN amount / 12.0
				WHEN 'quarterly' THEN amount / 3.0
				WHEN 'weekly' THEN amount * (52.0 / 12.0)
				ELSE amount
			END
		), 0)
		FROM recurring_commitments
		WHERE status = 'active'
	`).Scan(&commitmentsBurn)

	return exp30d + commitmentsBurn
}

func getLiveMonthlyBurn(dbConn *sql.DB) float64 {
	if burn := queryBurn(dbConn); burn > 0 {
		return burn
	}
	if ierpDB, err := db.Open(config.DBPath()); err == nil && ierpDB != nil {
		defer ierpDB.Close()
		if burn := queryBurn(ierpDB); burn > 0 {
			return burn
		}
	}
	return 0
}

// GenerateAdvisorPayload computes the complete advisor state without external quant signals.
func GenerateAdvisorPayload(snap *models.Snapshot, aiState *aistate.AIState, dbConn *sql.DB) *AdvisorPayload {
	return GenerateAdvisorPayloadWithSignals(snap, aiState, dbConn, nil, nil)
}

// GenerateAdvisorPayloadWithSignals computes the complete advisor state enriched with live idx-bei quant signals.
func GenerateAdvisorPayloadWithSignals(
	snap *models.Snapshot,
	aiState *aistate.AIState,
	dbConn *sql.DB,
	idxSignals map[string]IdxAlphaSignal,
	watchlist []IdxAlphaSignal,
) *AdvisorPayload {
	monthlyBurn := 5350000.0
	if liveBurn := getLiveMonthlyBurn(dbConn); liveBurn > 0 {
		monthlyBurn = liveBurn
	}
	liquidCash := snap.Totals.BankCashIDR
	netWorth := snap.Totals.NetWorthIDR
	dryPowderPct := 0.0
	if netWorth > 0 {
		dryPowderPct = math.Round((liquidCash/netWorth*100.0)*10) / 10
	}

	plan := ComputeSovereignPlan(liquidCash, monthlyBurn)

	// Analyze Equities
	var eqVerdicts []EquityVerdict
	for _, h := range snap.AllHoldings {
		if h.AssetClass == "Equities" && h.ValueIDR != nil && *h.ValueIDR > 0 {
			weight := (*h.ValueIDR / netWorth) * 100.0
			verdict := "⚖️ NEUTRAL (HOLD)"
			trend := "SIDEWAYS"
			if strings.Contains(strings.ToUpper(h.Asset), "BBCA") {
				verdict = "🛡️ CORE ANCHOR (HOLD)"
				trend = "BULLISH"
			} else if strings.Contains(strings.ToUpper(h.Asset), "SRI") || strings.Contains(strings.ToUpper(h.Asset), "KEHATI") {
				verdict = "🌿 ESG INDEX (ACCUMULATE)"
				trend = "BULLISH"
			} else if weight < 1.0 {
				verdict = "🧹 MICRO POSITION (SWEEP/CONSOLIDATE)"
				trend = "FRAGMENTED"
			}

			ev := EquityVerdict{
				Ticker:      h.Asset,
				ValueIDR:    *h.ValueIDR,
				WeightPct:   math.Round(weight*10) / 10,
				Verdict:     verdict,
				TrendRegime: trend,
			}

			if sig, ok := idxSignals[NormalizeTicker(h.Asset)]; ok {
				ev.WyckoffPhase = sig.WyckoffPhase
				ev.SmartMoneyDelta = sig.SmartMoneyDelta
				ev.NFF20D = sig.NetForeignFlowRP
				ev.CompounderScore = sig.CompounderScore
				switch sig.AnomalyType {
				case "INSTITUTIONAL_ABSORPTION":
					ev.Verdict = "🐋 ACCUMULATE / BUY (STEALTH ABSORPTION)"
					ev.TrendRegime = "ACCUMULATION"
				case "INSTITUTIONAL_MARKUP":
					ev.Verdict = "🚀 BREAKOUT / BUY (INSTITUTIONAL MARKUP)"
					ev.TrendRegime = "MARKUP"
				case "RETAIL_TRAP", "INSTITUTIONAL_LIQUIDATION":
					ev.Verdict = "⚠️ CAUTION / TRIM (" + sig.AnomalyType + ")"
					ev.TrendRegime = "DISTRIBUTION"
				default:
					if sig.DCAVerdict == "PRIME_DCA" {
						ev.Verdict = fmt.Sprintf("💎 PRIME DCA / BUY (SCORE %.0f)", sig.CompounderScore)
						ev.TrendRegime = "BULLISH"
					}
				}
			}

			eqVerdicts = append(eqVerdicts, ev)
		}
	}
	sort.Slice(eqVerdicts, func(i, j int) bool {
		return eqVerdicts[i].ValueIDR > eqVerdicts[j].ValueIDR
	})

	var adhdDirectives []string
	if aiState != nil {
		adhdDirectives = append(adhdDirectives, aiState.ADHDFocusMetrics.DepositRouter.OneStepAction)
		adhdDirectives = append(adhdDirectives, aiState.ADHDFocusMetrics.ClutterAudit.Directives...)
		if aiState.ADHDFocusMetrics.SBNReinvestmentPlaybook.HasUpcomingMaturity {
			adhdDirectives = append(adhdDirectives, aiState.ADHDFocusMetrics.SBNReinvestmentPlaybook.ActionPlan...)
		}
	}

	return &AdvisorPayload{
		Date:                    snap.Metadata.Date,
		NetWorthIDR:             netWorth,
		DryPowderIDR:            liquidCash,
		DryPowderPct:            dryPowderPct,
		RunwayMonths:            plan.RunwayMonths,
		RunwayStatus:            plan.Status,
		SovereignAllocationPlan: plan,
		ActionSummary:           plan.TacticalAdvice,
		EquitiesVerdicts:        eqVerdicts,
		IdxAlphaSignals:         watchlist,
		ADHDDirectives:          adhdDirectives,
	}
}

// GenerateMarkdownBriefing formats the advisor payload into crisp markdown.
func GenerateMarkdownBriefing(p *AdvisorPayload) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# 🏛️ Sovereign Wealth & Runway Advisor Briefing — %s\n\n", p.Date))
	sb.WriteString(fmt.Sprintf("### 1. Macro Runway & Reserve Status\n"))
	sb.WriteString(fmt.Sprintf("- **Net Worth**: **Rp %s**\n", formatNumber(p.NetWorthIDR)))
	sb.WriteString(fmt.Sprintf("- **Liquid Reserves**: **Rp %s** (%.1f%% dry powder)\n", formatNumber(p.DryPowderIDR), p.DryPowderPct))
	sb.WriteString(fmt.Sprintf("- **Runway Horizon**: **%.1f Months** (%s)\n", p.RunwayMonths, p.RunwayStatus))
	sb.WriteString(fmt.Sprintf("- **Tactical Directive**: %s\n\n", p.ActionSummary))

	sb.WriteString("### 2. Three-Tier Sovereign Runway Matrix\n")
	sb.WriteString("| Tier | Target Horizon | Allocated (IDR) | Weight | Vehicle |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
	t1 := p.SovereignAllocationPlan.Tier1OperatingReserve
	t2 := p.SovereignAllocationPlan.Tier2FortressBuffer
	t3 := p.SovereignAllocationPlan.Tier3DeployableSurplus
	sb.WriteString(fmt.Sprintf("| **Tier 1: Base Survival** | 0-12m | Rp %s | %.1f%% | %s |\n", formatNumber(t1.AllocatedIDR), t1.AllocationPct, t1.Vehicle))
	sb.WriteString(fmt.Sprintf("| **Tier 2: Fortress Defense** | 12-24m | Rp %s | %.1f%% | %s |\n", formatNumber(t2.AllocatedIDR), t2.AllocationPct, t2.Vehicle))
	sb.WriteString(fmt.Sprintf("| **Tier 3: Strategic Dry Powder** | >24m | Rp %s | %.1f%% | %s |\n\n", formatNumber(t3.AllocatedIDR), t3.AllocationPct, t3.Vehicle))

	if len(p.EquitiesVerdicts) > 0 {
		sb.WriteString("### 3. Equity Holdings & Tactical Verdicts\n")
		sb.WriteString("| Asset | Value (IDR) | Weight | Regime | Verdict |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, eq := range p.EquitiesVerdicts {
			sb.WriteString(fmt.Sprintf("| **%s** | Rp %s | %.1f%% | %s | %s |\n",
				eq.Ticker, formatNumber(eq.ValueIDR), eq.WeightPct, eq.TrendRegime, eq.Verdict))
		}
		sb.WriteString("\n")
	}

	if len(p.IdxAlphaSignals) > 0 {
		sb.WriteString("### 4. IDX-BEI Quant Alpha Watchlist\n")
		sb.WriteString("| Ticker | Signal / DCA | Wyckoff Phase | Smart Money Δ | Score |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, sig := range p.IdxAlphaSignals {
			label := sig.AnomalyType
			if label == "" {
				label = sig.DCAVerdict
			}
			phase := sig.WyckoffPhase
			if phase == "" {
				phase = sig.ValuationStatus
			}
			sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %.2f | %.0f |\n",
				sig.Ticker, label, phase, sig.SmartMoneyDelta, sig.CompounderScore))
		}
		sb.WriteString("\n")
	}

	if len(p.ADHDDirectives) > 0 {
		sb.WriteString("### 5. Zero-Friction ADHD Action Card\n")
		for _, d := range p.ADHDDirectives {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SaveAdvisorPayload saves latest_advisor.json to data directory.
func SaveAdvisorPayload(p *AdvisorPayload, dataDir string) error {
	_ = os.MkdirAll(dataDir, 0755)
	payloadBytes, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	target := filepath.Join(dataDir, "latest_advisor.json")
	return os.WriteFile(target, payloadBytes, 0644)
}

func formatNumber(v float64) string {
	return models.FormatNumber(v)
}
