package vendors_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/vendors"
)

func TestVendorsCRUDAndFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vendors.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	id, err := vendors.InsertVendor(database, vendors.Vendor{
		Name:     "ASA Tours and Travel",
		Category: "Motorbike Rental",
		Location: "Bali, Indonesia",
		Phone:    "+62 851-7300-3683",
		Favorite: false,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertVendor failed: id=%d, err=%v", id, err)
	}

	// Toggle favorite
	ok, err := vendors.ToggleFavorite(database, id)
	if err != nil || !ok {
		t.Fatalf("ToggleFavorite failed: ok=%v, err=%v", ok, err)
	}

	list, err := vendors.ListVendors(database, "", true)
	if err != nil || len(list) != 1 || !list[0].Favorite {
		t.Errorf("ListVendors favorite failed: %+v, err=%v", list, err)
	}

	// Delete
	ok, err = vendors.DeleteVendor(database, id)
	if err != nil || !ok {
		t.Errorf("DeleteVendor failed: ok=%v, err=%v", ok, err)
	}
}
