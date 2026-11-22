package dashboard_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nichsedge/ierp/internal/dashboard"
	"github.com/nichsedge/ierp/internal/db"
)

func setupTestServer(t *testing.T, opts ...dashboard.Option) (http.Handler, *dashboard.Server) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_dash.db")

	if err := db.InitDB(dbPath, false); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	server := dashboard.NewServer(database, "127.0.0.1", 8921, opts...)
	return server.Handler(), server
}

func TestDashboardAuthorization(t *testing.T) {
	authToken := "super-secret-token-12345"
	handler, _ := setupTestServer(t, dashboard.WithAuthToken(authToken))

	// 1. Unauthenticated request to /api/stats should return 401
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /api/stats, got %d", rec.Code)
	}

	// 2. Request with wrong token should return 401
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid Bearer token, got %d", rec.Code)
	}

	// 3. Request with valid Bearer token should succeed (200)
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid Bearer token, got %d", rec.Code)
	}

	// 4. Request with valid X-Auth-Token should succeed (200)
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("X-Auth-Token", authToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid X-Auth-Token, got %d", rec.Code)
	}

	// 5. Request with valid query parameter should succeed (200)
	req = httptest.NewRequest(http.MethodGet, "/api/stats?token="+authToken, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid token query param, got %d", rec.Code)
	}

	// 6. Request with valid cookie should succeed (200)
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "ierp_token", Value: authToken})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid cookie, got %d", rec.Code)
	}

	// 7. Unauthenticated POST to /webhook/location should return 401
	locPayload := `{"lat": -6.2088, "lon": 106.8456}`
	req = httptest.NewRequest(http.MethodPost, "/webhook/location", strings.NewReader(locPayload))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated webhook, got %d", rec.Code)
	}

	// 8. Authenticated POST to /webhook/location should succeed (200)
	req = httptest.NewRequest(http.MethodPost, "/webhook/location", strings.NewReader(locPayload))
	req.Header.Set("Authorization", "Bearer "+authToken)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated webhook, got %d: %s", rec.Code, rec.Body.String())
	}

	// 9. Root UI without token should return 401 with auth unlock form
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for root UI without token, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Authentication token required") {
		t.Fatalf("expected auth unlock form, got %s", rec.Body.String())
	}

	// 10. Root UI with ?token=... sets cookie and redirects to clean URL
	req = httptest.NewRequest(http.MethodGet, "/?token="+authToken, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect for ?token=..., got %d", rec.Code)
	}
	cookieHeader := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "ierp_token="+authToken) {
		t.Fatalf("expected Set-Cookie with ierp_token, got %s", cookieHeader)
	}

	// 11. Server with no auth token allows open localhost access
	openHandler, _ := setupTestServer(t)
	req = httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rec = httptest.NewRecorder()
	openHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for open localhost access, got %d", rec.Code)
	}
}

func TestDashboardMethods(t *testing.T) {
	handler, _ := setupTestServer(t)

	// POST to GET-only endpoint should return 405
	req := httptest.NewRequest(http.MethodPost, "/api/stats", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /api/stats, got %d", rec.Code)
	}

	// GET to POST-only webhook should return 405
	req = httptest.NewRequest(http.MethodGet, "/webhook/location", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /webhook/location, got %d", rec.Code)
	}

	// GET to DELETE-only expense item endpoint should return 405
	req = httptest.NewRequest(http.MethodGet, "/api/expenses/123", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /api/expenses/123, got %d", rec.Code)
	}
}

func TestDashboardValidationAndHardening(t *testing.T) {
	handler, _ := setupTestServer(t)

	// 1. Invalid JSON body
	req := httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader("{invalid-json"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d", rec.Code)
	}

	// 2. Reject unknown JSON fields
	unknownPayload := `{"name": "Alice", "unknown_field_attack": "bad"}`
	req = httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader(unknownPayload))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields in request body, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown_field_attack") && !strings.Contains(rec.Body.String(), "unknown field") {
		t.Fatalf("expected error message mentioning unknown field, got %s", rec.Body.String())
	}

	// 3. Required field validation: Contact name is required
	emptyContact := `{"name": ""}`
	req = httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader(emptyContact))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty contact name, got %d", rec.Code)
	}

	// 4. Required field validation: Event title is required
	emptyEvent := `{"title": ""}`
	req = httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(emptyEvent))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty event title, got %d", rec.Code)
	}

	// 5. Capped JSON body size (> 1MB should be rejected)
	largeData := strings.Repeat("A", 1024*1024+100)
	oversizedBody := fmt.Sprintf(`{"name": "%s"}`, largeData)
	req = httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader(oversizedBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 400 or 413 for oversized body, got %d", rec.Code)
	}
}

