package dashboard

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/finance"
)

//go:embed assets/*
var assetsFS embed.FS

// Option configures Server instances.
type Option func(*Server)

// WithAuthToken configures an authentication token for API, webhook, and dashboard access.
func WithAuthToken(token string) Option {
	return func(s *Server) {
		s.authToken = strings.TrimSpace(token)
	}
}

// Server encapsulates the web dashboard HTTP server.
type Server struct {
	db        *sql.DB
	port      int
	host      string
	authToken string
}

// NewServer initializes a dashboard server. Defaults to 127.0.0.1 and port 8921.
func NewServer(database *sql.DB, host string, port int, opts ...Option) *Server {
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 8921
	}
	s := &Server{
		db:   database,
		host: host,
		port: port,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetTailscaleIP attempts to detect the local Tailscale IPv4 address in the 100.64.0.0/10 CGNAT block.
func GetTailscaleIP() (string, error) {
	_, tailscaleNet, err := net.ParseCIDR("100.64.0.0/10")
	if err == nil {
		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip != nil && ip.To4() != nil && tailscaleNet.Contains(ip) {
						return ip.String(), nil
					}
				}
			}
		}
	}

	// Fallback to calling `tailscale ip -4`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tailscale", "ip", "-4")
	out, err := cmd.Output()
	if err == nil {
		ipStr := strings.TrimSpace(string(out))
		if parsed := net.ParseIP(ipStr); parsed != nil && parsed.To4() != nil {
			return ipStr, nil
		}
	}

	return "", fmt.Errorf("no Tailscale IPv4 address found (ensure Tailscale is running)")
}

func (s *Server) authenticate(r *http.Request) bool {
	if s.authToken == "" {
		return true
	}

	// 1. Authorization header: "Bearer <token>" or "<token>"
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		token := authHeader
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			token = parts[1]
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1 {
			return true
		}
	}

	// 2. X-Auth-Token header
	if token := r.Header.Get("X-Auth-Token"); token != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1 {
			return true
		}
	}

	// 3. Query parameter: ?token=... or ?auth_token=...
	if token := r.URL.Query().Get("token"); token != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1 {
			return true
		}
	}
	if token := r.URL.Query().Get("auth_token"); token != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) == 1 {
			return true
		}
	}

	// 4. Cookie: ierp_token=...
	if cookie, err := r.Cookie("ierp_token"); err == nil && cookie != nil {
		if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(s.authToken)) == 1 {
			return true
		}
	}

	return false
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticate(r) {
			sendError(w, http.StatusUnauthorized, "Unauthorized: invalid or missing auth token")
			return
		}
		next(w, r)
	}
}

const maxRequestBodySize = 1 << 20 // 1 MB limit

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected extra data after JSON body")
	}
	return nil
}

