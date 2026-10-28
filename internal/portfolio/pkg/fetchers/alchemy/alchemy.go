package alchemy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/edwards25519"
	"github.com/mr-tron/base58/base58"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

const (
	GrassTokenMint = "Grass7B4RdKfBCjTKgSqnXkqjwiGvQyFbuSCUJr3XXjs"
	GrassProgramID = "EyxPPowqBRTpZpiDb2ixUR6XUU1VJwTCNgJdK8eyc6kc"
)

type TokenPrice struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

type TokenMetadata struct {
	Decimals int    `json:"decimals"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
}

type TokenRaw struct {
	Address       string        `json:"address"`
	Network       string        `json:"network"`
	TokenAddress  string        `json:"tokenAddress"`
	TokenBalance  string        `json:"tokenBalance"`
	TokenMetadata TokenMetadata `json:"tokenMetadata"`
	TokenPrices   []TokenPrice  `json:"tokenPrices"`
}

type AlchemyResponse struct {
	Data struct {
		Tokens []TokenRaw `json:"tokens"`
	} `json:"data"`
}

type AssetEntry struct {
	Address      string  `json:"address"`
	Network      string  `json:"network"`
	TokenAddress string  `json:"token_address"`
	Symbol       string  `json:"symbol"`
	Name         string  `json:"name"`
	Quantity     float64 `json:"quantity"`
	Decimals     int     `json:"decimals"`
	PriceUSD     float64 `json:"price_usd"`
	ValueUSD     float64 `json:"value_usd"`
	Category     string  `json:"category,omitempty"`
	IsStaked     bool    `json:"is_staked,omitempty"`
}

type CuratedAlchemyData struct {
	Timestamp string       `json:"timestamp"`
	TotalUSD  float64      `json:"total_usd"`
	Assets    []AssetEntry `json:"assets"`
}

type DexScreenerResponse struct {
	Pairs []struct {
		BaseToken struct {
			Address string `json:"address"`
			Symbol  string `json:"symbol"`
		} `json:"baseToken"`
		PriceUSD string `json:"priceUsd"`
	} `json:"pairs"`
}

// fetchDexScreenerPrices queries DexScreener API for token USD prices.
func fetchDexScreenerPrices(tokenAddresses []string) map[string]float64 {
	prices := make(map[string]float64)
	if len(tokenAddresses) == 0 {
		return prices
	}

	url := fmt.Sprintf("https://api.dexscreener.com/latest/dex/tokens/%s", strings.Join(tokenAddresses, ","))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return prices
	}
	defer resp.Body.Close()

	var dexResp DexScreenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&dexResp); err != nil {
		return prices
	}

	for _, p := range dexResp.Pairs {
		addr := p.BaseToken.Address
		if _, exists := prices[addr]; !exists && p.PriceUSD != "" {
			var pv float64
			fmt.Sscanf(p.PriceUSD, "%f", &pv)
			if pv > 0 {
				prices[addr] = pv
			}
		}
	}
	return prices
}

// deriveGrassStakePDA derives the Solana Program Derived Address for Grass UserStakeInfo.
func deriveGrassStakePDA(userWallet string) (string, error) {
	userPK, err := base58.Decode(userWallet)
	if err != nil {
		return "", fmt.Errorf("invalid user wallet base58: %w", err)
	}
	programID, err := base58.Decode(GrassProgramID)
	if err != nil {
		return "", fmt.Errorf("invalid program id base58: %w", err)
	}

	for bump := 255; bump >= 0; bump-- {
		var buf []byte
		buf = append(buf, userPK...)
		buf = append(buf, []byte("userStakeInfo")...)
		buf = append(buf, byte(bump))
		buf = append(buf, programID...)
		buf = append(buf, []byte("ProgramDerivedAddress")...)

		h := sha256.Sum256(buf)
		if _, err := new(edwards25519.Point).SetBytes(h[:]); err != nil {
			return base58.Encode(h[:]), nil
		}
	}
	return "", fmt.Errorf("failed to derive valid PDA for Grass staking")
}

// fetchGrassStaked fetches the on-chain staked GRASS amount from the Grass Staking Program.
func fetchGrassStaked(apiKey, walletAddress string) (float64, error) {
	pda, err := deriveGrassStakePDA(walletAddress)
	if err != nil {
		return 0, err
	}

	rpcURL := fmt.Sprintf("https://solana-mainnet.g.alchemy.com/v2/%s", apiKey)
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getAccountInfo",
		"params": []interface{}{
			pda,
			map[string]string{"encoding": "base64"},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(rpcURL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("rpc status %d", resp.StatusCode)
	}

	var rpcResp struct {
		Result struct {
			Value *struct {
				Data []string `json:"data"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return 0, err
	}

	if rpcResp.Result.Value == nil || len(rpcResp.Result.Value.Data) == 0 {
		return 0, nil
	}

	rawB64 := rpcResp.Result.Value.Data[0]
	rawBytes, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return 0, err
	}

	// UserStakeInfo struct size is 2584 bytes; amount_staked (uint64) is at offset 2568..2576
	if len(rawBytes) < 2576 {
		return 0, nil
	}

	stakedUnits := binary.LittleEndian.Uint64(rawBytes[2568:2576])
	qty := float64(stakedUnits) / 1e9
	return qty, nil
}

