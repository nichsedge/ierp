package portfolio

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nichsedge/ierp/internal/finance"
)

type SnapshotPayload struct {
	Metadata struct {
		Date string `json:"date"`
	} `json:"metadata"`
	Totals struct {
		TotalLiabilitiesIDR float64 `json:"total_liabilities_idr"`
	} `json:"totals"`
	Allocation struct {
		ByCategory []struct {
			Category string  `json:"category"`
			ValueIDR float64 `json:"value_idr"`
		} `json:"by_category"`
	} `json:"allocation"`
	LiquidCashAccounts []struct {
		ValueIDR float64 `json:"value_idr"`
	} `json:"liquid_cash_accounts"`
}

// SyncPortfolio ingests the latest portfolio snapshot into events.db.
func SyncPortfolio(db *sql.DB, portfolioDataDir string) error {
	if portfolioDataDir == "" {
		home, _ := os.UserHomeDir()
		portfolioDataDir = filepath.Join(home, "Projects", "portfolio-integration", "data")
	}

	latestFile := filepath.Join(portfolioDataDir, "latest_snapshot.json")
	if _, err := os.Stat(latestFile); os.IsNotExist(err) {
		// Try to find the latest *_snapshot.json
		entries, err := os.ReadDir(portfolioDataDir)
		if err != nil {
			return fmt.Errorf("cannot read portfolio data directory: %w", err)
		}
		var files []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), "_snapshot.json") && !strings.HasPrefix(e.Name(), "latest") {
				files = append(files, filepath.Join(portfolioDataDir, e.Name()))
			}
		}
		if len(files) == 0 {
			return fmt.Errorf("no portfolio snapshot file found in %s", portfolioDataDir)
		}
		sort.Strings(files)
		latestFile = files[len(files)-1]
	}

	data, err := os.ReadFile(latestFile)
	if err != nil {
		return fmt.Errorf("failed reading snapshot file: %w", err)
	}

	var payload SnapshotPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed parsing snapshot JSON: %w", err)
	}

	dateStr := payload.Metadata.Date
	if dateStr == "" {
		return fmt.Errorf("snapshot is missing metadata.date")
	}

	var liquidCash float64
	var investments float64

	for _, cat := range payload.Allocation.ByCategory {
		if cat.Category == "Bank Account" || cat.Category == "Stablecoin" {
			liquidCash += cat.ValueIDR
		} else {
			investments += cat.ValueIDR
		}
	}

	if liquidCash == 0 && len(payload.LiquidCashAccounts) > 0 {
		for _, a := range payload.LiquidCashAccounts {
			liquidCash += a.ValueIDR
		}
	}

	liabilities := payload.Totals.TotalLiabilitiesIDR

	existing, err := finance.GetLatestSnapshot(db)
	if err != nil || existing == nil || existing.SnapshotDate != dateStr {
		snap := finance.Snapshot{
			SnapshotDate: dateStr,
			LiquidCash:   liquidCash,
			Investments:  investments,
			HardAssets:   0,
			Liabilities:  liabilities,
			Currency:     "IDR",
			Notes:        "Automated SSOT sync from portfolio-integration (Go)",
		}
		id, err := finance.InsertSnapshot(db, snap)
		if err != nil {
			return fmt.Errorf("failed inserting snapshot: %w", err)
		}
		fmt.Printf("✅ Ingested snapshot for %s (ID: %d | Liquid: Rp %.0f, Invest: Rp %.0f)\n",
			dateStr, id, liquidCash, investments)
	} else {
		fmt.Printf("ℹ️ Snapshot for %s already recorded in iERP\n", dateStr)
	}

	// Seed baseline commitments if none active
	activeCommitments, _ := finance.ListCommitments(db, "active", "")
	if len(activeCommitments) == 0 {
		fmt.Println("🌱 Seeding initial baseline commitments...")
		_, _ = finance.InsertCommitment(db, finance.Commitment{
			Name:      "Living Expenses (Food, Transit, Household)",
			Amount:    3000000.0,
			Category:  "lifestyle",
			Frequency: "monthly",
			Notes:     "Baseline living expenses derived from SansFinance 90-day burn",
		})
		_, _ = finance.InsertCommitment(db, finance.Commitment{
			Name:      "Cloud, Hosting & Domain Infrastructure",
			Amount:    250000.0,
			Category:  "infrastructure",
			Frequency: "monthly",
			Notes:     "Workstation, Cloudflare, Tailscale, VPS, and DNS tooling",
		})
	}

	runway, err := finance.ComputeRunway(db)
	if err == nil {
		fmt.Printf("📊 Sovereign Runway: %.1f months (Liquid: Rp %.0f | Monthly Burn: Rp %.0f)\n",
			runway.RunwayMonths, runway.LiquidCash, runway.MonthlyBurn)
	}

	return nil
}
