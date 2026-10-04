package commerce_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/db"
)

func TestCommerceAccountsAndReferrals(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_commerce.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Payment Account
	pID, err := commerce.InsertPaymentAccount(database, commerce.PaymentAccount{
		Name:      "Bank Central Asia (BCA)",
		Category:  "bank",
		Number:    "1234567890",
		Recipient: "Al Ichsanul",
	})
	if err != nil || pID <= 0 {
		t.Fatalf("InsertPaymentAccount failed: id=%d, err=%v", pID, err)
	}

	accounts, err := commerce.ListPaymentAccounts(database, "bank")
	if err != nil || len(accounts) != 1 {
		t.Fatalf("ListPaymentAccounts failed: %+v, err=%v", accounts, err)
	}

	ok, err := commerce.DeletePaymentAccount(database, pID)
	if err != nil || !ok {
		t.Errorf("DeletePaymentAccount failed: ok=%v, err=%v", ok, err)
	}

	// Referral
	rID, err := commerce.InsertReferral(database, commerce.Referral{
		Name:     "DigitalOcean",
		Category: "cloud",
		Code:     "NICHS-DO",
		Link:     "https://m.do.co/c/...",
		Benefit:  "$200 free credit over 60 days",
	})
	if err != nil || rID <= 0 {
		t.Fatalf("InsertReferral failed: id=%d, err=%v", rID, err)
	}

	refs, err := commerce.ListReferrals(database, "cloud", "ACTIVE")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListReferrals failed: %+v, err=%v", refs, err)
	}

	ok, err = commerce.DeleteReferral(database, rID)
	if err != nil || !ok {
		t.Errorf("DeleteReferral failed: ok=%v, err=%v", ok, err)
	}
}
