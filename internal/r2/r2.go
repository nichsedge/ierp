package r2

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DefaultBucket = "ichsanul-dev"
	DefaultKey    = "db/ierp_latest.sqlite"
)

// Credentials holds Cloudflare R2 / S3 credentials.
type Credentials struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
}

// ObjectMeta represents remote object metadata.
type ObjectMeta struct {
	Size         int64
	ETag         string
	LastModified time.Time
}

// SyncState tracks the last synced ETag and timestamp locally.
type SyncState struct {
	LastSyncedETag string    `json:"last_synced_etag"`
	LastSyncedTime string    `json:"last_synced_time"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// LoadCredentials finds R2 credentials from environment, ~/.secrets, or local candidate files.
func LoadCredentials() (Credentials, error) {
	c := Credentials{
		AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		BucketName:      os.Getenv("R2_BUCKET_NAME"),
	}

	if c.AccountID == "" {
		c.AccountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	}
	if c.AccessKeyID == "" {
		c.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if c.SecretAccessKey == "" {
		c.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if c.BucketName == "" {
		c.BucketName = DefaultBucket
	}

	if c.AccountID != "" && c.AccessKeyID != "" && c.SecretAccessKey != "" {
		return c, nil
	}

	// Try reading ~/.secrets
	home, _ := os.UserHomeDir()
	secretsPath := filepath.Join(home, ".secrets")
	if f, err := os.Open(secretsPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "export ") {
				line = strings.TrimPrefix(line, "export ")
				k, v, found := strings.Cut(line, "=")
				if found {
					k = strings.TrimSpace(k)
					v = strings.Trim(strings.TrimSpace(v), `"'`)
					switch k {
					case "R2_ACCOUNT_ID", "CLOUDFLARE_ACCOUNT_ID":
						if c.AccountID == "" {
							c.AccountID = v
						}
					case "R2_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID":
						if c.AccessKeyID == "" {
							c.AccessKeyID = v
						}
					case "R2_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY":
						if c.SecretAccessKey == "" {
							c.SecretAccessKey = v
						}
					case "R2_BUCKET_NAME":
						if c.BucketName == DefaultBucket {
							c.BucketName = v
						}
					}
				}
			}
		}
		f.Close()
	}

	if c.AccountID != "" && c.AccessKeyID != "" && c.SecretAccessKey != "" {
		return c, nil
	}

	// Candidate JSON paths
	candidates := []string{
		filepath.Join(home, "Projects", "creds", "cloudflare", "r2_cred.json"),
		filepath.Join(home, "Projects", "sansfinance", "app", "src", "main", "assets", "r2_cred.json"),
		filepath.Join(home, "Projects", "fitly", "android", "app", "src", "main", "assets", "r2_cred.json"),
	}

	for _, cand := range candidates {
		data, err := os.ReadFile(cand)
		if err == nil {
			var credFile struct {
				AccountID string `json:"accountId"`
				AccessKey string `json:"accessKeyId"`
				SecretKey string `json:"secretAccessKey"`
				Bucket    string `json:"bucketName"`
			}
			if err := json.Unmarshal(data, &credFile); err == nil {
				if c.AccountID == "" {
					c.AccountID = credFile.AccountID
				}
				if c.AccessKeyID == "" {
					c.AccessKeyID = credFile.AccessKey
				}
				if c.SecretAccessKey == "" {
					c.SecretAccessKey = credFile.SecretKey
				}
				if credFile.Bucket != "" && c.BucketName == DefaultBucket {
					c.BucketName = credFile.Bucket
				}
				if c.AccountID != "" && c.AccessKeyID != "" && c.SecretAccessKey != "" {
					return c, nil
				}
			}
		}
	}

	return c, errors.New("Cloudflare R2 credentials not found. Export R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY")
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func s3Request(ctx context.Context, method, key string, creds Credentials, payload []byte) (*http.Request, error) {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", creds.AccountID)
	endpointURL := fmt.Sprintf("https://%s/%s/%s", host, creds.BucketName, key)

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	payloadHash := sha256Hex(payload)
	canonicalURI := fmt.Sprintf("/%s/%s", creds.BucketName, key)

	var canonicalHeaders string
	var signedHeaders string

	if method == "PUT" {
		canonicalHeaders = fmt.Sprintf("content-type:application/x-sqlite3\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
			host, payloadHash, amzDate)
		signedHeaders = "content-type;host;x-amz-content-sha256;x-amz-date"
	} else {
		canonicalHeaders = fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
			host, payloadHash, amzDate)
		signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	}

	canonicalRequest := fmt.Sprintf("%s\n%s\n\n%s\n%s\n%s", method, canonicalURI, canonicalHeaders, signedHeaders, payloadHash)
	credentialScope := fmt.Sprintf("%s/auto/s3/aws4_request", dateStamp)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, sha256Hex([]byte(canonicalRequest)))

	signingKey := getSignatureKey(creds.SecretAccessKey, dateStamp, "auto", "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaders, signature)

	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpointURL, body)
	if err != nil {
		return nil, err
	}

	if method == "PUT" {
		req.Header.Set("Content-Type", "application/x-sqlite3")
	}
	req.Header.Set("Host", host)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("Authorization", authHeader)

	return req, nil
}

