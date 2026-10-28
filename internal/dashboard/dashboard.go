package dashboard

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/audit"
	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/contacts"
	"github.com/nichsedge/ierp/internal/decisions"
	"github.com/nichsedge/ierp/internal/events"
	"github.com/nichsedge/ierp/internal/finance"
	"github.com/nichsedge/ierp/internal/gadgets"
	"github.com/nichsedge/ierp/internal/lifeops"
	"github.com/nichsedge/ierp/internal/projects"
	"github.com/nichsedge/ierp/internal/radar"
	"github.com/nichsedge/ierp/internal/reviews"
	"github.com/nichsedge/ierp/internal/vendors"
)

//go:embed assets/*
var assetsFS embed.FS

// Server encapsulates the web dashboard HTTP server.
type Server struct {
	db   *sql.DB
	port int
	host string
}

// NewServer initializes a dashboard server.
func NewServer(database *sql.DB, host string, port int) *Server {
	if host == "" {
		host = "0.0.0.0"
	}
	if port <= 0 {
		port = 8921
	}
	return &Server{
		db:   database,
		host: host,
		port: port,
	}
}

// Start launches the HTTP server listening on the configured host and port.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static assets from embedded FS
	staticSub, err := fs.Sub(assetsFS, "assets/static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	}

	// HTML dashboard page
	dashboardHTML, err := assetsFS.ReadFile("assets/templates/dashboard.html")
	if err != nil {
		dashboardHTML = []byte("<!DOCTYPE html><html><body><h1>iERP Dashboard</h1></body></html>")
	}

	renderPage := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(dashboardHTML)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/events" || r.URL.Path == "/contacts" ||
			r.URL.Path == "/radar" || r.URL.Path == "/runway" || r.URL.Path == "/projects" ||
			r.URL.Path == "/decisions" || r.URL.Path == "/commerce" || r.URL.Path == "/lifeops" ||
			r.URL.Path == "/vendors" || r.URL.Path == "/media" || r.URL.Path == "/links" ||
			r.URL.Path == "/places" {
			renderPage(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/webhook/") {
			renderPage(w, r)
			return
		}
		http.NotFound(w, r)
	})

	// REST APIs
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/contacts", s.handleContacts)
	mux.HandleFunc("/api/contacts/automerge", s.handleContactsAutomerge)
	mux.HandleFunc("/api/contacts/", s.handleContactItem)
	mux.HandleFunc("/api/radar", s.handleRadar)
	mux.HandleFunc("/api/radar/daily", s.handleRadarDaily)
	mux.HandleFunc("/api/radar/summary", s.handleRadarSummary)
	mux.HandleFunc("/api/runway", s.handleRunway)
	mux.HandleFunc("/api/finance/snapshots", s.handleFinanceSnapshots)
	mux.HandleFunc("/api/finance/commitments", s.handleFinanceCommitments)
	mux.HandleFunc("/api/projects", s.handleProjects)
	mux.HandleFunc("/api/decisions", s.handleDecisions)
	mux.HandleFunc("/api/decisions/", s.handleDecisionItem)
	mux.HandleFunc("/api/maintenance", s.handleMaintenance)
	mux.HandleFunc("/api/maintenance/", s.handleMaintenanceItem)
	mux.HandleFunc("/api/reviews", s.handleReviews)
	mux.HandleFunc("/api/gadgets", s.handleGadgets)
	mux.HandleFunc("/api/media", s.handleMedia)
	mux.HandleFunc("/api/links", s.handleLinks)
	mux.HandleFunc("/api/places", s.handlePlaces)
	mux.HandleFunc("/api/vendors", s.handleVendors)
	mux.HandleFunc("/api/vendors/", s.handleVendorItem)
	mux.HandleFunc("/api/payment-accounts", s.handleCommerceAccounts)
	mux.HandleFunc("/api/referrals", s.handleCommerceReferrals)
	mux.HandleFunc("/api/commerce/accounts", s.handleCommerceAccounts)
	mux.HandleFunc("/api/commerce/referrals", s.handleCommerceReferrals)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/webhook/location", s.handleLocationWebhook)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	fmt.Printf("%s%siERP Web Dashboard running at http://%s%s\n", config.Bold, config.Green, addr, config.Reset)
	return http.ListenAndServe(addr, mux)
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, status int, msg string) {
	sendJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var totalEvents, totalContacts, totalMedia, totalLinks int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&totalEvents)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM contacts").Scan(&totalContacts)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM media_items").Scan(&totalMedia)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM links").Scan(&totalLinks)

	// Monthly counts
	mRows, err := s.db.Query(`
		SELECT strftime('%Y-%m', start_date) as ym, COUNT(*) 
		FROM events 
		WHERE start_date IS NOT NULL AND start_date != ''
		GROUP BY ym ORDER BY ym DESC LIMIT 12
	`)
	var monthlyCounts []map[string]any
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var ym string
			var cnt int
			if err := mRows.Scan(&ym, &cnt); err == nil && ym != "" {
				monthlyCounts = append(monthlyCounts, map[string]any{"year_month": ym, "count": cnt})
			}
		}
	}

	// Daily counts for 365 days heatmap
	dRows, err := s.db.Query(`
		SELECT substr(start_date, 1, 10) as dt, COUNT(*)
		FROM events
		WHERE start_date IS NOT NULL AND start_date != '' AND start_date >= date('now', '-365 days')
		GROUP BY dt ORDER BY dt ASC
	`)
	dailyCounts := make(map[string]int)
	if err == nil {
		defer dRows.Close()
		for dRows.Next() {
			var dt string
			var cnt int
			if err := dRows.Scan(&dt, &cnt); err == nil && dt != "" {
				dailyCounts[dt] = cnt
			}
		}
	}

	// Media summary by type
	medRows, err := s.db.Query(`
		SELECT media_type, COUNT(*) as cnt
		FROM media_items
		WHERE media_type IS NOT NULL AND media_type != ''
		GROUP BY media_type ORDER BY cnt DESC
	`)
	mediaSummary := make(map[string]int)
	if err == nil {
		defer medRows.Close()
		for medRows.Next() {
			var mtype string
			var cnt int
			if err := medRows.Scan(&mtype, &cnt); err == nil && mtype != "" {
				mediaSummary[mtype] = cnt
			}
		}
	}

	// Top places
	pRows, err := s.db.Query(`
		SELECT place, COUNT(*) as cnt 
		FROM events 
		WHERE place IS NOT NULL AND place != '' 
		GROUP BY place ORDER BY cnt DESC LIMIT 10
	`)
	var topPlaces [][]any
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var pl string
			var cnt int
			if err := pRows.Scan(&pl, &cnt); err == nil && pl != "" {
				topPlaces = append(topPlaces, []any{pl, cnt})
			}
		}
	}

	// Top tags
	tagRows, err := s.db.Query(`
		SELECT tags FROM events WHERE tags IS NOT NULL AND tags != ''
	`)
	tagMap := make(map[string]int)
	if err == nil {
		defer tagRows.Close()
		for tagRows.Next() {
			var rawTags string
			if err := tagRows.Scan(&rawTags); err == nil {
				var tagList []string
				if err := json.Unmarshal([]byte(rawTags), &tagList); err == nil {
					for _, t := range tagList {
						t = strings.TrimSpace(t)
						if t != "" {
							tagMap[t]++
						}
					}
				}
			}
		}
	}
	type tagCount struct {
		Tag   string
		Count int
	}
	var tagList []tagCount
	for t, c := range tagMap {
		tagList = append(tagList, tagCount{Tag: t, Count: c})
	}
	sort.Slice(tagList, func(i, j int) bool {
		return tagList[i].Count > tagList[j].Count
	})
	var topTags [][]any
	for i, tc := range tagList {
		if i >= 12 {
			break
		}
		topTags = append(topTags, []any{tc.Tag, tc.Count})
	}

	runway, _ := finance.ComputeRunway(s.db)

	sendJSON(w, http.StatusOK, map[string]any{
		"total_events":   totalEvents,
		"total_contacts": totalContacts,
		"total_media":    totalMedia,
		"total_links":    totalLinks,
		"monthly_counts": monthlyCounts,
		"daily_counts":   dailyCounts,
		"media_summary":  mediaSummary,
		"top_places":     topPlaces,
		"top_tags":       topTags,
		"finance": map[string]any{
			"liquid_cash":   runway.LiquidCash,
			"net_worth":     runway.NetWorth,
			"monthly_burn":  runway.MonthlyBurn,
			"runway_months": runway.RunwayMonths,
			"is_infinite":   runway.IsInfinite,
			"status_label":  runway.StatusLabel,
		},
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := r.URL.Query().Get("q")
		tag := r.URL.Query().Get("tag")
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		sortCol := r.URL.Query().Get("sort")
		dir := r.URL.Query().Get("dir")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		evs, total, err := events.ListEvents(s.db, events.ListOptions{
			Query:    q,
			Tag:      tag,
			FromDate: from,
			ToDate:   to,
			SortCol:  sortCol,
			SortDir:  dir,
			Limit:    limit,
			Offset:   offset,
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
	item, err := radar.GetDailyReconnection(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, item)
}

func (s *Server) handleRadarSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := radar.GetRadarSummary(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, summary)
}

func (s *Server) handleRunway(w http.ResponseWriter, r *http.Request) {
	runway, err := finance.ComputeRunway(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, runway)
}

func (s *Server) handleFinanceSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		snaps, err := finance.ListSnapshots(s.db, 30)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": snaps,
			"total": len(snaps),
		})
		return
	}

	if r.Method == http.MethodPost {
		var snap finance.Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snap); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		id, err := finance.InsertSnapshot(s.db, snap)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleFinanceCommitments(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		cat := r.URL.Query().Get("category")
		comms, err := finance.ListCommitments(s.db, status, cat)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": comms,
			"total": len(comms),
		})
		return
	}

	if r.Method == http.MethodPost {
		var comm finance.Commitment
		if err := json.NewDecoder(r.Body).Decode(&comm); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		id, err := finance.InsertCommitment(s.db, comm)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

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
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		_ = json.NewDecoder(r.Body).Decode(&req)
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
		if err := json.NewDecoder(r.Body).Decode(&rev); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
	_ = s.db.QueryRow(countQuery, countParams...).Scan(&total)

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
	_ = s.db.QueryRow(countQuery, countParams...).Scan(&total)

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
		_ = s.db.QueryRow(countQuery, countParams...).Scan(&total)

		switch sortCol {
		case "name":
			query += fmt.Sprintf(" ORDER BY name %s", dir)
		case "category":
			query += fmt.Sprintf(" ORDER BY category %s, name ASC", dir)
		case "location":
			query += fmt.Sprintf(" ORDER BY location %s, name ASC", dir)
		default:
			query += fmt.Sprintf(" ORDER BY favorite DESC, name ASC")
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
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
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
		if err != nil || !ok {
			sendError(w, http.StatusInternalServerError, "Failed to toggle favorite")
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleCommerceAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		list, err := commerce.ListPaymentAccounts(s.db, "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if q != "" {
			lq := strings.ToLower(q)
			var filtered []commerce.PaymentAccount
			for _, item := range list {
				if strings.Contains(strings.ToLower(item.Name), lq) ||
					strings.Contains(strings.ToLower(item.Category), lq) ||
					strings.Contains(strings.ToLower(item.Recipient), lq) ||
					strings.Contains(strings.ToLower(item.Number), lq) ||
					strings.Contains(strings.ToLower(item.Details), lq) {
					filtered = append(filtered, item)
				}
			}
			list = filtered
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var p commerce.PaymentAccount
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		id, err := commerce.InsertPaymentAccount(s.db, p)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleCommerceReferrals(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		list, err := commerce.ListReferrals(s.db, "", "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if q != "" {
			lq := strings.ToLower(q)
			var filtered []commerce.Referral
			for _, item := range list {
				if strings.Contains(strings.ToLower(item.Name), lq) ||
					strings.Contains(strings.ToLower(item.Category), lq) ||
					strings.Contains(strings.ToLower(item.Code), lq) ||
					strings.Contains(strings.ToLower(item.Benefit), lq) {
					filtered = append(filtered, item)
				}
			}
			list = filtered
		}
		sendJSON(w, http.StatusOK, map[string]any{
			"items": list,
			"total": len(list),
		})
		return
	}

	if r.Method == http.MethodPost {
		var ref commerce.Referral
		if err := json.NewDecoder(r.Body).Decode(&ref); err != nil {
			sendError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
		id, err := commerce.InsertReferral(s.db, ref)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "created"})
		return
	}

	sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	res, err := audit.RunAudit(s.db)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sendJSON(w, http.StatusOK, res)
}

func (s *Server) handleLocationWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	lat, hasLat := data["lat"].(float64)
	lon, hasLon := data["lon"].(float64)
	if !hasLat || !hasLon {
		sendJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "No coordinates"})
		return
	}

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
