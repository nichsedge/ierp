package portfolio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/advisor"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/aistate"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/db"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/enricher"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/alchemy"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/binance"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/hyperliquid"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/ksei"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/sansfinance"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fx"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/integrator"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/mcp"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/notify"
	"github.com/nichsedge/ierp/internal/r2"
)

func loadEnv() {
	_ = godotenv.Load()
	home, err := os.UserHomeDir()
	if err == nil {
		_ = godotenv.Overload(filepath.Join(home, ".secrets"))
		_ = godotenv.Load(filepath.Join(home, ".env"))
	}
	dir, err := os.Getwd()
	if err == nil {
		for i := 0; i < 5; i++ {
			envPath := filepath.Join(dir, ".env")
			if _, err := os.Stat(envPath); err == nil {
				_ = godotenv.Load(envPath)
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func getDataDir() string {
	d := os.Getenv("PORTFOLIO_DATA_DIR")
	if d == "" {
		d = os.Getenv("DATA_DIR")
	}
	if d != "" {
		home, err := os.UserHomeDir()
		if err == nil && strings.HasPrefix(d, "/home/al") && !dirExists(d) {
			d = filepath.Join(home, strings.TrimPrefix(d, "/home/al"))
		}
		if dirExists(d) {
			return d
		}
	}
	// Fallback to ~/Projects/ierp/data/portfolio
	home, err := os.UserHomeDir()
	if err == nil {
		target := filepath.Join(home, "Projects", "ierp", "data", "portfolio")
		if dirExists(target) {
			return target
		}
	}
	// Fallback to local data/
	dir, err := os.Getwd()
	if err == nil {
		localPortfolio := filepath.Join(dir, "data", "portfolio")
		if dirExists(localPortfolio) {
			return localPortfolio
		}
		localData := filepath.Join(dir, "data")
		if dirExists(localData) {
			return localData
		}
	}
	return "./data"
}

// Run executes the integrated portfolio command suite with injectable I/O streams.
func Run(ctx context.Context, cliArgs []string, stdout, stderr io.Writer) error {
	loadEnv()

	if len(cliArgs) < 1 {
		printHelp(stdout)
		return nil
	}

	command := cliArgs[0]
	args := cliArgs[1:]

	switch command {
	case "run-all", "run":
		return runAll(ctx, args, stdout, stderr)
	case "fetch":
		return runFetch(args, stdout)
	case "integrate":
		return runIntegrate(args, stdout, stderr)
	case "ai-state":
		return runAIState(args, stdout)
	case "advisor":
		return runAdvisor(args, stdout, stderr)
	case "mcp":
		return runMCP(args, stderr)
	case "r2":
		return runR2(ctx, args, stdout)
	case "help", "--help", "-h":
		printHelp(stdout)
		return nil
	default:
		fmt.Fprintf(stderr, "Unknown portfolio command: %s\n\n", command)
		printHelp(stderr)
		return fmt.Errorf("unknown portfolio command: %s", command)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "🚀 iERP Portfolio Engine")
	fmt.Fprintln(w, "\nUsage:")
	fmt.Fprintln(w, "  ierp portfolio run             Run full multi-asset pipeline (fetch, integrate, ai-state, r2)")
	fmt.Fprintln(w, "  ierp portfolio fetch <src>     Fetch single source (ksei, binance, alchemy, hyperliquid, sansfinance)")
	fmt.Fprintln(w, "  ierp portfolio integrate       Integrate raw data, enrich yields, persist snapshot & SQLite")
	fmt.Fprintln(w, "  ierp portfolio ai-state        Generate latest_ai_state.json & latest_ai_digest.md")
	fmt.Fprintln(w, "  ierp portfolio advisor         Quantitative sovereign runway matrix & rebalance directives")
	fmt.Fprintln(w, "  ierp portfolio mcp             Run stdio Model Context Protocol (MCP) server for AI agents")
	fmt.Fprintln(w, "  ierp portfolio r2 <action>     R2 cloud operations (push, status)")
}

func runAll(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("run-all", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dateFlag := fs.String("date", time.Now().Format("2006-01-02"), "Date in YYYY-MM-DD")
	noUpload := fs.Bool("no-upload", false, "Skip Cloudflare R2 backup")
	monthlyWeekday := fs.Bool("monthly-weekday", false, "Only run on the 25th if weekday, or the following Monday if 25th falls on a weekend")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *monthlyWeekday && !isMonthlyWeekdayRun(time.Now()) {
		fmt.Fprintln(stdout, "ℹ️ Skipping: outside 25th-or-next-weekday run window.")
		return nil
	}

	date := *dateFlag
	dataDir, _ := filepath.Abs(getDataDir())
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data directory %s: %w", dataDir, err)
	}

	fmt.Fprintf(stdout, "🚀 Running full portfolio pipeline for %s\n", date)
	fmt.Fprintf(stdout, "📁 Data directory: %s\n\n", dataDir)

	var allHoldings []models.Holding

	// 1. Fetch KSEI
	fmt.Print("⏳ [1/5] Fetching KSEI... ")
	kseiUser := os.Getenv("KSEI_USERNAME")
	if kseiUser == "" {
		kseiUser = os.Getenv("GOKSEI_USERNAME")
	}
	kseiPass := os.Getenv("KSEI_PASSWORD")
	if kseiPass == "" {
		kseiPass = os.Getenv("GOKSEI_PASSWORD")
	}
	plainPass := os.Getenv("KSEI_PLAIN_PASSWORD") != "false" && os.Getenv("GOKSEI_PLAIN_PASSWORD") != "false"

	rawKseiPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_ksei.json", date))
	if kseiUser != "" && kseiPass != "" {
		rawKsei, kseiHoldings, err := ksei.Fetch(kseiUser, kseiPass, plainPass)
		if err == nil {
			if rawKsei.IsDegraded() {
				if prev := findLatestValidKseiCache(dataDir, date); prev != nil {
					if rawKsei.FillMissingFromPrevious(prev) {
						kseiHoldings = ksei.Standardize(rawKsei)
						fmt.Printf("⚠️ (KSEI off-hours: preserved %d equities & %d bonds from cache) ", len(prev.Equity.Data), len(prev.Bond.Data))
					}
				}
			}
			_ = ksei.SaveRaw(rawKsei, rawKseiPath)
			allHoldings = append(allHoldings, kseiHoldings...)
			fmt.Printf("✓ %d holdings\n", len(kseiHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawKseiPath, date, dataDir, "ksei", ksei.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no credentials) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawKseiPath, date, dataDir, "ksei", ksei.Standardize)...)
	}

	// 2. Fetch SansFinance
	fmt.Print("⏳ [2/5] Fetching SansFinance... ")
	r2Acc := os.Getenv("R2_ACCOUNT_ID")
	r2Key := os.Getenv("R2_ACCESS_KEY_ID")
	r2Sec := os.Getenv("R2_SECRET_ACCESS_KEY")
	r2Bucket := os.Getenv("R2_BUCKET_NAME")
	if r2Bucket == "" {
		r2Bucket = "ichsanul-dev"
	}

	rawSansPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_sansfinance.json", date))
	if r2Acc != "" && r2Key != "" && r2Sec != "" {
		rawSans, sansHoldings, err := sansfinance.Fetch(r2Acc, r2Key, r2Sec, r2Bucket)
		if err == nil {
			_ = sansfinance.SaveRaw(rawSans, rawSansPath)
			allHoldings = append(allHoldings, sansHoldings...)
			fmt.Printf("✓ %d accounts\n", len(sansHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawSansPath, date, dataDir, "sansfinance", sansfinance.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no R2 creds) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawSansPath, date, dataDir, "sansfinance", sansfinance.Standardize)...)
	}

	// 3. Fetch Binance
	fmt.Print("⏳ [3/5] Fetching Binance... ")
	binKey := os.Getenv("BINANCE_API_KEY")
	if binKey == "" {
		binKey = os.Getenv("BINANCE_KEY")
	}
	binSec := os.Getenv("BINANCE_API_SECRET")
	if binSec == "" {
		binSec = os.Getenv("BINANCE_SECRET")
	}
	rawBinPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_binance.json", date))
	if binKey != "" && binSec != "" {
		rawBin, binHoldings, err := binance.Fetch(binKey, binSec)
		if err == nil {
			_ = binance.SaveRaw(rawBin, rawBinPath)
			allHoldings = append(allHoldings, binHoldings...)
			fmt.Printf("✓ %d balances\n", len(binHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawBinPath, date, dataDir, "binance", binance.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no credentials) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawBinPath, date, dataDir, "binance", binance.Standardize)...)
	}

	// 4. Fetch Alchemy (Solana + EVM)
	fmt.Print("⏳ [4/5] Fetching Alchemy (Solana + EVM)... ")
	solAddr := os.Getenv("SOL_ADDRESS")
	ethAddr := os.Getenv("ETH_ADDRESS")
	alchKey := os.Getenv("ALCHEMY_API_KEY")
	rawAlchPath := filepath.Join(dataDir, fmt.Sprintf("%s_curated_alchemy.json", date))
	if (solAddr != "" || ethAddr != "") && alchKey != "" {
		rawAlch, alchHoldings, err := alchemy.Fetch(alchKey, solAddr, ethAddr)
		if err == nil {
			_ = alchemy.SaveCurated(rawAlch, rawAlchPath)
			allHoldings = append(allHoldings, alchHoldings...)
			fmt.Printf("✓ %d tokens\n", len(alchHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawAlchPath, date, dataDir, "alchemy", alchemy.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no Alchemy credentials) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawAlchPath, date, dataDir, "alchemy", alchemy.Standardize)...)
	}

	// 5. Fetch Hyperliquid
	fmt.Print("⏳ [5/5] Fetching Hyperliquid (EVM DeFi)... ")
	rawHlPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_hyperliquid.json", date))
	if ethAddr != "" {
		rawHl, hlHoldings, err := hyperliquid.Fetch(ethAddr)
		if err == nil {
			_ = hyperliquid.SaveRaw(rawHl, rawHlPath)
			allHoldings = append(allHoldings, hlHoldings...)
			fmt.Printf("✓ %d positions\n", len(hlHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawHlPath, date, dataDir, "hyperliquid", hyperliquid.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no ETH_ADDRESS) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawHlPath, date, dataDir, "hyperliquid", hyperliquid.Standardize)...)
	}

	// Step 6: Yield Enrichment & Integration
	fmt.Println("\n--- Integrating Portfolio ---")
	allHoldings = enricher.EnrichHoldings(allHoldings)

	rate := fx.GetExchangeRate(dataDir)
	fmt.Printf("💵 Exchange Rate: 1 USD = Rp %.2f\n", rate)

	dbPath := filepath.Join(dataDir, "portfolio.db")
	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		fmt.Printf("⚠️ Could not open SQLite DB (%v)\n", err)
	} else {
		defer dbConn.Close()
	}

	snap, err := integrator.Integrate(date, allHoldings, rate, dataDir, dbConn)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Integration failed: %v\n", err)
		return err
	}

	printSummary(snap)

	// Step 7: AI Financial State & Sovereign Runway Generator
	fmt.Fprint(stdout, "🧠 Generating token-optimized AI State & Sovereign Digest... ")
	aiStateObj, err := aistate.GenerateAIState(snap, dbConn)
	if err == nil {
		digestMD := aistate.GenerateAIDigestMarkdown(aiStateObj)
		if saveErr := aistate.SaveAIState(aiStateObj, digestMD, dataDir, dbConn); saveErr != nil {
			fmt.Fprintf(stderr, "⚠️ Failed to save AI state: %v\n", saveErr)
		}

		advPayload := advisor.GenerateAdvisorPayload(snap, aiStateObj, dbConn)
		if saveErr := advisor.SaveAdvisorPayload(advPayload, dataDir); saveErr != nil {
			fmt.Fprintf(stderr, "⚠️ Failed to save advisor payload: %v\n", saveErr)
		}
		fmt.Fprintln(stdout, "✓ Generated latest_ai_state.json & latest_ai_digest.md")
	} else {
		fmt.Fprintf(stdout, "⚠️ (%v)\n", err)
	}

	// Step 8: Cloud Upload
	if !*noUpload && r2Acc != "" && r2Key != "" && r2Sec != "" {
		fmt.Fprintln(stdout, "☁️ Uploading portfolio snapshot, AI state, and DB to Cloudflare R2...")
		creds := r2.Credentials{
			AccountID:       r2Acc,
			AccessKeyID:     r2Key,
			SecretAccessKey: r2Sec,
			BucketName:      r2Bucket,
		}
		if err := uploadPortfolioArtifacts(ctx, creds, dataDir, date, dbPath); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "✓ Cloudflare R2 upload complete.")
	}

	// Step 9: Telegram Notification
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	tgChatID := os.Getenv("TELEGRAM_CHAT_ID")
	if tgToken != "" && tgChatID != "" {
		msg := fmt.Sprintf(
			"📊 <b>iERP Portfolio Snapshot (%s)</b>\n"+
				"💰 Net Worth: <b>Rp %s</b> ($%.2f)\n"+
				"📈 Investments: Rp %s ($%.2f)\n"+
				"🏦 Liquid Cash: Rp %s ($%.2f)\n"+
				"📦 Holdings: %d items",
			snap.Metadata.Date,
			formatIDR(snap.Totals.NetWorthIDR), snap.Totals.NetWorthUSD,
			formatIDR(snap.Totals.InvestmentsIDR), snap.Totals.InvestmentsUSD,
			formatIDR(snap.Totals.BankCashIDR), snap.Totals.BankCashUSD,
			len(snap.Holdings),
		)
		if err := notify.SendTelegram(tgToken, tgChatID, msg); err != nil {
			fmt.Fprintf(stderr, "⚠️ Telegram notification failed: %v\n", err)
		} else {
			fmt.Fprintln(stdout, "✓ Telegram notification sent.")
		}
	}

	fmt.Fprintln(stdout, "\n✨ Pipeline completed successfully!")
	return nil
}

func isMonthlyWeekdayRun(now time.Time) bool {
	wib := time.FixedZone("WIB", 7*3600)
	t := now.In(wib)
	day := t.Day()
	wd := t.Weekday()
	if day == 25 && wd >= time.Monday && wd <= time.Friday {
		return true
	}
	if (day == 26 || day == 27) && wd == time.Monday {
		return true
	}
	return false
}

func findLatestValidKseiCache(dataDir, today string) *ksei.RawKseiData {
	pattern := filepath.Join(dataDir, "*_raw_ksei.json")
	matches, _ := filepath.Glob(pattern)
	var latestMatch string
	for _, m := range matches {
		base := filepath.Base(m)
		if !strings.HasPrefix(base, today) && !strings.HasPrefix(base, "latest") {
			raw, err := os.ReadFile(m)
			if err != nil {
				continue
			}
			var d ksei.RawKseiData
			if err := json.Unmarshal(raw, &d); err != nil {
				continue
			}
			if !d.IsDegraded() {
				if m > latestMatch {
					latestMatch = m
				}
			}
		}
	}
	if latestMatch != "" {
		raw, err := os.ReadFile(latestMatch)
		if err == nil {
			var d ksei.RawKseiData
			if err := json.Unmarshal(raw, &d); err == nil {
				return &d
			}
		}
	}
	return nil
}

func tryFallbackCache(targetFile, today, dataDir, source string) {
	if fi, err := os.Stat(targetFile); err == nil && fi.Size() > 0 {
		if source == "ksei" {
			raw, err := os.ReadFile(targetFile)
			if err == nil {
				var d ksei.RawKseiData
				if err := json.Unmarshal(raw, &d); err == nil && !d.IsDegraded() {
					return
				}
			}
		} else {
			return
		}
	}
	if source == "ksei" {
		if valid := findLatestValidKseiCache(dataDir, today); valid != nil {
			_ = ksei.SaveRaw(valid, targetFile)
			fmt.Printf("  ↳ Reused valid non-degraded KSEI cache\n")
			return
		}
	}
	pattern := filepath.Join(dataDir, fmt.Sprintf("*_raw_%s.json", source))
	if source == "alchemy" {
		pattern = filepath.Join(dataDir, fmt.Sprintf("*_curated_%s.json", source))
	}
	matches, _ := filepath.Glob(pattern)
	var latestMatch string
	for _, m := range matches {
		base := filepath.Base(m)
		if !strings.HasPrefix(base, today) && !strings.HasPrefix(base, "latest") {
			if m > latestMatch {
				latestMatch = m
			}
		}
	}
	if latestMatch != "" {
		content, err := os.ReadFile(latestMatch)
		if err == nil {
			_ = os.WriteFile(targetFile, content, 0644)
			fmt.Printf("  ↳ Reused cache from %s\n", filepath.Base(latestMatch))
			return
		}
	}
	fmt.Println("  ↳ No previous cache found")
}

func loadCachedHoldings[T any](targetPath, date, dataDir, source string, standardize func(*T) []models.Holding) []models.Holding {
	tryFallbackCache(targetPath, date, dataDir, source)
	raw, err := os.ReadFile(targetPath)
	if err != nil {
		return nil
	}
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}
	h := standardize(&data)
	if len(h) > 0 {
		fmt.Printf("✓ loaded %d cached holdings\n", len(h))
	}
	return h
}

