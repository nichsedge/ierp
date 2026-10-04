package gadgets_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/gadgets"
)

func TestGadgetsCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_gadgets.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	id, err := gadgets.InsertGadget(database, gadgets.Gadget{
		Name:          "ThinkPad X1 Carbon Gen 12",
		Brand:         "Lenovo",
		Model:         "21KC",
		Category:      "laptop",
		PurchasePrice: 28000000,
		PurchaseDate:  "2026-03-15",
		IsPublic:      true,
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertGadget failed: id=%d, err=%v", id, err)
	}

	g, err := gadgets.GetGadget(database, id)
	if err != nil || g == nil || g.Brand != "Lenovo" {
		t.Fatalf("GetGadget failed: %+v, err=%v", g, err)
	}

	ok, err := gadgets.UpdateGadget(database, id, map[string]any{"status": "archived"})
	if err != nil || !ok {
		t.Errorf("UpdateGadget failed: ok=%v, err=%v", ok, err)
	}

	list, err := gadgets.ListGadgets(database, "laptop", "archived")
	if err != nil || len(list) != 1 {
		t.Errorf("ListGadgets failed: %+v, err=%v", list, err)
	}

	ok, err = gadgets.DeleteGadget(database, id)
	if err != nil || !ok {
		t.Errorf("DeleteGadget failed: ok=%v, err=%v", ok, err)
	}
}
