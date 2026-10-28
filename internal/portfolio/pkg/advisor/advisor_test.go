package advisor

import (
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