func printSummary(snap *models.Snapshot) {
	fmt.Println("\n=======================================================")
	fmt.Printf("📊 PORTFOLIO SNAPSHOT: %s\n", snap.Metadata.Date)
	fmt.Printf("💰 Net Worth:    Rp %s ($%.2f)\n", formatIDR(snap.Totals.NetWorthIDR), snap.Totals.NetWorthUSD)
	fmt.Printf("   Assets:       Rp %s\n", formatIDR(snap.Totals.TotalAssetsIDR))
	if snap.Totals.TotalLiabilitiesIDR > 0 {
		fmt.Printf("   Liabilities:  Rp %s\n", formatIDR(snap.Totals.TotalLiabilitiesIDR))
	}
	fmt.Printf("   Investments:  Rp %s ($%.2f)\n", formatIDR(snap.Totals.InvestmentsIDR), snap.Totals.InvestmentsUSD)
	fmt.Printf("   Liquid Cash:  Rp %s ($%.2f)\n", formatIDR(snap.Totals.BankCashIDR), snap.Totals.BankCashUSD)
	fmt.Println("-------------------------------------------------------")
	fmt.Println("Asset Allocation:")
	for _, ac := range snap.Allocation.ByAssetClass {
		fmt.Printf("  %-20s  Rp %-14s  (%5.1f%%)  [%d]\n", ac.AssetClass, formatIDR(ac.ValueIDR), ac.Percentage, ac.Count)
	}
	fmt.Println("=======================================================")
}

