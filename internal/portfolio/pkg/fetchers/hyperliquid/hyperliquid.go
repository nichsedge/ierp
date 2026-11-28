package hyperliquid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

const DefaultInfoURL = "https://api.hyperliquid.xyz/info"

type VaultEquity struct {
	VaultAddress         string `json:"vaultAddress"`
	Equity               string `json:"equity"`
	LockedUntilTimestamp int64  `json:"lockedUntilTimestamp,omitempty"`
}

type ClearinghouseState struct {
	MarginSummary struct {
		AccountValue string `json:"accountValue"`
	} `json:"marginSummary"`
	Withdrawable string `json:"withdrawable"`
}

type SpotBalance struct {
	Coin  string `json:"coin"`
	Token int    `json:"token"`
	Total string `json:"total"`
	Hold  string `json:"hold"`
}

type SpotClearinghouseState struct {
	Balances []SpotBalance `json:"balances"`
}

type RawHyperliquidData struct {
	Timestamp    string                 `json:"timestamp"`
	User         string                 `json:"user"`
	Vaults       []VaultEquity          `json:"vaults"`
	PerpState    ClearinghouseState     `json:"perp_state"`
	SpotState    SpotClearinghouseState `json:"spot_state"`
}

// Fetch queries the Hyperliquid public Info API for vault equities, perp margin, and spot balances.
func Fetch(ethAddress string) (*RawHyperliquidData, []models.Holding, error) {
	return FetchWithURL(DefaultInfoURL, ethAddress)
}

// FetchWithURL queries a custom Hyperliquid Info API endpoint (useful for testing).
func FetchWithURL(infoURL, ethAddress string) (*RawHyperliquidData, []models.Holding, error) {
	if ethAddress == "" {
		return nil, nil, fmt.Errorf("ETH address not provided")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	raw := &RawHyperliquidData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		User:      ethAddress,
	}

	if err := postInfo(client, infoURL, map[string]string{"type": "userVaultEquities", "user": ethAddress}, &raw.Vaults); err != nil {
		return nil, nil, fmt.Errorf("hyperliquid userVaultEquities failed: %w", err)
	}
	if err := postInfo(client, infoURL, map[string]string{"type": "clearinghouseState", "user": ethAddress}, &raw.PerpState); err != nil {
		return nil, nil, fmt.Errorf("hyperliquid clearinghouseState failed: %w", err)
	}
	if err := postInfo(client, infoURL, map[string]string{"type": "spotClearinghouseState", "user": ethAddress}, &raw.SpotState); err != nil {
		return nil, nil, fmt.Errorf("hyperliquid spotClearinghouseState failed: %w", err)
	}

	return raw, Standardize(raw), nil
}

func postInfo(client *http.Client, url string, payload map[string]string, out interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Standardize converts raw Hyperliquid data into standardized portfolio holdings.
func Standardize(data *RawHyperliquidData) []models.Holding {
	if data == nil {
		return nil
	}
	var holdings []models.Holding
	usdThreshold := models.FilterThresholds["USD"]

	for _, v := range data.Vaults {
		eq, err := strconv.ParseFloat(v.Equity, 64)
		if err != nil || eq < usdThreshold {
			continue
		}
		price := 1.0
		val := eq
		holdings = append(holdings, models.Holding{
			Source:     "hyperliquid",
			Category:   "Yield / LP",
			Asset:      "USDC",
			Ticker:     "USDC",
			Name:       "Hyperliquid Vault USDC",
			Currency:   "USD",
			Quantity:   eq,
			Price:      &price,
			ValueUSD:   &val,
			Account:    "Hyperliquid",
			Details:    fmt.Sprintf("Main-Account Vault (%s)", v.VaultAddress),
			AssetClass: models.GetAssetClass("Yield / LP"),
		})
	}

	if perpVal, err := strconv.ParseFloat(data.PerpState.MarginSummary.AccountValue, 64); err == nil && perpVal >= usdThreshold {
		price := 1.0
		val := perpVal
		holdings = append(holdings, models.Holding{
			Source:     "hyperliquid",
			Category:   "Stablecoin",
			Asset:      "USDC",
			Ticker:     "USDC",
			Name:       "Hyperliquid Perps USDC",
			Currency:   "USD",
			Quantity:   perpVal,
			Price:      &price,
			ValueUSD:   &val,
			Account:    "Hyperliquid Perps",
			Details:    "Clearinghouse Margin Account",
			AssetClass: models.GetAssetClass("Stablecoin"),
		})
	}

	for _, sb := range data.SpotState.Balances {
		if sb.Coin != "USDC" {
			continue
		}
		qty, err := strconv.ParseFloat(sb.Total, 64)
		if err != nil || qty < usdThreshold {
			continue
		}
		price := 1.0
		val := qty
		holdings = append(holdings, models.Holding{
			Source:     "hyperliquid",
			Category:   "Stablecoin",
			Asset:      "USDC",
			Ticker:     "USDC",
			Name:       "Hyperliquid Spot USDC",
			Currency:   "USD",
			Quantity:   qty,
			Price:      &price,
			ValueUSD:   &val,
			Account:    "Hyperliquid Spot",
			Details:    "Spot Balance",
			AssetClass: models.GetAssetClass("Stablecoin"),
		})
	}

	return holdings
}

// SaveRaw dumps raw Hyperliquid data to disk.
func SaveRaw(data *RawHyperliquidData, targetPath string) error {
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, b, 0644)
}
