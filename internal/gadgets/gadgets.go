package gadgets

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Gadget represents a physical hardware asset.
type Gadget struct {
	ID            int     `json:"id"`
	Slug          string  `json:"slug"`
	Name          string  `json:"name"`
	Brand         string  `json:"brand,omitempty"`
	Model         string  `json:"model,omitempty"`
	Category      string  `json:"category,omitempty"`
	Status        string  `json:"status"`
	PurchaseDate  string  `json:"purchase_date,omitempty"`
	PurchasePrice float64 `json:"purchase_price,omitempty"`
	Currency      string  `json:"currency"`
	SpecsJSON     string  `json:"specs_json,omitempty"`
	SerialNumber  string  `json:"serial_number,omitempty"`
	VendorID      *int    `json:"vendor_id,omitempty"`
	VendorName    string  `json:"vendor_name,omitempty"`
	EventID       *int    `json:"event_id,omitempty"`
	Notes         string  `json:"notes,omitempty"`
	IsPublic      bool    `json:"is_public"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

var nonSlugRegex = regexp.MustCompile(`[^\w\s-]`)
var spaceHyphenRegex = regexp.MustCompile(`[\s_-]+`)

// GenerateGadgetSlug creates a URL-safe slug for a gadget.
func GenerateGadgetSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlugRegex.ReplaceAllString(s, "")
	s = spaceHyphenRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fmt.Sprintf("gadget-%d", time.Now().Unix())
	}
	return s
}

// InsertGadget records a new gadget.
func InsertGadget(database *sql.DB, g Gadget) (int, error) {
	if g.Slug == "" {
		g.Slug = GenerateGadgetSlug(g.Name)
	}
	if g.Status == "" {
		g.Status = "active"
	}
	if g.Currency == "" {
		g.Currency = "IDR"
	}

	var existingID int
	if err := database.QueryRow("SELECT id FROM gadgets WHERE slug = ?", g.Slug).Scan(&existingID); err == nil {
		g.Slug = fmt.Sprintf("%s-%d", g.Slug, time.Now().Unix())
	}

	isPublicInt := 1
	if !g.IsPublic {
		isPublicInt = 0
	}

	res, err := database.Exec(`
		INSERT INTO gadgets (
			slug, name, brand, model, category, status, purchase_date, purchase_price,
			currency, specs_json, serial_number, vendor_id, event_id, notes, is_public
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, g.Slug, strings.TrimSpace(g.Name), g.Brand, g.Model, g.Category, g.Status,
		g.PurchaseDate, g.PurchasePrice, g.Currency, g.SpecsJSON, g.SerialNumber,
		g.VendorID, g.EventID, g.Notes, isPublicInt)
	if err != nil {
		return 0, fmt.Errorf("failed to insert gadget: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListGadgets retrieves gadgets with optional filters.
func ListGadgets(database *sql.DB, category string, status string) ([]Gadget, error) {
	query := `
		SELECT g.id, g.slug, g.name, COALESCE(g.brand, ''), COALESCE(g.model, ''),
		       COALESCE(g.category, 'other'), COALESCE(g.status, 'active'),
		       COALESCE(g.purchase_date, ''), COALESCE(g.purchase_price, 0),
		       COALESCE(g.currency, 'IDR'), COALESCE(g.specs_json, '{}'),
		       COALESCE(g.serial_number, ''), g.vendor_id, COALESCE(v.name, ''),
		       g.event_id, COALESCE(g.notes, ''), COALESCE(g.is_public, 1),
		       COALESCE(g.created_at, ''), COALESCE(g.updated_at, '')
		FROM gadgets g
		LEFT JOIN vendors v ON v.id = g.vendor_id
		WHERE 1=1
	`
	var params []any
	if category != "" {
		query += " AND g.category = ?"
		params = append(params, strings.ToLower(category))
	}
	if status != "" && strings.ToLower(status) != "all" {
		query += " AND g.status = ?"
		params = append(params, strings.ToLower(status))
	}

	query += " ORDER BY g.id DESC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Gadget
	for rows.Next() {
		var g Gadget
		var isPublicInt int
		if err := rows.Scan(
			&g.ID, &g.Slug, &g.Name, &g.Brand, &g.Model,
			&g.Category, &g.Status, &g.PurchaseDate, &g.PurchasePrice,
			&g.Currency, &g.SpecsJSON, &g.SerialNumber,
			&g.VendorID, &g.VendorName, &g.EventID,
			&g.Notes, &isPublicInt,
			&g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		g.IsPublic = (isPublicInt == 1)
		list = append(list, g)
	}
	return list, nil
}

// GetGadget retrieves a gadget by ID.
func GetGadget(database *sql.DB, id int) (*Gadget, error) {
	row := database.QueryRow(`
		SELECT g.id, g.slug, g.name, COALESCE(g.brand, ''), COALESCE(g.model, ''),
		       COALESCE(g.category, 'other'), COALESCE(g.status, 'active'),
		       COALESCE(g.purchase_date, ''), COALESCE(g.purchase_price, 0),
		       COALESCE(g.currency, 'IDR'), COALESCE(g.specs_json, '{}'),
		       COALESCE(g.serial_number, ''), g.vendor_id, COALESCE(v.name, ''),
		       g.event_id, COALESCE(g.notes, ''), COALESCE(g.is_public, 1),
		       COALESCE(g.created_at, ''), COALESCE(g.updated_at, '')
		FROM gadgets g
		LEFT JOIN vendors v ON v.id = g.vendor_id
		WHERE g.id = ?
	`, id)

	var g Gadget
	var isPublicInt int
	if err := row.Scan(
		&g.ID, &g.Slug, &g.Name, &g.Brand, &g.Model,
		&g.Category, &g.Status, &g.PurchaseDate, &g.PurchasePrice,
		&g.Currency, &g.SpecsJSON, &g.SerialNumber,
		&g.VendorID, &g.VendorName, &g.EventID,
		&g.Notes, &isPublicInt,
		&g.CreatedAt, &g.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	g.IsPublic = (isPublicInt == 1)
	return &g, nil
}

// UpdateGadget updates fields of a gadget.
func UpdateGadget(database *sql.DB, id int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	allowed := map[string]bool{
		"name": true, "brand": true, "model": true, "category": true,
		"status": true, "purchase_date": true, "purchase_price": true,
		"currency": true, "specs_json": true, "serial_number": true,
		"vendor_id": true, "event_id": true, "notes": true, "is_public": true,
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

	query := fmt.Sprintf("UPDATE gadgets SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteGadget removes a gadget by ID.
func DeleteGadget(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM gadgets WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
