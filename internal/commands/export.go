package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nichsedge/ierp/internal/commerce"
	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/garden"
	"github.com/nichsedge/ierp/internal/ghrepos"
	"github.com/nichsedge/ierp/internal/transparency"
)

func handleExportGarden(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-garden", flag.ContinueOnError)
	fs.SetOutput(stderr)
	gardenPath := fs.String("path", "", "Target Digital Garden content directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	summary, err := garden.ExportAll(database, *gardenPath)
	if err != nil {
		fmt.Fprintf(stderr, "%sGarden export failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sDigital Garden Deep Export Complete!%s\n", config.Green, config.Reset)
	fmt.Fprintf(stdout, "  Strategic Initiatives:  %d notes\n", summary.Projects)
	fmt.Fprintf(stdout, "  Decision Journal (PDR): %d notes\n", summary.Decisions)
	fmt.Fprintf(stdout, "  Sprint Retrospectives:  %d notes\n", summary.Retrospectives)
	fmt.Fprintf(stdout, "  Hardware Gadgets:       %d notes\n", summary.Gadgets)
	fmt.Fprintf(stdout, "  Anti-Vision Guardrails: %d items\n", summary.AntiVision)
	fmt.Fprintf(stdout, "  Role & Skills Matrix:   %d entries\n", summary.Career)
	return nil
}

func handleExportTransparency(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-transparency", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("path", "", "Target portfolio data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	outPath, err := transparency.Export(database, *dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "%sTransparency export failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sTransparency Export Complete!%s\n", config.Green, config.Reset)
	fmt.Fprintf(stdout, "  Privacy-safe metrics written: %s\n", outPath)
	return nil
}

func handleExportCommerce(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-commerce", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("path", "", "Target portfolio data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	payCount, refCount, err := commerce.ExportCommerceFiles(database, *dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "%sCommerce export failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sCommerce Data Export Complete!%s\n", config.Green, config.Reset)
	fmt.Fprintf(stdout, "  Payment Accounts: %d accounts -> pay.json\n", payCount)
	fmt.Fprintf(stdout, "  Referral Codes:   %d active links -> referrals.json\n", refCount)
	return nil
}

func handleExportGitHub(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-github", flag.ContinueOnError)
	fs.SetOutput(stderr)
	output := fs.String("output", "", "Output file path (default: nichsedge.github.io/data/github_repos_all.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	outPath := *output
	if outPath == "" {
		home, _ := os.UserHomeDir()
		outPath = filepath.Join(home, "Projects", "nichsedge.github.io", "data", "github_repos_all.json")
	}

	count, err := ghrepos.ExportGitHubRepos(ctx, database, outPath)
	if err != nil {
		fmt.Fprintf(stderr, "%sGitHub export failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	fmt.Fprintf(stdout, "%s✓ Exported %d repositories to %s%s\n", config.Green, count, outPath, config.Reset)
	return nil
}

func handleExportCheck(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("export-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fix := fs.Bool("fix", false, "Automatically regenerate lagging downstream exports")
	fs.BoolVar(fix, "f", false, "Alias for --fix")
	quiet := fs.Bool("quiet", false, "Silent mode (exit code only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, "Projects", "nichsedge.github.io", "data")
	dbPath := config.LocalDBPath()

	var dbMTime time.Time
	if fi, err := os.Stat(dbPath); err == nil {
		dbMTime = fi.ModTime().UTC()
	}

	targets := []struct {
		Name     string
		Path     string
		Category string
	}{
		{"pay.json", filepath.Join(dataDir, "pay.json"), "commerce"},
		{"referrals.json", filepath.Join(dataDir, "referrals.json"), "commerce"},
		{"github_repos_all.json", filepath.Join(dataDir, "github_repos_all.json"), "gh-projects"},
	}

	type targetStatus struct {
		Name     string
		Path     string
		Status   string
		DiffSec  float64
		Category string
	}

	var statuses []targetStatus
	hasStale := false

	for _, t := range targets {
		fi, err := os.Stat(t.Path)
		if os.IsNotExist(err) {
			statuses = append(statuses, targetStatus{
				Name:     t.Name,
				Path:     t.Path,
				Status:   "MISSING",
				Category: t.Category,
			})
			hasStale = true
			continue
		}

		targetMTime := fi.ModTime().UTC()
		diff := dbMTime.Sub(targetMTime).Seconds()
		if diff > 60 {
			statuses = append(statuses, targetStatus{
				Name:     t.Name,
				Path:     t.Path,
				Status:   "STALE",
				DiffSec:  diff,
				Category: t.Category,
			})
			hasStale = true
		} else {
			statuses = append(statuses, targetStatus{
				Name:     t.Name,
				Path:     t.Path,
				Status:   "FRESH",
				DiffSec:  diff,
				Category: t.Category,
			})
		}
	}

	if *fix && hasStale {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		needCommerce := false
		needGH := false
		for _, s := range statuses {
			if s.Status != "FRESH" {
				if s.Category == "commerce" {
					needCommerce = true
				} else if s.Category == "gh-projects" {
					needGH = true
				}
			}
		}

		if needCommerce {
			_, _, _ = commerce.ExportCommerceFiles(database, dataDir)
		}
		if needGH {
			ghPath := filepath.Join(dataDir, "github_repos_all.json")
			_, _ = ghrepos.ExportGitHubRepos(ctx, database, ghPath)
		}

		if !*quiet {
			fmt.Fprintf(stdout, "%s✓ Downstream Export Guardrail: Regenerated stale export files in nichsedge.github.io/data%s\n",
				config.Green, config.Reset)
		}
		return nil
	}

	if *quiet {
		if hasStale {
			return fmt.Errorf("downstream exports are stale")
		}
		return nil
	}

	fmt.Fprintln(stdout, "================================================================================")
	if hasStale {
		fmt.Fprintf(stdout, "%s⚠️  DOWNSTREAM EXPORT GUARDRAIL: STALE EXPORTS DETECTED%s\n", config.Yellow, config.Reset)
	} else {
		fmt.Fprintf(stdout, "%s✅ DOWNSTREAM EXPORT GUARDRAIL: ALL EXPORTS IN-SYNC%s\n", config.Green, config.Reset)
	}
	fmt.Fprintln(stdout, "================================================================================")
	for _, s := range statuses {
		color := config.Green
		icon := "✓"
		if s.Status == "STALE" {
			color = config.Yellow
			icon = "⚠️"
		} else if s.Status == "MISSING" {
			color = config.Red
			icon = "❌"
		}
		fmt.Fprintf(stdout, " %s%s %-25s [%-7s]%s %s\n",
			color, icon, s.Name, s.Status, config.Reset, s.Path)
	}
	fmt.Fprintln(stdout, "================================================================================")

	if hasStale {
		if !*fix {
			fmt.Fprintf(stdout, "Tip: Run 'ierp export check --fix' or 'ierp export [commerce|gh-projects]' to regenerate.\n")
			return fmt.Errorf("downstream exports are stale")
		}
	}
	return nil
}

func handleExport(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && (args[0] == "gh-projects" || args[0] == "github") {
		return handleExportGitHub(ctx, args[1:], stdout, stderr)
	} else if len(args) > 0 && args[0] == "garden" {
		return handleExportGarden(ctx, args[1:], stdout, stderr)
	} else if len(args) > 0 && args[0] == "commerce" {
		return handleExportCommerce(ctx, args[1:], stdout, stderr)
	} else if len(args) > 0 && args[0] == "transparency" {
		return handleExportTransparency(ctx, args[1:], stdout, stderr)
	} else if len(args) > 0 && (args[0] == "check" || args[0] == "guardrail") {
		return handleExportCheck(ctx, args[1:], stdout, stderr)
	}
	fmt.Fprintln(stdout, "Usage: ierp export [gh-projects|garden|commerce|transparency|check]")
	return nil
}