func TestDashboardCRUDContracts(t *testing.T) {
	handler, _ := setupTestServer(t)

	// 1. Create contact via POST /api/contacts
	contactBody := `{"name": "Alice Wonderland", "org": "Wonderland Inc", "tier": 1}`
	req := httptest.NewRequest(http.MethodPost, "/api/contacts", strings.NewReader(contactBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for POST /api/contacts, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("failed unmarshaling create response: %v", err)
	}
	contactIDFloat, ok := createResp["id"].(float64)
	if !ok || contactIDFloat <= 0 {
		t.Fatalf("expected positive contact ID, got %v", createResp["id"])
	}
	contactID := int(contactIDFloat)

	// 2. Fetch contacts via GET /api/contacts
	req = httptest.NewRequest(http.MethodGet, "/api/contacts", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/contacts, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Alice Wonderland") {
		t.Fatalf("expected Alice Wonderland in contacts list, got %s", rec.Body.String())
	}

	// 3. Update tier via POST /api/contacts/{id}/tier
	tierBody := `{"tier": 2, "cadence_days": 45}`
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/contacts/%d/tier", contactID), strings.NewReader(tierBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST /api/contacts/{id}/tier, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Create event via POST /api/events
	eventBody := fmt.Sprintf(`{"title": "Sync with Alice", "start_date": "2026-11-22", "contacts": ["Alice Wonderland"], "notes": "Discussion"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(eventBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for POST /api/events, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Fetch events via GET /api/events
	req = httptest.NewRequest(http.MethodGet, "/api/events", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/events, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sync with Alice") {
		t.Fatalf("expected Sync with Alice in events list, got %s", rec.Body.String())
	}

	// 6. Create expense via POST /api/expenses
	expBody := `{"date": "2026-11-22", "title": "Coffee and Croissant", "amount": 65000, "category": "food", "currency": "IDR", "type": "EXPENSE"}`
	req = httptest.NewRequest(http.MethodPost, "/api/expenses", strings.NewReader(expBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for POST /api/expenses, got %d: %s", rec.Code, rec.Body.String())
	}
	var expResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &expResp); err != nil {
		t.Fatalf("failed unmarshaling expense response: %v", err)
	}
	expID := int(expResp["id"].(float64))

	// 7. Read expenses via GET /api/expenses
	req = httptest.NewRequest(http.MethodGet, "/api/expenses", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/expenses, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Coffee and Croissant") {
		t.Fatalf("expected Coffee and Croissant in expenses, got %s", rec.Body.String())
	}

	// 8. Delete expense via DELETE /api/expenses/{id}
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/expenses/%d", expID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE /api/expenses/{id}, got %d: %s", rec.Code, rec.Body.String())
	}

	// 9. Deleting again returns 404
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/expenses/%d", expID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for deleting nonexistent expense, got %d", rec.Code)
	}
}

func TestDashboardErrorResponses(t *testing.T) {
	handler, _ := setupTestServer(t)

	// Non-existent API route
	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent API route, got %d", rec.Code)
	}

	// Invalid ID in path
	req = httptest.NewRequest(http.MethodPost, "/api/contacts/not-an-int/tier", strings.NewReader(`{"tier": 1}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-integer contact ID, got %d", rec.Code)
	}

	// Non-existent expense delete with bad ID format
	req = httptest.NewRequest(http.MethodDelete, "/api/expenses/bad-id", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad expense ID format, got %d", rec.Code)
	}
}

func drainResponseBody(t *testing.T, res *http.Response) []byte {
	t.Helper()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	return body
}

func jsonBuffer(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed marshaling JSON: %v", err)
	}
	return bytes.NewBuffer(b)
}
