package commands

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/events"
)

func handleEvents(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return handleListEvents(ctx, nil, stdout, stderr)
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "add", "insert":
		if len(subArgs) > 0 && (strings.HasSuffix(subArgs[0], ".md") || strings.HasSuffix(subArgs[0], ".json")) {
			return handleIngestEvents(ctx, subArgs, stdout, stderr)
		}
		return handleInsertEvent(ctx, subArgs, stdout, stderr)
	case "list":
		return handleListEvents(ctx, subArgs, stdout, stderr)
	case "search":
		return handleSearchEvents(ctx, subArgs, stdout, stderr)
	case "ingest", "import":
		return handleIngestEvents(ctx, subArgs, stdout, stderr)
	case "delete", "rm":
		return handleDeleteEvent(ctx, subArgs, stdout, stderr)
	default:
		if strings.HasPrefix(sub, "-") {
			return handleListEvents(ctx, args, stdout, stderr)
		} else if strings.HasSuffix(sub, ".md") || strings.HasSuffix(sub, ".json") {
			return handleIngestEvents(ctx, args, stdout, stderr)
		}
		return handleInsertEvent(ctx, args, stdout, stderr)
	}
}

func handleAdd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%sUsage: ierp add [event|events|expense|expenses|contact|...] [args]%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing target for add")
	}

	target := args[0]
	subArgs := args[1:]

	switch target {
	case "event", "events":
		if len(subArgs) > 0 && (strings.HasSuffix(subArgs[0], ".md") || strings.HasSuffix(subArgs[0], ".json")) {
			return handleIngestEvents(ctx, subArgs, stdout, stderr)
		}
		return handleInsertEvent(ctx, subArgs, stdout, stderr)
	case "expense", "expenses":
		if len(subArgs) > 0 && (strings.HasSuffix(subArgs[0], ".md") || strings.HasSuffix(subArgs[0], ".json")) {
			return handleIngestExpenses(ctx, subArgs, stdout, stderr)
		}
		return handleInsertExpense(ctx, subArgs, stdout, stderr)
	case "contact", "contacts":
		return handleInsertContact(ctx, subArgs, stdout, stderr)
	case "snapshot", "snapshots":
		return handleInsertSnapshot(ctx, subArgs, stdout, stderr)
	case "commitment", "commitments":
		return handleInsertCommitment(ctx, subArgs, stdout, stderr)
	case "project", "projects":
		return handleInsertProject(ctx, subArgs, stdout, stderr)
	case "decision", "decisions":
		return handleInsertDecision(ctx, subArgs, stdout, stderr)
	default:
		if strings.HasSuffix(target, ".md") || strings.HasSuffix(target, ".json") {
			return handleIngestEvents(ctx, args, stdout, stderr)
		}
		return handleInsertEvent(ctx, args, stdout, stderr)
	}
}

func handleIngestEvents(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("events ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tag := fs.String("tag", "", "Default tag to attach to ingested event (e.g. travel,singapore)")
	contactsList := fs.String("contacts", "", "Comma-separated contact names or IDs")
	place := fs.String("place", "", "Location/place override")
	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintf(stderr, "%sUsage: ierp event ingest <file.md|file.json> [--tag <tag>] [--contacts <contacts>] [--place <place>]%s\n", config.Red, config.Reset)
		return fmt.Errorf("missing input file for event ingest")
	}

	targetPath := remaining[0]
	if strings.HasPrefix(targetPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			targetPath = filepath.Join(home, targetPath[2:])
		}
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	res, err := events.IngestFile(database, targetPath, *tag, *contactsList, *place)
	if err != nil {
		fmt.Fprintf(stderr, "%sError ingesting event: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s✅ Successfully logged timeline event #%d from %s!%s\n", config.Green, res.EventID, res.SourceDoc, config.Reset)
	fmt.Fprintf(stdout, "  • Title:    %s\n", res.Title)
	fmt.Fprintf(stdout, "  • Date:     %s", res.StartDate)
	if res.EndDate != "" && res.EndDate != res.StartDate {
		fmt.Fprintf(stdout, " to %s", res.EndDate)
	}
	fmt.Fprintln(stdout)
	if res.Place != "" {
		fmt.Fprintf(stdout, "  • Place:    %s\n", res.Place)
	}
	if len(res.Contacts) > 0 {
		fmt.Fprintf(stdout, "  • Contacts: %s\n", strings.Join(res.Contacts, ", "))
	}
	if len(res.Tags) > 0 {
		fmt.Fprintf(stdout, "  • Tags:     %s\n", strings.Join(res.Tags, ", "))
	}
	return nil
}

func handleInsertEvent(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "Event title (required)")
	place := fs.String("place", "", "Location/place")
	start := fs.String("start", "", "Start date (YYYY-MM-DD)")
	end := fs.String("end", "", "End date (YYYY-MM-DD)")
	tags := fs.String("tags", "", "Comma-separated tags")
	url := fs.String("url", "", "URL reference")
	notes := fs.String("notes", "", "Notes or description")
	contactsList := fs.String("contacts", "", "Comma-separated contact names or IDs")
	projectID := fs.Int("project-id", 0, "Associated project ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *title == "" {
		if len(fs.Args()) > 0 {
			if len(fs.Args()) == 1 && (strings.HasSuffix(fs.Args()[0], ".md") || strings.HasSuffix(fs.Args()[0], ".json")) {
				return handleIngestEvents(ctx, args, stdout, stderr)
			}
			*title = strings.Join(fs.Args(), " ")
		} else {
			fmt.Fprintf(stderr, "%sError: --title is required (or provide title directly)%s\n", config.Red, config.Reset)
			return fmt.Errorf("title is required")
		}
	}

	if *place == "" && strings.Contains(*title, " @ ") {
		parts := strings.SplitN(*title, " @ ", 2)
		*title = strings.TrimSpace(parts[0])
		*place = strings.TrimSpace(parts[1])
	}

	if *start == "" {
		*start = time.Now().Format("2006-01-02")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var tagSlice []string
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			if tr := strings.TrimSpace(t); tr != "" {
				tagSlice = append(tagSlice, tr)
			}
		}
	}

	var contactRefs []string
	if *contactsList != "" {
		for _, c := range strings.Split(*contactsList, ",") {
			if cr := strings.TrimSpace(c); cr != "" {
				contactRefs = append(contactRefs, cr)
			}
		}
	}

	var pID *int
	if *projectID > 0 {
		pID = projectID
	}

	ev := events.Event{
		Title:     *title,
		Place:     *place,
		StartDate: *start,
		EndDate:   *end,
		RawDate:   *start,
		Tags:      tagSlice,
		URL:       *url,
		Notes:     *notes,
		ProjectID: pID,
	}

	id, linked, err := events.InsertEvent(database, ev, contactRefs)
	if err != nil {
		fmt.Fprintf(stderr, "%sFailed to insert event: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sEvent #%d logged:%s %s\n", config.Green, id, config.Reset, *title)
	if len(linked) > 0 {
		fmt.Fprintf(stdout, "  Linked contacts: %s\n", strings.Join(linked, ", "))
	}
	return nil
}

