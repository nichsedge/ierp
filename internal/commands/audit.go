package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/audit"
	"github.com/nichsedge/ierp/internal/config"
)

func handleAudit(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	result, err := audit.RunAudit(database)
	if err != nil {
		fmt.Fprintf(stderr, "%sError running life audit: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== iERP Life Audit & Pulse Report (%s) ===%s\n", config.Bold, result.GeneratedAt, config.Reset)
	fmt.Fprintf(stdout, "Total Findings: %d (Action Needed: %d, Warnings: %d, Tips: %d)\n\n",
		result.TotalFindings, result.ActionsNeeded, result.Warnings, result.Tips)

	for _, f := range result.Findings {
		var icon, color string
		switch f.Severity {
		case "action_needed":
			icon = "🚨"
			color = config.Red
		case "warning":
			icon = "⚠️ "
			color = config.Yellow
		default:
			icon = "💡"
			color = config.Cyan
		}
		fmt.Fprintf(stdout, "%s %s[%s] %s%s\n", icon, color, f.Domain, f.Title, config.Reset)
		fmt.Fprintf(stdout, "   %s\n", f.Description)
		if f.Command != "" {
			fmt.Fprintf(stdout, "   %sFix:%s %s\n", config.Bold, config.Reset, f.Command)
		}
		fmt.Fprintln(stdout)
	}
	return nil
}
