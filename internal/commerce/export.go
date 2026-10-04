package commerce

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ExportCommerceFiles exports pay.json and referrals.json to the portfolio data directory.
func ExportCommerceFiles(database *sql.DB, targetDir string) (payCount int, refCount int, err error) {
	if targetDir == "" {
		if env := os.Getenv("PORTFOLIO_DATA"); env != "" {
			targetDir = env
		} else {
			home, err := os.UserHomeDir()
			if err == nil {
				targetDir = filepath.Join(home, "Projects", "nichsedge.github.io", "data")
			} else {
				targetDir = "data"
			}
		}
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("failed creating target directory: %w", err)
	}

	// 1. Export pay.json
	payAccounts, err := ListPaymentAccounts(database, "")
	if err != nil {
		return 0, 0, fmt.Errorf("failed listing payment accounts: %w", err)
	}

	type PayJSONItem struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Category  string `json:"category"`
		Number    string `json:"number"`
		Recipient string `json:"recipient"`
		Details   string `json:"details"`
		DetailsID string `json:"details_id"`
	}

	var payList []PayJSONItem
	for _, p := range payAccounts {
		payList = append(payList, PayJSONItem{
			ID:        p.Slug,
			Name:      p.Name,
			Category:  p.Category,
			Number:    p.Number,
			Recipient: p.Recipient,
			Details:   p.Details,
			DetailsID: p.DetailsID,
		})
	}

	payJSONData, err := json.MarshalIndent(payList, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	payFile := filepath.Join(targetDir, "pay.json")
	if err := os.WriteFile(payFile, payJSONData, 0644); err != nil {
		return 0, 0, fmt.Errorf("failed writing pay.json: %w", err)
	}

	// 2. Export referrals.json
	referrals, err := ListReferrals(database, "", "ACTIVE")
	if err != nil {
		return len(payList), 0, fmt.Errorf("failed listing referrals: %w", err)
	}

	type ReferralJSONItem struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
		Code     string `json:"code"`
		Link     string `json:"link"`
		Benefit  string `json:"benefit"`
		Status   string `json:"status"`
	}

	var refList []ReferralJSONItem
	for _, r := range referrals {
		if !r.IsPublic {
			continue
		}
		refList = append(refList, ReferralJSONItem{
			ID:       r.Slug,
			Name:     r.Name,
			Category: r.Category,
			Code:     r.Code,
			Link:     r.Link,
			Benefit:  r.Benefit,
			Status:   r.Status,
		})
	}

	refJSONData, err := json.MarshalIndent(refList, "", "  ")
	if err != nil {
		return len(payList), 0, err
	}
	refFile := filepath.Join(targetDir, "referrals.json")
	if err := os.WriteFile(refFile, refJSONData, 0644); err != nil {
		return len(payList), 0, fmt.Errorf("failed writing referrals.json: %w", err)
	}

	return len(payList), len(refList), nil
}
