package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// IdxAlphaSignal represents a quant alpha signal from the idx-bei engine.
type IdxAlphaSignal struct {
	Ticker           string  `json:"ticker"`
	AnomalyType      string  `json:"anomaly_type,omitempty"`
	WyckoffPhase     string  `json:"wyckoff_phase,omitempty"`
	SmartMoneyDelta  float64 `json:"smart_money_delta,omitempty"`
	NetForeignFlowRP float64 `json:"net_foreign_flow_rp,omitempty"`
	ZComposite       float64 `json:"z_composite,omitempty"`
	CompounderScore  float64 `json:"compounder_score,omitempty"`
	DCAVerdict       string  `json:"dca_verdict,omitempty"`
	ValuationStatus  string  `json:"valuation_status,omitempty"`
	Playbook         string  `json:"playbook,omitempty"`
}

type bandarAnomalyDTO struct {
	Ticker           string  `json:"ticker"`
	AnomalyType      string  `json:"anomaly_type"`
	WyckoffPhase     string  `json:"wyckoff_phase"`
	SmartMoneyDelta  float64 `json:"smart_money_delta"`
	NetForeignFlowRP float64 `json:"net_foreign_flow_rp"`
	ZComposite       float64 `json:"z_composite"`
	Playbook         string  `json:"playbook"`
}

type compounderDTO struct {
	Ticker          string  `json:"ticker"`
	Code            string  `json:"code"`
	Symbol          string  `json:"symbol"`
	CompounderScore float64 `json:"compounder_score"`
	DCAVerdict      string  `json:"dca_verdict"`
	ValuationStatus string  `json:"valuation_status"`
}

// NormalizeTicker strips exchange suffixes like .JK and uppercases the symbol.
func NormalizeTicker(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.TrimSuffix(s, ".JK")
	return s
}

func fetchJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("idx-bei %s returned HTTP %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// FetchIdxSignals queries live bandarmology anomalies and compounder screeners from idx-bei.
// Fails fast on network or HTTP errors without synthetic fallbacks.
func FetchIdxSignals(ctx context.Context, baseURL string) (map[string]IdxAlphaSignal, []IdxAlphaSignal, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "http://127.0.0.1:8000"
	}

	client := &http.Client{Timeout: 8 * time.Second}

	var anomalies []bandarAnomalyDTO
	if err := fetchJSON(ctx, client, base+"/api/bandar/anomalies", &anomalies); err != nil {
		return nil, nil, err
	}

	var compounders []compounderDTO
	if err := fetchJSON(ctx, client, base+"/api/compounder-screen?min_score=65&limit=25", &compounders); err != nil {
		return nil, nil, err
	}

	byTicker := make(map[string]IdxAlphaSignal)
	for _, a := range anomalies {
		t := NormalizeTicker(a.Ticker)
		if t == "" {
			continue
		}
		sig := byTicker[t]
		sig.Ticker = t
		sig.AnomalyType = a.AnomalyType
		sig.WyckoffPhase = a.WyckoffPhase
		sig.SmartMoneyDelta = a.SmartMoneyDelta
		sig.NetForeignFlowRP = a.NetForeignFlowRP
		sig.ZComposite = a.ZComposite
		sig.Playbook = a.Playbook
		byTicker[t] = sig
	}

	for _, c := range compounders {
		raw := c.Ticker
		if raw == "" {
			raw = c.Code
		}
		if raw == "" {
			raw = c.Symbol
		}
		t := NormalizeTicker(raw)
		if t == "" {
			continue
		}
		sig := byTicker[t]
		sig.Ticker = t
		sig.CompounderScore = c.CompounderScore
		sig.DCAVerdict = c.DCAVerdict
		sig.ValuationStatus = c.ValuationStatus
		byTicker[t] = sig
	}

	watchlist := make([]IdxAlphaSignal, 0, len(byTicker))
	for _, sig := range byTicker {
		watchlist = append(watchlist, sig)
	}
	sort.Slice(watchlist, func(i, j int) bool {
		scoreI := watchlist[i].ZComposite*20 + watchlist[i].CompounderScore
		scoreJ := watchlist[j].ZComposite*20 + watchlist[j].CompounderScore
		if scoreI == scoreJ {
			return watchlist[i].Ticker < watchlist[j].Ticker
		}
		return scoreI > scoreJ
	})
	if len(watchlist) > 10 {
		watchlist = watchlist[:10]
	}

	return byTicker, watchlist, nil
}
