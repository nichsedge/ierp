package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEnvOverrides(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ierp-config-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fakeDB := filepath.Join(tempDir, "custom_events.db")
	fakeMedia := filepath.Join(tempDir, "custom_media")

	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("IERP_TURSO_URL", "")
	t.Setenv("IERP_DB", fakeDB)
	t.Setenv("IERP_MEDIA_DIR", fakeMedia)

	if DBPath() != fakeDB {
		t.Errorf("expected DBPath to be %s, got %s", fakeDB, DBPath())
	}
	if MediaDir() != fakeMedia {
		t.Errorf("expected MediaDir to be %s, got %s", fakeMedia, MediaDir())
	}
}

func TestConfigDefaults(t *testing.T) {
	t.Setenv("IERP_DB", "")
	t.Setenv("IERP_MEDIA_DIR", "")
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("IERP_TURSO_URL", "")

	dbPath := DBPath()
	if dbPath == "" {
		t.Errorf("expected non-empty default DBPath")
	}

	mediaDir := MediaDir()
	if mediaDir == "" {
		t.Errorf("expected non-empty default MediaDir")
	}
}

func TestTursoConfig(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "libsql://test-db.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "test-token")

	if TursoURL() != "libsql://test-db.turso.io" {
		t.Errorf("expected TursoURL to match, got %s", TursoURL())
	}
	if DBPath() != "libsql://test-db.turso.io" {
		t.Errorf("expected DBPath to prioritize TursoURL, got %s", DBPath())
	}
	if TursoAuthToken() != "test-token" {
		t.Errorf("expected TursoAuthToken to match, got %s", TursoAuthToken())
	}
	if !IsRemoteDB(TursoURL()) {
		t.Errorf("expected IsRemoteDB to return true for libsql:// URL")
	}
}
