package r2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Checkpoint non-existent DB should not error
	if err := CheckpointDB(dbPath); err != nil {
		t.Fatalf("CheckpointDB on non-existent file failed: %v", err)
	}

	// Create dummy sqlite DB
	if err := os.WriteFile(dbPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSha256Hex(t *testing.T) {
	data := []byte("hello world")
	hash := sha256Hex(data)
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if hash != expected {
		t.Errorf("expected %s, got %s", expected, hash)
	}
}
