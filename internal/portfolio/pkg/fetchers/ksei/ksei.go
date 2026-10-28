package ksei

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chickenzord/goksei"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

// RawKseiData represents the structure saved to {date}_raw_ksei.json
type RawKseiData struct {
	Cash       CashSection       `json:"cash"`
	Equity     InvestmentSection `json:"equity"`
	MutualFund InvestmentSection `json:"mutual_fund"`
	Bond       InvestmentSection `json:"bond"`
}

type CashSection struct {
	TotalSaldo float64              `json:"totalSaldo"`
	Data       []goksei.CashBalance `json:"data"`
}

type InvestmentEntry struct {
	Efek           string  `json:"efek"`
	Jumlah         float64 `json:"jumlah"`
	Harga          float64 `json:"harga"`
	NilaiInvestasi float64 `json:"nilaiInvestasi"`
	Partisipan     string  `json:"partisipan"`
	Rekening       string  `json:"rekening"`
}

type InvestmentSection struct {
	TotalInvestasi float64           `json:"totalInvestasi"`
	Data           []InvestmentEntry `json:"data"`
}

// Fetch connects to AKSES-KSEI and retrieves balances.
func Fetch(username, password string, plainPassword bool) (*RawKseiData, []models.Holding, error) {
	if username == "" || password == "" {
		return nil, nil, fmt.Errorf("KSEI credentials not provided")
	}

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache/ksei")
	_ = os.MkdirAll(cacheDir, 0700)

	authStore, err := goksei.NewFileAuthStore(filepath.Join(cacheDir, "auth.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init ksei auth store: %w", err)
	}

	client := goksei.NewClient(goksei.ClientOpts{
		Username:      username,
		Password:      password,
		PlainPassword: plainPassword,
		AuthStore:     authStore,
	})

	// 1. Fetch cash
	cashResp, err := client.GetCashBalances()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch KSEI cash: %w", err)
	}

	// 2. Fetch equity
	equityResp, err := client.GetShareBalances(goksei.EquityType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch KSEI equity: %w", err)
	}

	// 3. Fetch mutual funds
	mfResp, err := client.GetShareBalances(goksei.MutualFundType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch KSEI mutual funds: %w", err)
	}

	// 4. Fetch bonds
	bondResp, err := client.GetShareBalances(goksei.BondType)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch KSEI bonds: %w", err)
	}

	raw := &RawKseiData{
		Cash: CashSection{
			Data: cashResp.Data,
		},
		Equity:     InvestmentSection{Data: make([]InvestmentEntry, 0)},
		MutualFund: InvestmentSection{Data: make([]InvestmentEntry, 0)},
		Bond:       InvestmentSection{Data: make([]InvestmentEntry, 0)},
	}

	for _, c := range cashResp.Data {
		raw.Cash.TotalSaldo += c.CurrentBalance()
	}

	for _, eq := range equityResp.Data {
		val := eq.CurrentValue()
		raw.Equity.TotalInvestasi += val
		raw.Equity.Data = append(raw.Equity.Data, InvestmentEntry{
			Efek:           eq.FullName,
			Jumlah:         eq.Amount,
			Harga:          eq.ClosingPrice,
			NilaiInvestasi: val,
			Partisipan:     eq.Participant,
			Rekening:       eq.Account,
		})
	}

	for _, mf := range mfResp.Data {
		val := mf.CurrentValue()
		raw.MutualFund.TotalInvestasi += val
		raw.MutualFund.Data = append(raw.MutualFund.Data, InvestmentEntry{
			Efek:           mf.FullName,
			Jumlah:         mf.Amount,
			Harga:          mf.ClosingPrice,
			NilaiInvestasi: val,
			Partisipan:     mf.Participant,
			Rekening:       mf.Account,
		})
	}

	for _, b := range bondResp.Data {
		val := b.CurrentValue()
		raw.Bond.TotalInvestasi += val
		raw.Bond.Data = append(raw.Bond.Data, InvestmentEntry{
			Efek:           b.FullName,
			Jumlah:         b.Amount,
			Harga:          b.ClosingPrice,
			NilaiInvestasi: val,
			Partisipan:     b.Participant,
			Rekening:       b.Account,
		})
	}

	holdings := Standardize(raw)
	return raw, holdings, nil
}

