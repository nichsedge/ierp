package commands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/nichsedge/ierp/internal/config"
	"github.com/nichsedge/ierp/internal/dashboard"
)

func handleDashboard(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	fs.SetOutput(stderr)
	host := fs.String("host", "127.0.0.1", "Host address")
	port := fs.Int("port", 8921, "Port number")
	token := fs.String("token", "", "Dashboard auth token (or set IERP_AUTH_TOKEN)")
	tailscale := fs.Bool("tailscale", false, "Expose dashboard exclusively on Tailscale interface IP")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *tailscale {
		tsIP, err := dashboard.GetTailscaleIP()
		if err != nil {
			return fmt.Errorf("failed to detect Tailscale IP: %w", err)
		}
		*host = tsIP
	}

	authToken := *token
	if authToken == "" {
		authToken = os.Getenv("IERP_AUTH_TOKEN")
		if authToken == "" {
			authToken = os.Getenv("IERP_DASHBOARD_TOKEN")
		}
	}

	if authToken == "" && *host != "127.0.0.1" && *host != "localhost" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		authToken = hex.EncodeToString(b)
		fmt.Fprintf(stderr, "%s[Security]%s Dashboard exposed beyond localhost (%s). Generated auth token: %s%s%s\n",
			config.Yellow, config.Reset, *host, config.Bold, authToken, config.Reset)
		fmt.Fprintf(stderr, "  Access: http://%s:%d/?token=%s\n", *host, *port, authToken)
	}

	database, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	var opts []dashboard.Option
	if authToken != "" {
		opts = append(opts, dashboard.WithAuthToken(authToken))
	}

	server := dashboard.NewServer(database, *host, *port, opts...)
	if err := server.Start(); err != nil {
		fmt.Fprintf(stderr, "%sDashboard server failed: %v%s\n", config.Red, err, config.Reset)
		return err
	}
	return nil
}
