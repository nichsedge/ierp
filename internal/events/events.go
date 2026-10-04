package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nichsedge/ierp/internal/contacts"
)

// EventContact represents a contact linked to an event.
type EventContact struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Org  string `json:"org,omitempty"`
}

// EventMedia represents an attached media file.
type EventMedia struct {
	ID               int    `json:"id"`
	OriginalFilename string `json:"original_filename"`
	StoredPath       string `json:"stored_path"`
}

// Event represents a journal event record.
type Event struct {
	ID           int            `json:"id"`
	Title        string         `json:"title"`
	Place        string         `json:"place"`
	StartDate    string         `json:"start_date"`
	EndDate      string         `json:"end_date"`
	RawDate      string         `json:"raw_date"`
	Tags         []string       `json:"tags"`
	URL          string         `json:"url"`
	Notes        string         `json:"notes"`
	CreatedAt    string         `json:"created_at"`
	ProjectID    *int           `json:"project_id,omitempty"`
	ProjectTitle string         `json:"project_title,omitempty"`
	Contacts     []EventContact `json:"contacts"`
	Media        []EventMedia   `json:"media,omitempty"`
}

var nonAlphanumericRegex = regexp.MustCompile(`["'*():^~+\-{}[\]]`)

// SanitizeFTS5Query sanitizes user input into a safe FTS5 query with prefix matching.
func SanitizeFTS5Query(query string) string {
	cleaned := strings.TrimSpace(nonAlphanumericRegex.ReplaceAllString(query, " "))
	words := strings.Fields(cleaned)
	if len(words) == 0 {
		return ""
	}
	var quoted []string
	for _, w := range words {
		quoted = append(quoted, fmt.Sprintf(`"%s"*`, w))
	}
	return strings.Join(quoted, " ")
}

