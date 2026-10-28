package fx

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type FXCache struct {
	Rate      float64   `json:"rate"`
	Timestamp time.Time `json:"timestamp"`
}

type openExchangeResp struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
}

type frankfurterResp struct {
	Rates map[string]float64 `json:"rates"`
}

// GetExchangeRate returns the USD/IDR exchange rate with caching and fallbacks.
func GetExchangeRate(dataDir string) float64 {
	cachePath := filepath.Join(dataDir, ".fx_rate_cache.json")

	// 1. Try reading valid cache (< 24 hours old)
	if data, err := os.ReadFile(cachePath); err == nil {
		var cache FXCache
		if err := json.Unmarshal(data, &cache); err == nil && cache.Rate > 10000 {
			if time.Since(cache.Timestamp) < 24*time.Hour {
				return cache.Rate
			}
		}
	}

	// 2. Fetch from open.er-api.com
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://open.er-api.com/v6/latest/USD")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var res openExchangeResp
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
				if rate, ok := res.Rates["IDR"]; ok && rate > 10000 {
					saveCache(cachePath, rate)
					return rate
				}
			}
		}
	}

	// 3. Fallback to Frankfurter API
	resp2, err := client.Get("https://api.frankfurter.dev/v1/latest?base=USD&symbols=IDR")
	if err == nil {
		defer resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			var res frankfurterResp
			if err := json.NewDecoder(resp2.Body).Decode(&res); err == nil {
				if rate, ok := res.Rates["IDR"]; ok && rate > 10000 {
					saveCache(cachePath, rate)
					return rate
				}
			}
		}
	}

	// 4. Stale cache fallback
	if data, err := os.ReadFile(cachePath); err == nil {
		var cache FXCache
		if err := json.Unmarshal(data, &cache); err == nil && cache.Rate > 10000 {
			return cache.Rate
		}
	}

	// 5. Ultimate fallback
	return 16500.0
}

func saveCache(cachePath string, rate float64) {
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	c := FXCache{
		Rate:      rate,
		Timestamp: time.Now(),
	}
	if b, err := json.MarshalIndent(c, "", "  "); err == nil {
		_ = os.WriteFile(cachePath, b, 0644)
	}
}