// Fetch queries the Alchemy API for Solana holdings and enriches with DexScreener & DeFi staking.
func Fetch(apiKey, walletAddress string) (*CuratedAlchemyData, []models.Holding, error) {
	if apiKey == "" || walletAddress == "" {
		return nil, nil, fmt.Errorf("Alchemy API key or wallet address not provided")
	}

	url := fmt.Sprintf("https://api.g.alchemy.com/data/v1/%s/assets/tokens/by-address", apiKey)
	payload := map[string]interface{}{
		"addresses": []map[string]interface{}{
			{
				"address":  walletAddress,
				"networks": []string{"solana-mainnet"},
			},
		},
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("Alchemy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Alchemy API returned status %d", resp.StatusCode)
	}

	var alchResp AlchemyResponse
	if err := json.NewDecoder(resp.Body).Decode(&alchResp); err != nil {
		return nil, nil, fmt.Errorf("failed to decode Alchemy response: %w", err)
	}

	curated := &CuratedAlchemyData{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Assets:    make([]AssetEntry, 0),
	}

	for _, t := range alchResp.Data.Tokens {
		// Convert hex balance to big.Int then float
		balanceInt := new(big.Int)
		hexStr := t.TokenBalance
		if len(hexStr) >= 2 && hexStr[:2] == "0x" {
			hexStr = hexStr[2:]
		}
		balanceInt.SetString(hexStr, 16)

		decimals := t.TokenMetadata.Decimals
		if decimals == 0 {
			decimals = 9
		}
		divisor := math.Pow(10, float64(decimals))
		fBalance, _ := new(big.Float).SetInt(balanceInt).Float64()
		qty := fBalance / divisor

		var priceUSD float64
		for _, p := range t.TokenPrices {
			if p.Currency == "usd" {
				var pv float64
				fmt.Sscanf(p.Value, "%f", &pv)
				priceUSD = pv
				break
			}
		}

		symbol := t.TokenMetadata.Symbol
		name := t.TokenMetadata.Name

		// Handle native SOL where Alchemy leaves tokenAddress null/empty and symbol/name empty
		// Handle native SOL where Alchemy leaves tokenAddress null/empty and symbol/name empty or UNKNOWN
		if (t.TokenAddress == "" || t.TokenAddress == "null") && (strings.Contains(strings.ToLower(t.Network), "solana") || t.Network == "") {
			if symbol == "" || symbol == "UNKNOWN" {
				symbol = "SOL"
			}
			if name == "" || name == "UNKNOWN" {
				name = "Solana"
			}
		}

		if symbol == "" {
			symbol = "UNKNOWN"
		}
		if name == "" {
			name = symbol
		}

		valUSD := qty * priceUSD

		curated.TotalUSD += valUSD
		curated.Assets = append(curated.Assets, AssetEntry{
			Address:      t.Address,
			Network:      t.Network,
			TokenAddress: t.TokenAddress,
			Symbol:       symbol,
			Name:         name,
			Quantity:     qty,
			Decimals:     decimals,
			PriceUSD:     priceUSD,
			ValueUSD:     valUSD,
		})
	}

	// Enrich missing token prices via DexScreener (e.g. JLP, GRASS)
	var needPriceAddrs []string
	for _, a := range curated.Assets {
		if a.PriceUSD == 0 && a.TokenAddress != "" && a.TokenAddress != "null" {
			needPriceAddrs = append(needPriceAddrs, a.TokenAddress)
		}
	}
	needPriceAddrs = append(needPriceAddrs, GrassTokenMint)

	dexPrices := fetchDexScreenerPrices(needPriceAddrs)

	// Apply enriched prices and recalculate totals
	curated.TotalUSD = 0
	for i := range curated.Assets {
		a := &curated.Assets[i]
		if a.PriceUSD == 0 && a.TokenAddress != "" {
			if dp, ok := dexPrices[a.TokenAddress]; ok && dp > 0 {
				a.PriceUSD = dp
				a.ValueUSD = a.Quantity * dp
			}
		}
		curated.TotalUSD += a.ValueUSD
	}

	// Fetch Grass DeFi Staked position on Solana
	stakedGrassQty, err := fetchGrassStaked(apiKey, walletAddress)
	if err == nil && stakedGrassQty > 0 {
		grassPrice := dexPrices[GrassTokenMint]
		if grassPrice == 0 {
			for _, a := range curated.Assets {
				if a.Symbol == "GRASS" && a.PriceUSD > 0 {
					grassPrice = a.PriceUSD
					break
				}
			}
		}
		grassValUSD := stakedGrassQty * grassPrice
		curated.TotalUSD += grassValUSD
		curated.Assets = append(curated.Assets, AssetEntry{
			Address:      walletAddress,
			Network:      "solana-mainnet",
			TokenAddress: GrassTokenMint,
			Symbol:       "GRASS",
			Name:         "Grass (Staked)",
			Quantity:     stakedGrassQty,
			Decimals:     9,
			PriceUSD:     grassPrice,
			ValueUSD:     grassValUSD,
			Category:     "Staked",
			IsStaked:     true,
		})
	}

	holdings := Standardize(curated)
	return curated, holdings, nil
}