// Handler builds and returns the configured http.Handler for the dashboard and API routes.
func (s *Server) Handler() http.Handler {
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
		if s.authToken != "" {
			if tokenQuery := r.URL.Query().Get("token"); tokenQuery != "" {
				if subtle.ConstantTimeCompare([]byte(tokenQuery), []byte(s.authToken)) == 1 {
					http.SetCookie(w, &http.Cookie{
						Name:     "ierp_token",
						Value:    tokenQuery,
						Path:     "/",
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
					u := *r.URL
					q := u.Query()
					q.Del("token")
					u.RawQuery = q.Encode()
					target := u.RequestURI()
					if target == "" {
						target = "/"
					}
					http.Redirect(w, r, target, http.StatusFound)
					return
				}
			}

			if !s.authenticate(r) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>iERP Dashboard Auth</title><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{background:#0f172a;color:#f8fafc;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;}form{background:#1e293b;padding:2rem;border-radius:0.75rem;box-shadow:0 10px 25px rgba(0,0,0,0.5);width:100%%;max-width:360px;}h2{margin-top:0;}input{width:100%%;padding:0.75rem;border-radius:0.375rem;border:1px solid #334155;background:#0f172a;color:#fff;margin-bottom:1rem;box-sizing:border-box;}button{width:100%%;padding:0.75rem;border-radius:0.375rem;border:none;background:#2563eb;color:#fff;font-weight:bold;cursor:pointer;}</style></head><body><form method="GET" action="/"><h2>iERP Dashboard</h2><p style="color:#94a3b8;font-size:0.875rem">Authentication token required.</p><input type="password" name="token" placeholder="Enter auth token" required autofocus><button type="submit">Unlock</button></form></body></html>`)
				return
			}
		}

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

	// REST APIs (authenticated)
	mux.HandleFunc("/api/stats", s.requireAuth(s.handleStats))
	mux.HandleFunc("/api/events", s.requireAuth(s.handleEvents))
	mux.HandleFunc("/api/events/", s.requireAuth(s.handleEventItem))
	mux.HandleFunc("/api/contacts", s.requireAuth(s.handleContacts))
	mux.HandleFunc("/api/contacts/automerge", s.requireAuth(s.handleContactsAutomerge))
	mux.HandleFunc("/api/contacts/", s.requireAuth(s.handleContactItem))
	mux.HandleFunc("/api/radar", s.requireAuth(s.handleRadar))
	mux.HandleFunc("/api/radar/daily", s.requireAuth(s.handleRadarDaily))
	mux.HandleFunc("/api/radar/summary", s.requireAuth(s.handleRadarSummary))
	mux.HandleFunc("/api/runway", s.requireAuth(s.handleRunway))
	mux.HandleFunc("/api/finance/snapshots", s.requireAuth(s.handleFinanceSnapshots))
	mux.HandleFunc("/api/finance/commitments", s.requireAuth(s.handleFinanceCommitments))
	mux.HandleFunc("/api/expenses", s.requireAuth(s.handleExpenses))
	mux.HandleFunc("/api/expenses/", s.requireAuth(s.handleExpenseItem))
	mux.HandleFunc("/api/projects", s.requireAuth(s.handleProjects))
	mux.HandleFunc("/api/decisions", s.requireAuth(s.handleDecisions))
	mux.HandleFunc("/api/decisions/", s.requireAuth(s.handleDecisionItem))
	mux.HandleFunc("/api/maintenance", s.requireAuth(s.handleMaintenance))
	mux.HandleFunc("/api/maintenance/", s.requireAuth(s.handleMaintenanceItem))
	mux.HandleFunc("/api/reviews", s.requireAuth(s.handleReviews))
	mux.HandleFunc("/api/gadgets", s.requireAuth(s.handleGadgets))
	mux.HandleFunc("/api/media", s.requireAuth(s.handleMedia))
	mux.HandleFunc("/api/links", s.requireAuth(s.handleLinks))
	mux.HandleFunc("/api/places", s.requireAuth(s.handlePlaces))
	mux.HandleFunc("/api/vendors", s.requireAuth(s.handleVendors))
	mux.HandleFunc("/api/vendors/", s.requireAuth(s.handleVendorItem))
	mux.HandleFunc("/api/payment-accounts", s.requireAuth(s.handleCommerceAccounts))
	mux.HandleFunc("/api/referrals", s.requireAuth(s.handleCommerceReferrals))
	mux.HandleFunc("/api/commerce/accounts", s.requireAuth(s.handleCommerceAccounts))
	mux.HandleFunc("/api/commerce/referrals", s.requireAuth(s.handleCommerceReferrals))
	mux.HandleFunc("/api/audit", s.requireAuth(s.handleAudit))
	mux.HandleFunc("/webhook/location", s.requireAuth(s.handleLocationWebhook))

	return mux
}

// Start launches the HTTP server listening on the configured host and port.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	fmt.Printf("%s%siERP Web Dashboard running at http://%s%s\n", config.Bold, config.Green, addr, config.Reset)
	if s.authToken != "" {
		fmt.Printf("%sDashboard authentication enabled.%s\n", config.Cyan, config.Reset)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}
	return srv.ListenAndServe()
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
	if r.Method != http.MethodGet {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var totalEvents, totalLifeEvents, totalContacts, totalMedia, totalLinks int
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{"SELECT COUNT(*) FROM events", &totalEvents},
		{"SELECT COUNT(*) FROM events WHERE (tags IS NULL OR (tags NOT LIKE '%timeline%' AND tags NOT LIKE '%atracker%' AND tags NOT LIKE '%deepwork%'))", &totalLifeEvents},
		{"SELECT COUNT(*) FROM contacts", &totalContacts},
		{"SELECT COUNT(*) FROM media_items", &totalMedia},
		{"SELECT COUNT(*) FROM links", &totalLinks},
	} {
		if err := s.db.QueryRow(q.sql).Scan(q.dst); err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

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
		"total_events":      totalEvents,
		"total_life_events": totalLifeEvents,
		"total_contacts":    totalContacts,
		"total_media":       totalMedia,
		"total_links":       totalLinks,
		"monthly_counts":    monthlyCounts,
		"daily_counts":      dailyCounts,
		"media_summary":     mediaSummary,
		"top_places":        topPlaces,
		"top_tags":          topTags,
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
