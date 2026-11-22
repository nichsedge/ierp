package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/r2"
)

func handleR2(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if config.TursoURL() != "" {
		fmt.Fprintf(stdout, "%sℹ️  Tier 1 Cloud SSOT Active: Turso libSQL Cloud is the primary read/write endpoint.%s\n", config.Cyan, config.Reset)
	}

	subcmd := "auto"
	if len(args) > 0 {
		subcmd = args[0]
	}

	creds, err := r2.LoadCredentials()
	if err != nil {
		fmt.Fprintf(stderr, "%sR2 credentials error: %v%s\n", config.Red, err, config.Reset)
		return err
	}

	dbPath := config.LocalDBPath()

	switch subcmd {
	case "status":
		if err := r2.Status(ctx, creds, dbPath, r2.DefaultKey); err != nil {
			fmt.Fprintf(stderr, "%sR2 status failed: %v%s\n", config.Red, err, config.Reset)
			return err
		}
	case "push":
		if err := r2.Push(ctx, creds, dbPath, r2.DefaultKey); err != nil {
			fmt.Fprintf(stderr, "%sR2 push failed: %v%s\n", config.Red, err, config.Reset)
			return err
		}
	case "pull":
		if err := r2.Pull(ctx, creds, dbPath, r2.DefaultKey); err != nil {
			fmt.Fprintf(stderr, "%sR2 pull failed: %v%s\n", config.Red, err, config.Reset)
			return err
		}
	case "auto":
		if err := r2.AutoSync(ctx, creds, dbPath, r2.DefaultKey); err != nil {
			fmt.Fprintf(stderr, "%sR2 auto-sync failed: %v%s\n", config.Red, err, config.Reset)
			return err
		}
	default:
		fmt.Fprintf(stderr, "%sUnknown R2 action: %s. Use status, push, pull, or auto%s\n", config.Red, subcmd, config.Reset)
		return fmt.Errorf("unknown R2 action: %s", subcmd)
	}
	return nil
}
