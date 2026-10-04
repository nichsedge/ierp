package config

import (
	"os"
	"path/filepath"
)

// ANSI terminal colors
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Green   = "\033[32m"
	Cyan    = "\033[36m"
	Yellow  = "\033[33m"
	Red     = "\033[31m"
	Magenta = "\033[35m"
)

// DBPath returns the configured or default database path.
func DBPath() string {
	if p := os.Getenv("IERP_DB"); p != "" {
		abs, err := filepath.Abs(p)
		if err == nil {
			return abs
		}
		return p
	}

	// If events.db exists in current working directory, use it
	if fi, err := os.Stat("events.db"); err == nil && !fi.IsDir() {
		abs, err := filepath.Abs("events.db")
		if err == nil {
			return abs
		}
		return "events.db"
	}

	// Workstation SSOT path
	if home, err := os.UserHomeDir(); err == nil {
		wsDB := filepath.Join(home, "Projects", "ierp", "events.db")
		if fi, err := os.Stat(wsDB); err == nil && !fi.IsDir() {
			return wsDB
		}
		return wsDB
	}

	return "events.db"
}

// MediaDir returns the configured or default media storage directory.
func MediaDir() string {
	if p := os.Getenv("IERP_MEDIA_DIR"); p != "" {
		abs, err := filepath.Abs(p)
		if err == nil {
			return abs
		}
		return p
	}

	if fi, err := os.Stat("events_media"); err == nil && fi.IsDir() {
		abs, err := filepath.Abs("events_media")
		if err == nil {
			return abs
		}
		return "events_media"
	}

	if home, err := os.UserHomeDir(); err == nil {
		wsMedia := filepath.Join(home, "Projects", "ierp", "events_media")
		return wsMedia
	}

	return "events_media"
}
