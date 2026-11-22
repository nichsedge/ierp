package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/reviews"
)

func handleInsertReview(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	start := fs.String("start", "", "Period start date (YYYY-MM-DD, required)")
	end := fs.String("end", "", "Period end date (YYYY-MM-DD, required)")
	pType := fs.String("type", "weekly", "Period type (weekly, monthly, quarterly)")
	wins := fs.String("wins", "", "Wins and accomplishments")
	drains := fs.String("drains", "", "Energy drains and burnout points")
	lessons := fs.String("lessons", "", "Key lessons learned")
	focus := fs.String("focus", "", "Focus for next sprint")
	rating := fs.Int("rating", 7, "Rating (1-10)")
	notes := fs.String("notes", "", "Additional notes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *start == "" || *end == "" {
		fmt.Fprintf(stderr, "%sError: --start and --end dates are required%s\n", config.Red, config.Reset)
		return fmt.Errorf("start and end dates are required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := reviews.InsertRetrospective(database, reviews.Retrospective{
		PeriodStart:   *start,
		PeriodEnd:     *end,
		PeriodType:    *pType,
		Wins:          *wins,
		DrainsBurnout: *drains,
		Lessons:       *lessons,
		FocusNext:     *focus,
		Rating:        *rating,
		Notes:         *notes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sRetrospective #%d recorded (%s to %s)%s\n", config.Green, id, *start, *end, config.Reset)
	return nil
}

func handleListReviews(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("reviews", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pType := fs.String("type", "", "Filter type")
	limit := fs.Int("limit", 10, "Limit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := reviews.ListRetrospectives(database, *pType, *limit)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Retrospectives (%d entries) ===%s\n", config.Bold, len(list), config.Reset)
	for _, r := range list {
		fmt.Fprintf(stdout, "#%-3d | %-9s | %s to %s | rating: %d/10\n", r.ID, r.PeriodType, r.PeriodStart, r.PeriodEnd, r.Rating)
	}
	return nil
}