// GetRemoteMetadata fetches object metadata from R2 via HEAD.
func GetRemoteMetadata(ctx context.Context, creds Credentials, key string) (*ObjectMeta, error) {
	req, err := s3Request(ctx, http.MethodHead, key, creds, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("R2 HEAD returned HTTP %d", resp.StatusCode)
	}

	etag := strings.Trim(resp.Header.Get("ETag"), `"`)
	var modTime time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := time.Parse(time.RFC1123, lm); err == nil {
			modTime = t.UTC()
		}
	}

	return &ObjectMeta{
		Size:         resp.ContentLength,
		ETag:         etag,
		LastModified: modTime,
	}, nil
}

// CheckpointDB flushes SQLite WAL journal to the main file.
func CheckpointDB(dbPath string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	return err
}

func fileMD5(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := md5.Sum(data)
	return hex.EncodeToString(h[:]), nil
}

func getStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ierp_sync_state.json")
}

func loadState() SyncState {
	var st SyncState
	data, err := os.ReadFile(getStatePath())
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func saveState(etag, mtimeISO string) {
	st := SyncState{
		LastSyncedETag: etag,
		LastSyncedTime: mtimeISO,
		UpdatedAt:      time.Now().UTC(),
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err == nil {
		_ = os.WriteFile(getStatePath(), data, 0644)
	}
}

// Push uploads the local SQLite DB to R2 after WAL checkpointing.
func Push(ctx context.Context, creds Credentials, dbPath, key string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("local database not found at %s", dbPath)
	}

	_ = CheckpointDB(dbPath)

	data, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("failed to read database: %w", err)
	}

	localMD5 := fmt.Sprintf("%x", md5.Sum(data))

	// Backup locally first
	home, _ := os.UserHomeDir()
	backupDir := filepath.Join(home, "Projects", "ierp", "backups")
	_ = os.MkdirAll(backupDir, 0755)
	latestBackup := filepath.Join(backupDir, "events_backup_latest.sqlite")
	prevBackup := filepath.Join(backupDir, "events_backup_previous.sqlite")
	if _, err := os.Stat(latestBackup); err == nil {
		_ = copyFile(latestBackup, prevBackup)
	}
	_ = copyFile(dbPath, latestBackup)

	req, err := s3Request(ctx, http.MethodPut, key, creds, data)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload to R2: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("R2 PUT returned HTTP %d", resp.StatusCode)
	}

	// Fetch remote metadata to align timestamp and record state
	meta, err := GetRemoteMetadata(ctx, creds, key)
	if err == nil && meta != nil && !meta.LastModified.IsZero() {
		_ = os.Chtimes(dbPath, meta.LastModified, meta.LastModified)
		saveState(meta.ETag, meta.LastModified.Format(time.RFC3339))
	} else {
		saveState(localMD5, time.Now().UTC().Format(time.RFC3339))
	}

	fmt.Printf("🚀 PUSH SUCCESS: Local events.db (%d bytes | MD5: %.8s) uploaded to Cloudflare R2 (%s).\n",
		len(data), localMD5, key)
	return nil
}

