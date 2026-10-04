package garden_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/garden"
	"github.com/nichsedge/ierp/internal/projects"
)

func TestGardenExport(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_garden.db")
	gardenDir := filepath.Join(tmpDir, "content")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Insert test project
	_, err = projects.InsertProject(database, projects.Project{
		Title:       "Test Initiative",
		Description: "Testing Garden Export",
		Priority:    "high",
		StartDate:   "2026-10-01",
	})
	if err != nil {
		t.Fatalf("InsertProject failed: %v", err)
	}

	summary, err := garden.ExportAll(database, gardenDir)
	if err != nil {
		t.Fatalf("ExportAll failed: %v", err)
	}
	if summary.Projects != 1 {
		t.Errorf("expected 1 exported project, got %d", summary.Projects)
	}
}
