package lifeops

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MaintenanceItem represents a preventive maintenance or renewal task.
type MaintenanceItem struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	DueDate      string   `json:"due_date"`
	IntervalDays *int     `json:"interval_days,omitempty"`
	Status       string   `json:"status"`
	Cost         float64  `json:"cost"`
	Notes        string   `json:"notes"`
	GadgetID     *int     `json:"gadget_id,omitempty"`
	GadgetName   string   `json:"gadget_name,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	IsOverdue    bool     `json:"is_overdue"`
	DaysOverdue  int      `json:"days_overdue"`
}

// InsertMaintenance creates a maintenance schedule item.
func InsertMaintenance(database *sql.DB, item MaintenanceItem) (int, error) {
	cat := strings.ToLower(strings.TrimSpace(item.Category))
	if cat == "" {
		cat = "general"
	}
	status := strings.ToLower(strings.TrimSpace(item.Status))
	if status == "" {
		status = "pending"
	}

	res, err := database.Exec(`
		INSERT INTO maintenance_items (name, category, due_date, interval_days, cost, notes, gadget_id, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(item.Name), cat, item.DueDate, item.IntervalDays, item.Cost, item.Notes, item.GadgetID, status)
	if err != nil {
		return 0, fmt.Errorf("failed to insert maintenance item: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListMaintenance retrieves maintenance tasks with overdue status computation.
func ListMaintenance(database *sql.DB, status string, category string, overdueOnly bool) ([]MaintenanceItem, error) {
	query := `
		SELECT m.id, m.name, COALESCE(m.category, 'general'), m.due_date,
		       m.interval_days, COALESCE(m.status, 'pending'), COALESCE(m.cost, 0),
		       COALESCE(m.notes, ''), m.gadget_id, COALESCE(g.name, ''),
		       COALESCE(m.created_at, ''), COALESCE(m.updated_at, '')
		FROM maintenance_items m
		LEFT JOIN gadgets g ON g.id = m.gadget_id
		WHERE 1=1
	`
	var params []any
	if status != "" && strings.ToLower(status) != "all" {
		query += " AND m.status = ?"
		params = append(params, strings.ToLower(status))
	}
	if category != "" {
		query += " AND m.category = ?"
		params = append(params, strings.ToLower(category))
	}

	query += " ORDER BY m.due_date ASC, m.id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	todayStr := now.Format("2006-01-02")
	var items []MaintenanceItem

	for rows.Next() {
		var it MaintenanceItem
		if err := rows.Scan(
			&it.ID, &it.Name, &it.Category, &it.DueDate,
			&it.IntervalDays, &it.Status, &it.Cost,
			&it.Notes, &it.GadgetID, &it.GadgetName,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if it.Status == "pending" && it.DueDate != "" {
			if it.DueDate < todayStr {
				it.IsOverdue = true
				if t, err := time.Parse("2006-01-02", it.DueDate[:10]); err == nil {
					it.DaysOverdue = int(now.Sub(t).Hours() / 24)
				}
			}
		}

		if overdueOnly && !it.IsOverdue {
			continue
		}

		items = append(items, it)
	}

	return items, nil
}

// CompleteMaintenance marks a task completed and reschedules if recurring.
func CompleteMaintenance(database *sql.DB, itemID int, completionDate string, cost *float64) (map[string]any, error) {
	if completionDate == "" {
		completionDate = time.Now().Format("2006-01-02")
	}
	baseDate, err := time.Parse("2006-01-02", completionDate[:10])
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("invalid date format: %s", completionDate)}, nil
	}

	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var it MaintenanceItem
	row := tx.QueryRow(`
		SELECT id, name, category, due_date, interval_days, cost, notes, gadget_id
		FROM maintenance_items WHERE id = ?
	`, itemID)

	if err := row.Scan(&it.ID, &it.Name, &it.Category, &it.DueDate, &it.IntervalDays, &it.Cost, &it.Notes, &it.GadgetID); err != nil {
		if err == sql.ErrNoRows {
			return map[string]any{"success": false, "error": "item not found"}, nil
		}
		return nil, err
	}

	finalCost := it.Cost
	if cost != nil {
		finalCost = *cost
	}

	if _, err := tx.Exec(`
		UPDATE maintenance_items
		SET status = 'completed', cost = ?, updated_at = datetime('now', 'localtime')
		WHERE id = ?
	`, finalCost, itemID); err != nil {
		return nil, err
	}

	var nextID *int
	var nextDue string
	if it.IntervalDays != nil && *it.IntervalDays > 0 {
		nextTime := baseDate.AddDate(0, 0, *it.IntervalDays)
		nextDue = nextTime.Format("2006-01-02")

		res, err := tx.Exec(`
			INSERT INTO maintenance_items (name, category, due_date, interval_days, cost, notes, gadget_id, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
		`, it.Name, it.Category, nextDue, it.IntervalDays, it.Cost, it.Notes, it.GadgetID)
		if err != nil {
			return nil, err
		}
		nid64, err := res.LastInsertId()
		if err == nil {
			nid := int(nid64)
			nextID = &nid
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	result := map[string]any{
		"success":           true,
		"completed_id":      itemID,
		"rescheduled":       nextID != nil,
		"next_due_date":     nextDue,
		"next_item_id":      nextID,
	}
	return result, nil
}

// DeleteMaintenance deletes a maintenance item.
func DeleteMaintenance(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM maintenance_items WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