func formatIDR(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if len(s) > 0 {
		parts = append([]string{s}, parts...)
	}
	return strings.Join(parts, ",")
}

func runFetch(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("please specify source: ksei, binance, alchemy, hyperliquid, sansfinance")
	}
	dataDir, _ := filepath.Abs(getDataDir())
	date := time.Now().Format("2006-01-02")

	switch args[0] {
	case "ksei":
		user := os.Getenv("KSEI_USERNAME")
		pass := os.Getenv("KSEI_PASSWORD")
		raw, _, err := ksei.Fetch(user, pass, true)
		if err != nil {
			return err
		}
		if raw.IsDegraded() {
			if prev := findLatestValidKseiCache(dataDir, date); prev != nil {
				if raw.FillMissingFromPrevious(prev) {
					fmt.Fprintf(stdout, "⚠️ KSEI returned 0 equities & bonds (off-hours). Preserved %d equities & %d bonds from cache.\n", len(prev.Equity.Data), len(prev.Bond.Data))
				}
			}
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_ksei.json", date))
		if err := ksei.SaveRaw(raw, target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Saved to %s\n", target)
	case "sansfinance":
		r2Acc := os.Getenv("R2_ACCOUNT_ID")
		r2Key := os.Getenv("R2_ACCESS_KEY_ID")
		r2Sec := os.Getenv("R2_SECRET_ACCESS_KEY")
		raw, _, err := sansfinance.Fetch(r2Acc, r2Key, r2Sec, "ichsanul-dev")
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_sansfinance.json", date))
		if err := sansfinance.SaveRaw(raw, target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Saved to %s\n", target)
	case "binance":
		binKey := os.Getenv("BINANCE_API_KEY")
		if binKey == "" {
			binKey = os.Getenv("BINANCE_KEY")
		}
		binSec := os.Getenv("BINANCE_API_SECRET")
		if binSec == "" {
			binSec = os.Getenv("BINANCE_SECRET")
		}
		raw, holdings, err := binance.Fetch(binKey, binSec)
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_binance.json", date))
		if err := binance.SaveRaw(raw, target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Saved %d balances to %s\n", len(holdings), target)
	case "alchemy":
		solAddr := os.Getenv("SOL_ADDRESS")
		ethAddr := os.Getenv("ETH_ADDRESS")
		alchKey := os.Getenv("ALCHEMY_API_KEY")
		raw, holdings, err := alchemy.Fetch(alchKey, solAddr, ethAddr)
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_curated_alchemy.json", date))
		if err := alchemy.SaveCurated(raw, target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Saved %d tokens to %s\n", len(holdings), target)
	case "hyperliquid":
		ethAddr := os.Getenv("ETH_ADDRESS")
		raw, holdings, err := hyperliquid.Fetch(ethAddr)
		if err != nil {
			return err
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_hyperliquid.json", date))
		if err := hyperliquid.SaveRaw(raw, target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ Saved %d positions to %s\n", len(holdings), target)
	default:
		return fmt.Errorf("fetch for %s not implemented or run via run-all", args[0])
	}
	return nil
}

func runIntegrate(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("integrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dateFlag := fs.String("date", time.Now().Format("2006-01-02"), "Date in YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}

	date := *dateFlag
	dataDir, _ := filepath.Abs(getDataDir())
	rate := fx.GetExchangeRate(dataDir)

	var allHoldings []models.Holding

	kseiPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_ksei.json", date))
	var kseiData *ksei.RawKseiData
	if raw, err := os.ReadFile(kseiPath); err == nil {
		var d ksei.RawKseiData
		if json.Unmarshal(raw, &d) == nil {
			kseiData = &d
		}
	}
	if kseiData == nil || kseiData.IsDegraded() {
		if prev := findLatestValidKseiCache(dataDir, date); prev != nil {
			if kseiData == nil {
				kseiData = prev
			} else {
				kseiData.FillMissingFromPrevious(prev)
			}
		}
	}
	if kseiData != nil {
		allHoldings = append(allHoldings, ksei.Standardize(kseiData)...)
	}

	hlPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_hyperliquid.json", date))
	if raw, err := os.ReadFile(hlPath); err == nil {
		var d hyperliquid.RawHyperliquidData
		if json.Unmarshal(raw, &d) == nil {
			allHoldings = append(allHoldings, hyperliquid.Standardize(&d)...)
		}
	}

	binPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_binance.json", date))
	if raw, err := os.ReadFile(binPath); err == nil {
		var d binance.RawBinanceData
		if json.Unmarshal(raw, &d) == nil {
			allHoldings = append(allHoldings, binance.Standardize(&d)...)
		}
	}

	alchPath := filepath.Join(dataDir, fmt.Sprintf("%s_curated_alchemy.json", date))
	if raw, err := os.ReadFile(alchPath); err == nil {
		var d alchemy.CuratedAlchemyData
		if json.Unmarshal(raw, &d) == nil {
			allHoldings = append(allHoldings, alchemy.Standardize(&d)...)
		}
	}

	sansPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_sansfinance.json", date))
	if raw, err := os.ReadFile(sansPath); err == nil {
		var d sansfinance.RawSansfinanceData
		if json.Unmarshal(raw, &d) == nil {
			allHoldings = append(allHoldings, sansfinance.Standardize(&d)...)
		}
	}

	allHoldings = enricher.EnrichHoldings(allHoldings)

	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	snap, err := integrator.Integrate(date, allHoldings, rate, dataDir, dbConn)
	if err != nil {
		return fmt.Errorf("integration failed: %w", err)
	}

	printSummary(snap)

	// Generate AI state & digest
	state, err := aistate.GenerateAIState(snap, dbConn)
	if err == nil {
		digestMD := aistate.GenerateAIDigestMarkdown(state)
		if saveErr := aistate.SaveAIState(state, digestMD, dataDir, dbConn); saveErr != nil {
			fmt.Fprintf(stderr, "⚠️ Failed to save AI state: %v\n", saveErr)
		}
		advPayload := advisor.GenerateAdvisorPayload(snap, state, dbConn)
		if saveErr := advisor.SaveAdvisorPayload(advPayload, dataDir); saveErr != nil {
			fmt.Fprintf(stderr, "⚠️ Failed to save advisor payload: %v\n", saveErr)
		}
		fmt.Fprintln(stdout, "✓ Generated latest_ai_state.json & latest_ai_digest.md")
	}
	return nil
}

func runAIState(args []string, stdout io.Writer) error {
	dataDir, _ := filepath.Abs(getDataDir())
	latestPath := filepath.Join(dataDir, "latest_snapshot.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return fmt.Errorf("read latest_snapshot.json: %w", err)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse latest_snapshot.json: %w", err)
	}

	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	state, err := aistate.GenerateAIState(&snap, dbConn)
	if err != nil {
		return fmt.Errorf("generate AI state: %w", err)
	}
	digestMD := aistate.GenerateAIDigestMarkdown(state)
	if err := aistate.SaveAIState(state, digestMD, dataDir, dbConn); err != nil {
		return fmt.Errorf("save AI state: %w", err)
	}

	advPayload := advisor.GenerateAdvisorPayload(&snap, state, dbConn)
	if err := advisor.SaveAdvisorPayload(advPayload, dataDir); err != nil {
		return fmt.Errorf("save advisor payload: %w", err)
	}

	fmt.Fprintln(stdout, "✨ Successfully refreshed latest_ai_state.json & latest_ai_digest.md")
	fmt.Fprintf(stdout, "   Net Worth: Rp %s ($%.2f)\n", formatIDR(state.MacroMetrics.NetWorthIDR), state.MacroMetrics.NetWorthUSD)
	fmt.Fprintf(stdout, "   Passive Cashflow: Rp %s/mo (FI Coverage: %.1f%%)\n",
		formatIDR(state.PassiveIncome.ProjectedMonthlyPassiveIncomeIDR), state.PassiveIncome.FICoveragePct)
	fmt.Fprintf(stdout, "   ADHD 1-Step Directive: %s\n", state.ADHDFocusMetrics.DepositRouter.OneStepAction)
	return nil
}

func runAdvisor(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("advisor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mdFlag := fs.Bool("markdown", false, "Output non-ANSI Markdown briefing")
	jsonFlag := fs.Bool("json", false, "Output structured JSON payload")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dataDir, _ := filepath.Abs(getDataDir())
	latestPath := filepath.Join(dataDir, "latest_snapshot.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return fmt.Errorf("read latest_snapshot.json: %w", err)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("parse latest_snapshot.json: %w", err)
	}

	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	state, _ := aistate.GenerateAIState(&snap, dbConn)
	payload := advisor.GenerateAdvisorPayload(&snap, state, dbConn)

	if *jsonFlag {
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}

	if *mdFlag {
		fmt.Fprintln(stdout, advisor.GenerateMarkdownBriefing(payload))
		return nil
	}

	// Default terminal report
	fmt.Fprintln(stdout, "\n🏛️  SOVEREIGN RUNWAY & MACRO ADVISOR")
	fmt.Fprintln(stdout, "=======================================================")
	fmt.Fprintf(stdout, "Date:             %s\n", payload.Date)
	fmt.Fprintf(stdout, "Net Worth:        Rp %s\n", formatIDR(payload.NetWorthIDR))
	fmt.Fprintf(stdout, "Liquid Reserves:  Rp %s (%.1f%% dry powder)\n", formatIDR(payload.DryPowderIDR), payload.DryPowderPct)
	fmt.Fprintf(stdout, "Runway Horizon:   %.1f Months [%s]\n", payload.RunwayMonths, payload.RunwayStatus)
	fmt.Fprintln(stdout, "-------------------------------------------------------")
	fmt.Fprintf(stdout, "Directive: %s\n", payload.ActionSummary)
	fmt.Fprintln(stdout, "-------------------------------------------------------")
	fmt.Fprintln(stdout, "3-Tier Sovereign Runway Matrix:")
	p := payload.SovereignAllocationPlan
	fmt.Fprintf(stdout, "  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier1OperatingReserve.Name, formatIDR(p.Tier1OperatingReserve.AllocatedIDR), p.Tier1OperatingReserve.AllocationPct)
	fmt.Fprintf(stdout, "  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier2FortressBuffer.Name, formatIDR(p.Tier2FortressBuffer.AllocatedIDR), p.Tier2FortressBuffer.AllocationPct)
	fmt.Fprintf(stdout, "  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier3DeployableSurplus.Name, formatIDR(p.Tier3DeployableSurplus.AllocatedIDR), p.Tier3DeployableSurplus.AllocationPct)
	fmt.Fprintln(stdout, "=======================================================")
	return nil
}

func runMCP(args []string, stderr io.Writer) error {
	dataDir, _ := filepath.Abs(getDataDir())
	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	srv := mcp.NewServer(dataDir, dbConn)
	if err := srv.Run(); err != nil {
		fmt.Fprintf(stderr, "MCP server error: %v\n", err)
		return err
	}
	return nil
}

func uploadPortfolioArtifacts(ctx context.Context, creds r2.Credentials, dataDir, date, dbPath string) error {
	uploads := []struct {
		localPath   string
		key         string
		contentType string
		optional    bool
	}{
		{filepath.Join(dataDir, fmt.Sprintf("%s_snapshot.json", date)), fmt.Sprintf("portfolio/%s_snapshot.json", date), "application/json", true},
		{filepath.Join(dataDir, "latest_snapshot.json"), "portfolio/latest_snapshot.json", "application/json", true},
		{filepath.Join(dataDir, "latest_ai_state.json"), "portfolio/latest_ai_state.json", "application/json", true},
		{filepath.Join(dataDir, "latest_ai_digest.md"), "portfolio/latest_ai_digest.md", "text/markdown", true},
		{filepath.Join(dataDir, "latest_advisor.json"), "portfolio/latest_advisor.json", "application/json", true},
		{dbPath, "db/portfolio_latest.sqlite", "application/x-sqlite3", true},
	}

	for _, u := range uploads {
		if _, err := os.Stat(u.localPath); err != nil {
			if u.optional && os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat %s: %w", u.localPath, err)
		}
		if err := r2.UploadFile(ctx, creds, u.localPath, u.key, u.contentType); err != nil {
			return err
		}
	}
	return nil
}

func runR2(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintln(stdout, "Usage: portfolio r2 [push|status]")
		return nil
	}
	switch args[0] {
	case "push":
		dataDir, _ := filepath.Abs(getDataDir())
		dbPath := filepath.Join(dataDir, "portfolio.db")
		r2Bucket := os.Getenv("R2_BUCKET_NAME")
		if r2Bucket == "" {
			r2Bucket = r2.DefaultBucket
		}
		creds := r2.Credentials{
			AccountID:       os.Getenv("R2_ACCOUNT_ID"),
			AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
			BucketName:      r2Bucket,
		}
		date := time.Now().Format("2006-01-02")
		if err := uploadPortfolioArtifacts(ctx, creds, dataDir, date, dbPath); err != nil {
			return fmt.Errorf("R2 push failed: %w", err)
		}
		fmt.Fprintln(stdout, "✓ Cloudflare R2 portfolio snapshot, state files & DB synced.")
	case "status":
		fmt.Fprintf(stdout, "Cloudflare R2 bucket: %s\n", r2.DefaultBucket)
	default:
		return fmt.Errorf("unknown portfolio r2 action: %s", args[0])
	}
	return nil
}
