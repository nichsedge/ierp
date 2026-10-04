package projects

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Project represents a strategic initiative.
type Project struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	StartDate   string `json:"start_date,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

var nonSlugRegex = regexp.MustCompile(`[^\w\s-]`)
var spaceHyphenRegex = regexp.MustCompile(`[\s_-]+`)

// GenerateProjectSlug creates a URL-safe slug from a project title.
func GenerateProjectSlug(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonSlugRegex.ReplaceAllString(s, "")
	s = spaceHyphenRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fmt.Sprintf("project-%d", time.Now().Unix())
	}
	return s
}

// InsertProject records a new project.
func InsertProject(database *sql.DB, p Project) (int, error) {
	if p.Slug == "" {
		p.Slug = GenerateProjectSlug(p.Title)
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Priority == "" {
		p.Priority = "medium"
	}

	// Ensure unique slug
	var existingID int
	if err := database.QueryRow("SELECT id FROM projects WHERE slug = ?", p.Slug).Scan(&existingID); err == nil {
		p.Slug = fmt.Sprintf("%s-%d", p.Slug, time.Now().Unix())
	}

	res, err := database.Exec(`
		INSERT INTO projects (slug, title, description, status, priority, start_date, target_date)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, p.Slug, strings.TrimSpace(p.Title), p.Description, p.Status, p.Priority, p.StartDate, p.TargetDate)
	if err != nil {
		return 0, fmt.Errorf("failed to insert project: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListProjects lists projects with optional status filter.
func ListProjects(database *sql.DB, status string) ([]Project, error) {
	query := `
		SELECT id, slug, title, COALESCE(description, ''),
		       COALESCE(status, 'active'), COALESCE(priority, 'medium'),
		       COALESCE(start_date, ''), COALESCE(target_date, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM projects
		WHERE 1=1
	`
	var params []any
	if status != "" && strings.ToLower(status) != "all" {
		query += " AND status = ?"
		params = append(params, strings.ToLower(status))
	}
	query += " ORDER BY id DESC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Title, &p.Description,
			&p.Status, &p.Priority, &p.StartDate, &p.TargetDate,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

// GetProject retrieves a project by ID.
func GetProject(database *sql.DB, id int) (*Project, error) {
	row := database.QueryRow(`
		SELECT id, slug, title, COALESCE(description, ''),
		       COALESCE(status, 'active'), COALESCE(priority, 'medium'),
		       COALESCE(start_date, ''), COALESCE(target_date, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM projects
		WHERE id = ?
	`, id)

	var p Project
	if err := row.Scan(
		&p.ID, &p.Slug, &p.Title, &p.Description,
		&p.Status, &p.Priority, &p.StartDate, &p.TargetDate,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// UpdateProject updates fields of a project.
func UpdateProject(database *sql.DB, id int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	allowed := map[string]bool{
		"title": true, "description": true, "status": true,
		"priority": true, "start_date": true, "target_date": true,
	}

	var setClauses []string
	var params []any
	for k, v := range updates {
		if allowed[k] {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", k))
			params = append(params, v)
		}
	}

	if len(setClauses) == 0 {
		return false, nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now', 'localtime')")
	params = append(params, id)

	query := fmt.Sprintf("UPDATE projects SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteProject deletes a project by ID.
func DeleteProject(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
