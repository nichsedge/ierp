package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/radar"
)

func handleRadar(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("radar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tierVal := fs.Int("tier", 0, "Filter by tier (1, 2, or 3)")
	overdueOnly := fs.Bool("overdue-only", false, "Show only overdue relationships")
	limit := fs.Int("limit", 50, "Limit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var t *int
	if *tierVal > 0 {
		t = tierVal
	}

	items, err := radar.ComputeRadar(database, t, *overdueOnly, *limit, 0)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Reconnection Radar (%d contacts) ===%s\n", config.Bold, len(items), config.Reset)
	for _, it := range items {
		status := fmt.Sprintf("%dd ago", it.DaysSinceLastTouch)
		if it.IsOverdue {
			status = fmt.Sprintf("%sOVERDUE (%dd late)%s", config.Red, it.DaysOverdue, config.Reset)
		}
		fmt.Fprintf(stdout, "Tier %d | #%-3d | %-22s | %-14s | %s\n", it.Tier, it.ID, it.Name, fmt.Sprintf("cadence: %dd", it.CadenceDays), status)
	}
	return nil
}

func handleDailyReconnection(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	item, err := radar.GetDailyReconnection(database)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if item == nil {
		fmt.Fprintf(stdout, "%sAll relationships are currently within target cadence. Radar clear!%s\n", config.Green, config.Reset)
		return nil
	}

	fmt.Fprintf(stdout, "%s🎯 Daily Reconnection Priority:%s\n", config.Bold, config.Reset)
	fmt.Fprintf(stdout, "  %s%s%s (Tier %d)\n", config.Cyan, item.Name, config.Reset, item.Tier)
	fmt.Fprintf(stdout, "  Cadence: every %d days | Overdue by: %s%d days%s\n", item.CadenceDays, config.Red, item.DaysOverdue, config.Reset)
	if item.LastSeenDate != "" {
		fmt.Fprintf(stdout, "  Last seen: %s\n", item.LastSeenDate)
	}
	if item.Org != "" {
		fmt.Fprintf(stdout, "  Organization: %s\n", item.Org)
	}
	return nil
}
