package commands

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/db"
)

func handleInitDB(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init-db", flag.ContinueOnError)
	fs.SetOutput(stderr)
	verbose := fs.Bool("verbose", true, "Print detailed status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := db.InitDB(config.DBPath(), *verbose); err != nil {
		fmt.Fprintf(stderr, "%sFailed to initialize database: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	return nil
}
