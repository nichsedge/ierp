package commerce_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/db"
)

func TestCommerceExportFiles(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_comm.db")
	outDir := filepath.Join(tmpDir, "data")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	_, err = commerce.InsertPaymentAccount(database, commerce.PaymentAccount{
		Name:     "BCA Test",
		Category: "bank",
		Number:   "123456",
	})
	if err != nil {
		t.Fatalf("InsertPaymentAccount failed: %v", err)
	}

	_, err = commerce.InsertReferral(database, commerce.Referral{
		Name:     "Ref Test",
		Category: "cloud",
		Code:     "TEST",
		IsPublic: true,
	})
	if err != nil {
		t.Fatalf("InsertReferral failed: %v", err)
	}

	payCount, refCount, err := commerce.ExportCommerceFiles(database, outDir)
	if err != nil {
		t.Fatalf("ExportCommerceFiles failed: %v", err)
	}
	if payCount != 1 || refCount != 1 {
		t.Errorf("expected 1 pay and 1 ref, got pay=%d, ref=%d", payCount, refCount)
	}
}
