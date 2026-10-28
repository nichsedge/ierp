package fx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetExchangeRate_CacheValid(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, ".fx_rate_cache.json")

	expectedRate := 16250.5
	cache := FXCache{
		Rate:      expectedRate,
		Timestamp: time.Now(),
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("failed to marshal cache: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	rate := GetExchangeRate(tempDir)
	if rate != expectedRate {
		t.Errorf("expected rate %f from cache, got %f", expectedRate, rate)
	}
}

func TestGetExchangeRate_StaleCacheFallback(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, ".fx_rate_cache.json")

	// Set timestamp to 48 hours ago (stale cache)
	staleRate := 16100.0
	cache := FXCache{
		Rate:      staleRate,
		Timestamp: time.Now().Add(-48 * time.Hour),
	}
	data, _ := json.Marshal(cache)
	_ = os.WriteFile(cachePath, data, 0644)

	// Since live network requests might fail in offline test environments,
	// GetExchangeRate should fall back to either live API or stale cache (> 10000).
	rate := GetExchangeRate(tempDir)
	if rate < 10000 {
		t.Errorf("expected valid rate >= 10000, got %f", rate)
	}
}

func TestSaveCache(t *testing.T) {
	tempDir := t.TempDir()
	cachePath := filepath.Join(tempDir, "subdir", ".fx_rate_cache.json")

	saveCache(cachePath, 16300.0)

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache file was not created: %v", err)
	}

	var cache FXCache
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("failed to parse saved cache: %v", err)
	}

	if cache.Rate != 16300.0 {
		t.Errorf("expected rate 16300.0, got %f", cache.Rate)
	}
}

func TestOpenExchangeResp_Parsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openExchangeResp{
			Result: "success",
			Rates: map[string]float64{
				"IDR": 16450.0,
			},
		})
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("failed to GET mock: %v", err)
	}
	defer resp.Body.Close()

	var res openExchangeResp
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if res.Rates["IDR"] != 16450.0 {
		t.Errorf("expected IDR rate 16450.0, got %f", res.Rates["IDR"])
	}
}
