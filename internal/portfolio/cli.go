package portfolio

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/debank"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/ksei"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fetchers/sansfinance"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/fx"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/integrator"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/mcp"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/models"
	"github.com/nichsedge/ierp/internal/portfolio/pkg/r2"
)

func loadEnv() {
	_ = godotenv.Load()
	home, err := os.UserHomeDir()
	if err == nil {
		_ = godotenv.Load(filepath.Join(home, ".secrets"))
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
		return d
	}
	// Fallback to local data/
	dir, err := os.Getwd()
	if err == nil {
		localData := filepath.Join(dir, "data")
		if dirExists(localData) {
			return localData
		}
	}
	return "./data"
}

// RunCLI runs the integrated portfolio command suite.
func RunCLI(cliArgs []string) {
	loadEnv()

	if len(cliArgs) < 1 {
		printHelp()
		return
	}

	command := cliArgs[0]
	args := cliArgs[1:]

	switch command {
	case "run-all", "run":
		runAll(args)
	case "fetch":
		runFetch(args)
	case "integrate":
		runIntegrate(args)
	case "ai-state":
		runAIState(args)
	case "advisor":
		runAdvisor(args)
	case "mcp":
		runMCP(args)
	case "r2":
		runR2(args)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Printf("Unknown portfolio command: %s\n\n", command)
		printHelp()
	}
}

func printHelp() {
	fmt.Println("🚀 iERP Portfolio Engine")
	fmt.Println("\nUsage:")
	fmt.Println("  ierp portfolio run             Run full multi-asset pipeline (fetch, integrate, ai-state, r2)")
	fmt.Println("  ierp portfolio fetch <src>     Fetch single source (ksei, debank, binance, alchemy, sansfinance)")
	fmt.Println("  ierp portfolio integrate       Integrate raw data, enrich yields, persist snapshot & SQLite")
	fmt.Println("  ierp portfolio ai-state        Generate latest_ai_state.json & latest_ai_digest.md")
	fmt.Println("  ierp portfolio advisor         Quantitative sovereign runway matrix & rebalance directives")
	fmt.Println("  ierp portfolio mcp             Run stdio Model Context Protocol (MCP) server for AI agents")
	fmt.Println("  ierp portfolio r2 <action>     R2 cloud operations (push, status)")
}