// Standardize converts RawKseiData into standardized portfolio holdings.
func Standardize(data *RawKseiData) []models.Holding {
	var holdings []models.Holding
	idrThreshold := models.FilterThresholds["IDR"]
	usdThreshold := models.FilterThresholds["USD"]

	// Process Cash
	for _, entry := range data.Cash.Data {
		curr := entry.Currency
		if curr == "" {
			curr = "IDR"
		}

		var valIDR, valUSD *float64
		if curr == "IDR" {
			v := entry.BalanceIDR
			if v == 0 {
				v = entry.Balance
			}
			if v < idrThreshold {
				continue
			}
			valIDR = &v
		} else {
			v := entry.Balance
			if v < usdThreshold {
				continue
			}
			valUSD = &v
		}

		bankCode := entry.BankID
		assetName := bankCode
		if bankCode == "PRMT2" {
			assetName = "Ajaib RDN"
		} else if bankCode == "JAGO1" {
			assetName = "Stockbit RDN"
		}

		price := 1.0
		holdings = append(holdings, models.Holding{
			Source:     "KSEI",
			Category:   "Bank Account",
			Asset:      assetName,
			Name:       assetName,
			Currency:   curr,
			Quantity:   entry.Balance,
			Price:      &price,
			PriceIDR:   &price,
			ValueIDR:   valIDR,
			ValueUSD:   valUSD,
			Account:    entry.AccountNumber,
			Details:    fmt.Sprintf("Bank: %s, Account: %s", bankCode, entry.AccountNumber),
			AssetClass: "Cash & Equivalents",
		})
	}

	// Process Equity
	for _, entry := range data.Equity.Data {
		if entry.NilaiInvestasi < idrThreshold {
			continue
		}

		ticker := strings.Split(entry.Efek, " - ")[0]
		price := entry.Harga
		valIDR := entry.NilaiInvestasi

		holdings = append(holdings, models.Holding{
			Source:     "KSEI",
			Category:   "Indo Stocks",
			Asset:      ticker,
			Ticker:     ticker,
			Name:       entry.Efek,
			Currency:   "IDR",
			Quantity:   entry.Jumlah,
			Price:      &price,
			PriceIDR:   &price,
			ValueIDR:   &valIDR,
			Account:    entry.Rekening,
			Details:    fmt.Sprintf("Stock: %s, Broker: %s", entry.Efek, entry.Partisipan),
			AssetClass: "Equities",
		})
	}

	// Process Mutual Funds
	for _, entry := range data.MutualFund.Data {
		if entry.NilaiInvestasi < idrThreshold {
			continue
		}

		category := "Equity Fund"
		for _, term := range []string{"Bond", "Fixed Income", "SBN", "Obligasi"} {
			if strings.Contains(entry.Efek, term) {
				isGov := false
				for _, gov := range []string{"SBN", "Sovereign", "Government", "SST", "INDON", "INDOGB"} {
					if strings.Contains(entry.Efek, gov) {
						isGov = true
						break
					}
				}
				if isGov {
					category = "SBN"
				} else {
					category = "Corporate Bond"
				}
				break
			}
		}
		for _, term := range []string{"Pasar Uang", "Money Market", "Liquidity"} {
			if strings.Contains(entry.Efek, term) {
				category = "Money Market Fund"
				break
			}
		}

		var price float64
		if entry.Jumlah > 0 {
			price = entry.NilaiInvestasi / entry.Jumlah
		}
		valIDR := entry.NilaiInvestasi

		holdings = append(holdings, models.Holding{
			Source:     "KSEI",
			Category:   category,
			Asset:      entry.Efek,
			Name:       entry.Efek,
			Currency:   "IDR",
			Quantity:   entry.Jumlah,
			Price:      &price,
			PriceIDR:   &price,
			ValueIDR:   &valIDR,
			Account:    entry.Rekening,
			Details:    fmt.Sprintf("Fund: %s, Manager: %s", entry.Efek, entry.Partisipan),
			AssetClass: models.GetAssetClass(category),
		})
	}

	// Process Bonds
	for _, entry := range data.Bond.Data {
		if entry.NilaiInvestasi < idrThreshold {
			continue
		}

		category := "SBN"
		isGov := false
		for _, term := range []string{"ORI", "SR", "ST", "FR", "SBN", "PBS", "INDON", "INDOGB"} {
			if strings.Contains(entry.Efek, term) {
				isGov = true
				break
			}
		}
		if !isGov {
			category = "Corporate Bond"
		}

		var price float64
		if entry.Jumlah > 0 {
			price = entry.NilaiInvestasi / entry.Jumlah
		}
		valIDR := entry.NilaiInvestasi

		holdings = append(holdings, models.Holding{
			Source:     "KSEI",
			Category:   category,
			Asset:      entry.Efek,
			Name:       entry.Efek,
			Currency:   "IDR",
			Quantity:   entry.Jumlah,
			Price:      &price,
			PriceIDR:   &price,
			ValueIDR:   &valIDR,
			Account:    entry.Rekening,
			Details:    fmt.Sprintf("Bond: %s, Issuer: %s", entry.Efek, entry.Partisipan),
			AssetClass: "Fixed Income",
		})
	}

	return holdings
}

// SaveRaw dumps raw KSEI data to disk.
func SaveRaw(data *RawKseiData, targetPath string) error {
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, b, 0644)
}

// IsDegraded returns true if both Equity and Bond sections have 0 data entries.
// Outside market hours (pre-market or weekends), AKSES KSEI APIs often return empty
// arrays for equities and bonds while still returning cash or mutual funds.
func (r *RawKseiData) IsDegraded() bool {
	if r == nil {
		return true
	}
	return len(r.Equity.Data) == 0 && len(r.Bond.Data) == 0
}

// FillMissingFromPrevious fills missing Equity or Bond sections from previous data
// when the current fetch appears to have suffered off-hours or pre-market downtime.
func (r *RawKseiData) FillMissingFromPrevious(prev *RawKseiData) bool {
	if r == nil || prev == nil {
		return false
	}
	modified := false
	if len(r.Equity.Data) == 0 && len(prev.Equity.Data) > 0 {
		r.Equity = prev.Equity
		modified = true
	}
	if len(r.Bond.Data) == 0 && len(prev.Bond.Data) > 0 {
		r.Bond = prev.Bond
		modified = true
	}
	return modified
}

