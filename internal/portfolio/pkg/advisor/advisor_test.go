package advisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

func TestComputeSovereignPlan(t *testing.T) {
	// 100M liquid cash with 5M burn => 20 months => STRONG (12-24m)
	plan := ComputeSovereignPlan(100000000.0, 5000000.0)

	if plan.RunwayMonths != 20.0 {
		t.Fatalf("expected 20 months, got %f", plan.RunwayMonths)
	}
	if plan.Status != "STRONG (12-24m)" {
		t.Fatalf("expected STRONG (12-24m), got %s", plan.Status)
	}

	// 60M tier 1, 40M tier 2, 0 surplus
	if plan.Tier1OperatingReserve.AllocatedIDR != 60000000.0 {
		t.Fatalf("expected Tier 1 60M, got %f", plan.Tier1OperatingReserve.AllocatedIDR)
	}
	if plan.Tier2FortressBuffer.AllocatedIDR != 40000000.0 {
		t.Fatalf("expected Tier 2 40M, got %f", plan.Tier2FortressBuffer.AllocatedIDR)
	}
	if plan.Tier3DeployableSurplus.AllocatedIDR != 0.0 {
		t.Fatalf("expected Tier 3 0, got %f", plan.Tier3DeployableSurplus.AllocatedIDR)
	}
}

func TestGenerateAdvisorPayloadAndMarkdown(t *testing.T) {
	val := 150000000.0
	eqVal := 20000000.0
	snap := &models.Snapshot{
		Metadata: models.Metadata{Date: "2026-10-01"},
		Totals: models.Totals{
			NetWorthIDR: val + eqVal,
			BankCashIDR: val,
		},
		AllHoldings: []models.Holding{
			{
				Asset:      "BBCA",
				AssetClass: "Equities",
				ValueIDR:   &eqVal,
			},
		},
	}

	payload := GenerateAdvisorPayload(snap, nil, nil)
	if payload.RunwayMonths <= 0 {
		t.Fatalf("expected positive runway months, got %f", payload.RunwayMonths)
	}

	md := GenerateMarkdownBriefing(payload)
	if !strings.Contains(md, "Sovereign Wealth & Runway Advisor") {
		t.Fatalf("markdown missing title")
	}
}

func TestFetchIdxSignalsAndEnrichAdvisor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/bandar/anomalies":
			_, _ = w.Write([]byte(`[
				{
					"ticker": "BBRI",
					"anomaly_type": "INSTITUTIONAL_ABSORPTION",
					"wyckoff_phase": "Phase C — Silent Absorption",
					"smart_money_delta": 4.25,
					"net_foreign_flow_rp": 15000000000,
					"z_composite": 3.8,
					"playbook": "Accumulate quietly"
				}
			]`))
		case "/api/compounder-screen":
			_, _ = w.Write([]byte(`[
				{
					"ticker": "BBCA",
					"compounder_score": 88.5,
					"dca_verdict": "PRIME_DCA",
					"valuation_status": "FAIR_VALUE"
				}
			]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	idxMap, watchlist, err := FetchIdxSignals(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected FetchIdxSignals error: %v", err)
	}
	if len(watchlist) != 2 {
		t.Fatalf("expected 2 watchlist items, got %d", len(watchlist))
	}

	cash := 150000000.0
	bbriVal := 30000000.0
	bbcaVal := 50000000.0
	snap := &models.Snapshot{
		Metadata: models.Metadata{Date: "2026-11-28"},
		Totals: models.Totals{
			NetWorthIDR: cash + bbriVal + bbcaVal,
			BankCashIDR: cash,
		},
		AllHoldings: []models.Holding{
			{Asset: "BBRI.JK", AssetClass: "Equities", ValueIDR: &bbriVal},
			{Asset: "BBCA", AssetClass: "Equities", ValueIDR: &bbcaVal},
		},
	}

	payload := GenerateAdvisorPayloadWithSignals(snap, nil, nil, idxMap, watchlist)
	if len(payload.EquitiesVerdicts) != 2 {
		t.Fatalf("expected 2 verdicts, got %d", len(payload.EquitiesVerdicts))
	}

	var foundBBRI, foundBBCA bool
	for _, ev := range payload.EquitiesVerdicts {
		if ev.Ticker == "BBRI.JK" {
			foundBBRI = true
			if ev.TrendRegime != "ACCUMULATION" || !strings.Contains(ev.Verdict, "BUY") {
				t.Fatalf("expected BBRI ACCUMULATION/BUY verdict, got %s (%s)", ev.Verdict, ev.TrendRegime)
			}
			if ev.SmartMoneyDelta != 4.25 {
				t.Fatalf("expected SmartMoneyDelta 4.25, got %f", ev.SmartMoneyDelta)
			}
		}
		if ev.Ticker == "BBCA" {
			foundBBCA = true
			if ev.CompounderScore != 88.5 || !strings.Contains(ev.Verdict, "PRIME DCA") {
				t.Fatalf("expected BBCA PRIME DCA 88.5, got %s (%.1f)", ev.Verdict, ev.CompounderScore)
			}
		}
	}
	if !foundBBRI || !foundBBCA {
		t.Fatalf("missing expected tickers in verdicts")
	}

	md := GenerateMarkdownBriefing(payload)
	if !strings.Contains(md, "IDX-BEI Quant Alpha Watchlist") {
		t.Fatalf("markdown missing IDX-BEI Quant Alpha Watchlist section")
	}
}

func TestFetchIdxSignalsFailFast(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, _, err := FetchIdxSignals(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("expected error on HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected HTTP 500 in error, got: %v", err)
	}
}