// InsertEvent creates a new event and links contacts.
func InsertEvent(database *sql.DB, ev Event, contactRefs []string) (int, []string, error) {
	tagsJSON, err := json.Marshal(ev.Tags)
	if err != nil {
		tagsJSON = []byte("[]")
	}

	res, err := database.Exec(`
		INSERT INTO events (title, place, start_date, end_date, raw_date, tags, url, notes, project_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ev.Title, ev.Place, ev.StartDate, ev.EndDate, ev.RawDate, string(tagsJSON), ev.URL, ev.Notes, ev.ProjectID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to insert event: %w", err)
	}

	evID64, err := res.LastInsertId()
	if err != nil {
		return 0, nil, err
	}
	evID := int(evID64)

	var linkedNames []string
	for _, ref := range contactRefs {
		cid, cname, err := contacts.ResolveContact(database, ref)
		if err == nil && cid > 0 {
			_, _ = database.Exec("INSERT OR IGNORE INTO event_contacts (event_id, contact_id) VALUES (?, ?)", evID, cid)
			if cname != "" {
				linkedNames = append(linkedNames, cname)
			}
		}
	}

	return evID, linkedNames, nil
}

// ListOptions specifies parameters for listing events.
type ListOptions struct {
	Query    string
	Tag      string
	FromDate string
	ToDate   string
	Limit    int
	Offset   int
	SortCol  string
	SortDir  string
}

// ListEvents queries events with filtering and pagination.
func ListEvents(database *sql.DB, opts ListOptions) ([]Event, int, error) {
	var whereClauses []string
	var params []any

	if opts.Query != "" {
		ftsQ := SanitizeFTS5Query(opts.Query)
		if ftsQ != "" {
			whereClauses = append(whereClauses, "e.id IN (SELECT rowid FROM events_fts WHERE events_fts MATCH ?)")
			params = append(params, ftsQ)
		} else {
			whereClauses = append(whereClauses, "(e.title LIKE ? OR e.place LIKE ? OR e.notes LIKE ?)")
			likeQ := "%" + opts.Query + "%"
			params = append(params, likeQ, likeQ, likeQ)
		}
	}

	if opts.Tag != "" {
		whereClauses = append(whereClauses, "e.tags LIKE ?")
		params = append(params, "%"+opts.Tag+"%")
	}

	if opts.FromDate != "" {
		whereClauses = append(whereClauses, "(e.start_date >= ? OR (e.start_date IS NULL AND e.raw_date >= ?))")
		params = append(params, opts.FromDate, opts.FromDate)
	}

	if opts.ToDate != "" {
		toBound := opts.ToDate
		if len(opts.ToDate) == 10 {
			toBound += " 23:59:59"
		}
		whereClauses = append(whereClauses, "(e.start_date <= ? OR (e.start_date IS NULL AND e.raw_date <= ?))")
		params = append(params, toBound, toBound)
	}

	whereSQL := "1=1"
	if len(whereClauses) > 0 {
		whereSQL = strings.Join(whereClauses, " AND ")
	}

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM events e WHERE %s", whereSQL)
	var total int
	if err := database.QueryRow(countSQL, params...).Scan(&total); err != nil {
		return nil, 0, err
	}

	validSort := map[string]bool{
		"e.id": true, "e.title": true, "e.place": true,
		"e.start_date": true, "e.end_date": true, "e.created_at": true,
	}
	sortCol := opts.SortCol
	if !validSort[sortCol] {
		sortCol = "e.start_date"
	}
	sortDir := "DESC"
	if strings.ToUpper(opts.SortDir) == "ASC" {
		sortDir = "ASC"
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	fetchSQL := fmt.Sprintf(`
		SELECT e.id, e.title, COALESCE(e.place, ''), COALESCE(e.start_date, ''),
		       COALESCE(e.end_date, ''), COALESCE(e.raw_date, ''), COALESCE(e.tags, '[]'),
		       COALESCE(e.notes, ''), COALESCE(e.created_at, '')
		FROM events e
		WHERE %s
		ORDER BY %s %s, e.id DESC
		LIMIT ? OFFSET ?
	`, whereSQL, sortCol, sortDir)

	fetchParams := append(params, limit, opts.Offset)
	rows, err := database.Query(fetchSQL, fetchParams...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var eventList []Event
	var eventIDs []int
	for rows.Next() {
		var ev Event
		var tagsRaw string
		if err := rows.Scan(
			&ev.ID, &ev.Title, &ev.Place, &ev.StartDate,
			&ev.EndDate, &ev.RawDate, &tagsRaw,
			&ev.Notes, &ev.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(tagsRaw), &ev.Tags)
		if ev.Tags == nil {
			ev.Tags = []string{}
		}
		eventList = append(eventList, ev)
		eventIDs = append(eventIDs, ev.ID)
	}

	// Fetch contacts for these events
	if len(eventIDs) > 0 {
		contactsMap := make(map[int][]EventContact)
		placeholders := strings.TrimRight(strings.Repeat("?,", len(eventIDs)), ",")
		cArgs := make([]any, len(eventIDs))
		for i, id := range eventIDs {
			cArgs[i] = id
		}

		cQuery := fmt.Sprintf(`
			SELECT ec.event_id, c.id, c.name, COALESCE(c.org, '')
			FROM contacts c
			JOIN event_contacts ec ON c.id = ec.contact_id
			WHERE ec.event_id IN (%s)
		`, placeholders)

		cRows, err := database.Query(cQuery, cArgs...)
		if err == nil {
			defer cRows.Close()
			for cRows.Next() {
				var evID int
				var ec EventContact
				if err := cRows.Scan(&evID, &ec.ID, &ec.Name, &ec.Org); err == nil {
					contactsMap[evID] = append(contactsMap[evID], ec)
				}
			}
		}

		for i := range eventList {
			if list, ok := contactsMap[eventList[i].ID]; ok {
				eventList[i].Contacts = list
			} else {
				eventList[i].Contacts = []EventContact{}
			}
		}
	}

	return eventList, total, nil
}

// GetEvent retrieves a single event with linked contacts and media.
func GetEvent(database *sql.DB, eventID int) (*Event, error) {
	row := database.QueryRow(`
		SELECT e.id, e.title, COALESCE(e.place, ''), COALESCE(e.start_date, ''),
		       COALESCE(e.end_date, ''), COALESCE(e.raw_date, ''), COALESCE(e.tags, '[]'),
		       COALESCE(e.url, ''), COALESCE(e.notes, ''), COALESCE(e.created_at, ''),
		       e.project_id, COALESCE(p.title, '')
		FROM events e
		LEFT JOIN projects p ON p.id = e.project_id
		WHERE e.id = ?
	`, eventID)

	var ev Event
	var tagsRaw string
	if err := row.Scan(
		&ev.ID, &ev.Title, &ev.Place, &ev.StartDate,
		&ev.EndDate, &ev.RawDate, &tagsRaw,
		&ev.URL, &ev.Notes, &ev.CreatedAt,
		&ev.ProjectID, &ev.ProjectTitle,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(tagsRaw), &ev.Tags)
	if ev.Tags == nil {
		ev.Tags = []string{}
	}

	// Contacts
	cRows, err := database.Query(`
		SELECT c.id, c.name, COALESCE(c.org, '')
		FROM contacts c
		JOIN event_contacts ec ON c.id = ec.contact_id
		WHERE ec.event_id = ?
	`, eventID)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var ec EventContact
			if err := cRows.Scan(&ec.ID, &ec.Name, &ec.Org); err == nil {
				ev.Contacts = append(ev.Contacts, ec)
			}
		}
	}

	// Media
	mRows, err := database.Query(`
		SELECT id, original_filename, stored_path
		FROM event_media
		WHERE event_id = ?
	`, eventID)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var em EventMedia
			if err := mRows.Scan(&em.ID, &em.OriginalFilename, &em.StoredPath); err == nil {
				ev.Media = append(ev.Media, em)
			}
		}
	}

	return &ev, nil
}

// SearchResult represents a full-text search match.
type SearchResult struct {
	ID         int      `json:"id"`
	Title      string   `json:"title"`
	Place      string   `json:"place"`
	StartDate  string   `json:"start_date"`
	EndDate    string   `json:"end_date"`
	RawDate    string   `json:"raw_date"`
	Tags       []string `json:"tags"`
	Notes      string   `json:"notes"`
	Rank       float64  `json:"rank"`
	TitleSnip  string   `json:"title_snip"`
	NotesSnip  string   `json:"notes_snip"`
}

// SearchEvents conducts a full-text search with FTS5 BM25 relevance and snippet extraction.
func SearchEvents(database *sql.DB, query string, limit int) ([]SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	ftsQ := SanitizeFTS5Query(query)
	if ftsQ != "" {
		sqlQuery := `
			SELECT e.id, e.title, COALESCE(e.place, ''), COALESCE(e.start_date, ''),
			       COALESCE(e.end_date, ''), COALESCE(e.raw_date, ''), COALESCE(e.tags, '[]'),
			       COALESCE(e.notes, ''), bm25(events_fts) as rank,
			       snippet(events_fts, 0, '[MATCH]', '[/MATCH]', '...', 12) as title_snip,
			       snippet(events_fts, 2, '[MATCH]', '[/MATCH]', '...', 16) as notes_snip
			FROM events_fts
			JOIN events e ON e.id = events_fts.rowid
			WHERE events_fts MATCH ?
			ORDER BY rank ASC, e.start_date DESC
			LIMIT ?
		`
		rows, err := database.Query(sqlQuery, ftsQ, limit)
		if err == nil {
			defer rows.Close()
			var results []SearchResult
			for rows.Next() {
				var r SearchResult
				var tagsRaw string
				if err := rows.Scan(
					&r.ID, &r.Title, &r.Place, &r.StartDate,
					&r.EndDate, &r.RawDate, &tagsRaw,
					&r.Notes, &r.Rank, &r.TitleSnip, &r.NotesSnip,
				); err == nil {
					_ = json.Unmarshal([]byte(tagsRaw), &r.Tags)
					results = append(results, r)
				}
			}
			return results, nil
		}
	}

	// Fallback to SQL LIKE
	likeQ := "%" + query + "%"
	likeQuery := `
		SELECT e.id, e.title, COALESCE(e.place, ''), COALESCE(e.start_date, ''),
		       COALESCE(e.end_date, ''), COALESCE(e.raw_date, ''), COALESCE(e.tags, '[]'),
		       COALESCE(e.notes, '')
		FROM events e
		WHERE e.title LIKE ? OR e.place LIKE ? OR e.notes LIKE ?
		ORDER BY e.start_date DESC
		LIMIT ?
	`
	rows, err := database.Query(likeQuery, likeQ, likeQ, likeQ, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var tagsRaw string
		if err := rows.Scan(
			&r.ID, &r.Title, &r.Place, &r.StartDate,
			&r.EndDate, &r.RawDate, &tagsRaw,
			&r.Notes,
		); err == nil {
			_ = json.Unmarshal([]byte(tagsRaw), &r.Tags)
			r.Rank = 0
			results = append(results, r)
		}
	}

	return results, nil
}

// DeleteEvent removes an event by ID.
func DeleteEvent(database *sql.DB, eventID int) (bool, error) {
	res, err := database.Exec("DELETE FROM events WHERE id = ?", eventID)
	if err != nil {
		return false, err
	}
	ra, err := res.RowsAffected()
	return ra > 0, err
}
