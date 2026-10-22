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

	dbPath := DBPath()
	if dbPath == "" {
		t.Errorf("expected non-empty default DBPath")
	}

	mediaDir := MediaDir()
	if mediaDir == "" {
		t.Errorf("expected non-empty default MediaDir")
	}
}
