package ksei

import (
	"testing"

	"github.com/chickenzord/goksei"
)

func TestIsDegraded(t *testing.T) {
	var nilData *RawKseiData
	if !nilData.IsDegraded() {
		t.Errorf("expected nil RawKseiData to be degraded")
	}

	empty := &RawKseiData{}
	if !empty.IsDegraded() {
		t.Errorf("expected empty RawKseiData to be degraded")
	}

	withCashOnly := &RawKseiData{
		Cash: CashSection{
			TotalSaldo: 1000000,
			Data: []goksei.CashBalance{
				{AccountNumber: "123", BankID: "JAGO1", Balance: 1000000},
			},
		},
	}
	if !withCashOnly.IsDegraded() {
		t.Errorf("expected cash-only RawKseiData to be degraded")
	}

	withEquity := &RawKseiData{
		Equity: InvestmentSection{
			TotalInvestasi: 5000000,
			Data: []InvestmentEntry{
				{Efek: "BBCA - Bank BCA", NilaiInvestasi: 5000000},
			},
		},
	}
	if withEquity.IsDegraded() {
		t.Errorf("expected RawKseiData with equity not to be degraded")
	}

	withBond := &RawKseiData{
		Bond: InvestmentSection{
			TotalInvestasi: 10000000,
			Data: []InvestmentEntry{
				{Efek: "ST012T4 - Sukuk Tabungan", NilaiInvestasi: 10000000},
			},
		},
	}
	if withBond.IsDegraded() {
		t.Errorf("expected RawKseiData with bond not to be degraded")
	}
}

func TestFillMissingFromPrevious(t *testing.T) {
	prev := &RawKseiData{
		Equity: InvestmentSection{
			TotalInvestasi: 10000000,
			Data: []InvestmentEntry{
				{Efek: "BBCA - Bank Central Asia", NilaiInvestasi: 10000000},
			},
		},
		Bond: InvestmentSection{
			TotalInvestasi: 50000000,
			Data: []InvestmentEntry{
				{Efek: "ST013T2 - Sukuk Tabungan", NilaiInvestasi: 50000000},
			},
		},
	}

	curr := &RawKseiData{
		Cash: CashSection{
			TotalSaldo: 500000,
			Data: []goksei.CashBalance{
				{AccountNumber: "123", BankID: "JAGO1", Balance: 500000},
			},
		},
	}

	if !curr.IsDegraded() {
		t.Fatalf("curr should be degraded before fill")
	}

	modified := curr.FillMissingFromPrevious(prev)
	if !modified {
		t.Errorf("expected FillMissingFromPrevious to report modified=true")
	}

	if curr.IsDegraded() {
		t.Errorf("curr should no longer be degraded after fill")
	}

	if len(curr.Equity.Data) != 1 || curr.Equity.TotalInvestasi != 10000000 {
		t.Errorf("equity was not correctly filled from previous")
	}

	if len(curr.Bond.Data) != 1 || curr.Bond.TotalInvestasi != 50000000 {
		t.Errorf("bond was not correctly filled from previous")
	}

	// Calling again should not modify since curr is already filled
	modifiedAgain := curr.FillMissingFromPrevious(prev)
	if modifiedAgain {
		t.Errorf("expected second fill to return false")
	}
}

func TestStandardize(t *testing.T) {
	raw := &RawKseiData{
		Cash: CashSection{
			TotalSaldo: 1000000,
			Data: []goksei.CashBalance{
				{
					AccountNumber: "114538727842",
					BankID:        "JAGO1",
					Currency:      "IDR",
					Balance:       1000000,
					BalanceIDR:    1000000,
				},
			},
		},
		Equity: InvestmentSection{
			TotalInvestasi: 20000000,
			Data: []InvestmentEntry{
				{
					Efek:           "BBCA - BANK CENTRAL ASIA Tbk",
					Jumlah:         2000,
					Harga:          10000,
					NilaiInvestasi: 20000000,
					Partisipan:     "PT AJAIB SEKURITAS",
					Rekening:       "XC001",
				},
			},
		},
		MutualFund: InvestmentSection{
			TotalInvestasi: 5000000,
			Data: []InvestmentEntry{
				{
					Efek:           "NI002 - REKSA DANA INDEKS BNI AM SRI KEHATI",
					Jumlah:         5000,
					NilaiInvestasi: 5000000,
					Partisipan:     "BNI AM",
					Rekening:       "MF001",
				},
			},
		},
		Bond: InvestmentSection{
			TotalInvestasi: 50000000,
			Data: []InvestmentEntry{
				{
					Efek:           "ST012T4 - SUKUK TABUNGAN SERI ST012T4",
					Jumlah:         50000000,
					Harga:          1,
					NilaiInvestasi: 50000000,
					Partisipan:     "PT Binaartha Sekuritas",
					Rekening:       "AR001",
				},
			},
		},
	}

	holdings := Standardize(raw)
	if len(holdings) != 4 {
		t.Fatalf("expected 4 holdings, got %d", len(holdings))
	}

	categories := make(map[string]bool)
	for _, h := range holdings {
		categories[h.Category] = true
	}

	if !categories["Bank Account"] {
		t.Errorf("missing Bank Account holding")
	}
	if !categories["Indo Stocks"] {
		t.Errorf("missing Indo Stocks holding")
	}
	if !categories["Equity Fund"] {
		t.Errorf("missing Equity Fund holding")
	}
	if !categories["SBN"] {
		t.Errorf("missing SBN holding")
	}
}
