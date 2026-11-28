package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nichsedge/ierp/internal/audit"
	"github.com/nichsedge/ierp/internal/decisions"
	"github.com/nichsedge/ierp/internal/gadgets"
	"github.com/nichsedge/ierp/internal/lifeops"
	"github.com/nichsedge/ierp/internal/projects"
	"github.com/nichsedge/ierp/internal/reviews"
	"github.com/nichsedge/ierp/internal/vendors"
)

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		list, err := projects.ListProjects(s.db, status)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var p projects.Project
		if err := decodeJSON(w, r, &p); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := projects.InsertProject(s.db, p)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		list, err := decisions.ListDecisions(s.db, status, nil)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var d decisions.Decision
		if err := decodeJSON(w, r, &d); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := decisions.InsertDecision(s.db, d)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleDecisionItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid decision ID")
		return
	}

	// POST /api/decisions/{id}/review
	if len(parts) == 4 && parts[3] == "review" && r.Method == http.MethodPost {
		var req struct {
			ActualOutcome string `json:"actual_outcome"`
			Status        string `json:"status"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		ok, err := decisions.ReviewDecision(s.db, id, req.ActualOutcome, req.Status)
		if err != nil || !ok {
			sendError(w, http.StatusInternalServerError, "Failed to review decision")
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		cat := r.URL.Query().Get("category")
		overdueOnly := r.URL.Query().Get("overdue") == "true"
		items, err := lifeops.ListMaintenance(s.db, status, cat, overdueOnly)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"total": len(items),
		})
		return
	}

	if r.Method == http.MethodPost {
		var item lifeops.MaintenanceItem
		if err := decodeJSON(w, r, &item); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := lifeops.InsertMaintenance(s.db, item)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleMaintenanceItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid maintenance ID")
		return
	}

	// POST /api/maintenance/{id}/complete
	if len(parts) == 4 && parts[3] == "complete" && r.Method == http.MethodPost {
		var req struct {
			Date string   `json:"date"`
			Cost *float64 `json:"cost"`
		}
		if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		res, err := lifeops.CompleteMaintenance(s.db, id, req.Date, req.Cost)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, res)
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		pType := r.URL.Query().Get("type")
		list, err := reviews.ListRetrospectives(s.db, pType, 20)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var rev reviews.Retrospective
		if err := decodeJSON(w, r, &rev); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := reviews.InsertRetrospective(s.db, rev)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleGadgets(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cat := r.URL.Query().Get("category")
		status := r.URL.Query().Get("status")
		list, err := gadgets.ListGadgets(s.db, cat, status)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var g gadgets.Gadget
		if err := decodeJSON(w, r, &g); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := gadgets.InsertGadget(s.db, g)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	mediaType := r.URL.Query().Get("type")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol := r.URL.Query().Get("sort")
	dir := strings.ToUpper(r.URL.Query().Get("dir"))
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 24
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	query := "SELECT id, media_type, title, COALESCE(source, ''), COALESCE(data_json, '{}'), COALESCE(created_at, '') FROM media_items WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM media_items WHERE 1=1"
	var params []any
	var countParams []any

	if mediaType != "" {
		query += " AND media_type = ?"
		countQuery += " AND media_type = ?"
		params = append(params, mediaType)
		countParams = append(countParams, mediaType)
	}
	if q != "" {
		likeQ := "%" + q + "%"
		query += " AND (title LIKE ? OR source LIKE ? OR data_json LIKE ?)"
		countQuery += " AND (title LIKE ? OR source LIKE ? OR data_json LIKE ?)"
		params = append(params, likeQ, likeQ, likeQ)
		countParams = append(countParams, likeQ, likeQ, likeQ)
	}

	var total int
	if err := s.db.QueryRow(countQuery, countParams...).Scan(&total); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch sortCol {
	case "title":
		query += fmt.Sprintf(" ORDER BY title %s", dir)
	case "media_type":
		query += fmt.Sprintf(" ORDER BY media_type %s, id DESC", dir)
	default:
		query += fmt.Sprintf(" ORDER BY id %s", dir)
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := s.db.Query(query, params...)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type MediaItemResp struct {
		ID        int    `json:"id"`
		MediaType string `json:"media_type"`
		Title     string `json:"title"`
		Source    string `json:"source"`
		Author    string `json:"author,omitempty"`
		Status    string `json:"status,omitempty"`
		Rating    any    `json:"rating,omitempty"`
		Date      string `json:"date,omitempty"`
		CreatedAt string `json:"created_at"`
	}

	var items []MediaItemResp
	for rows.Next() {
		var it MediaItemResp
		var rawData string
		if err := rows.Scan(&it.ID, &it.MediaType, &it.Title, &it.Source, &rawData, &it.CreatedAt); err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(rawData), &parsed); err == nil {
			if a, ok := parsed["author"].(string); ok && a != "" {
				it.Author = a
			} else if a, ok := parsed["Author"].(string); ok && a != "" {
				it.Author = a
			}
			if st, ok := parsed["Bookshelves"].(string); ok && st != "" {
				it.Status = st
			} else if st, ok := parsed["status"].(string); ok && st != "" {
				it.Status = st
			}
			if rt, ok := parsed["rating"]; ok && rt != nil {
				it.Rating = rt
			} else if rt, ok := parsed["Average Rating"]; ok && rt != nil {
				it.Rating = rt
			}
			if dt, ok := parsed["date_logged"].(string); ok && dt != "" {
				it.Date = dt
			} else if dt, ok := parsed["Date Added"].(string); ok && dt != "" {
				it.Date = dt
			} else {
				it.Date = it.CreatedAt
			}
		}
		items = append(items, it)
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"media": items,
		"total": total,
	})
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	category := r.URL.Query().Get("category")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	sortCol := r.URL.Query().Get("sort")
	dir := strings.ToUpper(r.URL.Query().Get("dir"))
	if dir != "ASC" && dir != "DESC" {
		dir = "ASC"
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 24
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	query := "SELECT id, label, url, COALESCE(category, 'profile'), COALESCE(is_public, 1), COALESCE(notes, ''), COALESCE(created_at, '') FROM links WHERE 1=1"
	countQuery := "SELECT COUNT(*) FROM links WHERE 1=1"
	var params []any
	var countParams []any

	if category != "" {
		query += " AND category = ?"
		countQuery += " AND category = ?"
		params = append(params, category)
		countParams = append(countParams, category)
	}
	if q != "" {
		likeQ := "%" + q + "%"
		query += " AND (label LIKE ? OR url LIKE ? OR notes LIKE ?)"
		countQuery += " AND (label LIKE ? OR url LIKE ? OR notes LIKE ?)"
		params = append(params, likeQ, likeQ, likeQ)
		countParams = append(countParams, likeQ, likeQ, likeQ)
	}

	var total int
	if err := s.db.QueryRow(countQuery, countParams...).Scan(&total); err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch sortCol {
	case "label":
		query += fmt.Sprintf(" ORDER BY label %s", dir)
	case "created_at":
		query += fmt.Sprintf(" ORDER BY created_at %s", dir)
	default:
		query += fmt.Sprintf(" ORDER BY category %s, id ASC", dir)
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := s.db.Query(query, params...)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type LinkResp struct {
		ID        int    `json:"id"`
		Label     string `json:"label"`
		URL       string `json:"url"`
		Category  string `json:"category"`
		IsPublic  bool   `json:"is_public"`
		Notes     string `json:"notes"`
		CreatedAt string `json:"created_at"`
	}

	var items []LinkResp
	for rows.Next() {
		var l LinkResp
		var isPub int
		if err := rows.Scan(&l.ID, &l.Label, &l.URL, &l.Category, &isPub, &l.Notes, &l.CreatedAt); err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		l.IsPublic = (isPub == 1)
		items = append(items, l)
	}

	catRows, _ := s.db.Query("SELECT DISTINCT category FROM links WHERE category IS NOT NULL AND category != '' ORDER BY category ASC")
	var categories []string
	if catRows != nil {
		defer catRows.Close()
		for catRows.Next() {
			var c string
			if err := catRows.Scan(&c); err == nil {
				categories = append(categories, c)
			}
		}
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"links":      items,
		"total":      total,
		"categories": categories,
	})
}

func (s *Server) handleVendors(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cat := r.URL.Query().Get("category")
		fav := r.URL.Query().Get("favorite") == "true" || r.URL.Query().Get("favorite") == "1"
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		sortCol := r.URL.Query().Get("sort")
		dir := strings.ToUpper(r.URL.Query().Get("dir"))
		if dir != "ASC" && dir != "DESC" {
			dir = "DESC"
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 24
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if offset < 0 {
			offset = 0
		}

		query := "SELECT id, name, COALESCE(category, 'General'), COALESCE(location, ''), COALESCE(phone, ''), COALESCE(email, ''), COALESCE(notes, ''), favorite, source, created_at FROM vendors WHERE 1=1"
		countQuery := "SELECT COUNT(*) FROM vendors WHERE 1=1"
		var params []any
		var countParams []any

		if cat != "" {
			query += " AND category = ?"
			countQuery += " AND category = ?"
			params = append(params, cat)
			countParams = append(countParams, cat)
		}
		if fav {
			query += " AND favorite = 1"
			countQuery += " AND favorite = 1"
		}
		if q != "" {
			likeQ := "%" + q + "%"
			query += " AND (name LIKE ? OR category LIKE ? OR location LIKE ? OR notes LIKE ? OR phone LIKE ?)"
			countQuery += " AND (name LIKE ? OR category LIKE ? OR location LIKE ? OR notes LIKE ? OR phone LIKE ?)"
			params = append(params, likeQ, likeQ, likeQ, likeQ, likeQ)
			countParams = append(countParams, likeQ, likeQ, likeQ, likeQ, likeQ)
		}

		var total int
		if err := s.db.QueryRow(countQuery, countParams...).Scan(&total); err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		switch sortCol {
		case "name":
			query += fmt.Sprintf(" ORDER BY name %s", dir)
		case "category":
			query += fmt.Sprintf(" ORDER BY category %s, name ASC", dir)
		case "location":
			query += fmt.Sprintf(" ORDER BY location %s, name ASC", dir)
		default:
			query += " ORDER BY favorite DESC, name ASC"
		}

		query += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

		rows, err := s.db.Query(query, params...)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var list []vendors.Vendor
		for rows.Next() {
			var v vendors.Vendor
			var favInt int
			if err := rows.Scan(&v.ID, &v.Name, &v.Category, &v.Location, &v.Phone, &v.Email, &v.Notes, &favInt, &v.Source, &v.CreatedAt); err != nil {
				sendError(w, http.StatusInternalServerError, err.Error())
				return
			}
			v.Favorite = (favInt == 1)
			list = append(list, v)
		}

		catRows, _ := s.db.Query("SELECT DISTINCT category FROM vendors WHERE category IS NOT NULL AND category != '' ORDER BY category ASC")
		var categories []string
		if catRows != nil {
			defer catRows.Close()
			for catRows.Next() {
				var c string
				if err := catRows.Scan(&c); err == nil {
					categories = append(categories, c)
				}
			}
		}

		sendJSON(w, http.StatusOK, map[string]any{
			"items":      list,
			"vendors":    list,
			"total":      total,
			"categories": categories,
		})
		return
	}

	if r.Method == http.MethodPost {
		var v vendors.Vendor
		if err := decodeJSON(w, r, &v); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		id, err := vendors.InsertVendor(s.db, v)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleVendorItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid vendor ID")
		return
	}

	// POST /api/vendors/{id}/favorite
	if len(parts) == 4 && parts[3] == "favorite" && r.Method == http.MethodPost {
		ok, err := vendors.ToggleFavorite(s.db, id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, "Failed to toggle favorite")
			return
		}
		if !ok {
			sendError(w, http.StatusNotFound, "Vendor not found")
			return
		}
		var favInt int
		if err := s.db.QueryRow("SELECT favorite FROM vendors WHERE id = ?", id).Scan(&favInt); err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true, "favorite": favInt == 1})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	res, err := audit.RunAudit(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, res)
}
