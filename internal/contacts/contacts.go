package contacts

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Contact represents a CRM contact record.
type Contact struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Org         string `json:"org"`
	Client      string `json:"client"`
	Location    string `json:"location"`
	Notes       string `json:"notes"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Source      string `json:"source"`
	GoogleID    string `json:"google_id"`
	CreatedAt   string `json:"created_at"`
	Tier        int    `json:"tier"`
	CadenceDays int    `json:"cadence_days"`
	EventCount  int    `json:"event_count,omitempty"`
}

// ResolveContact resolves a contact by numeric ID or exact/substring name.
func ResolveContact(database *sql.DB, ref string) (int, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, "", nil
	}

	// Try numeric ID
	if id, err := strconv.Atoi(ref); err == nil {
		var name string
		row := database.QueryRow("SELECT id, name FROM contacts WHERE id = ?", id)
		if err := row.Scan(&id, &name); err == nil {
			return id, name, nil
		}
	}

	// Try exact case-insensitive match
	var id int
	var name string
	row := database.QueryRow("SELECT id, name FROM contacts WHERE LOWER(name) = LOWER(?)", ref)
	if err := row.Scan(&id, &name); err == nil {
		return id, name, nil
	}

	// Fallback: substring match
	rows, err := database.Query("SELECT id, name FROM contacts WHERE name LIKE ?", "%"+ref+"%")
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()

	var matches []struct {
		id   int
		name string
	}
	for rows.Next() {
		var m struct {
			id   int
			name string
		}
		if err := rows.Scan(&m.id, &m.name); err == nil {
			matches = append(matches, m)
		}
	}

	if len(matches) == 1 {
		return matches[0].id, matches[0].name, nil
	}

	return 0, "", nil
}

// DefaultCadenceByTier maps Dunbar tiers to cadence days.
var DefaultCadenceByTier = map[int]int{
	0: 0,
	1: 14,
	2: 60,
	3: 180,
}

// InsertContact inserts a structured contact record.
func InsertContact(database *sql.DB, c Contact) (int, error) {
	if c.Tier < 0 {
		c.Tier = 0
	}
	if c.Tier > 3 {
		c.Tier = 3
	}
	if c.CadenceDays <= 0 {
		c.CadenceDays = DefaultCadenceByTier[c.Tier]
	}
	if c.Source == "" {
		c.Source = "manual"
	}

	res, err := database.Exec(`
		INSERT INTO contacts (name, org, client, location, email, phone, notes, google_id, source, tier, cadence_days)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.Name, c.Org, c.Client, c.Location, c.Email, c.Phone, c.Notes, c.GoogleID, c.Source, c.Tier, c.CadenceDays)
	if err != nil {
		return 0, fmt.Errorf("failed to insert contact: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// ListOptions defines query filters for listing contacts.
type ListOptions struct {
	SourceFilter string
	Query        string
	Tier         *int
	Limit        int
	Offset       int
	SortCol      string
	SortDir      string
}

// ListContacts retrieves contacts with filtering and pagination.
func ListContacts(database *sql.DB, opts ListOptions) ([]Contact, int, error) {
	var whereClauses []string
	var params []any

	if opts.SourceFilter != "" && strings.ToLower(opts.SourceFilter) != "all" {
		whereClauses = append(whereClauses, "LOWER(c.source) = ?")
		params = append(params, strings.ToLower(opts.SourceFilter))
	}

	if opts.Tier != nil {
		whereClauses = append(whereClauses, "c.tier = ?")
		params = append(params, *opts.Tier)
	}

	if opts.Query != "" {
		whereClauses = append(whereClauses, "(c.name LIKE ? OR c.org LIKE ? OR c.client LIKE ? OR c.email LIKE ? OR c.phone LIKE ? OR c.notes LIKE ? OR c.location LIKE ?)")
		qLike := "%" + opts.Query + "%"
		for i := 0; i < 7; i++ {
			params = append(params, qLike)
		}
	}

	whereSQL := "1=1"
	if len(whereClauses) > 0 {
		whereSQL = strings.Join(whereClauses, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM contacts c WHERE %s", whereSQL)
	var total int
	if err := database.QueryRow(countQuery, params...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort validation
	validSort := map[string]bool{
		"c.id": true, "c.name": true, "c.org": true, "c.email": true,
		"c.tier": true, "c.cadence_days": true, "c.created_at": true, "c.source": true,
	}
	sortCol := opts.SortCol
	if !validSort[sortCol] {
		sortCol = "c.name"
	}
	sortDir := "ASC"
	if strings.ToUpper(opts.SortDir) == "DESC" {
		sortDir = "DESC"
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	fetchSQL := fmt.Sprintf(`
		SELECT c.id, c.name, COALESCE(c.org, ''), COALESCE(c.client, ''), COALESCE(c.location, ''),
		       COALESCE(c.notes, ''), COALESCE(c.email, ''), COALESCE(c.phone, ''), COALESCE(c.source, 'manual'),
		       COALESCE(c.google_id, ''), COALESCE(c.created_at, ''), c.tier, c.cadence_days,
		       (SELECT COUNT(*) FROM event_contacts ec WHERE ec.contact_id = c.id) as event_count
		FROM contacts c
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereSQL, sortCol, sortDir)

	fetchParams := append(params, limit, opts.Offset)
	rows, err := database.Query(fetchSQL, fetchParams...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Org, &c.Client, &c.Location,
			&c.Notes, &c.Email, &c.Phone, &c.Source,
			&c.GoogleID, &c.CreatedAt, &c.Tier, &c.CadenceDays,
			&c.EventCount,
		); err != nil {
			return nil, 0, err
		}
		contacts = append(contacts, c)
	}

	return contacts, total, nil
}

// GetContact retrieves a single contact by ID.
func GetContact(database *sql.DB, contactID int) (*Contact, error) {
	row := database.QueryRow(`
		SELECT c.id, c.name, COALESCE(c.org, ''), COALESCE(c.client, ''), COALESCE(c.location, ''),
		       COALESCE(c.notes, ''), COALESCE(c.email, ''), COALESCE(c.phone, ''), COALESCE(c.source, 'manual'),
		       COALESCE(c.google_id, ''), COALESCE(c.created_at, ''), c.tier, c.cadence_days,
		       (SELECT COUNT(*) FROM event_contacts ec WHERE ec.contact_id = c.id) as event_count
		FROM contacts c
		WHERE c.id = ?
	`, contactID)

	var c Contact
	if err := row.Scan(
		&c.ID, &c.Name, &c.Org, &c.Client, &c.Location,
		&c.Notes, &c.Email, &c.Phone, &c.Source,
		&c.GoogleID, &c.CreatedAt, &c.Tier, &c.CadenceDays,
		&c.EventCount,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// UpdateContact updates editable fields of a contact.
func UpdateContact(database *sql.DB, contactID int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}

	var setClauses []string
	var params []any

	allowed := map[string]bool{
		"name": true, "org": true, "client": true, "location": true,
		"notes": true, "email": true, "phone": true, "tier": true,
		"cadence_days": true, "source": true,
	}

	for k, v := range updates {
		if allowed[k] {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", k))
			params = append(params, v)
		}
	}

	if len(setClauses) == 0 {
		return false, nil
	}

	params = append(params, contactID)
	query := fmt.Sprintf("UPDATE contacts SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	res, err := database.Exec(query, params...)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}

// DeleteContact removes a contact by ID.
func DeleteContact(database *sql.DB, contactID int) (bool, error) {
	res, err := database.Exec("DELETE FROM contacts WHERE id = ?", contactID)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