func runAll(args []string) {
	fs := flag.NewFlagSet("run-all", flag.ExitOnError)
	dateFlag := fs.String("date", time.Now().Format("2006-01-02"), "Date in YYYY-MM-DD")
	noUpload := fs.Bool("no-upload", false, "Skip Cloudflare R2 backup")
	_ = fs.Parse(args)

	date := *dateFlag
	dataDir, _ := filepath.Abs(getDataDir())
	_ = os.MkdirAll(dataDir, 0755)

	fmt.Printf("🚀 Running full portfolio pipeline for %s\n", date)
	fmt.Printf("📁 Data directory: %s\n\n", dataDir)

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

	// 4. Fetch Alchemy (Solana)
	fmt.Print("⏳ [4/5] Fetching Alchemy (Solana)... ")
	solAddr := os.Getenv("SOL_ADDRESS")
	alchKey := os.Getenv("ALCHEMY_API_KEY")
	rawAlchPath := filepath.Join(dataDir, fmt.Sprintf("%s_curated_alchemy.json", date))
	if solAddr != "" && alchKey != "" {
		rawAlch, alchHoldings, err := alchemy.Fetch(alchKey, solAddr)
		if err == nil {
			_ = alchemy.SaveCurated(rawAlch, rawAlchPath)
			allHoldings = append(allHoldings, alchHoldings...)
			fmt.Printf("✓ %d tokens\n", len(alchHoldings))
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedHoldings(rawAlchPath, date, dataDir, "alchemy", alchemy.Standardize)...)
		}
	} else {
		fmt.Println("⚠️ (no SOL credentials) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedHoldings(rawAlchPath, date, dataDir, "alchemy", alchemy.Standardize)...)
	}

	// 5. Fetch DeBank
	fmt.Print("⏳ [5/5] Fetching DeBank (EVM)... ")
	ethAddr := os.Getenv("ETH_ADDRESS")
	rawDebankPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_debank.json", date))
	if ethAddr != "" {
		err := debank.Scrape(ethAddr, rawDebankPath)
		if err == nil {
			if _, debankHoldings, err := debank.LoadRaw(rawDebankPath); err == nil {
				allHoldings = append(allHoldings, debankHoldings...)
				fmt.Printf("✓ %d assets\n", len(debankHoldings))
			}
		} else {
			fmt.Printf("⚠️ (%v) - checking cache fallback\n", err)
			allHoldings = append(allHoldings, loadCachedDebankHoldings(rawDebankPath, date, dataDir)...)
		}
	} else {
		fmt.Println("⚠️ (no ETH_ADDRESS) - checking cache fallback")
		allHoldings = append(allHoldings, loadCachedDebankHoldings(rawDebankPath, date, dataDir)...)
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
		fmt.Printf("❌ Integration failed: %v\n", err)
		os.Exit(1)
	}

	printSummary(snap)

	// Step 7: AI Financial State & Sovereign Runway Generator
	fmt.Print("🧠 Generating token-optimized AI State & Sovereign Digest... ")
	aiStateObj, err := aistate.GenerateAIState(snap, dbConn)
	if err == nil {
		digestMD := aistate.GenerateAIDigestMarkdown(aiStateObj)
		_ = aistate.SaveAIState(aiStateObj, digestMD, dataDir, dbConn)

		advPayload := advisor.GenerateAdvisorPayload(snap, aiStateObj, dbConn)
		_ = advisor.SaveAdvisorPayload(advPayload, dataDir)
		fmt.Println("✓ Generated latest_ai_state.json & latest_ai_digest.md")
	} else {
		fmt.Printf("⚠️ (%v)\n", err)
	}

	// Step 8: Cloud Upload
	if !*noUpload && r2Acc != "" && r2Key != "" && r2Sec != "" {
		fmt.Println("☁️ Uploading portfolio snapshot, AI state, and DB to Cloudflare R2...")
		snapFile := filepath.Join(dataDir, fmt.Sprintf("%s_snapshot.json", date))
		latestFile := filepath.Join(dataDir, "latest_snapshot.json")
		aiStateFile := filepath.Join(dataDir, "latest_ai_state.json")
		aiDigestFile := filepath.Join(dataDir, "latest_ai_digest.md")
		advisorFile := filepath.Join(dataDir, "latest_advisor.json")

		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, snapFile, fmt.Sprintf("portfolio/%s_snapshot.json", date))
		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, latestFile, "portfolio/latest_snapshot.json")
		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, aiStateFile, "portfolio/latest_ai_state.json")
		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, aiDigestFile, "portfolio/latest_ai_digest.md")
		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, advisorFile, "portfolio/latest_advisor.json")
		_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, dbPath, "db/portfolio_latest.sqlite")
		fmt.Println("✓ Cloudflare R2 upload complete.")
	}

	fmt.Println("\n✨ Pipeline completed successfully!")
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

