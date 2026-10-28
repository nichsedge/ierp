package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

type SpotAccountResp struct {
	Balances []struct {
		Asset  string `json:"asset"`
		Free   string `json:"free"`
		Locked string `json:"locked"`
	} `json:"balances"`
}

type SimpleEarnResp struct {
	Rows []struct {
		Asset       string `json:"asset"`
		TotalAmount string `json:"totalAmount"`
	} `json:"rows"`
}

type TickerPrice struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

type BinanceAssetEntry struct {
	Symbol   string  `json:"symbol"`
	Quantity float64 `json:"quantity"`
	PriceUSD float64 `json:"price_usd"`
	ValueUSD float64 `json:"value_usd"`
}

type RawBinanceData struct {
	Timestamp string              `json:"timestamp"`
	TotalUSD  float64             `json:"total_usd"`
	Assets    []BinanceAssetEntry `json:"assets"`
}

// dohResolver resolves host via Cloudflare DoH to bypass DNS hijacking.
func resolveDoH(host string) (string, error) {
	dohURL := fmt.Sprintf("https://cloudflare-dns.com/dns-query?name=%s&type=A", url.QueryEscape(host))
	req, _ := http.NewRequest(http.MethodGet, dohURL, nil)
	req.Header.Set("Accept", "application/dns-json")

	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	for _, ans := range result.Answer {
		if ans.Type == 1 && ans.Data != "" {
			return ans.Data, nil
		}
	}

	return "", fmt.Errorf("no A record found for %s", host)
}

func getDoHHttpClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err == nil && (strings.HasSuffix(host, "binance.com") || strings.HasSuffix(host, "binance.vision")) {
				if ip, err := resolveDoH(host); err == nil && ip != "" {
					return dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
				}
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

func signQuery(query string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(query))
	return hex.EncodeToString(mac.Sum(nil))
}

