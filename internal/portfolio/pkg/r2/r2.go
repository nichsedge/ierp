package r2

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// GetSignatureKey derives AWS SigV4 signing key.
func GetSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kDate := HmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := HmacSHA256(kDate, []byte(regionName))
	kService := HmacSHA256(kRegion, []byte(serviceName))
	kSigning := HmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// HmacSHA256 calculates HMAC-SHA256 for key and data.
func HmacSHA256(key []byte, data []byte) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write(data)
	return hash.Sum(nil)
}

// Sha256Hex returns SHA256 hex string for data.
func Sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// UploadFile uploads a local file to Cloudflare R2 using S3 SigV4 PUT with wrangler fallback.
func UploadFile(accountID, accessKey, secretKey, bucket, localPath, r2Key string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", accountID)
	endpointURL := fmt.Sprintf("https://%s/%s/%s", host, bucket, r2Key)

	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	region := "auto"
	service := "s3"
	payloadHash := Sha256Hex(data)

	canonicalURI := fmt.Sprintf("/%s/%s", bucket, r2Key)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := fmt.Sprintf("PUT\n%s\n\n%s\n%s\n%s", canonicalURI, canonicalHeaders, signedHeaders, payloadHash)

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, Sha256Hex([]byte(canonicalRequest)))

	signingKey := GetSignatureKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(HmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest(http.MethodPut, endpointURL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Host", host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		body, _ := io.ReadAll(resp.Body)
		if _, pathErr := exec.LookPath("wrangler"); pathErr == nil {
			cmd := exec.Command("wrangler", "r2", "object", "put", fmt.Sprintf("%s/%s", bucket, r2Key), fmt.Sprintf("--file=%s", localPath), "--remote")
			if wErr := cmd.Run(); wErr == nil {
				return nil
			}
		}
		return fmt.Errorf("R2 PUT %s failed (status %d): %s", r2Key, resp.StatusCode, string(body))
	}

	if _, pathErr := exec.LookPath("wrangler"); pathErr == nil {
		cmd := exec.Command("wrangler", "r2", "object", "put", fmt.Sprintf("%s/%s", bucket, r2Key), fmt.Sprintf("--file=%s", localPath), "--remote")
		return cmd.Run()
	}
	return fmt.Errorf("failed to upload to R2 (%s): %w", r2Key, err)
}

// DownloadFile downloads a file from Cloudflare R2 using S3 SigV4 GET with wrangler fallback.
func DownloadFile(accountID, accessKey, secretKey, bucket, r2Key, destPath string) error {
	host := fmt.Sprintf("%s.r2.cloudflarestorage.com", accountID)
	endpointURL := fmt.Sprintf("https://%s/%s/%s", host, bucket, r2Key)

	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	region := "auto"
	service := "s3"
	payloadHash := Sha256Hex([]byte("")) // empty payload for GET

	canonicalURI := fmt.Sprintf("/%s/%s", bucket, r2Key)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := fmt.Sprintf("GET\n%s\n\n%s\n%s\n%s", canonicalURI, canonicalHeaders, signedHeaders, payloadHash)

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, Sha256Hex([]byte(canonicalRequest)))

	signingKey := GetSignatureKey(secretKey, dateStamp, region, service)
	signature := hex.EncodeToString(HmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest(http.MethodGet, endpointURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Host", host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			_ = os.MkdirAll(filepath.Dir(destPath), 0755)
			out, createErr := os.Create(destPath)
			if createErr != nil {
				return createErr
			}
			defer out.Close()

			_, copyErr := io.Copy(out, resp.Body)
			return copyErr
		}
		body, _ := io.ReadAll(resp.Body)
		if _, pathErr := exec.LookPath("wrangler"); pathErr == nil {
			cmd := exec.Command("wrangler", "r2", "object", "get", fmt.Sprintf("%s/%s", bucket, r2Key), fmt.Sprintf("--file=%s", destPath), "--remote")
			if wErr := cmd.Run(); wErr == nil {
				return nil
			}
		}
		return fmt.Errorf("R2 GET %s failed (status %d): %s", r2Key, resp.StatusCode, string(body))
	}

	if _, pathErr := exec.LookPath("wrangler"); pathErr == nil {
		cmd := exec.Command("wrangler", "r2", "object", "get", fmt.Sprintf("%s/%s", bucket, r2Key), fmt.Sprintf("--file=%s", destPath), "--remote")
		return cmd.Run()
	}
	return fmt.Errorf("failed to download from R2 (%s): %w", r2Key, err)
}