func loadCachedDebankHoldings(targetPath, date, dataDir string) []models.Holding {
	tryFallbackCache(targetPath, date, dataDir, "debank")
	if _, dHoldings, err := debank.LoadRaw(targetPath); err == nil {
		if len(dHoldings) > 0 {
			fmt.Printf("✓ loaded %d cached holdings\n", len(dHoldings))
		}
		return dHoldings
	}
	return nil
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

func runFetch(args []string) {
	if len(args) == 0 {
		fmt.Println("Please specify source: ksei, debank, binance, alchemy, sansfinance")
		os.Exit(1)
	}
	dataDir, _ := filepath.Abs(getDataDir())
	date := time.Now().Format("2006-01-02")

	switch args[0] {
	case "ksei":
		user := os.Getenv("KSEI_USERNAME")
		pass := os.Getenv("KSEI_PASSWORD")
		raw, _, err := ksei.Fetch(user, pass, true)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if raw.IsDegraded() {
			if prev := findLatestValidKseiCache(dataDir, date); prev != nil {
				if raw.FillMissingFromPrevious(prev) {
					fmt.Printf("⚠️ KSEI returned 0 equities & bonds (off-hours). Preserved %d equities & %d bonds from cache.\n", len(prev.Equity.Data), len(prev.Bond.Data))
				}
			}
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_ksei.json", date))
		_ = ksei.SaveRaw(raw, target)
		fmt.Printf("✓ Saved to %s\n", target)
	case "sansfinance":
		r2Acc := os.Getenv("R2_ACCOUNT_ID")
		r2Key := os.Getenv("R2_ACCESS_KEY_ID")
		r2Sec := os.Getenv("R2_SECRET_ACCESS_KEY")
		raw, _, err := sansfinance.Fetch(r2Acc, r2Key, r2Sec, "ichsanul-dev")
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_sansfinance.json", date))
		_ = sansfinance.SaveRaw(raw, target)
		fmt.Printf("✓ Saved to %s\n", target)
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
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_binance.json", date))
		_ = binance.SaveRaw(raw, target)
		fmt.Printf("✓ Saved %d balances to %s\n", len(holdings), target)
	case "alchemy":
		solAddr := os.Getenv("SOL_ADDRESS")
		alchKey := os.Getenv("ALCHEMY_API_KEY")
		raw, holdings, err := alchemy.Fetch(alchKey, solAddr)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		target := filepath.Join(dataDir, fmt.Sprintf("%s_curated_alchemy.json", date))
		_ = alchemy.SaveCurated(raw, target)
		fmt.Printf("✓ Saved %d tokens to %s\n", len(holdings), target)
	case "debank":
		ethAddr := os.Getenv("ETH_ADDRESS")
		target := filepath.Join(dataDir, fmt.Sprintf("%s_raw_debank.json", date))
		err := debank.Scrape(ethAddr, target)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		_, holdings, err := debank.LoadRaw(target)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Saved %d assets to %s\n", len(holdings), target)
	default:
		fmt.Printf("Fetch for %s not implemented or run via run-all\n", args[0])
	}
}

func runIntegrate(args []string) {
	fs := flag.NewFlagSet("integrate", flag.ExitOnError)
	dateFlag := fs.String("date", time.Now().Format("2006-01-02"), "Date in YYYY-MM-DD")
	_ = fs.Parse(args)

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

	debankPath := filepath.Join(dataDir, fmt.Sprintf("%s_raw_debank.json", date))
	if _, h, err := debank.LoadRaw(debankPath); err == nil {
		allHoldings = append(allHoldings, h...)
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
		fmt.Printf("❌ Integration failed: %v\n", err)
		os.Exit(1)
	}

	printSummary(snap)

	// Generate AI state & digest
	state, err := aistate.GenerateAIState(snap, dbConn)
	if err == nil {
		digestMD := aistate.GenerateAIDigestMarkdown(state)
		_ = aistate.SaveAIState(state, digestMD, dataDir, dbConn)
		advPayload := advisor.GenerateAdvisorPayload(snap, state, dbConn)
		_ = advisor.SaveAdvisorPayload(advPayload, dataDir)
		fmt.Println("✓ Generated latest_ai_state.json & latest_ai_digest.md")
	}
}

func runAIState(args []string) {
	dataDir, _ := filepath.Abs(getDataDir())
	latestPath := filepath.Join(dataDir, "latest_snapshot.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		fmt.Printf("❌ Could not read latest_snapshot.json: %v\n", err)
		os.Exit(1)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		fmt.Printf("❌ Could not parse latest_snapshot.json: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	state, err := aistate.GenerateAIState(&snap, dbConn)
	if err != nil {
		fmt.Printf("❌ Failed to generate AI state: %v\n", err)
		os.Exit(1)
	}
	digestMD := aistate.GenerateAIDigestMarkdown(state)
	if err := aistate.SaveAIState(state, digestMD, dataDir, dbConn); err != nil {
		fmt.Printf("❌ Failed to save AI state: %v\n", err)
		os.Exit(1)
	}

	advPayload := advisor.GenerateAdvisorPayload(&snap, state, dbConn)
	_ = advisor.SaveAdvisorPayload(advPayload, dataDir)

	fmt.Println("✨ Successfully refreshed latest_ai_state.json & latest_ai_digest.md")
	fmt.Printf("   Net Worth: Rp %s ($%.2f)\n", formatIDR(state.MacroMetrics.NetWorthIDR), state.MacroMetrics.NetWorthUSD)
	fmt.Printf("   Passive Cashflow: Rp %s/mo (FI Coverage: %.1f%%)\n",
		formatIDR(state.PassiveIncome.ProjectedMonthlyPassiveIncomeIDR), state.PassiveIncome.FICoveragePct)
	fmt.Printf("   ADHD 1-Step Directive: %s\n", state.ADHDFocusMetrics.DepositRouter.OneStepAction)
}

func runAdvisor(args []string) {
	fs := flag.NewFlagSet("advisor", flag.ExitOnError)
	mdFlag := fs.Bool("markdown", false, "Output non-ANSI Markdown briefing")
	jsonFlag := fs.Bool("json", false, "Output structured JSON payload")
	_ = fs.Parse(args)

	dataDir, _ := filepath.Abs(getDataDir())
	latestPath := filepath.Join(dataDir, "latest_snapshot.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		fmt.Printf("❌ Could not read latest_snapshot.json: %v\n", err)
		os.Exit(1)
	}
	var snap models.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		fmt.Printf("❌ Could not parse latest_snapshot.json: %v\n", err)
		os.Exit(1)
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
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(b))
		return
	}

	if *mdFlag {
		fmt.Println(advisor.GenerateMarkdownBriefing(payload))
		return
	}

	// Default terminal report
	fmt.Println("\n🏛️  SOVEREIGN RUNWAY & MACRO ADVISOR")
	fmt.Println("=======================================================")
	fmt.Printf("Date:             %s\n", payload.Date)
	fmt.Printf("Net Worth:        Rp %s\n", formatIDR(payload.NetWorthIDR))
	fmt.Printf("Liquid Reserves:  Rp %s (%.1f%% dry powder)\n", formatIDR(payload.DryPowderIDR), payload.DryPowderPct)
	fmt.Printf("Runway Horizon:   %.1f Months [%s]\n", payload.RunwayMonths, payload.RunwayStatus)
	fmt.Println("-------------------------------------------------------")
	fmt.Printf("Directive: %s\n", payload.ActionSummary)
	fmt.Println("-------------------------------------------------------")
	fmt.Println("3-Tier Sovereign Runway Matrix:")
	p := payload.SovereignAllocationPlan
	fmt.Printf("  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier1OperatingReserve.Name, formatIDR(p.Tier1OperatingReserve.AllocatedIDR), p.Tier1OperatingReserve.AllocationPct)
	fmt.Printf("  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier2FortressBuffer.Name, formatIDR(p.Tier2FortressBuffer.AllocatedIDR), p.Tier2FortressBuffer.AllocationPct)
	fmt.Printf("  %-30s  Rp %-14s  (%.1f%%)\n", p.Tier3DeployableSurplus.Name, formatIDR(p.Tier3DeployableSurplus.AllocatedIDR), p.Tier3DeployableSurplus.AllocationPct)
	fmt.Println("=======================================================")
}

func runMCP(args []string) {
	dataDir, _ := filepath.Abs(getDataDir())
	dbPath := filepath.Join(dataDir, "portfolio.db")
	var dbConn *sql.DB
	if conn, err := db.OpenDB(dbPath); err == nil {
		dbConn = conn
		defer dbConn.Close()
	}

	srv := mcp.NewServer(dataDir, dbConn)
	if err := srv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func runR2(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: portfolio r2 [push|status]")
		return
	}
	switch args[0] {
	case "push":
		dataDir, _ := filepath.Abs(getDataDir())
		dbPath := filepath.Join(dataDir, "portfolio.db")
		r2Acc := os.Getenv("R2_ACCOUNT_ID")
		r2Key := os.Getenv("R2_ACCESS_KEY_ID")
		r2Sec := os.Getenv("R2_SECRET_ACCESS_KEY")
		r2Bucket := os.Getenv("R2_BUCKET_NAME")
		if r2Bucket == "" {
			r2Bucket = "ichsanul-dev"
		}
		if err := r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, dbPath, "db/portfolio_latest.sqlite"); err != nil {
			fmt.Printf("❌ Failed to upload DB: %v\n", err)
		} else {
			fmt.Println("✓ Uploaded db/portfolio_latest.sqlite to Cloudflare R2")
		}

		date := time.Now().Format("2006-01-02")
		snapFile := filepath.Join(dataDir, fmt.Sprintf("%s_snapshot.json", date))
		latestFile := filepath.Join(dataDir, "latest_snapshot.json")
		aiStateFile := filepath.Join(dataDir, "latest_ai_state.json")
		aiDigestFile := filepath.Join(dataDir, "latest_ai_digest.md")
		advisorFile := filepath.Join(dataDir, "latest_advisor.json")

		if _, err := os.Stat(snapFile); err == nil {
			_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, snapFile, fmt.Sprintf("portfolio/%s_snapshot.json", date))
		}
		if _, err := os.Stat(latestFile); err == nil {
			_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, latestFile, "portfolio/latest_snapshot.json")
		}
		if _, err := os.Stat(aiStateFile); err == nil {
			_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, aiStateFile, "portfolio/latest_ai_state.json")
		}
		if _, err := os.Stat(aiDigestFile); err == nil {
			_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, aiDigestFile, "portfolio/latest_ai_digest.md")
		}
		if _, err := os.Stat(advisorFile); err == nil {
			_ = r2.UploadFile(r2Acc, r2Key, r2Sec, r2Bucket, advisorFile, "portfolio/latest_advisor.json")
		}
		fmt.Println("✓ Cloudflare R2 portfolio snapshot & state files synced.")
	case "status":
		fmt.Println("Cloudflare R2 bucket: ichsanul-dev")
	}
}
