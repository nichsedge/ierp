package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

const scrapingJS = `() => {
    const data = {
      timestamp: new Date().toISOString(),
      wallet: {},
      social: {},
      tokens: [],
      protocols: [],
      nfts: []
    };

    // 1. Overview & Social
    const netWorthEl = document.querySelector("div[class*='HeaderInfo_totalAssetInner'], div[class*='HeaderInfo_totalAsset'], div[class*='HeaderInfo_totalAssetValue']");
    const changeEl = document.querySelector("div[class*='HeaderInfo_changePercent'], [class*='HeaderInfo_isLoss'], [class*='HeaderInfo_isProfit']");

    data.wallet.total_net_worth = netWorthEl ? (netWorthEl.querySelector("[class*='Value']")?.innerText.trim() || netWorthEl.innerText.split('\n')[0].trim()) : null;
    data.wallet.change_24h = changeEl ? changeEl.innerText.trim() : null;

    const rankingEl = document.querySelector("a[href='/ranking'][class*='RankingTag_rankingTag']");
    data.social.ranking = rankingEl ? rankingEl.innerText.trim() : null;

    const infoItems = document.querySelectorAll("div[class*='HeaderInfo_infoItem']");
    infoItems.forEach(item => {
      const text = item.innerText.trim();
      if (text.includes('Followers')) data.social.followers = text.replace('Followers', '').trim();
      else if (text.includes('Following')) data.social.following = text.replace('Following', '').trim();
      else if (text.includes('TVF')) data.social.tvf = text.replace('TVF', '').trim();
    });

    // 2. Wallet Tokens
    const tokenRows = document.querySelectorAll("div[class*='TokenWallet_table'] .db-table-row");
    tokenRows.forEach(row => {
      const symbol = row.querySelector("[class*='TokenWallet_detailLink']")?.innerText.trim();
      const cells = Array.from(row.querySelectorAll(".db-table-cell"));

      const chainLogoImg = row.querySelector("img[class*='TokenWallet_tokenChainIcon']");
      let chain = null;
      if (chainLogoImg && chainLogoImg.src) {
        const urlParts = chainLogoImg.src.split('/');
        const logoUrlIndex = urlParts.indexOf('logo_url');
        if (logoUrlIndex !== -1 && logoUrlIndex + 1 < urlParts.length) {
          chain = urlParts[logoUrlIndex + 1];
        }
      }

      if (symbol && cells.length >= 4) {
        data.tokens.push({
          symbol: symbol,
          chain: chain,
          price: cells[1]?.innerText.trim() || null,
          amount: cells[2]?.innerText.trim() || null,
          value: cells[3]?.innerText.trim() || null
        });
      }
    });

    // 3. Protocols
    const protocolContainers = document.querySelectorAll("div[class*='Project_project__']");
    protocolContainers.forEach(container => {
      const nameEl = container.querySelector("[class*='ProjectTitle_projectTitle'], [class*='ProjectTitle_name'], [class*='Project_projectName']");
      const valueEl = container.querySelector("[class*='projectTitle-number'], [class*='ProjectTitle_number'], [class*='Project_projectValue']");
      
      if (nameEl) {
        let name = nameEl.innerText.trim().split('\n')[0].replace(/\$.*/, '').trim();
        const value = valueEl ? valueEl.innerText.trim() : null;
        
        const protocolData = {
          name: name,
          value: value,
          positions: []
        };
        
        const categories = container.querySelectorAll("div[class*='Panel_container__']");
        categories.forEach(cat => {
          const typeEl = cat.querySelector("div[class*='Panel_panelHead__']");
          const type = typeEl ? typeEl.innerText.trim() : "Other";
          
          const headers = Array.from(cat.querySelectorAll("div[class*='table_header__'] > div")).map(h => h.innerText.trim().toLowerCase());
          const balanceIdx = headers.indexOf('balance');
          const rewardsIdx = headers.indexOf('rewards');
          const usdValueIdx = headers.lastIndexOf('usd value');

          const rows = cat.querySelectorAll("div[class*='table_contentRow__']");
          rows.forEach(row => {
            const cells = Array.from(row.children);
            if (cells.length >= 2) {
              const poolName = cells[0].innerText.trim().replace(/\n/g, ' ');
              const positionValue = usdValueIdx !== -1 && cells[usdValueIdx] ? cells[usdValueIdx].innerText.trim() : cells[cells.length - 1].innerText.trim();
              
              const getCleanedEntries = (cell) => {
                if (!cell) return [];
                const entries = [];
                const tokenLinks = cell.querySelectorAll("a[class*='utils_detailLink__'], a[class*='TokenWallet_detailLink__']");
                
                if (tokenLinks.length === 0) {
                   const text = cell.innerText.trim().replace(/\n/g, ' ');
                   if (text) entries.push({ symbol: null, balance: text });
                } else {
                  tokenLinks.forEach(link => {
                    const symbol = link.innerText.trim();
                    const cellClone = cell.cloneNode(true);
                    cellClone.querySelectorAll('button').forEach(btn => btn.remove());
                    let balanceText = cellClone.innerText.trim().replace(/\n/g, ' ');
                    balanceText = balanceText.replace(/\(\$.*?\)/g, '').trim();
                    entries.push({ symbol: symbol, balance: balanceText });
                  });
                }
                return entries;
              };

              const tokens = [];
              if (balanceIdx !== -1) tokens.push(...getCleanedEntries(cells[balanceIdx]));
              if (rewardsIdx !== -1) tokens.push(...getCleanedEntries(cells[rewardsIdx]));
              
              const uniqueTokens = [];
              const seen = new Set();
              tokens.forEach(t => {
                const key = (t.symbol || '') + '|' + (t.balance || '');
                if (!seen.has(key)) {
                  uniqueTokens.push(t);
                  seen.add(key);
                }
              });

              protocolData.positions.push({
                type: type,
                pool: poolName,
                value: positionValue,
                tokens: uniqueTokens
              });
            }
          });
        });
        
        if (name && name !== 'Wallet' && !data.protocols.find(p => p.name === name)) {
          data.protocols.push(protocolData);
        }
      }
    });

    // Fallback for summary-only items
    const summaryItems = document.querySelectorAll("[class*='ProjectCell_assetsItem'], [class*='ProjectCell_projectCell'], [class*='ProjectCell_assetsItemWrap']");
    summaryItems.forEach(item => {
      const nameEl = item.querySelector("[class*='ProjectCell_assetsItemNameText'], [class*='ProjectCell_name']");
      const valueEl = item.querySelector("[class*='ProjectCell_assetsItemWorth'], [class*='ProjectCell_value']");

      if (nameEl) {
        const name = nameEl.innerText.trim().split('\n')[0].replace(/\$.*/, '').trim();
        const value = valueEl ? valueEl.innerText.trim() : null;

        if (name && name !== 'Wallet' && !data.protocols.find(p => p.name === name)) {
          data.protocols.push({ name: name, value: value, positions: [] });
        }
      }
    });

    return JSON.stringify(data);
}`

