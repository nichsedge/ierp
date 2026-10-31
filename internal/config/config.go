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
	if u := TursoURL(); u != "" {
		return u
	}
	return LocalDBPath()
}

// LocalDBPath returns the local SQLite database file path, ignoring TursoURL.
func LocalDBPath() string {
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

// TursoURL returns the configured Turso / libSQL database URL if set.
func TursoURL() string {
	if u := os.Getenv("TURSO_DATABASE_URL"); u != "" {
		return u
	}
	return os.Getenv("IERP_TURSO_URL")
}

// TursoAuthToken returns the configured Turso auth token if set.
func TursoAuthToken() string {
	if t := os.Getenv("TURSO_AUTH_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("IERP_TURSO_TOKEN")
}

// IsRemoteDB returns true if the database target is a remote libSQL / Turso instance.
func IsRemoteDB(target string) bool {
	return len(target) > 8 && (target[:9] == "libsql://" || target[:8] == "https://" || target[:7] == "http://")
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
