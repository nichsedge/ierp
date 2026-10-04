package projects_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
	"github.com/nichsedge/ierp/internal/projects"
)

func TestProjectsCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_projects.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	id, err := projects.InsertProject(database, projects.Project{
		Title:       "Migrate iERP to Go",
		Description: "Port all Python domain services and CLI to modern Go",
		Priority:    "high",
		StartDate:   "2026-10-04",
	})
	if err != nil || id <= 0 {
		t.Fatalf("InsertProject failed: id=%d, err=%v", id, err)
	}

	p, err := projects.GetProject(database, id)
	if err != nil || p == nil {
		t.Fatalf("GetProject failed: %+v, err=%v", p, err)
	}
	if p.Title != "Migrate iERP to Go" || p.Priority != "high" {
		t.Errorf("unexpected project: %+v", p)
	}

	ok, err := projects.UpdateProject(database, id, map[string]any{"status": "completed"})
	if err != nil || !ok {
		t.Errorf("UpdateProject failed: ok=%v, err=%v", ok, err)
	}

	projs, err := projects.ListProjects(database, "completed")
	if err != nil || len(projs) != 1 {
		t.Errorf("expected 1 completed project, got %d, err=%v", len(projs), err)
	}

	ok, err = projects.DeleteProject(database, id)
	if err != nil || !ok {
		t.Errorf("DeleteProject failed: ok=%v, err=%v", ok, err)
	}
}
