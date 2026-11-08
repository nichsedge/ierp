package career_test

import (
	"database/sql"
	"testing"

	"github.com/nichsedge/ierp/internal/career"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS role_explorations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		role TEXT NOT NULL,
		category TEXT,
		status TEXT DEFAULT 'explored',
		date_logged TEXT NOT NULL,
		notes TEXT,
		created_at TEXT DEFAULT (datetime('now', 'localtime')),
		updated_at TEXT DEFAULT (datetime('now', 'localtime'))
	);

	CREATE TABLE IF NOT EXISTS skills_inventory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		skill TEXT NOT NULL,
		skill_type TEXT NOT NULL,
		status TEXT DEFAULT 'active',
		date_logged TEXT NOT NULL,
		notes TEXT,
		created_at TEXT DEFAULT (datetime('now', 'localtime')),
		updated_at TEXT DEFAULT (datetime('now', 'localtime'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return db
}

func TestCareerLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// 1. Roles
	roleID, err := career.InsertRole(db, career.RoleExploration{
		Role:       "Quant / Algo Trader",
		Category:   "Trading / Finance",
		Status:     "explored",
		DateLogged: "2026-11-08",
		Notes:      "Explored algorithmic systems",
	})
	if err != nil || roleID <= 0 {
		t.Fatalf("failed inserting role: %v", err)
	}

	roles, err := career.ListRoles(db, "explored")
	if err != nil || len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d (err=%v)", len(roles), err)
	}
	if roles[0].Role != "Quant / Algo Trader" {
		t.Errorf("unexpected role name: %s", roles[0].Role)
	}

	// 2. Skills
	skillID, err := career.InsertSkill(db, career.SkillInventory{
		Skill:      "SQL",
		SkillType:  "hard",
		Status:     "active",
		DateLogged: "2026-11-08",
	})
	if err != nil || skillID <= 0 {
		t.Fatalf("failed inserting skill: %v", err)
	}

	skills, err := career.ListSkills(db, "hard")
	if err != nil || len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d (err=%v)", len(skills), err)
	}
	if skills[0].Skill != "SQL" {
		t.Errorf("unexpected skill name: %s", skills[0].Skill)
	}
}
