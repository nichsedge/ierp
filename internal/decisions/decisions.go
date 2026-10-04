package decisions

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Decision represents a judgment log entry.
type Decision struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	Context         string `json:"context,omitempty"`
	Choice          string `json:"choice"`
	ExpectedOutcome string `json:"expected_outcome,omitempty"`
	Confidence      int    `json:"confidence"`
	ReviewDate      string `json:"review_date,omitempty"`
	ActualOutcome   string `json:"actual_outcome,omitempty"`
	Status          string `json:"status"`
	ProjectID       *int   `json:"project_id,omitempty"`
	ProjectTitle    string `json:"project_title,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// InsertDecision records a new decision log.
func InsertDecision(database *sql.DB, d Decision) (int, error) {
	if d.Confidence < 1 {
		d.Confidence = 1
	}
	if d.Confidence > 10 {
		d.Confidence = 10
	}
	if d.Status == "" {
		d.Status = "pending"
	}

	res, err := database.Exec(`
		INSERT INTO decisions (
			title, context, choice, expected_outcome, confidence, review_date, project_id, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(d.Title), d.Context, strings.TrimSpace(d.Choice), d.ExpectedOutcome, d.Confidence, d.ReviewDate, d.ProjectID, d.Status)
	if err != nil {
		return 0, fmt.Errorf("failed to insert decision: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ReviewDecision records the retrospective review and actual outcome.
func ReviewDecision(database *sql.DB, id int, actualOutcome string, status string) (bool, error) {
	if status == "" {
		status = "reviewed"
	}
	res, err := database.Exec(`
		UPDATE decisions
		SET actual_outcome = ?, status = ?, updated_at = datetime('now', 'localtime')
		WHERE id = ?
	`, strings.TrimSpace(actualOutcome), status, id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// UpdateDecision updates fields of an existing decision.
func UpdateDecision(database *sql.DB, id int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	allowed := map[string]bool{
		"title": true, "choice": true, "context": true, "expected_outcome": true,
		"confidence": true, "review_date": true, "project_id": true, "status": true,
		"actual_outcome": true,
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

	query := fmt.Sprintf("UPDATE decisions SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// ListDecisions retrieves decisions with filtering.
func ListDecisions(database *sql.DB, status string, projectID *int) ([]Decision, error) {
	query := `
		SELECT d.id, d.title, COALESCE(d.context, ''), d.choice,
		       COALESCE(d.expected_outcome, ''), d.confidence, COALESCE(d.review_date, ''),
		       COALESCE(d.actual_outcome, ''), COALESCE(d.status, 'pending'),
		       d.project_id, COALESCE(p.title, ''),
		       COALESCE(d.created_at, ''), COALESCE(d.updated_at, '')
		FROM decisions d
		LEFT JOIN projects p ON p.id = d.project_id
		WHERE 1=1
	`
	var params []any
	if status != "" && strings.ToLower(status) != "all" {
		query += " AND d.status = ?"
		params = append(params, strings.ToLower(status))
	}
	if projectID != nil {
		query += " AND d.project_id = ?"
		params = append(params, *projectID)
	}

	query += " ORDER BY d.id DESC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Decision
	for rows.Next() {
		var d Decision
		if err := rows.Scan(
			&d.ID, &d.Title, &d.Context, &d.Choice,
			&d.ExpectedOutcome, &d.Confidence, &d.ReviewDate,
			&d.ActualOutcome, &d.Status,
			&d.ProjectID, &d.ProjectTitle,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

// GetDecision retrieves a decision by ID.
func GetDecision(database *sql.DB, id int) (*Decision, error) {
	row := database.QueryRow(`
		SELECT d.id, d.title, COALESCE(d.context, ''), d.choice,
		       COALESCE(d.expected_outcome, ''), d.confidence, COALESCE(d.review_date, ''),
		       COALESCE(d.actual_outcome, ''), COALESCE(d.status, 'pending'),
		       d.project_id, COALESCE(p.title, ''),
		       COALESCE(d.created_at, ''), COALESCE(d.updated_at, '')
		FROM decisions d
		LEFT JOIN projects p ON p.id = d.project_id
		WHERE d.id = ?
	`, id)

	var d Decision
	if err := row.Scan(
		&d.ID, &d.Title, &d.Context, &d.Choice,
		&d.ExpectedOutcome, &d.Confidence, &d.ReviewDate,
		&d.ActualOutcome, &d.Status,
		&d.ProjectID, &d.ProjectTitle,
		&d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

// DeleteDecision removes a decision by ID.
func DeleteDecision(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM decisions WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// GetDecisionAlerts finds pending decisions that are overdue or upcoming within daysAhead.
func GetDecisionAlerts(database *sql.DB, daysAhead int) (overdue []Decision, upcoming []Decision, err error) {
	if daysAhead <= 0 {
		daysAhead = 14
	}
	allPending, err := ListDecisions(database, "pending", nil)
	if err != nil {
		return nil, nil, err
	}

	today := time.Now().Format("2006-01-02")
	horizon := time.Now().AddDate(0, 0, daysAhead).Format("2006-01-02")

	for _, d := range allPending {
		if d.ReviewDate == "" {
			continue
		}
		if d.ReviewDate < today {
			overdue = append(overdue, d)
		} else if d.ReviewDate <= horizon {
			upcoming = append(upcoming, d)
		}
	}

	return overdue, upcoming, nil
}
