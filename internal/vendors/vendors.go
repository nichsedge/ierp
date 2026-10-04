package vendors

import (
	"database/sql"
	"fmt"
	"strings"
)

// Vendor represents a merchant or service provider.
type Vendor struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category,omitempty"`
	Location  string `json:"location,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	URL       string `json:"url,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Favorite  bool   `json:"favorite"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// InsertVendor records a new vendor.
func InsertVendor(database *sql.DB, v Vendor) (int, error) {
	favInt := 0
	if v.Favorite {
		favInt = 1
	}
	src := v.Source
	if src == "" {
		src = "manual"
	}

	res, err := database.Exec(`
		INSERT INTO vendors (name, category, location, phone, email, url, notes, favorite, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(v.Name), v.Category, v.Location, v.Phone, v.Email, v.URL, v.Notes, favInt, src)
	if err != nil {
		return 0, fmt.Errorf("failed to insert vendor: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListVendors retrieves vendors with category and favorite filters.
func ListVendors(database *sql.DB, category string, favoriteOnly bool) ([]Vendor, error) {
	query := `
		SELECT id, name, COALESCE(category, 'general'), COALESCE(location, ''),
		       COALESCE(phone, ''), COALESCE(email, ''), COALESCE(url, ''),
		       COALESCE(notes, ''), COALESCE(favorite, 0), COALESCE(source, 'manual'),
		       COALESCE(created_at, '')
		FROM vendors
		WHERE 1=1
	`
	var params []any
	if category != "" {
		query += " AND category = ?"
		params = append(params, strings.ToLower(category))
	}
	if favoriteOnly {
		query += " AND favorite = 1"
	}

	query += " ORDER BY favorite DESC, name ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Vendor
	for rows.Next() {
		var v Vendor
		var favInt int
		if err := rows.Scan(
			&v.ID, &v.Name, &v.Category, &v.Location,
			&v.Phone, &v.Email, &v.URL,
			&v.Notes, &favInt, &v.Source,
			&v.CreatedAt,
		); err != nil {
			return nil, err
		}
		v.Favorite = (favInt == 1)
		list = append(list, v)
	}
	return list, nil
}

// ToggleFavorite toggles the favorite status of a vendor.
func ToggleFavorite(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("UPDATE vendors SET favorite = CASE WHEN favorite = 1 THEN 0 ELSE 1 END WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// UpdateVendor updates fields of a vendor.
func UpdateVendor(database *sql.DB, id int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	allowed := map[string]bool{
		"name": true, "category": true, "location": true,
		"phone": true, "email": true, "url": true,
		"notes": true, "favorite": true, "source": true,
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

	params = append(params, id)
	query := fmt.Sprintf("UPDATE vendors SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteVendor deletes a vendor by ID.
func DeleteVendor(database *sql.DB, id int) (bool, error) {
	res, err := database.Exec("DELETE FROM vendors WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
