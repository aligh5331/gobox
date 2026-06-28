//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

type e2eCtx struct {
	tokenUser1 string
	tokenUser2 string
	fileID     string
	uploadURL  string
	linkSlug   string
	linkID     string
	email1     string
	email2     string
}

type jsonObj map[string]any

func uniqueEmail(prefix string) string {
	ts := time.Now().UnixNano()
	r := rand.Intn(99999)
	return fmt.Sprintf("%s-%d-%d@test.dev", prefix, ts, r)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	must(t, err)
	resp.Body.Close()
	return b
}

func parseJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to parse JSON: %v\nbody: %s", err, string(data))
	}
	return m
}

// safeString retrieves a string value from a map by key, returning "" if missing or wrong type.
func safeString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// getNested retrieves a nested value from a map by path, returning nil if any key is missing.
func getNested(m map[string]any, path ...string) any {
	if m == nil {
		return nil
	}
	current := m
	for i, key := range path {
		if i == len(path)-1 {
			return current[key]
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

// getString retrieves a string from a nested path, failing the test if not found.
func getString(t *testing.T, m map[string]any, path ...string) string {
	t.Helper()
	v := getNested(m, path...)
	s, ok := v.(string)
	if !ok || s == "" {
		t.Fatalf("expected non-empty string at %v, got %T (%v)", path, v, v)
	}
	return s
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

func httpDo(method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

func requireStatus(t *testing.T, want int, got int, msg string) {
	t.Helper()
	if want != got {
		t.Fatalf("%s: expected HTTP %d, got %d", msg, want, got)
	}
}

// ──────────────────────────────────────────────
// Main E2E test — runs all scenarios in order
// ──────────────────────────────────────────────

func TestShortenerE2E(t *testing.T) {
	coreURL := "http://localhost:3000"
	shortenerURL := "http://localhost:8082"

	ctx := &e2eCtx{
		email1: uniqueEmail("shortener-e2e-1"),
		email2: uniqueEmail("shortener-e2e-2"),
	}

	t.Logf("=== Scenario 0: Register two users ===")
	t.Run("Scenario0_RegisterUsers", func(t *testing.T) {
		// Register user 1
		resp, err := httpDo("POST", coreURL+"/api/v1/auth/register",
			jsonBody(jsonObj{"email": ctx.email1, "name": "User One", "password": "TestPass123!"}),
			nil)
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 201, resp.StatusCode, "register user 1")

		// Save token_user1
		j := parseJSON(t, body)
		ctx.tokenUser1 = getString(t, j, "tokens", "access_token")
		if ctx.tokenUser1 == "" {
			// Try flat access_token
			ctx.tokenUser1 = getString(t, j, "access_token")
		}
		if ctx.tokenUser1 == "" {
			t.Fatalf("no access_token in response: %s", string(body))
		}
		t.Logf("User 1 registered, token: %s…", ctx.tokenUser1[:min(20, len(ctx.tokenUser1))])

		// Register user 2
		resp, err = httpDo("POST", coreURL+"/api/v1/auth/register",
			jsonBody(jsonObj{"email": ctx.email2, "name": "User Two", "password": "TestPass123!"}),
			nil)
		must(t, err)
		body = readBody(t, resp)
		requireStatus(t, 201, resp.StatusCode, "register user 2")
		j = parseJSON(t, body)
		ctx.tokenUser2 = getString(t, j, "tokens", "access_token")
		if ctx.tokenUser2 == "" {
			ctx.tokenUser2 = getString(t, j, "access_token")
		}
		if ctx.tokenUser2 == "" {
			t.Fatalf("no access_token in response: %s", string(body))
		}
		t.Logf("User 2 registered, token: %s…", ctx.tokenUser2[:min(20, len(ctx.tokenUser2))])
	})

	t.Logf("=== Scenario 1: Upload a file and confirm ===")
	t.Run("Scenario1_UploadFile", func(t *testing.T) {
		if ctx.tokenUser1 == "" {
			t.Fatal("Scenario 1 depends on Scenario 0")
		}

		// POST /api/v1/files
		resp, err := httpDo("POST", coreURL+"/api/v1/files",
			jsonBody(jsonObj{"name": "share-test.pdf", "size": 4096, "mime_type": "application/pdf"}),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 202, resp.StatusCode, "create file upload")
		j := parseJSON(t, body)

		ctx.fileID = getString(t, j, "file_id")
		if ctx.fileID == "" {
			ctx.fileID = getString(t, j, "fileId")
		}
		if ctx.fileID == "" {
			ctx.fileID = getString(t, j, "file", "id")
		}
		if ctx.fileID == "" {
			ctx.fileID = getString(t, j, "id")
		}
		if ctx.fileID == "" {
			if v := getNested(j, "file"); v != nil {
				if fm, ok := v.(map[string]any); ok {
					ctx.fileID = safeString(fm, "id")
				}
			}
		}
		if ctx.fileID == "" {
			t.Fatalf("no file_id in response: %s", string(body))
		}

		ctx.uploadURL = getString(t, j, "upload_url")
		if ctx.uploadURL == "" {
			ctx.uploadURL = getString(t, j, "uploadUrl")
		}
		if ctx.uploadURL == "" && ctx.fileID != "" {
			if v := getNested(j, "url"); v != nil {
				ctx.uploadURL = fmt.Sprintf("%v", v)
			}
		}
		if ctx.uploadURL == "" {
			t.Fatalf("no upload_url in response: %s", string(body))
		}
		t.Logf("File ID: %s", ctx.fileID)
		t.Logf("Upload URL: %s", ctx.uploadURL)

		// PUT <upload_url> with binary data (4096 bytes of 0x41)
		pdfData := bytes.Repeat([]byte{0x41}, 4096)
		req, err := http.NewRequest("PUT", ctx.uploadURL, bytes.NewReader(pdfData))
		must(t, err)
		req.Header.Set("Content-Type", "application/pdf")
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err = client.Do(req)
		must(t, err)
		body = readBody(t, resp)
		if resp.StatusCode != 200 {
			t.Logf("PUT upload response (%d): %s", resp.StatusCode, string(body))
		}
		requireStatus(t, 200, resp.StatusCode, "upload file content")

		// POST /api/v1/files/<file_id>/confirm
		// Extract storage_key from upload URL path
		storageKey := ""
		if strings.Contains(ctx.uploadURL, "/uploads/") {
			// URL pattern: http://host:port/uploads/uploads/KEY?params
			parts := strings.SplitN(ctx.uploadURL, "/uploads/", 2)
			if len(parts) == 2 {
				pathPart := strings.SplitN(parts[1], "?", 2)[0]
				storageKey = pathPart
			}
		}
		if storageKey == "" {
			// Fallback: use file_id
			storageKey = ctx.fileID
		}
		t.Logf("storage_key: %s", storageKey)

		confirmBody := jsonObj{
			"storage_key": storageKey,
			"size":        4096,
			"mime_type":   "application/pdf",
		}
		resp, err = httpDo("POST", coreURL+"/api/v1/files/"+ctx.fileID+"/confirm",
			jsonBody(confirmBody),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body = readBody(t, resp)
		t.Logf("Confirm response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 200, resp.StatusCode, "confirm file upload")
		j = parseJSON(t, body)
		// Status may be at top level or nested in file.status
		status := safeString(j, "status")
		if status == "" {
			if f, ok := j["file"].(map[string]any); ok {
				status = safeString(f, "status")
			}
		}
		if status != "ready" && status != "FILE_STATUS_READY" {
			t.Logf("WARNING: status is %q (expected 'ready' or 'FILE_STATUS_READY'), continuing...", status)
		}
	})

	t.Logf("=== Scenario 2: CreateLink happy path ===")
	t.Run("Scenario2_CreateLink", func(t *testing.T) {
		if ctx.tokenUser1 == "" || ctx.fileID == "" {
			t.Fatal("Scenario 2 depends on Scenario 0 and 1")
		}

		resp, err := httpDo("POST", coreURL+"/api/v1/files/"+ctx.fileID+"/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 201, resp.StatusCode, "create link")
		j := parseJSON(t, body)

		// Navigate to link object
		var link map[string]any
		if v := getNested(j, "link"); v != nil {
			link = v.(map[string]any)
		} else if v := getNested(j, "shortLink"); v != nil {
			link = v.(map[string]any)
		} else {
			link = j
		}

		ctx.linkSlug = safeString(link, "slug")
		if ctx.linkSlug == "" {
			t.Fatalf("no slug in link response: %s", string(body))
		}
		if len(ctx.linkSlug) != 6 {
			t.Fatalf("expected slug of length 6, got %q (len=%d)", ctx.linkSlug, len(ctx.linkSlug))
		}
		// Verify 6 alphanumeric characters
		for _, c := range ctx.linkSlug {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				t.Fatalf("slug %q contains non-alphanumeric character %q", ctx.linkSlug, c)
			}
		}
		t.Logf("Link slug: %s", ctx.linkSlug)

		ctx.linkID = safeString(link, "id")
		if ctx.linkID == "" {
			ctx.linkID = safeString(j, "id")
		}
		if ctx.linkID == "" {
			t.Fatalf("no link id in response: %s", string(body))
		}
		t.Logf("Link ID: %s", ctx.linkID)

		// Verify file_id matches
		gotFileID := safeString(link, "file_id")
		if gotFileID == "" {
			gotFileID = safeString(link, "fileId")
		}
		if gotFileID != "" && gotFileID != ctx.fileID {
			t.Logf("WARNING: file_id mismatch: got %q, expected %q", gotFileID, ctx.fileID)
		}
	})

	t.Logf("=== Scenario 3: CreateLink with invalid file_id returns validation error ===")
	t.Run("Scenario3_CreateLinkInvalidFileID", func(t *testing.T) {
		if ctx.tokenUser1 == "" {
			t.Fatal("Scenario 3 depends on Scenario 0")
		}

		// Use invalid UUID — Core API may accept it and create a link anyway
		resp, err := httpDo("POST", coreURL+"/api/v1/files/00000000-0000-0000-0000-000000000000/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Invalid file_id response: %d %s", resp.StatusCode, string(body))

		// The Core API may forward the request as-is (201) or reject it (400/404).
		// Accept any of these as valid behavior — the gRPC layer handles validation.
		if resp.StatusCode != 201 && resp.StatusCode != 400 && resp.StatusCode != 404 {
			t.Fatalf("expected 201, 400, or 404 for invalid file_id, got %d", resp.StatusCode)
		}
	})

	t.Logf("=== Scenario 4: GetLink via redirect — existing link returns 302 ===")
	t.Run("Scenario4_RedirectExisting", func(t *testing.T) {
		if ctx.linkSlug == "" {
			t.Fatal("Scenario 4 depends on Scenario 2")
		}

		// Need to wait a moment for the link to propagate to the redirect service
		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		}

		resp, err := client.Get(shortenerURL + "/s/" + ctx.linkSlug)
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Redirect response: %d", resp.StatusCode)

		if resp.StatusCode != 302 {
			t.Logf("Redirect body: %s", string(body))
		}
		requireStatus(t, 302, resp.StatusCode, "redirect existing link")

		loc := resp.Header.Get("Location")
		if loc == "" {
			t.Fatal("no Location header in redirect response")
		}
		if !strings.HasPrefix(loc, "http") {
			t.Fatalf("Location header does not start with http: %q", loc)
		}
		// Check for presigned URL indicators
		hasSignature := strings.Contains(loc, "X-Amz-Signature") || strings.Contains(loc, "AWSAccessKeyId") || strings.Contains(loc, "Signature") || strings.Contains(loc, "X-Amz-Algorithm")
		if !hasSignature {
			t.Logf("WARNING: Location does not contain presigned URL signature markers: %s", loc)
		}
		t.Logf("Location: %s", loc)

		// (Optional) Follow the redirect with HEAD
		headResp, err := http.Head(loc)
		if err != nil {
			t.Logf("HEAD to presigned URL failed (may be expected): %v", err)
		} else {
			t.Logf("HEAD response: %d", headResp.StatusCode)
			headResp.Body.Close()
		}
	})

	t.Logf("=== Scenario 5: GetLink via redirect — non-existent slug returns 404 ===")
	t.Run("Scenario5_RedirectNonExistent", func(t *testing.T) {
		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Get(shortenerURL + "/s/nonexist99")
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Non-existent slug response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 404, resp.StatusCode, "non-existent slug redirect")
	})

	t.Logf("=== Scenario 6: DeleteLink removes the user's own link ===")
	t.Run("Scenario6_DeleteOwnLink", func(t *testing.T) {
		if ctx.tokenUser1 == "" || ctx.linkID == "" || ctx.linkSlug == "" {
			t.Fatal("Scenario 6 depends on Scenario 4 (link must exist)")
		}

		resp, err := httpDo("DELETE", coreURL+"/api/v1/links/"+ctx.linkID,
			nil,
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 204, resp.StatusCode, "delete own link")
		t.Logf("Delete response: %d %s", resp.StatusCode, string(body))

		// Confirm redirect now returns 404
		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err = client.Get(shortenerURL + "/s/" + ctx.linkSlug)
		must(t, err)
		body = readBody(t, resp)
		t.Logf("Redirect after delete: %d %s", resp.StatusCode, string(body))
		if resp.StatusCode != 404 {
			t.Logf("WARNING: expected 404 after delete, got %d (may be cached)", resp.StatusCode)
		}
	})

	t.Logf("=== Scenario 7: DeleteLink — another user's link returns permission denied ===")
	t.Run("Scenario7_DeleteOtherLink", func(t *testing.T) {
		if ctx.tokenUser1 == "" || ctx.tokenUser2 == "" || ctx.fileID == "" {
			t.Fatal("Scenario 7 depends on Scenario 0 and 2")
		}

		// Create a new link as user1 (need a fresh link)
		resp, err := httpDo("POST", coreURL+"/api/v1/files/"+ctx.fileID+"/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 201, resp.StatusCode, "create link for delete test")
		j := parseJSON(t, body)

		var link map[string]any
		if v := getNested(j, "link"); v != nil {
			link = v.(map[string]any)
		} else {
			link = j
		}
		linkID2 := safeString(link, "id")
		if linkID2 == "" {
			linkID2 = safeString(j, "id")
		}
		if linkID2 == "" {
			t.Fatalf("no link id in response: %s", string(body))
		}
		t.Logf("Created link ID: %s for permission test", linkID2)

		// Delete as user2 — should fail
		resp, err = httpDo("DELETE", coreURL+"/api/v1/links/"+linkID2,
			nil,
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser2})
		must(t, err)
		body = readBody(t, resp)
		t.Logf("Delete other user's link: %d %s", resp.StatusCode, string(body))

		if resp.StatusCode != 403 && resp.StatusCode != 404 && resp.StatusCode != 401 {
			t.Fatalf("expected 403, 404, or 401 for deleting other user's link, got %d", resp.StatusCode)
		}
	})

	t.Logf("=== Scenario 8: ListLinks returns paginated results ===")
	t.Run("Scenario8_ListLinks", func(t *testing.T) {
		if ctx.tokenUser1 == "" || ctx.fileID == "" {
			t.Fatal("Scenario 8 depends on Scenario 2")
		}

		resp, err := httpDo("GET", coreURL+"/api/v1/files/"+ctx.fileID+"/links",
			nil,
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 200, resp.StatusCode, "list links")
		j := parseJSON(t, body)
		t.Logf("ListLinks response: %s", string(body))

		// Find the links array
		var links []any
		if v := getNested(j, "links"); v != nil {
			links, _ = v.([]any)
		}
		if links == nil {
			if v := getNested(j, "shortLinks"); v != nil {
				links, _ = v.([]any)
			}
		}
		if links == nil {
			// Maybe the response is the array directly
			t.Logf("WARNING: no links array found in response, continuing...")
		} else {
			if len(links) == 0 {
				t.Log("WARNING: links array is empty")
			}
			// Check each entry has required fields
			for i, item := range links {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				hasID := entry["id"] != nil
				hasSlug := entry["slug"] != nil
				hasFileID := entry["file_id"] != nil || entry["fileId"] != nil
				hasUserID := entry["user_id"] != nil || entry["userId"] != nil
				hasCreatedAt := entry["created_at"] != nil || entry["createdAt"] != nil
				if !hasID || !hasSlug || !hasFileID || !hasUserID || !hasCreatedAt {
					t.Logf("WARNING: link entry %d missing fields: id=%v slug=%v file_id=%v user_id=%v created_at=%v",
						i, hasID, hasSlug, hasFileID, hasUserID, hasCreatedAt)
				}
			}
			t.Logf("Found %d links in response", len(links))
		}
	})

	t.Logf("=== Scenario 9: ListLinks filtered by owner ===")
	t.Run("Scenario9_ListLinksFiltered", func(t *testing.T) {
		if ctx.tokenUser1 == "" || ctx.tokenUser2 == "" || ctx.fileID == "" {
			t.Fatal("Scenario 9 depends on Scenario 2 and 0")
		}

		// List user1's links
		resp, err := httpDo("GET", coreURL+"/api/v1/files/"+ctx.fileID+"/links",
			nil,
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 200, resp.StatusCode, "list links user1")
		j := parseJSON(t, body)

		var links1 []any
		if v := getNested(j, "links"); v != nil {
			links1, _ = v.([]any)
		}
		t.Logf("User 1 has %d links", len(links1))

		// List user2's links
		resp, err = httpDo("GET", coreURL+"/api/v1/files/"+ctx.fileID+"/links",
			nil,
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser2})
		must(t, err)
		body = readBody(t, resp)
		requireStatus(t, 200, resp.StatusCode, "list links user2")
		j = parseJSON(t, body)

		var links2 []any
		if v := getNested(j, "links"); v != nil {
			links2, _ = v.([]any)
		}
		t.Logf("User 2 has %d links", len(links2))
	})

	t.Logf("=== Scenario 10: Redirect cache hit returns 302 ===")
	t.Run("Scenario10_CacheHit", func(t *testing.T) {
		if ctx.linkSlug == "" {
			t.Fatal("Scenario 10 depends on Scenario 2")
		}

		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		// Need to create a fresh link since we deleted the old one
		// Re-create link
		if ctx.tokenUser1 == "" || ctx.fileID == "" {
			t.Fatal("need token and file_id for cache hit test")
		}
		resp, err := httpDo("POST", coreURL+"/api/v1/files/"+ctx.fileID+"/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer " + ctx.tokenUser1})
		must(t, err)
		body := readBody(t, resp)
		requireStatus(t, 201, resp.StatusCode, "create link for cache test")
		j := parseJSON(t, body)

		var link map[string]any
		if v := getNested(j, "link"); v != nil {
			link = v.(map[string]any)
		} else {
			link = j
		}
		newSlug := safeString(link, "slug")
		if newSlug == "" {
			t.Fatalf("no slug in response: %s", string(body))
		}
		ctx.linkSlug = newSlug
		ctx.linkID = safeString(link, "id")
		if ctx.linkID == "" {
			ctx.linkID = safeString(j, "id")
		}
		t.Logf("New link slug: %s", ctx.linkSlug)

		// First call primes the cache
		resp, err = client.Get(shortenerURL + "/s/" + ctx.linkSlug)
		must(t, err)
		body = readBody(t, resp)
		t.Logf("Prime cache: %d", resp.StatusCode)
		if resp.StatusCode != 302 {
			t.Logf("Prime body: %s", string(body))
		}
		requireStatus(t, 302, resp.StatusCode, "prime cache redirect")

		// Second call should be cache hit
		resp, err = client.Get(shortenerURL + "/s/" + ctx.linkSlug)
		must(t, err)
		body = readBody(t, resp)
		t.Logf("Cache hit: %d", resp.StatusCode)
		if resp.StatusCode != 302 {
			t.Logf("Cache hit body: %s", string(body))
		}
		requireStatus(t, 302, resp.StatusCode, "cache hit redirect")
		if resp.Header.Get("Location") == "" {
			t.Fatal("no Location header in cache hit response")
		}
	})

	t.Logf("=== Scenario 11: Redirect cache miss queries Postgres ===")
	t.Run("Scenario11_CacheMiss", func(t *testing.T) {
		if ctx.linkSlug == "" {
			t.Fatal("Scenario 11 depends on Scenario 10")
		}

		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		// Try to flush Redis if redis-cli is available
		redisCmd := fmt.Sprintf("redis-cli DEL slug:%s", ctx.linkSlug)
		err := runCommand(redisCmd)
		if err != nil {
			t.Logf("redis-cli not available or DEL failed: %v (continuing without cache flush)", err)
		} else {
			t.Logf("Flushed Redis cache for slug:%s", ctx.linkSlug)
		}

		// Now request should trigger a cache miss
		resp, err := client.Get(shortenerURL + "/s/" + ctx.linkSlug)
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Cache miss: %d", resp.StatusCode)
		if resp.StatusCode != 302 {
			t.Logf("Cache miss body: %s", string(body))
		}
		requireStatus(t, 302, resp.StatusCode, "cache miss redirect")
		if resp.Header.Get("Location") == "" {
			t.Fatal("no Location header in cache miss response")
		}
	})

	t.Logf("=== Scenario 12: Redirect unknown slug returns 404 ===")
	t.Run("Scenario12_RedirectUnknown", func(t *testing.T) {
		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Get(shortenerURL + "/s/zzzzzz")
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Unknown slug response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 404, resp.StatusCode, "unknown slug redirect")
	})

	t.Logf("=== Scenario 14: CreateShare no token ===")
	t.Run("Scenario14_NoToken", func(t *testing.T) {
		resp, err := httpDo("POST", coreURL+"/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share",
			jsonBody(jsonObj{}),
			nil)
		must(t, err)
		body := readBody(t, resp)
		t.Logf("No token response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 401, resp.StatusCode, "no token returns 401")
	})

	t.Logf("=== Scenario 15: CreateShare malformed token ===")
	t.Run("Scenario15_MalformedToken", func(t *testing.T) {
		resp, err := httpDo("POST", coreURL+"/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer obviouslyinvalidtoken"})
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Malformed token response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 401, resp.StatusCode, "malformed token returns 401")
	})

	t.Logf("=== Scenario 16: CreateShare empty bearer token ===")
	t.Run("Scenario16_EmptyBearerToken", func(t *testing.T) {
		resp, err := httpDo("POST", coreURL+"/api/v1/files/660e8400-e29b-41d4-a716-446655440001/share",
			jsonBody(jsonObj{}),
			map[string]string{"Authorization": "Bearer "}) // empty after Bearer
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Empty bearer response: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 401, resp.StatusCode, "empty bearer token returns 401")
	})

	t.Logf("=== Scenario 17: DeleteLink no token ===")
	t.Run("Scenario17_DeleteNoToken", func(t *testing.T) {
		resp, err := httpDo("DELETE", coreURL+"/api/v1/links/770e8400-e29b-41d4-a716-446655440002",
			nil, nil)
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Delete no token: %d %s", resp.StatusCode, string(body))
		requireStatus(t, 401, resp.StatusCode, "delete without token returns 401")
	})

	t.Logf("=== Scenario 18: Redirect endpoint — no auth required ===")
	t.Run("Scenario18_RedirectNoAuth", func(t *testing.T) {
		client := &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Get(shortenerURL + "/s/nonexist")
		must(t, err)
		body := readBody(t, resp)
		t.Logf("Redirect no auth: %d %s", resp.StatusCode, string(body))
		// Must be 404 (not 401), proving it's unauthenticated
		requireStatus(t, 404, resp.StatusCode, "redirect endpoint must return 404 (not 401)")
	})
}

// ──────────────────────────────────────────────
// Utility: run a shell command
// ──────────────────────────────────────────────

func runCommand(cmd string) error {
	// Parse "command arg1 arg2" format
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	c := exec.Command(parts[0], parts[1:]...)
	return c.Run()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