// Pull downloads the remote SQLite DB from R2 to local path.
func Pull(ctx context.Context, creds Credentials, dbPath, key string) error {
	meta, err := GetRemoteMetadata(ctx, creds, key)
	if err != nil {
		return fmt.Errorf("failed checking remote metadata: %w", err)
	}
	if meta == nil {
		return fmt.Errorf("remote R2 object %s does not exist in bucket %s", key, creds.BucketName)
	}

	home, _ := os.UserHomeDir()
	backupDir := filepath.Join(home, "Projects", "ierp", "backups")
	_ = os.MkdirAll(backupDir, 0755)
	if _, err := os.Stat(dbPath); err == nil {
		backupBefore := filepath.Join(backupDir, "events_backup_before_pull.sqlite")
		_ = copyFile(dbPath, backupBefore)
	}

	req, err := s3Request(ctx, http.MethodGet, key, creds, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed downloading from R2: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("R2 GET returned HTTP %d", resp.StatusCode)
	}

	remoteBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed reading R2 response: %w", err)
	}

	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
	tmpFile := dbPath + ".tmp"
	if err := os.WriteFile(tmpFile, remoteBytes, 0644); err != nil {
		return err
	}

	// Verify SQLite integrity
	testDB, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("downloaded database failed to open: %w", err)
	}
	var integrity string
	_ = testDB.QueryRow("PRAGMA integrity_check;").Scan(&integrity)
	testDB.Close()

	if integrity != "ok" {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("integrity check failed for downloaded DB: %s", integrity)
	}

	if err := os.Rename(tmpFile, dbPath); err != nil {
		return err
	}

	if !meta.LastModified.IsZero() {
		_ = os.Chtimes(dbPath, meta.LastModified, meta.LastModified)
		saveState(meta.ETag, meta.LastModified.Format(time.RFC3339))
	}

	fmt.Printf("📥 PULL SUCCESS: Cloudflare R2 snapshot (%d bytes | ETag: %.8s) downloaded to %s.\n",
		len(remoteBytes), meta.ETag, dbPath)
	return nil
}

// Status inspects local vs remote sync state.
func Status(ctx context.Context, creds Credentials, dbPath, key string) error {
	_ = CheckpointDB(dbPath)

	remote, err := GetRemoteMetadata(ctx, creds, key)
	if err != nil {
		return err
	}

	st := loadState()

	fmt.Println("═════════════════════════════════════════════════════════════════")
	fmt.Println("📊 iERP Cloudflare R2 Synchronization Status (Go)")
	fmt.Println("═════════════════════════════════════════════════════════════════")
	fmt.Printf("Bucket:  %s\n", creds.BucketName)
	fmt.Printf("Object:  %s\n", key)
	fmt.Println("─────────────────────────────────────────────────────────────────")

	var localExists bool
	var localSize int64
	var localMTime time.Time
	var localMD5 string

	if fi, err := os.Stat(dbPath); err == nil {
		localExists = true
		localSize = fi.Size()
		localMTime = fi.ModTime().UTC()
		localMD5, _ = fileMD5(dbPath)
		fmt.Printf("Local DB:   %d bytes | MD5: %.8s | Modified: %s\n",
			localSize, localMD5, localMTime.Format("2006-01-02 15:04:05 UTC"))
	} else {
		fmt.Println("Local DB:   (does not exist)")
	}

	if remote != nil {
		fmt.Printf("Remote R2:  %d bytes | ETag: %.8s | Modified: %s\n",
			remote.Size, remote.ETag, remote.LastModified.Format("2006-01-02 15:04:05 UTC"))
	} else {
		fmt.Println("Remote R2:  (not found)")
	}
	fmt.Println("═════════════════════════════════════════════════════════════════")

	if remote == nil && localExists {
		fmt.Println("Status: 🚀 LOCAL ONLY (Remote missing, run 'push' to seed R2)")
	} else if remote != nil && !localExists {
		fmt.Println("Status: 📥 REMOTE ONLY (Local missing, run 'pull' to restore)")
	} else if remote != nil && localExists {
		if strings.EqualFold(localMD5, remote.ETag) {
			fmt.Println("Status: ✅ IN SYNC (Local database is byte-for-byte identical to Cloudflare R2)")
		} else if st.LastSyncedETag != "" && strings.EqualFold(st.LastSyncedETag, remote.ETag) {
			fmt.Println("Status: ⬆️ LOCAL NEWER (You made changes locally since last sync)")
		} else if !localMTime.IsZero() && !remote.LastModified.IsZero() {
			diff := localMTime.Sub(remote.LastModified).Seconds()
			if diff > 60 {
				fmt.Println("Status: ⬆️ LOCAL NEWER (Local timestamp is newer than R2)")
			} else if diff < -60 {
				fmt.Println("Status: ⬇️ REMOTE NEWER (Cloudflare R2 has newer data from another device)")
			} else {
				fmt.Println("Status: 🔄 DIVERGED (Hashes differ, timestamps within 60s)")
			}
		} else {
			fmt.Println("Status: 🔄 DIVERGED (Hashes differ)")
		}
	}
	return nil
}

