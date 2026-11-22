// R2/S3 uploader — stdlib SigV4 PUT. No AWS SDK.
package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// R2Client uploads objects to a Cloudflare R2 (or any S3-compatible) bucket.
type R2Client struct {
	AccountID string
	AccessKey string
	SecretKey string
	Bucket    string
	// Endpoint, if empty, defaults to https://<AccountID>.r2.cloudflarestorage.com
	Endpoint string
}

func NewR2(accountID, accessKey, secretKey, bucket string) *R2Client {
	return &R2Client{
		AccountID: accountID,
		AccessKey: accessKey,
		SecretKey: secretKey,
		Bucket:    bucket,
		Endpoint:  "https://" + accountID + ".r2.cloudflarestorage.com",
	}
}

var r2Client = &http.Client{Timeout: 60 * time.Second}

// Put uploads body to the given key (e.g. "2026/11/abc.jpg").
// region "auto" matches Cloudflare R2.
func (c *R2Client) Put(ctx context.Context, key, contentType string, body []byte) error {
	if c == nil {
		return nil
	}
	key = strings.TrimLeft(key, "/")
	u := c.Endpoint + "/" + url.PathEscape(c.Bucket) + "/" + encodePath(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", contentType)
	payloadHash := sha256hex(body)
	if err := signV4(req, c.AccessKey, c.SecretKey, "auto", "s3", payloadHash, time.Now().UTC()); err != nil {
		return err
	}
	resp, err := r2Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("R2 PUT %s: %d %s", key, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// TryPut uploads but only logs on error. Used from Save to avoid breaking
// local storage if R2 is temporarily unreachable.
func (c *R2Client) TryPut(ctx context.Context, key, contentType string, body []byte) {
	if c == nil {
		return
	}
	if err := c.Put(ctx, key, contentType, body); err != nil {
		log.Printf("R2 upload %q gagal: %v", key, err)
	}
}

// ---- SigV4 (stdlib only) ----

func signV4(req *http.Request, accessKey, secretKey, region, service, payloadHash string, now time.Time) error {
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Host", req.URL.Host)

	// Canonical request
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := req.URL.Query().Encode()

	// Signed headers (sorted): host, x-amz-content-sha256, x-amz-date, content-type (if present)
	var hdrs []string
	if req.Header.Get("Content-Type") != "" {
		hdrs = append(hdrs, "content-type")
	}
	hdrs = append(hdrs, "host", "x-amz-content-sha256", "x-amz-date")
	// already sorted since host < x-amz-*
	// but content-type < host alphabetically — so prepending above is fine
	var canonHdrs strings.Builder
	for _, h := range hdrs {
		var v string
		switch h {
		case "host":
			v = req.URL.Host
		default:
			v = strings.TrimSpace(req.Header.Get(h))
		}
		canonHdrs.WriteString(h + ":" + v + "\n")
	}
	signedHeaders := strings.Join(hdrs, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonHdrs.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	credScope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credScope,
		sha256hex([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credScope, signedHeaders, signature))
	return nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// encodePath escapes each path segment but keeps '/'. url.PathEscape escapes '/' too.
func encodePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
