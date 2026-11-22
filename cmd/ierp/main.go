package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nichsedge/ierp/internal/commands"
	"github.com/nichsedge/ierp/internal/portfolio"
)

func main() {
	if filepath.Base(os.Args[0]) == "portfolio" {
		portfolio.RunCLI(os.Args[1:])
		return
	}

	if err := commands.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
