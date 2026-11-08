package antivision

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AntiVisionItem represents a negative constraint / guardrail.
type AntiVisionItem struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Domains     string `json:"domains"`      // comma-separated domains
	Status      string `json:"status"`       // active, mitigated, transcended, deprecated
	Severity    int    `json:"severity"`     // 1 to 5
	DateLogged  string `json:"date_logged"`  // YYYY-MM-DD
	Notes       string `json:"notes,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// InsertAntiVision records a new guardrail.
func InsertAntiVision(database *sql.DB, item AntiVisionItem) (int, error) {
	if strings.TrimSpace(item.Description) == "" {
		return 0, fmt.Errorf("description cannot be empty")
	}
	if item.Severity < 1 {
		item.Severity = 1
	}
	if item.Severity > 5 {
		item.Severity = 5
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if item.DateLogged == "" {
		item.DateLogged = time.Now().Format("2006-01-02")
	}

	res, err := database.Exec(`
		INSERT INTO antivision_items (
			description, domains, status, severity, date_logged, notes
		) VALUES (?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(item.Description), strings.TrimSpace(item.Domains), item.Status, item.Severity, item.DateLogged, item.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert antivision item: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// UpdateAntiVision updates status and notes for an antivision item.
func UpdateAntiVision(database *sql.DB, id int, status string, severity *int, notes string) (bool, error) {
	var setClauses []string
	var params []any

	if status != "" {
		setClauses = append(setClauses, "status = ?")
		params = append(params, status)
	}
	if severity != nil {
		s := *severity
		if s < 1 {
			s = 1
		}
		if s > 5 {
			s = 5
		}
		setClauses = append(setClauses, "severity = ?")
		params = append(params, s)
	}
	if notes != "" {
		setClauses = append(setClauses, "notes = ?")
		params = append(params, notes)
	}

	if len(setClauses) == 0 {
		return false, nil
	}

	setClauses = append(setClauses, "updated_at = datetime('now', 'localtime')")
	params = append(params, id)

	query := fmt.Sprintf("UPDATE antivision_items SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteAntiVision removes an item by ID.
func DeleteAntiVision(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM antivision_items WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// ListAntiVision retrieves items filtered by domain and status.
func ListAntiVision(database *sql.DB, domain, status string) ([]AntiVisionItem, error) {
	var conditions []string
	var params []any

	if status != "" && status != "all" {
		conditions = append(conditions, "status = ?")
		params = append(params, status)
	}
	if domain != "" && domain != "all" {
		conditions = append(conditions, "domains LIKE ?")
		params = append(params, "%"+domain+"%")
	}

	query := `
		SELECT id, description, domains, status, severity, date_logged, COALESCE(notes, ''), created_at, updated_at
		FROM antivision_items
	`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY severity DESC, date_logged DESC, id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed querying antivision items: %w", err)
	}
	defer rows.Close()

	var items []AntiVisionItem
	for rows.Next() {
		var it AntiVisionItem
		if err := rows.Scan(
			&it.ID, &it.Description, &it.Domains, &it.Status, &it.Severity,
			&it.DateLogged, &it.Notes, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning antivision item: %w", err)
		}
		items = append(items, it)
	}

	return items, nil
}
