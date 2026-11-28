package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/events"
	"github.com/nichsedge/ierp/internal/radar"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := r.URL.Query().Get("q")
		tag := r.URL.Query().Get("tag")
		excludeTag := r.URL.Query().Get("exclude_tag")
		source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		sortCol := r.URL.Query().Get("sort")
		dir := r.URL.Query().Get("dir")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		var excludeTags []string
		if excludeTag != "" {
			for _, t := range strings.Split(excludeTag, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					excludeTags = append(excludeTags, t)
				}
			}
		}

		if source == "life" {
			excludeTags = append(excludeTags, "timeline", "atracker", "deepwork")
		} else if source == "atracker" || source == "deepwork" {
			if tag == "" {
				tag = "atracker"
			}
		} else if source == "timeline" || source == "location" {
			if tag == "" {
				tag = "timeline"
			}
		}

		evs, total, err := events.ListEvents(s.db, events.ListOptions{
			Query:       q,
			Tag:         tag,
			ExcludeTags: excludeTags,
			FromDate:    from,
			ToDate:      to,
			SortCol:     sortCol,
			SortDir:     dir,
			Limit:       limit,
			Offset:      offset,
		})
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items":  evs,
			"events": evs,
			"total":  total,
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Title     string   `json:"title"`
			Place     string   `json:"place"`
			StartDate string   `json:"start_date"`
			EndDate   string   `json:"end_date"`
			Tags      []string `json:"tags"`
			URL       string   `json:"url"`
			Notes     string   `json:"notes"`
			Contacts  []string `json:"contacts"`
			ProjectID *int     `json:"project_id"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		if req.Title == "" {
			sendError(w, http.StatusBadRequest, "Title is required")
			return
		}

		evID, linked, err := events.InsertEvent(s.db, events.Event{
			Title:     req.Title,
			Place:     req.Place,
			StartDate: req.StartDate,
			EndDate:   req.EndDate,
			Tags:      req.Tags,
			URL:       req.URL,
			Notes:     req.Notes,
			ProjectID: req.ProjectID,
		}, req.Contacts)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		sendJSON(w, http.StatusCreated, map[string]any{
			"id":              evID,
			"linked_contacts": linked,
			"status":          "created",
		})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleEventItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Missing event ID")
		return
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid event ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		ev, err := events.GetEvent(s.db, id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ev == nil {
			sendError(w, http.StatusNotFound, "Event not found")
			return
		}
		sendJSON(w, http.StatusOK, ev)
	case http.MethodDelete:
		ok, err := events.DeleteEvent(s.db, id)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			sendError(w, http.StatusNotFound, "Event not found")
			return
		}
		sendJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := r.URL.Query().Get("q")
		source := r.URL.Query().Get("source")
		sortCol := r.URL.Query().Get("sort")
		dir := r.URL.Query().Get("dir")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		var tier *int
		if tv := r.URL.Query().Get("tier"); tv != "" {
			if t, err := strconv.Atoi(tv); err == nil {
				tier = &t
			}
		}

		list, total, err := contacts.ListContacts(s.db, contacts.ListOptions{
			Query:        q,
			SourceFilter: source,
			SortCol:      sortCol,
			SortDir:      dir,
			Tier:         tier,
			Limit:        limit,
			Offset:       offset,
		})
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items":    list,
			"contacts": list,
			"total":    total,
		})
		return
	}

	if r.Method == http.MethodPost {
		var c contacts.Contact
		if err := decodeJSON(w, r, &c); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		if c.Name == "" {
			sendError(w, http.StatusBadRequest, "Name is required")
			return
		}

		id, err := contacts.InsertContact(s.db, c)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleContactsAutomerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	sendJSON(w, http.StatusOK, map[string]any{
		"merged_count": 0,
		"message":      "All contacts already merged",
	})
}

func (s *Server) handleContactItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		sendError(w, http.StatusBadRequest, "Invalid URL")
		return
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		sendError(w, http.StatusBadRequest, "Invalid contact ID")
		return
	}

	// POST /api/contacts/{id}/tier
	if len(parts) == 4 && parts[3] == "tier" && r.Method == http.MethodPost {
		var req struct {
			Tier        int `json:"tier"`
			CadenceDays int `json:"cadence_days"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
			return
		}
		days := req.CadenceDays
		if days <= 0 {
			days = contacts.DefaultCadenceByTier[req.Tier]
		}
		ok, err := contacts.UpdateContact(s.db, id, map[string]any{
			"tier":         req.Tier,
			"cadence_days": days,
		})
		if err != nil || !ok {
			sendError(w, http.StatusInternalServerError, "Failed to update tier")
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	if r.Method == http.MethodDelete {
		ok, err := contacts.DeleteContact(s.db, id)
		if err != nil || !ok {
			sendError(w, http.StatusInternalServerError, "Failed to delete contact")
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	overdueOnly := r.URL.Query().Get("overdue") == "true" || r.URL.Query().Get("overdue_only") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var tier *int
	if tv := r.URL.Query().Get("tier"); tv != "" {
		if t, err := strconv.Atoi(tv); err == nil {
			tier = &t
		}
	}

	items, err := radar.ComputeRadar(s.db, tier, overdueOnly, limit, offset)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	summary, _ := radar.GetRadarSummary(s.db)
	sendJSON(w, http.StatusOK, map[string]any{
		"items":   items,
		"total":   len(items),
		"summary": summary,
	})
}

func (s *Server) handleRadarDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	item, err := radar.GetDailyReconnection(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, item)
}

func (s *Server) handleRadarSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	summary, err := radar.GetRadarSummary(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, summary)
}

