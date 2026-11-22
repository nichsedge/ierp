package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/ghrepos"
	"github.com/nichsedge/ierp/internal/portfolio"
)

func handleSyncPortfolio(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("sync-portfolio", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("path", "", "Path to portfolio data directory")
	force := fs.Bool("force", false, "Force ingestion even if liquid cash deviates by >50%")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	if err := portfolio.SyncPortfolio(database, *path, *force); err != nil {
		fmt.Fprintf(stderr, "%sPortfolio sync failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	return nil
}

func handleSyncGitHub(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	fmt.Fprintf(stdout, "%sSyncing GitHub repositories via GraphQL...%s\n", config.Cyan, config.Reset)
	count, err := ghrepos.SyncGitHubRepos(ctx, database)
	if err != nil {
		fmt.Fprintf(stderr, "%sGitHub sync failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	fmt.Fprintf(stdout, "%s✓ Synced %d repositories into events.db%s\n", config.Green, count, config.Reset)
	return nil
}

func handleSync(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "portfolio" {
		return handleSyncPortfolio(ctx, args[1:], stdout, stderr)
	} else if len(args) > 0 && (args[0] == "gh-projects" || args[0] == "github") {
		return handleSyncGitHub(ctx, args[1:], stdout, stderr)
	}
	fmt.Fprintln(stdout, "Usage: ierp sync [portfolio|gh-projects]")
	return nil
}
