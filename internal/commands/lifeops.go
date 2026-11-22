package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/lifeops"
)

func handleInsertMaintenance(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-maintenance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Task name (required)")
	dueDate := fs.String("due-date", "", "Due date (YYYY-MM-DD, required)")
	cat := fs.String("category", "general", "Category")
	interval := fs.Int("interval", 0, "Recurring interval in days")
	cost := fs.Float64("cost", 0, "Estimated cost")
	notes := fs.String("notes", "", "Notes")
	gadgetID := fs.Int("gadget-id", 0, "Associated gadget ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" || *dueDate == "" {
		fmt.Fprintf(stderr, "%sError: --name and --due-date are required%s\n", config.Red, config.Reset)
		return fmt.Errorf("name and due-date are required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var intPtr *int
	if *interval > 0 {
		intPtr = interval
	}
	var gPtr *int
	if *gadgetID > 0 {
		gPtr = gadgetID
	}

	id, err := lifeops.InsertMaintenance(database, lifeops.MaintenanceItem{
		Name:         *name,
		DueDate:      *dueDate,
		Category:     *cat,
		IntervalDays: intPtr,
		Cost:         *cost,
		Notes:        *notes,
		GadgetID:     gPtr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sMaintenance task #%d scheduled for %s: %s%s\n", config.Green, id, *dueDate, *name, config.Reset)
	return nil
}

func handleListMaintenance(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("maintenance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "pending", "Status (pending, completed, all)")
	cat := fs.String("category", "", "Category")
	overdueOnly := fs.Bool("overdue-only", false, "Show only overdue items")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	items, err := lifeops.ListMaintenance(database, *status, *cat, *overdueOnly)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Preventive Maintenance (%d items) ===%s\n", config.Bold, len(items), config.Reset)
	for _, it := range items {
		dueStatus := it.DueDate
		if it.IsOverdue {
			dueStatus = fmt.Sprintf("%sOVERDUE by %dd (%s)%s", config.Red, it.DaysOverdue, it.DueDate, config.Reset)
		}
		fmt.Fprintf(stdout, "#%-3d | %-26s | %-12s | %s\n", it.ID, it.Name, it.Category, dueStatus)
	}
	return nil
}

func handleCompleteMaintenance(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%sUsage: ierp complete-maintenance <id> [--date YYYY-MM-DD] [--cost <amount>]%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing maintenance id")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		return fmt.Errorf("invalid maintenance id: %s", args[0])
	}

	fs := flag.NewFlagSet("complete-maintenance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dateStr := fs.String("date", "", "Completion date (YYYY-MM-DD)")
	cost := fs.Float64("cost", -1, "Actual cost incurred")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var costPtr *float64
	if *cost >= 0 {
		costPtr = cost
	}

	res, err := lifeops.CompleteMaintenance(database, id, *dateStr, costPtr)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	if success, _ := res["success"].(bool); !success {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, res["error"], config.Reset)
		return fmt.Errorf("failed completing maintenance: %v", res["error"])
	}

	fmt.Fprintf(stdout, "%sTask #%d marked completed!%s\n", config.Green, id, config.Reset)
	if rescheduled, _ := res["rescheduled"].(bool); rescheduled {
		fmt.Fprintf(stdout, "  Auto-rescheduled next occurrence for %s%v%s (#%v)\n",
			config.Cyan, res["next_due_date"], config.Reset, res["next_item_id"])
	}
	return nil
}
