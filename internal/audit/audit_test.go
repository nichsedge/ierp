package audit_test

import (
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/audit"
	"github.com/nichsedge/ierp/internal/db"
)

func TestAuditEngineScan(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// An empty database should flag missing net worth snapshot, missing commitments, etc.
	result, err := audit.RunAudit(database)
	if err != nil {
		t.Fatalf("RunAudit failed: %v", err)
	}

	if result.TotalFindings == 0 {
		t.Errorf("expected audit findings on empty database, got 0")
	}
	if result.ActionsNeeded == 0 {
		t.Errorf("expected action_needed findings, got 0")
	}

	hasTreasuryFinding := false
	for _, f := range result.Findings {
		if f.Domain == "Treasury" {
			hasTreasuryFinding = true
			break
		}
	}
	if !hasTreasuryFinding {
		t.Errorf("expected Treasury audit finding")
	}
}
