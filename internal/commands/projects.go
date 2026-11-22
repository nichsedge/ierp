package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/projects"
)

func handleInsertProject(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-project", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "Project title (required)")
	slug := fs.String("slug", "", "Unique slug")
	desc := fs.String("desc", "", "Description")
	priority := fs.String("priority", "medium", "Priority (low, medium, high)")
	start := fs.String("start", "", "Start date (YYYY-MM-DD)")
	target := fs.String("target", "", "Target date (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *title == "" {
		fmt.Fprintf(stderr, "%sError: --title is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("title is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := projects.InsertProject(database, projects.Project{
		Title:       *title,
		Slug:        *slug,
		Description: *desc,
		Priority:    *priority,
		StartDate:   *start,
		TargetDate:  *target,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sProject #%d saved: %s%s\n", config.Green, id, *title, config.Reset)
	return nil
}

func handleListProjects(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.String("status", "active", "Filter status (active, completed, all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	projs, err := projects.ListProjects(database, *status)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Strategic Projects (%d items) ===%s\n", config.Bold, len(projs), config.Reset)
	for _, p := range projs {
		fmt.Fprintf(stdout, "#%-3d | %-26s | %-8s | %-8s | %s\n", p.ID, p.Title, p.Priority, p.Status, p.Slug)
	}
	return nil
}
