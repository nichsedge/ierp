package r2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestUploadAndDownloadFile(t *testing.T) {
	stored := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			stored[r.URL.Path] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			data, ok := stored[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	creds := Credentials{
		AccountID:        "acc123",
		AccessKeyID:      "key123",
		SecretAccessKey:  "sec123",
		BucketName:       "test-bucket",
		EndpointOverride: srv.URL,
	}

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "sample.json")
	if err := os.WriteFile(srcPath, []byte(`{"ok":true}`), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := UploadFile(ctx, creds, srcPath, "portfolio/sample.json", "application/json"); err != nil {
		t.Fatalf("UploadFile failed: %v", err)
	}

	destPath := filepath.Join(tmpDir, "nested", "downloaded.json")
	if err := DownloadFile(ctx, creds, "portfolio/sample.json", destPath); err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("downloaded content mismatch: got %q", string(got))
	}
}