// AutoSync compares hashes and timestamps and safely pulls or pushes.
func AutoSync(ctx context.Context, creds Credentials, dbPath, key string) error {
	_ = CheckpointDB(dbPath)

	remote, err := GetRemoteMetadata(ctx, creds, key)
	if err != nil {
		return err
	}

	fi, err := os.Stat(dbPath)
	localExists := err == nil

	if remote == nil {
		if localExists {
			fmt.Println("Remote R2 snapshot does not exist. Pushing local DB...")
			return Push(ctx, creds, dbPath, key)
		}
		return errors.New("neither local nor remote database exists")
	}

	if !localExists {
		fmt.Println("Local database does not exist. Pulling from R2...")
		return Pull(ctx, creds, dbPath, key)
	}

	localMD5, _ := fileMD5(dbPath)
	if strings.EqualFold(localMD5, remote.ETag) {
		fmt.Println("✅ Already in sync with Cloudflare R2 (MD5 matches ETag).")
		if !remote.LastModified.IsZero() {
			saveState(remote.ETag, remote.LastModified.Format(time.RFC3339))
		}
		return nil
	}

	st := loadState()
	if st.LastSyncedETag != "" && strings.EqualFold(st.LastSyncedETag, remote.ETag) {
		fmt.Println("Local DB was modified since last sync. Pushing to R2...")
		return Push(ctx, creds, dbPath, key)
	}

	localMTime := fi.ModTime().UTC()
	diff := localMTime.Sub(remote.LastModified).Seconds()

	if diff > 60 {
		fmt.Printf("Local DB is newer (%.0fs diff). Pushing to R2...\n", diff)
		return Push(ctx, creds, dbPath, key)
	} else if diff < -60 {
		fmt.Printf("Remote R2 snapshot is newer (%.0fs diff). Pulling from R2...\n", -diff)
		return Pull(ctx, creds, dbPath, key)
	}

	// Tiebreaker: compare event timestamps
	localMaxEvent := getMaxEventTime(dbPath)
	if localMaxEvent != "" {
		fmt.Printf("Hashes differ with close timestamps. Pushing local DB to establish sync...\n")
		return Push(ctx, creds, dbPath, key)
	}

	return fmt.Errorf("databases diverged without clear winner; check 'ierp r2 status'")
}

func getMaxEventTime(dbPath string) string {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	var maxT sql.NullString
	_ = db.QueryRow("SELECT max(created_at) FROM events;").Scan(&maxT)
	return maxT.String
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