// Fetch retrieves spot and Simple Earn balances from Binance.
func Fetch(apiKey, secret string) (*RawBinanceData, []models.Holding, error) {
	if apiKey == "" || secret == "" {
		return nil, nil, fmt.Errorf("Binance API credentials not provided")
	}

	client := getDoHHttpClient()
	balances := make(map[string]float64)

	// 1. Fetch spot balances
	timestamp := time.Now().UnixMilli()
	query := fmt.Sprintf("timestamp=%d", timestamp)
	signature := signQuery(query, secret)
	spotURL := fmt.Sprintf("https://api.binance.com/api/v3/account?%s&signature=%s", query, signature)

	req, _ := http.NewRequest(http.MethodGet, spotURL, nil)
	req.Header.Set("X-MBX-APIKEY", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch Binance spot balances: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("binance spot API error (status %d): %s", resp.StatusCode, string(body))
	}
	var spot SpotAccountResp
	if err := json.NewDecoder(resp.Body).Decode(&spot); err != nil {
		return nil, nil, fmt.Errorf("failed to decode Binance spot balances: %w", err)
	}
	for _, b := range spot.Balances {
		free, _ := strconv.ParseFloat(b.Free, 64)
		locked, _ := strconv.ParseFloat(b.Locked, 64)
		tot := free + locked
		if tot > 0 {
			balances[b.Asset] += tot
		}
	}

	// 2. Fetch Simple Earn Flexible
	timestamp = time.Now().UnixMilli()
	query = fmt.Sprintf("timestamp=%d", timestamp)
	signature = signQuery(query, secret)
	flexURL := fmt.Sprintf("https://api.binance.com/sapi/v1/simple-earn/flexible/position?%s&signature=%s", query, signature)

	reqFlex, _ := http.NewRequest(http.MethodGet, flexURL, nil)
	reqFlex.Header.Set("X-MBX-APIKEY", apiKey)
	if resp, err := client.Do(reqFlex); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var flex SimpleEarnResp
		if err := json.NewDecoder(resp.Body).Decode(&flex); err == nil {
			for _, r := range flex.Rows {
				amt, _ := strconv.ParseFloat(r.TotalAmount, 64)
				if amt > 0 {
					balances[r.Asset] += amt
				}
			}
		}
	}

	// 3. Fetch Simple Earn Locked
	timestamp = time.Now().UnixMilli()
	query = fmt.Sprintf("timestamp=%d", timestamp)
	signature = signQuery(query, secret)
	lockURL := fmt.Sprintf("https://api.binance.com/sapi/v1/simple-earn/locked/position?%s&signature=%s", query, signature)

	reqLock, _ := http.NewRequest(http.MethodGet, lockURL, nil)
	reqLock.Header.Set("X-MBX-APIKEY", apiKey)
	if resp, err := client.Do(reqLock); err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var locked SimpleEarnResp
		if err := json.NewDecoder(resp.Body).Decode(&locked); err == nil {
			for _, r := range locked.Rows {
				amt, _ := strconv.ParseFloat(r.TotalAmount, 64)
				if amt > 0 {
					balances[r.Asset] += amt
				}
			}
		}
	}

	// 4. Fetch Tickers for price conversion
	prices := map[string]float64{
		"USDT": 1.0,
		"USDC": 1.0,
		"BUSD": 1.0,
		"DAI":  1.0,
		"FDUSD": 1.0,
	}

	tickerURL := "https://api.binance.com/api/v3/ticker/price"
	respTick, err := client.Get(tickerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch Binance ticker prices: %w", err)
	}
	defer respTick.Body.Close()
	if respTick.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respTick.Body)
		return nil, nil, fmt.Errorf("binance ticker prices error (%d): %s", respTick.StatusCode, string(body))
	}
	var tickers []TickerPrice
	if err := json.NewDecoder(respTick.Body).Decode(&tickers); err != nil {
		return nil, nil, fmt.Errorf("failed to decode Binance tickers: %w", err)
	}
	for _, t := range tickers {
		p, _ := strconv.ParseFloat(t.Price, 64)
		if strings.HasSuffix(t.Symbol, "USDT") {
			base := strings.TrimSuffix(t.Symbol, "USDT")
			prices[base] = p
		} else if strings.HasSuffix(t.Symbol, "USDC") {
			base := strings.TrimSuffix(t.Symbol, "USDC")
			if _, ok := prices[base]; !ok {
				prices[base] = p
			}
		}
	}

	raw := &RawBinanceData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Assets:    make([]BinanceAssetEntry, 0),
	}

	for asset, qty := range balances {
		price := prices[asset]
		valUSD := qty * price

		raw.Assets = append(raw.Assets, BinanceAssetEntry{
			Symbol:   asset,
			Quantity: qty,
			PriceUSD: price,
			ValueUSD: valUSD,
		})
		raw.TotalUSD += valUSD
	}

	holdings := Standardize(raw)
	return raw, holdings, nil
}

// Standardize converts raw Binance assets to standardized holdings.
func Standardize(data *RawBinanceData) []models.Holding {
	var holdings []models.Holding
	usdThreshold := models.FilterThresholds["USD"]

	for _, a := range data.Assets {
		if a.ValueUSD < usdThreshold {
			continue
		}

		category := models.GetStandardCategory("Cryptocurrency", a.Symbol)
		price := a.PriceUSD
		valUSD := a.ValueUSD

		holdings = append(holdings, models.Holding{
			Source:     "Binance",
			Category:   category,
			Asset:      a.Symbol,
			Ticker:     a.Symbol,
			Name:       a.Symbol,
			Currency:   "USD",
			Quantity:   a.Quantity,
			Price:      &price,
			ValueUSD:   &valUSD,
			Account:    "Binance Main Account",
			Details:    "",
			AssetClass: models.GetAssetClass(category),
		})
	}

	return holdings
}

// SaveRaw dumps raw Binance data to disk.
func SaveRaw(data *RawBinanceData, targetPath string) error {
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, b, 0644)
}
