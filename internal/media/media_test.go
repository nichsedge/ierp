package media_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/media"
)

func TestMediaAndLinks(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_media.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Upsert initial
	id1, err := media.UpsertMediaItem(database, media.MediaItem{
		MediaType: "book",
		Title:     "Designing Data-Intensive Applications",
		Source:    "goodreads",
		Data: map[string]any{
			"author": "Martin Kleppmann",
			"rating": 5,
		},
	})
	if err != nil || id1 <= 0 {
		t.Fatalf("UpsertMediaItem failed: id=%d, err=%v", id1, err)
	}

	// Idempotent upsert with merge
	id2, err := media.UpsertMediaItem(database, media.MediaItem{
		MediaType: "book",
		Title:     "Designing Data-Intensive Applications",
		Source:    "goodreads",
		Data: map[string]any{
			"status": "read",
		},
	})
	if err != nil || id2 != id1 {
		t.Fatalf("Expected same ID on merge, got %d vs %d", id2, id1)
	}

	items, err := media.ListMediaItems(database, "book", "goodreads")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListMediaItems failed: %+v, err=%v", items, err)
	}
	if items[0].Data["author"] != "Martin Kleppmann" || items[0].Data["status"] != "read" {
		t.Errorf("Unexpected merged data: %+v", items[0].Data)
	}

	// Link
	lID, err := media.UpsertLink(database, media.Link{
		Label:    "GitHub Profile",
		URL:      "https://github.com/nichsedge",
		Category: "social",
		IsPublic: true,
	})
	if err != nil || lID <= 0 {
		t.Fatalf("UpsertLink failed: id=%d, err=%v", lID, err)
	}

	links, err := media.ListLinks(database, "social")
	if err != nil || len(links) != 1 {
		t.Fatalf("ListLinks failed: %+v, err=%v", links, err)
	}
}
