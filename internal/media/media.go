package media

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// MediaItem represents an item consumed across trackers.
type MediaItem struct {
	ID        int            `json:"id"`
	MediaType string         `json:"media_type"`
	Title     string         `json:"title"`
	Source    string         `json:"source"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// UpsertMediaItem idempotently inserts or merges a media item.
func UpsertMediaItem(database *sql.DB, item MediaItem) (int, error) {
	if item.MediaType == "" {
		item.MediaType = "book"
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "Untitled"
	}

	var existingID int
	var existingDataRaw string
	err := database.QueryRow(`
		SELECT id, data_json FROM media_items
		WHERE media_type = ? AND source = ? AND title = ?
	`, item.MediaType, item.Source, title).Scan(&existingID, &existingDataRaw)

	nowStr := time.Now().Format("2006-01-02 15:04:05")

	if err == nil {
		merged := make(map[string]any)
		_ = json.Unmarshal([]byte(existingDataRaw), &merged)
		for k, v := range item.Data {
			if v != nil {
				merged[k] = v
			}
		}
		mergedJSON, _ := json.Marshal(merged)
		_, err := database.Exec(`
			UPDATE media_items SET data_json = ?, updated_at = ? WHERE id = ?
		`, string(mergedJSON), nowStr, existingID)
		return existingID, err
	}

	if item.Data == nil {
		item.Data = make(map[string]any)
	}
	dataJSON, _ := json.Marshal(item.Data)

	res, err := database.Exec(`
		INSERT INTO media_items (media_type, title, source, data_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.MediaType, title, item.Source, string(dataJSON), nowStr, nowStr)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// ListMediaItems retrieves media items with filtering.
func ListMediaItems(database *sql.DB, mediaType string, source string) ([]MediaItem, error) {
	query := `
		SELECT id, media_type, title, COALESCE(source, ''), COALESCE(data_json, '{}'),
		       COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM media_items
		WHERE 1=1
	`
	var params []any
	if mediaType != "" {
		query += " AND media_type = ?"
		params = append(params, mediaType)
	}
	if source != "" {
		query += " AND source = ?"
		params = append(params, source)
	}
	query += " ORDER BY id DESC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MediaItem
	for rows.Next() {
		var it MediaItem
		var raw string
		if err := rows.Scan(
			&it.ID, &it.MediaType, &it.Title, &it.Source, &raw,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		it.Data = make(map[string]any)
		_ = json.Unmarshal([]byte(raw), &it.Data)
		list = append(list, it)
	}

	return list, nil
}

// Link represents a social or profile link.
type Link struct {
	ID        int    `json:"id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	Category  string `json:"category,omitempty"`
	IsPublic  bool   `json:"is_public"`
	Notes     string `json:"notes,omitempty"`
	CreatedAt string `json:"created_at"`
}

// UpsertLink records a profile link.
func UpsertLink(database *sql.DB, l Link) (int, error) {
	isPub := 1
	if !l.IsPublic {
		isPub = 0
	}

	res, err := database.Exec(`
		INSERT INTO links (label, url, category, is_public, notes)
		VALUES (?, ?, ?, ?, ?)
	`, l.Label, l.URL, l.Category, isPub, l.Notes)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// ListLinks retrieves links.
func ListLinks(database *sql.DB, category string) ([]Link, error) {
	query := "SELECT id, label, url, COALESCE(category, 'profile'), COALESCE(is_public, 1), COALESCE(notes, ''), COALESCE(created_at, '') FROM links WHERE 1=1"
	var params []any
	if category != "" {
		query += " AND category = ?"
		params = append(params, category)
	}
	query += " ORDER BY id ASC"

	rows, err := database.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Link
	for rows.Next() {
		var l Link
		var pubInt int
		if err := rows.Scan(&l.ID, &l.Label, &l.URL, &l.Category, &pubInt, &l.Notes, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.IsPublic = pubInt == 1
		list = append(list, l)
	}
	return list, nil
}
