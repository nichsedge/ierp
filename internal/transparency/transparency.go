// Package transparency exports privacy-safe sovereign transparency metrics.
//
// Only percentages and runway figures are published — never absolute currency
// amounts, account names, or counterparty details.
package transparency

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nichsedge/ierp/internal/finance"
)

// Report is the privacy-safe transparency payload published to the portfolio site.
type Report struct {
	GeneratedAt   string             `json:"generated_at"`
	Currency      string             `json:"currency"`
	SnapshotDate  string             `json:"snapshot_date"`
	RunwayMonths  float64            `json:"runway_months"`
	StatusLabel   string             `json:"status_label"`
	AllocationPct map[string]float64 `json:"allocation_pct"`
	BurnPct       map[string]float64 `json:"burn_pct"`
	CommitmentCnt int                `json:"commitment_count"`
}

// BuildReport assembles the transparency report from the SSOT database.
func BuildReport(database *sql.DB) (*Report, error) {
	runway, err := finance.ComputeRunway(database)
	if err != nil {
		return nil, fmt.Errorf("failed computing runway: %w", err)
	}

	snapshot, err := finance.GetLatestSnapshot(database)
	if err != nil {
		return nil, fmt.Errorf("failed loading latest snapshot: %w", err)
	}

	report := &Report{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Currency:      "IDR",
		RunwayMonths:  runway.RunwayMonths,
		StatusLabel:   runway.StatusLabel,
		AllocationPct: make(map[string]float64),
		BurnPct:       make(map[string]float64),
		CommitmentCnt: runway.CommitmentsCount,
	}

	if snapshot != nil {
		report.Currency = snapshot.Currency
		report.SnapshotDate = snapshot.SnapshotDate

		totalAssets := snapshot.LiquidCash + snapshot.Investments + snapshot.HardAssets
		if totalAssets > 0 {
			report.AllocationPct["liquid_cash"] = round2(snapshot.LiquidCash / totalAssets * 100)
			report.AllocationPct["investments"] = round2(snapshot.Investments / totalAssets * 100)
			report.AllocationPct["hard_assets"] = round2(snapshot.HardAssets / totalAssets * 100)
			report.AllocationPct["liabilities"] = round2(snapshot.Liabilities / totalAssets * 100)
		}
	}

	if runway.MonthlyBurn > 0 {
		for category, amount := range runway.BurnByCategory {
			report.BurnPct[category] = round2(amount / runway.MonthlyBurn * 100)
		}
	}

	return report, nil
}

// Export writes transparency.json into the target directory and returns its path.
func Export(database *sql.DB, targetDir string) (string, error) {
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			targetDir = filepath.Join(home, "Projects", "nichsedge.github.io", "data")
		} else {
			targetDir = "data"
		}
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed creating target directory: %w", err)
	}

	report, err := BuildReport(database)
	if err != nil {
		return "", err
	}

	data, err := marshalReport(report)
	if err != nil {
		return "", err
	}

	outPath := filepath.Join(targetDir, "transparency.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed writing transparency.json: %w", err)
	}

	return outPath, nil
}

func marshalReport(report *Report) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
