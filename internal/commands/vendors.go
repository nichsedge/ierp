package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/vendors"
)

func handleInsertVendor(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-vendor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Vendor name (required)")
	cat := fs.String("category", "general", "Category")
	location := fs.String("location", "", "Location")
	phone := fs.String("phone", "", "Phone")
	email := fs.String("email", "", "Email")
	url := fs.String("url", "", "URL")
	fav := fs.Bool("favorite", false, "Mark as favorite")
	notes := fs.String("notes", "", "Notes")
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

	id, err := vendors.InsertVendor(database, vendors.Vendor{
		Name:     *name,
		Category: *cat,
		Location: *location,
		Phone:    *phone,
		Email:    *email,
		URL:      *url,
		Favorite: *fav,
		Notes:    *notes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sVendor #%d registered: %s%s\n", config.Green, id, *name, config.Reset)
	return nil
}

func handleListVendors(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("vendors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cat := fs.String("category", "", "Filter category")
	favOnly := fs.Bool("favorites", false, "Favorites only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := vendors.ListVendors(database, *cat, *favOnly)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Merchants & Vendors (%d items) ===%s\n", config.Bold, len(list), config.Reset)
	for _, v := range list {
		favBadge := " "
		if v.Favorite {
			favBadge = "★"
		}
		fmt.Fprintf(stdout, "%s #%-3d | %-24s | %-15s | %s\n", favBadge, v.ID, v.Name, v.Category, v.Location)
	}
	return nil
}
