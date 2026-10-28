package sansfinance

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/r2"
	_ "modernc.org/sqlite"
)

const (
	defaultBucketName = "ichsanul-dev"
	r2BlobName        = "db/sans_finance_latest.sqlite"
)

type AccountEntry struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

type RawSansfinanceData struct {
	Accounts []AccountEntry `json:"accounts"`
}

// DownloadFromR2 downloads the SQLite snapshot from Cloudflare R2 using r2.DownloadFile.
func DownloadFromR2(accountID, accessKey, secretKey, bucket, destPath string) error {
	if bucket == "" {
		bucket = defaultBucketName
	}
	return r2.DownloadFile(accountID, accessKey, secretKey, bucket, r2BlobName, destPath)
}

// Fetch extracts cash/p2p accounts from Sans Finance SQLite.
func Fetch(accountID, accessKey, secretKey, bucket string) (*RawSansfinanceData, []models.Holding, error) {
	tmpFile, err := os.CreateTemp("", "sansfinance-*.sqlite")
	if err != nil {
		return nil, nil, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := DownloadFromR2(accountID, accessKey, secretKey, bucket, tmpPath); err != nil {
		return nil, nil, fmt.Errorf("failed to download Sans Finance DB: %w", err)
	}

	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open Sans Finance SQLite: %w", err)
	}
	defer db.Close()

	query := "SELECT name, type, balance, currency FROM accounts WHERE type IN ('Cash', 'Bank Account', 'P2P Lending')"
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AccountEntry
	for rows.Next() {
		var name, accType, curr string
		var balanceCents float64
		if err := rows.Scan(&name, &accType, &balanceCents, &curr); err != nil {
			continue
		}
		if curr == "" {
			curr = "IDR"
		}
		accounts = append(accounts, AccountEntry{
			Name:     name,
			Type:     accType,
			Balance:  balanceCents / 100.0,
			Currency: curr,
		})
	}

	raw := &RawSansfinanceData{Accounts: accounts}
	holdings := Standardize(raw)
	return raw, holdings, nil
}

// Standardize converts accounts into standardized portfolio holdings.
func Standardize(data *RawSansfinanceData) []models.Holding {
	var holdings []models.Holding

	for _, acc := range data.Accounts {
		balance := acc.Balance
		if math.Abs(balance) < 1.0 {
			continue
		}

		currency := acc.Currency
		var category string
		var valIDR, valUSD *float64

		if balance < 0 {
			category = "Liabilities"
			posVal := -balance
			if currency == "IDR" {
				valIDR = &posVal
			} else {
				valUSD = &posVal
			}
		} else {
			if acc.Type == "P2P Lending" {
				category = "P2P Lending"
			} else if acc.Type == "Cash" {
				category = "Digital Bank"
			} else {
				category = "Bank Account"
			}
			if currency == "IDR" {
				valIDR = &balance
			} else {
				valUSD = &balance
			}
		}

		price := 1.0
		holdings = append(holdings, models.Holding{
			Source:     "SansFinance",
			Category:   category,
			Asset:      acc.Name,
			Name:       acc.Name,
			Currency:   currency,
			Quantity:   0,
			Price:      &price,
			PriceIDR:   &price,
			ValueIDR:   valIDR,
			ValueUSD:   valUSD,
			Account:    acc.Name,
			Details:    fmt.Sprintf("Type: %s", acc.Type),
			AssetClass: models.GetAssetClass(category),
		})
	}

	return holdings
}

// SaveRaw dumps raw Sans Finance data to disk.
func SaveRaw(data *RawSansfinanceData, targetPath string) error {
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, b, 0644)
}
