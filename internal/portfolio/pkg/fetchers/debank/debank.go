package debank

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/debank/scraper"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
)

type DebankToken struct {
	Symbol   string      `json:"symbol"`
	Price    interface{} `json:"price"`
	Quantity interface{} `json:"quantity"`
	Amount   interface{} `json:"amount"`
	Value    interface{} `json:"value"`
}

type DebankPosition struct {
	Pool   interface{} `json:"pool"`
	Type   string      `json:"type"`
	Value  interface{} `json:"value"`
	Tokens []struct {
		Balance string `json:"balance"`
	} `json:"tokens"`
}

type DebankProtocol struct {
	Name      string           `json:"name"`
	Chain     string           `json:"chain"`
	Value     interface{}      `json:"value"`
	Positions []DebankPosition `json:"positions"`
}

type DebankNFT struct {
	Collection string      `json:"collection"`
	AvgPrice   interface{} `json:"avg_price"`
	Amount     interface{} `json:"amount"`
}

type RawDebankData struct {
	Timestamp string           `json:"timestamp"`
	Tokens    []DebankToken    `json:"tokens"`
	Protocols []DebankProtocol `json:"protocols"`
	NFTs      []DebankNFT      `json:"nfts"`
}

func parseUSD(val interface{}) float64 {
	if val == nil {
		return 0.0
	}
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		clean := strings.ReplaceAll(v, "$", "")
		clean = strings.ReplaceAll(clean, ",", "")
		clean = strings.ReplaceAll(clean, "USD", "")
		clean = strings.ReplaceAll(clean, "<", "")
		clean = strings.ReplaceAll(clean, ">", "")
		clean = strings.TrimSpace(clean)
		f, _ := strconv.ParseFloat(clean, 64)
		return f
	}
	return 0.0
}

func parseAmount(val interface{}) float64 {
	if val == nil {
		return 0.0
	}
	switch v := val.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		clean := strings.ReplaceAll(v, ",", "")
		clean = strings.ReplaceAll(clean, "<", "")
		clean = strings.ReplaceAll(clean, ">", "")
		clean = strings.TrimSpace(clean)
		f, _ := strconv.ParseFloat(clean, 64)
		return f
	}
	return 0.0
}

// Scrape invokes native chromedp scraper.
func Scrape(address, targetPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s := scraper.New(scraper.Config{Headless: true})
	_, err := s.ScrapeToFile(ctx, address, targetPath)
	return err
}

// LoadRaw loads and parses a raw DeBank JSON file.
func LoadRaw(path string) (*RawDebankData, []models.Holding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var raw RawDebankData
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to decode debank raw json: %w", err)
	}

	holdings := Standardize(&raw)
	return &raw, holdings, nil
}

