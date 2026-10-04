package contacts_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/db"
)

func TestContactsCRUDAndResolution(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_contacts.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Insert
	cID, err := contacts.InsertContact(database, contacts.Contact{
		Name:     "Budi Raharjo",
		Org:      "ITB",
		Location: "Bandung",
		Email:    "budi@example.com",
		Phone:    "+62811111111",
		Tier:     2,
	})
	if err != nil {
		t.Fatalf("InsertContact failed: %v", err)
	}
	if cID <= 0 {
		t.Fatalf("expected valid ID, got %d", cID)
	}

	// Resolve by ID string
	id, name, err := contacts.ResolveContact(database, "1")
	if err != nil || id != cID || name != "Budi Raharjo" {
		t.Errorf("Resolve by ID failed: id=%d, name=%s, err=%v", id, name, err)
	}

	// Resolve by exact name
	id, name, err = contacts.ResolveContact(database, "Budi Raharjo")
	if err != nil || id != cID || name != "Budi Raharjo" {
		t.Errorf("Resolve by exact name failed: id=%d, name=%s, err=%v", id, name, err)
	}

	// Resolve by substring
	id, name, err = contacts.ResolveContact(database, "Raharjo")
	if err != nil || id != cID || name != "Budi Raharjo" {
		t.Errorf("Resolve by substring failed: id=%d, name=%s, err=%v", id, name, err)
	}

	// Update
	ok, err := contacts.UpdateContact(database, cID, map[string]any{"location": "Jakarta"})
	if err != nil || !ok {
		t.Errorf("UpdateContact failed: ok=%v, err=%v", ok, err)
	}

	// Get
	c, err := contacts.GetContact(database, cID)
	if err != nil || c == nil || c.Location != "Jakarta" {
		t.Errorf("unexpected contact: %+v, err=%v", c, err)
	}

	// Delete
	ok, err = contacts.DeleteContact(database, cID)
	if err != nil || !ok {
		t.Errorf("DeleteContact failed: ok=%v, err=%v", ok, err)
	}
}
