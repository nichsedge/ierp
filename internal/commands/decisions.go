package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/decisions"
)

func handleInsertDecision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-decision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "Decision title (required)")
	choice := fs.String("choice", "", "Chosen path/option (required)")
	contextNotes := fs.String("context", "", "Context and reasoning")
	expected := fs.String("expected", "", "Expected outcome")
	confidence := fs.Int("confidence", 7, "Confidence score (1-10)")
	reviewDate := fs.String("review-date", "", "Scheduled review date (YYYY-MM-DD)")
	projID := fs.Int("project-id", 0, "Associated project ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *title == "" || *choice == "" {
		fmt.Fprintf(stderr, "%sError: --title and --choice are required%s\n", config.Red, config.Reset)
		return fmt.Errorf("title and choice are required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var pPtr *int
	if *projID > 0 {
		pPtr = projID
	}

	id, err := decisions.InsertDecision(database, decisions.Decision{
		Title:           *title,
		Choice:          *choice,
		Context:         *contextNotes,
		ExpectedOutcome: *expected,
		Confidence:      *confidence,
		ReviewDate:      *reviewDate,
		ProjectID:       pPtr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sDecision #%d recorded in journal: %s%s\n", config.Green, id, *title, config.Reset)
	return nil
}

func handleListDecisions(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("decisions", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "", "Filter status (pending, reviewed, all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := decisions.ListDecisions(database, *status, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Decision Journal (%d entries) ===%s\n", config.Bold, len(list), config.Reset)
	for _, d := range list {
		reviewStr := d.ReviewDate
		if reviewStr == "" {
			reviewStr = "no review date"
		}
		fmt.Fprintf(stdout, "#%-3d | %-28s | %-9s | review: %s | conf: %d/10\n", d.ID, d.Title, d.Status, reviewStr, d.Confidence)
	}
	return nil
}

func handleReviewDecision(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintf(stderr, "%sUsage: ierp review-decision <id> --outcome <results>%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing decision id")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "%sInvalid ID: %s%s\n", config.Red, args[0], config.Reset)
		return fmt.Errorf("invalid decision id: %s", args[0])
	}

	fs := flag.NewFlagSet("review-decision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outcome := fs.String("outcome", "", "Actual outcome and retrospective analysis (required)")
	status := fs.String("status", "reviewed", "New status")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	if *outcome == "" {
		fmt.Fprintf(stderr, "%sError: --outcome is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("outcome is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	ok, err := decisions.ReviewDecision(database, id, *outcome, *status)
	if err != nil || !ok {
		fmt.Fprintf(stderr, "%sFailed to update decision #%d: %v%s\n", config.Red, id, err, config.Reset)
		return fmt.Errorf("failed to update decision #%d", id)
	}

	fmt.Fprintf(stdout, "%sDecision #%d reviewed and calibrated successfully!%s\n", config.Green, id, config.Reset)
	return nil
}
