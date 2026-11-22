package commands

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/nichsedge/ierp/internal/antivision"
	"github.com/nichsedge/ierp/internal/config"
)

func handleInsertAntiVision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-antivision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	desc := fs.String("desc", "", "Description of negative constraint (required)")
	domains := fs.String("domains", "", "Comma-separated domains (family, work/finance, friends, fitness, behaviour, topics)")
	severity := fs.Int("severity", 3, "Severity level (1-5)")
	status := fs.String("status", "active", "Status (active, mitigated, transcended, deprecated)")
	dateLogged := fs.String("date", time.Now().Format("2006-01-02"), "Date logged (YYYY-MM-DD)")
	notes := fs.String("notes", "", "Additional context or notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *desc == "" {
		fmt.Fprintf(stderr, "%sError: --desc is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("desc is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := antivision.InsertAntiVision(database, antivision.AntiVisionItem{
		Description: *desc,
		Domains:     *domains,
		Status:      *status,
		Severity:    *severity,
		DateLogged:  *dateLogged,
		Notes:       *notes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	fmt.Fprintf(stdout, "%sAnti-Vision constraint #%d recorded: %s%s\n", config.Green, id, *desc, config.Reset)
	return nil
}

func handleListAntiVision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("antivision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	domain := fs.String("domain", "", "Filter by domain (family, work/finance, friends, fitness, behaviour, topics)")
	status := fs.String("status", "active", "Filter by status (active, mitigated, all)")
	jsonOutput := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	items, err := antivision.ListAntiVision(database, *domain, *status)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if *jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(items)
		return nil
	}

	fmt.Fprintf(stdout, "%s=== Anti-Vision Constraints (%d entries) ===%s\n", config.Bold, len(items), config.Reset)
	for _, it := range items {
		fmt.Fprintf(stdout, "#%-3d | [sev %d/5] | %-12s | %-10s | %-32s | %s\n", it.ID, it.Severity, it.Status, it.DateLogged, it.Description, it.Domains)
	}
	return nil
}

func handleUpdateAntiVision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%sUsage: ierp update-antivision <id> [--status <val>] [--severity <1-5>] [--notes <text>]%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing antivision id")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		return fmt.Errorf("invalid antivision id: %s", args[0])
	}

	fs := flag.NewFlagSet("update-antivision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "", "Updated status (active, mitigated, transcended, deprecated)")
	severity := fs.Int("severity", 0, "Updated severity (1-5)")
	notes := fs.String("notes", "", "Updated notes")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	var sevPtr *int
	if *severity > 0 {
		sevPtr = severity
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ok, err := antivision.UpdateAntiVision(database, id, *status, sevPtr, *notes)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	if !ok {
		fmt.Fprintf(stdout, "%sAnti-Vision constraint #%d not found or unchanged%s\n", config.Yellow, id, config.Reset)
		return nil
	}
	fmt.Fprintf(stdout, "%sAnti-Vision constraint #%d updated successfully%s\n", config.Green, id, config.Reset)
	return nil
}

func handleDeleteAntiVision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%sUsage: ierp delete-antivision <id>%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing antivision id")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		return fmt.Errorf("invalid antivision id: %s", args[0])
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ok, err := antivision.DeleteAntiVision(database, id)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	if !ok {
		fmt.Fprintf(stdout, "%sAnti-Vision constraint #%d not found%s\n", config.Yellow, id, config.Reset)
		return nil
	}
	fmt.Fprintf(stdout, "%sAnti-Vision constraint #%d deleted%s\n", config.Green, id, config.Reset)
	return nil
}