func (s *Server) handleLocationWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var data struct {
		Lat *float64 `json:"lat"`
		Lon *float64 `json:"lon"`
	}
	if err := decodeJSON(w, r, &data); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if data.Lat == nil || data.Lon == nil {
		sendJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "No coordinates"})
		return
	}
	lat := *data.Lat
	lon := *data.Lon

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	title := fmt.Sprintf("GPS Location Ping (%.4f, %.4f)", lat, lon)
	tags := []string{"location", "gps", "webhook"}
	notes := fmt.Sprintf("Lat: %.6f, Lon: %.6f", lat, lon)

	id, _, err := events.InsertEvent(s.db, events.Event{
		Title:     title,
		StartDate: nowStr,
		RawDate:   nowStr,
		Tags:      tags,
		Notes:     notes,
	}, nil)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"status":   "success",
		"event_id": id,
		"time":     nowStr,
	})
}

func (s *Server) handlePlaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	cityFilter := strings.TrimSpace(r.URL.Query().Get("city"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	query := `
		SELECT p.place, COALESCE(p.display_name, p.place), p.latitude, p.longitude, COALESCE(p.city, ''), COUNT(e.id) as event_count
		FROM places_cache p
		LEFT JOIN events e ON e.place = p.place
		WHERE 1=1
	`
	var params []any
	if cityFilter != "" {
		query += " AND p.city = ?"
		params = append(params, cityFilter)
	}
	if q != "" {
		likeQ := "%" + q + "%"
		query += " AND (p.place LIKE ? OR p.display_name LIKE ? OR p.city LIKE ?)"
		params = append(params, likeQ, likeQ, likeQ)
	}
	query += " GROUP BY p.place ORDER BY event_count DESC, p.place ASC"

	rows, err := s.db.Query(query, params...)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type PlaceEvent struct {
		ID        int      `json:"id"`
		Title     string   `json:"title"`
		StartDate string   `json:"start_date"`
		Tags      []string `json:"tags,omitempty"`
	}

	type PlaceItem struct {
		Place       string       `json:"place"`
		DisplayName string       `json:"display_name"`
		Latitude    float64      `json:"latitude"`
		Longitude   float64      `json:"longitude"`
		City        string       `json:"city,omitempty"`
		EventCount  int          `json:"event_count"`
		Events      []PlaceEvent `json:"events"`
	}

	var places []PlaceItem
	placeIndex := make(map[string]int)

	for rows.Next() {
		var item PlaceItem
		if err := rows.Scan(&item.Place, &item.DisplayName, &item.Latitude, &item.Longitude, &item.City, &item.EventCount); err == nil {
			item.Events = []PlaceEvent{}
			placeIndex[item.Place] = len(places)
			places = append(places, item)
		}
	}

	// Fetch linked events
	evRows, err := s.db.Query(`
		SELECT id, title, COALESCE(start_date, ''), COALESCE(tags, '[]'), place
		FROM events
		WHERE place IS NOT NULL AND place != ''
		ORDER BY start_date DESC
	`)
	if err == nil {
		defer evRows.Close()
		for evRows.Next() {
			var evID int
			var title, startDate, rawTags, pl string
			if err := evRows.Scan(&evID, &title, &startDate, &rawTags, &pl); err == nil {
				if idx, ok := placeIndex[pl]; ok {
					var tags []string
					_ = json.Unmarshal([]byte(rawTags), &tags)
					places[idx].Events = append(places[idx].Events, PlaceEvent{
						ID:        evID,
						Title:     title,
						StartDate: startDate,
						Tags:      tags,
					})
				}
			}
		}
	}

	// Distinct cities
	var cities []string
	cRows, err := s.db.Query("SELECT DISTINCT city FROM places_cache WHERE city IS NOT NULL AND city != '' ORDER BY city ASC")
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var c string
			if err := cRows.Scan(&c); err == nil && c != "" {
				cities = append(cities, c)
			}
		}
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"items":  places,
		"total":  len(places),
		"cities": cities,
	})
}