// Standardize converts curated Alchemy data into standardized holdings.
func Standardize(data *CuratedAlchemyData) []models.Holding {
	var holdings []models.Holding
	usdThreshold := models.FilterThresholds["USD"]

	for _, a := range data.Assets {
		if a.ValueUSD < usdThreshold {
			continue
		}

		symbol := a.Symbol
		name := a.Name
		if (symbol == "UNKNOWN" || symbol == "") && (a.TokenAddress == "" || a.TokenAddress == "null" || strings.Contains(strings.ToLower(a.Network), "solana")) {
			symbol = "SOL"
			name = "Solana"
		}

		category := a.Category
		if category == "" {
			if symbol == "JLP" {
				category = "Yield / LP"
			} else {
				category = models.GetStandardCategory("Cryptocurrency", symbol)
			}
		}

		account := "Alchemy Wallet"
		details := fmt.Sprintf("Token: %s, Network: %s", name, a.Network)
		if a.IsStaked || category == "Staked" {
			account = "Grass Staking"
			details = "DeFi Staked in Grass Staking Program"
		}

		price := a.PriceUSD
		valUSD := a.ValueUSD

		holdings = append(holdings, models.Holding{
			Source:     "alchemy",
			Category:   category,
			Asset:      symbol,
			Ticker:     symbol,
			Name:       name,
			Currency:   "USD",
			Quantity:   a.Quantity,
			Price:      &price,
			ValueUSD:   &valUSD,
			Account:    account,
			Details:    details,
			AssetClass: models.GetAssetClass(category),
		})
	}

	return holdings
}

// SaveCurated dumps curated Alchemy data to disk.
func SaveCurated(data *CuratedAlchemyData, targetPath string) error {
	_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, b, 0644)
}