const autoScrollJS = `(async () => {
    await new Promise((resolve) => {
        let totalHeight = 0;
        let distance = 400;
        let timer = setInterval(() => {
            let scrollHeight = document.body.scrollHeight;
            window.scrollBy(0, distance);
            totalHeight += distance;
            if (totalHeight >= scrollHeight) {
                clearInterval(timer);
                resolve();
            }
        }, 100);
    });
})()`

const unfoldChainsJS = `(() => {
    const btn = document.querySelector("div[class*='AssetsOnChain_unfoldBtn']");
    if (btn) {
        btn.click();
        return true;
    }
    return false;
})()`

// FindChromePath looks up the Chromium binary in known locations and environment variables.
func FindChromePath() string {
	candidates := []string{
		os.Getenv("CHROME_PATH"),
		os.Getenv("BROWSER_PATH"),
		"/data/data/com.termux/files/usr/bin/chromium",
		"/data/data/com.termux/files/usr/bin/chromium-browser",
		"/root/.cache/ms-playwright/chromium-1234/chrome-linux/chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/google-chrome",
	}
	for _, p := range candidates {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "headless-shell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// CleanAddress trims whitespace, quotes, and normalizes address to lowercase.
func CleanAddress(addr string) string {
	cleaned := strings.TrimSpace(addr)
	cleaned = strings.Trim(cleaned, `"'`)
	return strings.ToLower(cleaned)
}

// Config specifies the scraper runtime options.
type Config struct {
	Headless   bool
	Timeout    time.Duration
	ChromePath string
	UserAgent  string
}

// Scraper coordinates DeBank page fetching and evaluation via chromedp.
type Scraper struct {
	cfg Config
}

// New creates a new DeBank Scraper instance with the provided config.
func New(cfg Config) *Scraper {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	if cfg.ChromePath == "" {
		cfg.ChromePath = FindChromePath()
	}
	return &Scraper{cfg: cfg}
}

// Scrape scrapes DeBank profile data for a given EVM wallet address.
func (s *Scraper) Scrape(ctx context.Context, address string) (*ScrapeResult, error) {
	addr := CleanAddress(address)
	if addr == "" {
		return nil, fmt.Errorf("invalid EVM address: address cannot be empty")
	}

	chromePath := s.cfg.ChromePath
	if chromePath == "" {
		chromePath = FindChromePath()
	}
	if chromePath == "" {
		return nil, fmt.Errorf("Chromium/Chrome executable not found in PATH or environment")
	}

	profileURL := fmt.Sprintf("https://debank.com/profile/%s", addr)
	log.Printf("Starting DeBank scrape for %s at %s", addr, profileURL)

	// Configure flags essential for PRoot Debian on ARM64
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("window-size", "1920,1080"),
		chromedp.UserAgent(s.cfg.UserAgent),
		chromedp.ExecPath(chromePath),
	)

	if s.cfg.Headless {
		allocOpts = append(allocOpts, chromedp.Flag("headless", true))
	} else {
		allocOpts = append(allocOpts, chromedp.Flag("headless", false))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	timeoutCtx, cancelTimeout := context.WithTimeout(taskCtx, s.cfg.Timeout)
	defer cancelTimeout()

	var rawJSON string
	var unfolded bool

	actions := []chromedp.Action{
		chromedp.Navigate(profileURL),
		// Wait for total assets indicator with graceful fallback
		chromedp.ActionFunc(func(c context.Context) error {
			waitCtx, cancelWait := context.WithTimeout(c, 15*time.Second)
			defer cancelWait()
			sel := "div[class*='HeaderInfo_totalAssetInner'], div[class*='HeaderInfo_totalAsset'], div[class*='HeaderInfo_totalAssetValue']"
			if err := chromedp.WaitVisible(sel).Do(waitCtx); err != nil {
				log.Printf("Warning: selector %s not visible within 15s: %v. Continuing...", sel, err)
			}
			return nil
		}),
		// Try unfolding chain breakdown
		chromedp.Evaluate(unfoldChainsJS, &unfolded),
		chromedp.Sleep(1 * time.Second),
		// Trigger smooth scroll to load lazy tokens and protocol cards
		chromedp.Evaluate(autoScrollJS, nil),
		chromedp.Sleep(2 * time.Second),
		// Extract parsed data as JSON string
		chromedp.Evaluate(fmt.Sprintf("(%s)()", scrapingJS), &rawJSON),
	}

	if err := chromedp.Run(timeoutCtx, actions...); err != nil {
		return nil, fmt.Errorf("chromedp run failed: %w", err)
	}

	var result ScrapeResult
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return nil, fmt.Errorf("failed to parse scraped JSON: %w (raw: %s)", err, rawJSON)
	}

	return &result, nil
}

// ScrapeToFile executes the scrape and writes the resulting JSON to the target output path.
func (s *Scraper) ScrapeToFile(ctx context.Context, address string, outputPath string) (*ScrapeResult, error) {
	result, err := s.Scrape(ctx, address)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write output file: %w", err)
	}

	return result, nil
}