func handleListEvents(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "Max events to list")
	tag := fs.String("tag", "", "Filter by tag")
	excludeTag := fs.String("exclude-tag", "", "Exclude events with this tag (comma-separated)")
	life := fs.Bool("life", false, "Show only personal/life events (exclude automated telemetry: timeline, atracker)")
	source := fs.String("source", "", "Filter by source preset: life, deepwork/atracker, timeline/location, all")
	query := fs.String("q", "", "Filter by query")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var excludeTags []string
	if *excludeTag != "" {
		for _, t := range strings.Split(*excludeTag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				excludeTags = append(excludeTags, t)
			}
		}
	}

	filterTag := *tag
	src := strings.ToLower(strings.TrimSpace(*source))
	if *life || src == "life" {
		excludeTags = append(excludeTags, "timeline", "atracker", "deepwork")
	} else if src == "atracker" || src == "deepwork" {
		if filterTag == "" {
			filterTag = "atracker"
		}
	} else if src == "timeline" || src == "location" {
		if filterTag == "" {
			filterTag = "timeline"
		}
	}

	evs, total, err := events.ListEvents(database, events.ListOptions{
		Limit:       *limit,
		Tag:         filterTag,
		ExcludeTags: excludeTags,
		Query:       *query,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Events (%d total) ===%s\n", config.Bold, total, config.Reset)
	for _, e := range evs {
		dateStr := e.StartDate
		if dateStr == "" {
			dateStr = e.RawDate
		}
		fmt.Fprintf(stdout, "#%d | %s | %s%s%s", e.ID, dateStr, config.Cyan, e.Title, config.Reset)
		if e.Place != "" {
			fmt.Fprintf(stdout, " @ %s", e.Place)
		}
		if len(e.Contacts) > 0 {
			var names []string
			for _, c := range e.Contacts {
				names = append(names, c.Name)
			}
			fmt.Fprintf(stdout, " [%s]", strings.Join(names, ", "))
		}
		fmt.Fprintln(stdout)
	}
	return nil
}

func handleSearchEvents(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "Max results")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := strings.Join(fs.Args(), " ")
	if query == "" {
		fmt.Fprintf(stderr, "%sError: search query required%s\n", config.Red, config.Reset)
		return fmt.Errorf("search query required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	results, err := events.SearchEvents(database, query, *limit)
	if err != nil {
		fmt.Fprintf(stderr, "%sError searching: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sSearch results for '%s' (%d hits):%s\n", config.Bold, query, len(results), config.Reset)
	for _, r := range results {
		fmt.Fprintf(stdout, "  #%d | %s | %s%s%s (rank: %.2f)\n", r.ID, r.StartDate, config.Cyan, r.Title, config.Reset, r.Rank)
		if r.NotesSnip != "" {
			fmt.Fprintf(stdout, "     Snippet: %s\n", r.NotesSnip)
		}
	}
	return nil
}

func handleDeleteEvent(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	idFlag := fs.Int("id", 0, "Event ID to delete")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var targetID int
	if *idFlag > 0 {
		targetID = *idFlag
	} else if len(fs.Args()) > 0 {
		id, err := strconv.Atoi(fs.Args()[0])
		if err != nil {
			fmt.Fprintf(stderr, "%sError: invalid event ID '%s'%s\n", config.Red, fs.Args()[0], config.Reset)
			return fmt.Errorf("invalid event ID: %s", fs.Args()[0])
		}
		targetID = id
	} else {
		fmt.Fprintf(stderr, "%sError: event ID is required (e.g. ierp delete <id> or ierp delete -id <id>)%s\n", config.Red, config.Reset)
		return fmt.Errorf("event ID is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	deleted, err := events.DeleteEvent(database, targetID)
	if err != nil {
		fmt.Fprintf(stderr, "%sFailed to delete event #%d: %v%s\n", config.Red, targetID, err, config.Reset)
		return err
	}

	if !deleted {
		fmt.Fprintf(stdout, "%sEvent #%d not found%s\n", config.Yellow, targetID, config.Reset)
		return nil
	}

	fmt.Fprintf(stdout, "%sEvent #%d deleted successfully.%s\n", config.Green, targetID, config.Reset)
	return nil
}
