package dashboard

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
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
		if r.URL.Path == "/" || r.URL.Path == "/events" || r.URL.Path == "/radar" ||
			r.URL.Path == "/runway" || r.URL.Path == "/projects" || r.URL.Path == "/decisions" ||
			r.URL.Path == "/commerce" || r.URL.Path == "/lifeops" || r.URL.Path == "/vendors" {
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
	mux.HandleFunc("/api/vendors", s.handleVendors)
	mux.HandleFunc("/api/vendors/", s.handleVendorItem)
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
	_ = s.db.QueryRow("SELECT COUNT(*) FROM event_media").Scan(&totalMedia)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM event_contacts").Scan(&totalLinks)

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

	runway, _ := finance.ComputeRunway(s.db)

	sendJSON(w, http.StatusOK, map[string]any{
		"total_events":   totalEvents,
		"total_contacts": totalContacts,
		"total_media":    totalMedia,
		"total_links":    totalLinks,
		"monthly_counts": monthlyCounts,
		"daily_counts":   dailyCounts,
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		evs, total, err := events.ListEvents(s.db, events.ListOptions{
			Query:    q,
			Tag:      tag,
			FromDate: from,
			ToDate:   to,
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		var tier *int
		if tv := r.URL.Query().Get("tier"); tv != "" {
			if t, err := strconv.Atoi(tv); err == nil {
				tier = &t
			}
		}

		list, total, err := contacts.ListContacts(s.db, contacts.ListOptions{
			Query:  q,
			Tier:   tier,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{
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
	overdueOnly := r.URL.Query().Get("overdue") == "true"
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
	sendJSON(w, http.StatusOK, items)
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
		sendJSON(w, http.StatusOK, snaps)
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
		sendJSON(w, http.StatusOK, comms)
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
		sendJSON(w, http.StatusOK, list)
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
		sendJSON(w, http.StatusOK, list)
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
		sendJSON(w, http.StatusOK, items)
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
		sendJSON(w, http.StatusOK, list)
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
		sendJSON(w, http.StatusOK, list)
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

func (s *Server) handleVendors(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cat := r.URL.Query().Get("category")
		fav := r.URL.Query().Get("favorite") == "true"
		list, err := vendors.ListVendors(s.db, cat, fav)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, list)
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
		list, err := commerce.ListPaymentAccounts(s.db, "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, list)
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
		list, err := commerce.ListReferrals(s.db, "", "")
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sendJSON(w, http.StatusOK, list)
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
