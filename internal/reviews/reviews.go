package reviews

import (
	"database/sql"
	"fmt"
	"strings"
)

// Retrospective represents a sprint review reflection log.
type Retrospective struct {
	ID            int    `json:"id"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	PeriodType    string `json:"period_type"`
	Wins          string `json:"wins,omitempty"`
	DrainsBurnout string `json:"drains_burnout,omitempty"`
	Lessons       string `json:"lessons,omitempty"`
	FocusNext     string `json:"focus_next,omitempty"`
	Rating        int    `json:"rating"`
	Notes         string `json:"notes,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// InsertRetrospective records a retrospective log.
func InsertRetrospective(database *sql.DB, r Retrospective) (int, error) {
	if r.PeriodType == "" {
		r.PeriodType = "monthly"
	}
	if r.Rating < 1 {
		r.Rating = 1
	}
	if r.Rating > 10 {
		r.Rating = 10
	}

	res, err := database.Exec(`
		INSERT INTO retrospectives (
			period_start, period_end, period_type, wins, drains_burnout, lessons, focus_next, rating, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.PeriodStart, r.PeriodEnd, r.PeriodType, r.Wins, r.DrainsBurnout, r.Lessons, r.FocusNext, r.Rating, r.Notes)
	if err != nil {
		return 0, fmt.Errorf("failed to insert retrospective: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListRetrospectives retrieves retrospectives sorted by period_start descending.
func ListRetrospectives(database *sql.DB, periodType string, limit int) ([]Retrospective, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, period_start, period_end, COALESCE(period_type, 'monthly'),
		       COALESCE(wins, ''), COALESCE(drains_burnout, ''), COALESCE(lessons, ''),
		       COALESCE(focus_next, ''), rating, COALESCE(notes, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM retrospectives
		WHERE 1=1
	`
	var params []any
	if periodType != "" && strings.ToLower(periodType) != "all" {
		query += " AND period_type = ?"
		params = append(params, strings.ToLower(periodType))
	}
	query += " ORDER BY period_start DESC, id DESC LIMIT ?"
	params = append(params, limit)

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Retrospective
	for rows.Next() {
		var r Retrospective
		if err := rows.Scan(
			&r.ID, &r.PeriodStart, &r.PeriodEnd, &r.PeriodType,
			&r.Wins, &r.DrainsBurnout, &r.Lessons, &r.FocusNext,
			&r.Rating, &r.Notes, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// GetRetrospective retrieves a retrospective by ID.
func GetRetrospective(database *sql.DB, id int) (*Retrospective, error) {
	row := database.QueryRow(`
		SELECT id, period_start, period_end, COALESCE(period_type, 'monthly'),
		       COALESCE(wins, ''), COALESCE(drains_burnout, ''), COALESCE(lessons, ''),
		       COALESCE(focus_next, ''), rating, COALESCE(notes, ''),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM retrospectives
		WHERE id = ?
	`, id)

	var r Retrospective
	if err := row.Scan(
		&r.ID, &r.PeriodStart, &r.PeriodEnd, &r.PeriodType,
		&r.Wins, &r.DrainsBurnout, &r.Lessons, &r.FocusNext,
		&r.Rating, &r.Notes, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// UpdateRetrospective updates fields of a retrospective.
func UpdateRetrospective(database *sql.DB, id int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	allowed := map[string]bool{
		"period_start": true, "period_end": true, "period_type": true,
		"wins": true, "drains_burnout": true, "lessons": true,
		"focus_next": true, "rating": true, "notes": true,
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

	query := fmt.Sprintf("UPDATE retrospectives SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteRetrospective deletes a retrospective by ID.
func DeleteRetrospective(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM retrospectives WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
