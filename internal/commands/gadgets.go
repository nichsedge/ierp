package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/gadgets"
)

func handleInsertGadget(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("insert-gadget", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Gadget name (required)")
	brand := fs.String("brand", "", "Brand")
	model := fs.String("model", "", "Model")
	cat := fs.String("category", "computing", "Category")
	price := fs.Float64("price", 0, "Purchase price")
	date := fs.String("date", "", "Purchase date (YYYY-MM-DD)")
	serial := fs.String("serial", "", "Serial number")
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

	id, err := gadgets.InsertGadget(database, gadgets.Gadget{
		Name:          *name,
		Brand:         *brand,
		Model:         *model,
		Category:      *cat,
		PurchasePrice: *price,
		PurchaseDate:  *date,
		SerialNumber:  *serial,
		IsPublic:      true,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%sHardware asset #%d saved: %s%s\n", config.Green, id, *name, config.Reset)
	return nil
}

func handleListGadgets(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gadgets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cat := fs.String("category", "", "Filter category")
	status := fs.String("status", "active", "Filter status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	list, err := gadgets.ListGadgets(database, *cat, *status)
	if err != nil {
		fmt.Fprintf(stderr, "%sError: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	fmt.Fprintf(stdout, "%s=== Hardware Assets (%d items) ===%s\n", config.Bold, len(list), config.Reset)
	for _, g := range list {
		fmt.Fprintf(stdout, "#%-3d | %-24s | %-12s | %-12s | %s %.2f\n", g.ID, g.Name, g.Brand, g.Category, g.Currency, g.PurchasePrice)
	}
	return nil
}