// Standardize converts RawDebankData into standardized portfolio holdings.
func Standardize(data *RawDebankData) []models.Holding {
	if data == nil {
		return nil
	}

	var holdings []models.Holding
	usdThreshold := models.FilterThresholds["USD"]

	// 1. Process Tokens with aggregation by symbol
	type aggToken struct {
		symbol   string
		quantity float64
		price    float64
		valueUSD float64
	}
	tokenMap := make(map[string]*aggToken)

	for _, t := range data.Tokens {
		valUSD := parseUSD(t.Value)
		if valUSD < usdThreshold {
			continue
		}

		qty := parseAmount(t.Quantity)
		if qty == 0 {
			qty = parseAmount(t.Amount)
		}
		price := parseUSD(t.Price)
		if price == 0 && qty > 0 && valUSD > 0 {
			price = valUSD / qty
		}

		sym := strings.TrimSpace(t.Symbol)
		if sym == "" {
			sym = "Unknown"
		}

		if existing, ok := tokenMap[sym]; ok {
			existing.quantity += qty
			existing.valueUSD += valUSD
			if existing.quantity > 0 {
				existing.price = existing.valueUSD / existing.quantity
			}
		} else {
			tokenMap[sym] = &aggToken{
				symbol:   sym,
				quantity: qty,
				price:    price,
				valueUSD: valUSD,
			}
		}
	}

	for _, agg := range tokenMap {
		cat := models.GetStandardCategory("Cryptocurrency", agg.symbol)
		p := agg.price
		vUSD := agg.valueUSD
		holdings = append(holdings, models.Holding{
			Source:     "debank",
			Category:   cat,
			Asset:      agg.symbol,
			Ticker:     agg.symbol,
			Name:       agg.symbol,
			Currency:   "USD",
			Quantity:   agg.quantity,
			Price:      &p,
			ValueUSD:   &vUSD,
			Account:    "DeBank Wallet",
			Details:    "",
			AssetClass: models.GetAssetClass(cat),
		})
	}

	// 2. Process Protocols
	for _, proto := range data.Protocols {
		pName := strings.TrimSpace(proto.Name)
		if pName == "" {
			pName = "Unknown"
		}

		if len(proto.Positions) > 0 {
			for _, pos := range proto.Positions {
				valUSD := parseUSD(pos.Value)
				if valUSD < usdThreshold {
					continue
				}

				poolStr := "Unknown Pool"
				if s, ok := pos.Pool.(string); ok && strings.TrimSpace(s) != "" {
					poolStr = strings.TrimSpace(s)
				}

				posType := strings.TrimSpace(pos.Type)
				if posType == "" {
					posType = "Protocol"
				}

				cat := "Yield / LP"
				posTypeLower := strings.ToLower(posType)
				if strings.Contains(posTypeLower, "borrow") || strings.Contains(posTypeLower, "debt") {
					cat = "Liabilities"
				} else if strings.Contains(posTypeLower, "staked") {
					cat = "Staked"
				}

				assetName := fmt.Sprintf("%s - %s", pName, poolStr)
				if poolStr == "Unknown Pool" {
					if posType != "Protocol" && posType != "Other" {
						assetName = fmt.Sprintf("%s (%s)", pName, posType)
					} else {
						assetName = pName
					}
				} else if posType != "" && posType != "Other" && posType != "Protocol" {
					assetName = fmt.Sprintf("%s (%s) - %s", pName, posType, poolStr)
				}
				assetName = strings.ReplaceAll(assetName, "\n", " ")

				var tokenStrs []string
				for _, tok := range pos.Tokens {
					if tok.Balance != "" {
						tokenStrs = append(tokenStrs, strings.TrimSpace(tok.Balance))
					}
				}

				p := valUSD
				vUSD := valUSD
				holdings = append(holdings, models.Holding{
					Source:     "debank",
					Category:   cat,
					Asset:      assetName,
					Name:       assetName,
					Currency:   "USD",
					Quantity:   1.0,
					Price:      &p,
					ValueUSD:   &vUSD,
					Account:    "DeBank Protocol",
					Details:    fmt.Sprintf("Protocol: %s, Position: %s, Tokens: %s", pName, posType, strings.Join(tokenStrs, ", ")),
					AssetClass: models.GetAssetClass(cat),
				})
			}
		} else {
			valUSD := parseUSD(proto.Value)
			if valUSD >= usdThreshold {
				p := valUSD
				vUSD := valUSD
				cat := "Yield / LP"
				holdings = append(holdings, models.Holding{
					Source:     "debank",
					Category:   cat,
					Asset:      pName,
					Name:       pName,
					Currency:   "USD",
					Quantity:   1.0,
					Price:      &p,
					ValueUSD:   &vUSD,
					Account:    "DeBank Protocol",
					Details:    fmt.Sprintf("Protocol: %s (Summary)", pName),
					AssetClass: models.GetAssetClass(cat),
				})
			}
		}
	}

	// 3. Process NFTs
	for _, nft := range data.NFTs {
		avgPrice := parseUSD(nft.AvgPrice)
		amount := parseAmount(nft.Amount)
		if amount == 0 {
			amount = 1.0
		}
		valUSD := avgPrice * amount
		if valUSD >= usdThreshold {
			p := avgPrice
			vUSD := valUSD
			cat := "Spot"
			holdings = append(holdings, models.Holding{
				Source:     "debank",
				Category:   cat,
				Asset:      nft.Collection,
				Name:       nft.Collection,
				Currency:   "USD",
				Quantity:   amount,
				Price:      &p,
				ValueUSD:   &vUSD,
				Account:    "DeBank NFT",
				Details:    fmt.Sprintf("Collection: %s", nft.Collection),
				AssetClass: models.GetAssetClass(cat),
			})
		}
	}

	return holdings
}
