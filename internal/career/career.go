package career

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RoleExploration represents a vocational path or role experiment.
type RoleExploration struct {
	ID         int    `json:"id"`
	Role       string `json:"role"`
	Category   string `json:"category,omitempty"` // engineering, design, trading, writing, education
	Status     string `json:"status"`             // explored, active, pivoted, shelved
	DateLogged string `json:"date_logged"`        // YYYY-MM-DD
	Notes      string `json:"notes,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// SkillInventory represents a soft or hard skill tracked.
type SkillInventory struct {
	ID         int    `json:"id"`
	Skill      string `json:"skill"`
	SkillType  string `json:"skill_type"`  // soft, hard
	Status     string `json:"status"`      // active, learning, dormant
	DateLogged string `json:"date_logged"` // YYYY-MM-DD
	Notes      string `json:"notes,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// InsertRole records a role exploration entry.
func InsertRole(database *sql.DB, r RoleExploration) (int, error) {
	if strings.TrimSpace(r.Role) == "" {
		return 0, fmt.Errorf("role cannot be empty")
	}
	if r.Status == "" {
		r.Status = "explored"
	}
	if r.DateLogged == "" {
		r.DateLogged = time.Now().Format("2006-01-02")
	}

	res, err := database.Exec(`
		INSERT INTO role_explorations (
			role, category, status, date_logged, notes
		) VALUES (?, ?, ?, ?, ?)
	`, strings.TrimSpace(r.Role), strings.TrimSpace(r.Category), r.Status, r.DateLogged, r.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert role exploration: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListRoles retrieves role explorations.
func ListRoles(database *sql.DB, status string) ([]RoleExploration, error) {
	var conditions []string
	var params []any

	if status != "" && status != "all" {
		conditions = append(conditions, "status = ?")
		params = append(params, status)
	}

	query := `
		SELECT id, role, COALESCE(category, ''), status, date_logged, COALESCE(notes, ''), created_at, updated_at
		FROM role_explorations
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed querying role explorations: %w", err)
	}
	defer rows.Close()

	var roles []RoleExploration
	for rows.Next() {
		var r RoleExploration
		if err := rows.Scan(
			&r.ID, &r.Role, &r.Category, &r.Status,
			&r.DateLogged, &r.Notes, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning role: %w", err)
		}
		roles = append(roles, r)
	}

	return roles, nil
}

// InsertSkill records a skill inventory entry.
func InsertSkill(database *sql.DB, s SkillInventory) (int, error) {
	if strings.TrimSpace(s.Skill) == "" {
		return 0, fmt.Errorf("skill cannot be empty")
	}
	if s.SkillType == "" {
		s.SkillType = "soft"
	}
	if s.Status == "" {
		s.Status = "active"
	}
	if s.DateLogged == "" {
		s.DateLogged = time.Now().Format("2006-01-02")
	}

	res, err := database.Exec(`
		INSERT INTO skills_inventory (
			skill, skill_type, status, date_logged, notes
		) VALUES (?, ?, ?, ?, ?)
	`, strings.TrimSpace(s.Skill), strings.TrimSpace(s.SkillType), s.Status, s.DateLogged, s.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert skill: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListSkills retrieves skills filtered by skill_type.
func ListSkills(database *sql.DB, skillType string) ([]SkillInventory, error) {
	var conditions []string
	var params []any

	if skillType != "" && skillType != "all" {
		conditions = append(conditions, "skill_type = ?")
		params = append(params, skillType)
	}

	query := `
		SELECT id, skill, skill_type, status, date_logged, COALESCE(notes, ''), created_at, updated_at
		FROM skills_inventory
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY skill_type ASC, id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed querying skills: %w", err)
	}
	defer rows.Close()

	var skills []SkillInventory
	for rows.Next() {
		var sk SkillInventory
		if err := rows.Scan(
			&sk.ID, &sk.Skill, &sk.SkillType, &sk.Status,
			&sk.DateLogged, &sk.Notes, &sk.CreatedAt, &sk.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning skill: %w", err)
		}
		skills = append(skills, sk)
	}

	return skills, nil
}
