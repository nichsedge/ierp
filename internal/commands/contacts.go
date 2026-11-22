package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/contacts"
)

func handleListContacts(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("contacts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 50, "Limit")
	q := fs.String("q", "", "Search query")
	tierVal := fs.Int("tier", -1, "Filter by tier (0-3)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var t *int
	if *tierVal >= 0 {
		t = tierVal
	}

	list, total, err := contacts.ListContacts(database, contacts.ListOptions{
		Limit: *limit,
		Query: *q,
		Tier:  t,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Contacts (%d total) ===%s\n", config.Bold, total, config.Reset)
	for _, c := range list {
		tierBadge := fmt.Sprintf("Tier %d (%dd)", c.Tier, c.CadenceDays)
		fmt.Fprintf(stdout, "#%-3d | %-24s | %-12s | %-15s | %d events\n", c.ID, c.Name, tierBadge, c.Org, c.EventCount)
	}
	return nil
}

func handleInsertContact(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-contact", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Contact name (required)")
	org := fs.String("org", "", "Organization/Company")
	client := fs.String("client", "", "Client relation")
	location := fs.String("location", "", "Location")
	email := fs.String("email", "", "Email")
	phone := fs.String("phone", "", "Phone")
	notes := fs.String("notes", "", "Notes")
	tier := fs.Int("tier", 0, "Dunbar tier (0=Untracked, 1=14d, 2=60d, 3=180d)")
	cadence := fs.Int("cadence", 0, "Custom touch cadence in days")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		fmt.Fprintf(stderr, "%sError: --name is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("name is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	id, err := contacts.InsertContact(database, contacts.Contact{
		Name:        *name,
		Org:         *org,
		Client:      *client,
		Location:    *location,
		Email:       *email,
		Phone:       *phone,
		Notes:       *notes,
		Tier:        *tier,
		CadenceDays: *cadence,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sContact #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
	return nil
}

func handleSetTier(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("set-tier", flag.ContinueOnError)
	fs.SetOutput(stderr)
	contactID := fs.Int("contact-id", 0, "Contact ID (required)")
	tier := fs.Int("tier", 0, "Dunbar tier (0-3)")
	cadence := fs.Int("cadence", 0, "Cadence days")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *contactID <= 0 {
		fmt.Fprintf(stderr, "%sError: --contact-id is required%s\n", config.Red, config.Reset)
		return fmt.Errorf("contact-id is required")
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	days := *cadence
	if days <= 0 {
		days = contacts.DefaultCadenceByTier[*tier]
	}

	updates := map[string]any{
		"tier":         *tier,
		"cadence_days": days,
	}
	ok, err := contacts.UpdateContact(database, *contactID, updates)
	if err != nil || !ok {
		fmt.Fprintf(stderr, "%sFailed to update contact #%d: %v%s\n", config.Red, *contactID, err, config.Reset)
		return fmt.Errorf("failed to update contact #%d", *contactID)
	}

	fmt.Fprintf(stdout, "%sUpdated contact #%d: Tier %d (%d days cadence)%s\n", config.Green, *contactID, *tier, days, config.Reset)
	return nil
}
