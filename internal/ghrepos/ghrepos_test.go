package ghrepos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nichsedge/ierp/internal/db"
)

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Errorf("expected 1 for true, got %d", boolToInt(true))
	}
	if boolToInt(false) != 0 {
		t.Errorf("expected 0 for false, got %d", boolToInt(false))
	}
}

func TestExportShowcase(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ierp-ghrepos-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "events.db")
	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// Insert mock repos
	_, err = database.Exec(`
		INSERT INTO github_repositories (
			repo_id, name, full_name, owner_login, owner_url, html_url,
			homepage, description, topics, language, private, fork,
			archived, template, disabled, created_at, updated_at, pushed_at,
			default_branch, default_branch_oid, stargazers_count, watchers_count,
			forks_count, open_issues_count, open_prs_count, license_spdx, license_name
		) VALUES (
			'R_1', 'ierp', 'nichsedge/ierp', 'nichsedge', 'https://github.com/nichsedge', 'https://github.com/nichsedge/ierp',
			'https://ichsanul.dev', 'Personal ERP', '["go", "sqlite", "wal"]', 'Go', 0, 0,
			0, 0, 0, '2026-01-01T00:00:00Z', '2026-10-23T00:00:00Z', '2026-10-23T00:00:00Z',
			'main', 'abc1234', 10, 10,
			2, 0, 0, 'MIT', 'MIT License'
		), (
			'R_2', 'notes', 'nichsedge/notes', 'nichsedge', 'https://github.com/nichsedge', 'https://github.com/nichsedge/notes',
			NULL, NULL, NULL, 'Markdown', 1, 0,
			0, 0, 0, '2026-02-01T00:00:00Z', '2026-10-20T00:00:00Z', '2026-10-20T00:00:00Z',
			'main', 'def5678', 1, 1,
			0, 0, 0, NULL, NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed inserting mock repos: %v", err)
	}

	outputPath := filepath.Join(tempDir, "data", "github_repos_all.json")
	count, err := ExportGitHubRepos(t.Context(), database, outputPath)
	if err != nil {
		t.Fatalf("ExportGitHubRepos failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 exported repos, got %d", count)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed reading exported file: %v", err)
	}

	var output struct {
		GeneratedAtUTC string         `json:"generated_at_utc"`
		Source         string         `json:"source"`
		Count          int            `json:"count"`
		Repos          []ExportedRepo `json:"repos"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("failed parsing exported JSON: %v", err)
	}

	if output.Source != "ierp (events.db)" {
		t.Errorf("expected source 'ierp (events.db)', got %q", output.Source)
	}
	if output.Count != 2 || len(output.Repos) != 2 {
		t.Fatalf("expected 2 repos in payload, got %d", output.Count)
	}

	// Verify repo 1
	r1 := output.Repos[0]
	if r1.Name != "ierp" || r1.Language == nil || *r1.Language != "Go" {
		t.Errorf("unexpected repo 1 data: %+v", r1)
	}
	if len(r1.Topics) != 3 || r1.Topics[0] != "go" {
		t.Errorf("unexpected topics in repo 1: %v", r1.Topics)
	}
	if r1.LicenseSPDX == nil || *r1.LicenseSPDX != "MIT" {
		t.Errorf("unexpected license in repo 1: %v", r1.LicenseSPDX)
	}

	// Verify repo 2 with null topics -> must be non-nil empty slice
	r2 := output.Repos[1]
	if r2.Name != "notes" || !r2.Private {
		t.Errorf("unexpected repo 2 data: %+v", r2)
	}
	if r2.Topics == nil {
		t.Errorf("expected non-nil topics slice for repo 2, got nil")
	}
	if len(r2.Topics) != 0 {
		t.Errorf("expected empty topics slice for repo 2, got %v", r2.Topics)
	}
}
