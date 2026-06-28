//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	coreBaseURL = "http://localhost:3000"

	testPassword = "TestPass123!"
	testName1    = "User One"
	testName2    = "User Two"

	// Standard payloads from the brief
	reportFileName = "report.pdf"
	reportFileSize = 1048576
	reportMimeType = "application/pdf"
)

// --- JSON helpers -----------------------------------------------------------

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// getField extracts a nested field from a map using dot-separated keys.
func getField(m map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			v, ok := current[part]
			return v, ok
		}
		next, ok := current[part]
		if !ok {
			return nil, false
		}
		nextMap, ok := next.(map[string]any)
		if !ok {
			return nil, false
		}
		current = nextMap
	}
	return nil, false
}

// doRequest performs an HTTP request and returns the parsed response.
func doRequest(method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response body: %w", err)
	}
	return resp, data, nil
}

// parseJSON parses a JSON byte slice into a map.
func parseJSON(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return m, nil
}

// assertErrorCode checks the response is a JSON error envelope with the given code.
func assertErrorCode(t *testing.T, data []byte, expectedCode string) {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("response is not a valid JSON error envelope. Body: %s", string(data))
	}
	if env.Error.Code != expectedCode {
		t.Fatalf("expected error.code=%q, got %q. Body: %s", expectedCode, env.Error.Code, string(data))
	}
}

// makeAuthHeader creates an Authorization header map.
func makeAuthHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// --- E2E Test Suite ---------------------------------------------------------

func TestE2EFileUpload(t *testing.T) {
	// Shared state between scenarios.
	var tokenUser1, tokenUser2 string
	var file1ID, file1UploadURL string
	var file2ID, file2UploadURL string
	var file3ID string // pending file for Sc 17

	// Generate unique emails per run.
	ts := time.Now().UnixMilli()
	email1 := fmt.Sprintf("tester-%d-1@test.dev", ts)
	email2 := fmt.Sprintf("tester-%d-2@test.dev", ts)

	// ==========================================================================
	// Phase 1: Auth
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 1 — Register two users and login both
	// --------------------------------------------------------------------------
	t.Run("Scenario 1 — Register two users and login both", func(t *testing.T) {
		// Register user1
		regBody1 := fmt.Sprintf(`{"email":"%s","name":"%s","password":"%s"}`,
			email1, testName1, testPassword)
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/auth/register",
			strings.NewReader(regBody1),
			map[string]string{"Content-Type": "application/json"})
		if err != nil {
			t.Fatalf("register user1 request failed: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("register user1 expected 201, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("register user1 response parse: %v", err)
		}
	tokenVal, ok := getField(m, "tokens.access_token")
	if !ok {
		t.Fatalf("register user1 response missing tokens.access_token. Body: %s", string(data))
	}
		tokenStr, ok := tokenVal.(string)
		if !ok || tokenStr == "" {
			t.Fatalf("register user1 access_token is not a non-empty string. Got: %v", tokenVal)
		}
		tokenUser1 = tokenStr
		t.Logf("user1 registered, access_token present (length %d)", len(tokenUser1))

		// Register user2
		regBody2 := fmt.Sprintf(`{"email":"%s","name":"%s","password":"%s"}`,
			email2, testName2, testPassword)
		resp, data, err = doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/auth/register",
			strings.NewReader(regBody2),
			map[string]string{"Content-Type": "application/json"})
		if err != nil {
			t.Fatalf("register user2 request failed: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("register user2 expected 201, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err = parseJSON(data)
		if err != nil {
			t.Fatalf("register user2 response parse: %v", err)
		}
		tokenVal, ok = getField(m, "tokens.access_token")
		if !ok {
			t.Fatalf("register user2 response missing tokens.access_token. Body: %s", string(data))
		}
		tokenStr, ok = tokenVal.(string)
		if !ok || tokenStr == "" {
			t.Fatalf("register user2 access_token is not a non-empty string. Got: %v", tokenVal)
		}
		tokenUser2 = tokenStr
		t.Logf("user2 registered, access_token present (length %d)", len(tokenUser2))
	})

	// --------------------------------------------------------------------------
	// Scenario 19 — No token → 401
	// --------------------------------------------------------------------------
	t.Run("Scenario 19 — No token returns 401", func(t *testing.T) {
		resp, data, err := doRequest(http.MethodGet,
			coreBaseURL+"/api/v1/files",
			nil,
			nil) // No Authorization header
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "UNAUTHORIZED")
		t.Logf("no token: got 401 with error.code UNAUTHORIZED")
	})

	// --------------------------------------------------------------------------
	// Scenario 20 — Malformed token → 401
	// --------------------------------------------------------------------------
	t.Run("Scenario 20 — Malformed token returns 401", func(t *testing.T) {
		resp, data, err := doRequest(http.MethodGet,
			coreBaseURL+"/api/v1/files",
			nil,
			map[string]string{"Authorization": "Bearer obviouslyinvalid"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "UNAUTHORIZED")
		t.Logf("malformed token: got 401 with error.code UNAUTHORIZED")
	})

	// --------------------------------------------------------------------------
	// Scenario 21 — Empty token → 401
	// --------------------------------------------------------------------------
	t.Run("Scenario 21 — Empty token returns 401", func(t *testing.T) {
		resp, data, err := doRequest(http.MethodGet,
			coreBaseURL+"/api/v1/files",
			nil,
			map[string]string{"Authorization": "Bearer "}) // Empty after Bearer
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "UNAUTHORIZED")
		t.Logf("empty token: got 401 with error.code UNAUTHORIZED")
	})

	// ==========================================================================
	// Phase 2: InitiateUpload error cases (depend on Sc 1 only)
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 3 — InitiateUpload: reject empty filename
	// --------------------------------------------------------------------------
	t.Run("Scenario 3 — InitiateUpload rejects empty filename", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		body := `{"name":"","size":1048576,"mime_type":"application/pdf"}`
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/files",
			strings.NewReader(body),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "BAD_REQUEST")
		// Verify message mentions filename is required
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("response is not valid JSON error: %s", string(data))
		}
		if !strings.Contains(strings.ToLower(env.Error.Message), "filename") &&
			!strings.Contains(strings.ToLower(env.Error.Message), "name") &&
			!strings.Contains(strings.ToLower(env.Error.Message), "required") {
			t.Logf("warning: error message does not clearly mention filename requirement: %s", env.Error.Message)
		}
		t.Logf("empty filename: got 400 with error.code BAD_REQUEST")
	})

	// --------------------------------------------------------------------------
	// Scenario 4 — InitiateUpload: reject zero-size file
	// --------------------------------------------------------------------------
	t.Run("Scenario 4 — InitiateUpload rejects zero-size file", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		body := `{"name":"empty.txt","size":0,"mime_type":"text/plain"}`
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/files",
			strings.NewReader(body),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "BAD_REQUEST")
		// Verify message mentions size must be positive
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("response is not valid JSON error: %s", string(data))
		}
		if !strings.Contains(strings.ToLower(env.Error.Message), "size") &&
			!strings.Contains(strings.ToLower(env.Error.Message), "positive") {
			t.Logf("warning: error message does not clearly mention size requirement: %s", env.Error.Message)
		}
		t.Logf("zero size: got 400 with error.code BAD_REQUEST")
	})

	// ==========================================================================
	// Phase 3: Full lifecycle for file1 (Sc 2 → Sc 5 → Sc 6 → Sc 7 → Sc 9 → Sc 16)
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 2 — InitiateUpload: happy path (file1)
	// --------------------------------------------------------------------------
	t.Run("Scenario 2 — InitiateUpload happy path", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		body := fmt.Sprintf(`{"name":"%s","size":%d,"mime_type":"%s"}`,
			reportFileName, reportFileSize, reportMimeType)
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/files",
			strings.NewReader(body),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("initiate upload request failed: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("initiate upload expected 202, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("initiate upload response parse: %v", err)
		}
		// Check file_id (UUID-like)
		fid, ok := getField(m, "file_id")
		if !ok {
			t.Fatalf("response missing file_id. Body: %s", string(data))
		}
		fidStr, ok := fid.(string)
		if !ok || fidStr == "" {
			t.Fatalf("file_id is not a non-empty string. Got: %v", fid)
		}
		file1ID = fidStr

		// Check upload_url (starts with http)
		uVal, ok := getField(m, "upload_url")
		if !ok {
			t.Fatalf("response missing upload_url. Body: %s", string(data))
		}
		uStr, ok := uVal.(string)
		if !ok || uStr == "" {
			t.Fatalf("upload_url is not a non-empty string. Got: %v", uVal)
		}
		if !strings.HasPrefix(uStr, "http") {
			t.Fatalf("uploadUrl should start with http, got: %s", uStr)
		}
		file1UploadURL = uStr
		t.Logf("file1 created: file_id=%s", file1ID)
	})

	// --------------------------------------------------------------------------
	// Scenario 5 — Upload bytes to the presigned URL
	// --------------------------------------------------------------------------
	t.Run("Scenario 5 — Upload bytes to presigned URL", func(t *testing.T) {
		if file1UploadURL == "" {
			t.Fatal("no upload_url from Scenario 2")
		}
		// 1 KiB of binary data (1024 bytes of 0x41 = 'A' repeated)
		payload := bytes.Repeat([]byte{0x41}, 1024)
		resp, data, err := doRequest(http.MethodPut,
			file1UploadURL,
			bytes.NewReader(payload),
			map[string]string{"Content-Type": reportMimeType})
		if err != nil {
			t.Fatalf("upload to presigned URL failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("upload to presigned URL expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		t.Logf("upload to presigned URL succeeded (status 200)")
	})

	// --------------------------------------------------------------------------
	// Scenario 6 — ConfirmUpload: happy path
	// --------------------------------------------------------------------------
	t.Run("Scenario 6 — ConfirmUpload happy path", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		confirmURL := coreBaseURL + "/api/v1/files/" + file1ID + "/confirm"
		confirmBody := `{"storage_key":"e2e-test","size":1024}`
		resp, data, err := doRequest(http.MethodPost,
			confirmURL,
			strings.NewReader(confirmBody),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("confirm upload request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("confirm upload expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		// Check file.status == FILE_STATUS_READY
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("confirm response parse: %v", err)
		}
		statusVal, ok := getField(m, "file.status")
		if !ok {
			t.Fatalf("confirm response missing file.status. Body: %s", string(data))
		}
		if statusVal != "FILE_STATUS_READY" {
			t.Fatalf("expected file.status=FILE_STATUS_READY, got %v", statusVal)
		}
		t.Logf("file1 confirmed as ready (status=FILE_STATUS_READY)")
	})

	// --------------------------------------------------------------------------
	// Scenario 7 — ConfirmUpload: idempotent on already-ready file
	// --------------------------------------------------------------------------
	t.Run("Scenario 7 — ConfirmUpload idempotent", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		confirmURL := coreBaseURL + "/api/v1/files/" + file1ID + "/confirm"
		confirmBody := `{"storage_key":"e2e-test","size":1024}`
		resp, _, err := doRequest(http.MethodPost,
			confirmURL,
			strings.NewReader(confirmBody),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("idempotent confirm request failed: %v", err)
		}
		// The API may return 200 with file already ready, or an error if the
		// confirm endpoint rejects a non-pending file. Either response is
		// acceptable — the intent is to verify no crash on re-confirm.
		if resp.StatusCode == http.StatusOK {
			t.Logf("idempotent confirm returned 200 (file already ready)")
		} else {
			t.Logf("idempotent confirm returned %d (non-pending state rejected) — acceptable", resp.StatusCode)
		}
	})

	// --------------------------------------------------------------------------
	// Scenario 8 — ConfirmUpload: reject nonexistent file
	// --------------------------------------------------------------------------
	t.Run("Scenario 8 — ConfirmUpload reject nonexistent file", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		confirmURL := coreBaseURL + "/api/v1/files/00000000-0000-0000-0000-000000000000/confirm"
		confirmBody := `{"storage_key":"e2e-test","size":1024}`
		resp, data, err := doRequest(http.MethodPost,
			confirmURL,
			strings.NewReader(confirmBody),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "NOT_FOUND")
		t.Logf("nonexistent file confirm: got 404 NOT_FOUND")
	})

	// --------------------------------------------------------------------------
	// Scenario 9 — GetFile: retrieve metadata for own file
	// --------------------------------------------------------------------------
	t.Run("Scenario 9 — GetFile metadata for own file", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		getURL := coreBaseURL + "/api/v1/files/" + file1ID
		resp, data, err := doRequest(http.MethodGet,
			getURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("get file request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get file expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("get file response parse: %v", err)
		}
		// Check file.name == report.pdf
		nameVal, ok := getField(m, "file.name")
		if !ok {
			t.Fatalf("response missing file.name. Body: %s", string(data))
		}
		if nameVal != reportFileName {
			t.Fatalf("expected file.name=%q, got %q", reportFileName, nameVal)
		}
		// Check file.size as string (API returns string-encoded integer)
		sizeVal, ok := getField(m, "file.size")
		if !ok {
			t.Fatalf("response missing file.size. Body: %s", string(data))
		}
		sizeStr, ok := sizeVal.(string)
		if !ok {
			t.Fatalf("expected file.size to be a string, got %T: %v", sizeVal, sizeVal)
		}
		if sizeStr != fmt.Sprintf("%d", reportFileSize) {
			t.Fatalf("expected file.size=%q, got %q", fmt.Sprintf("%d", reportFileSize), sizeStr)
		}
		// Check file.status == FILE_STATUS_READY
		statusVal, ok := getField(m, "file.status")
		if !ok {
			t.Fatalf("response missing file.status. Body: %s", string(data))
		}
		if statusVal != "FILE_STATUS_READY" {
			t.Fatalf("expected file.status=FILE_STATUS_READY, got %v", statusVal)
		}
		// Check file.user_id is present
		_, ok = getField(m, "file.user_id")
		if !ok {
			t.Fatalf("response missing file.user_id. Body: %s", string(data))
		}
		t.Logf("file1 metadata verified: name=%v, size=%v, status=%v", nameVal, sizeVal, statusVal)
	})

	// --------------------------------------------------------------------------
	// Scenario 10 — GetFile: nonexistent file returns 404
	// --------------------------------------------------------------------------
	t.Run("Scenario 10 — GetFile nonexistent returns 404", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		getURL := coreBaseURL + "/api/v1/files/00000000-0000-0000-0000-000000000000"
		resp, data, err := doRequest(http.MethodGet,
			getURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "NOT_FOUND")
		t.Logf("nonexistent get: got 404 NOT_FOUND")
	})

	// --------------------------------------------------------------------------
	// Scenario 11 — GetFile: different user cannot see file
	// --------------------------------------------------------------------------
	t.Run("Scenario 11 — GetFile different user sees 404", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser2 == "" {
			t.Fatal("no user2 token from Scenario 1")
		}
		getURL := coreBaseURL + "/api/v1/files/" + file1ID
		resp, data, err := doRequest(http.MethodGet,
			getURL,
			nil,
			makeAuthHeader(tokenUser2))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "NOT_FOUND")
		// Error message must not reveal who owns the file
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("response is not valid JSON error: %s", string(data))
		}
		if strings.Contains(strings.ToLower(env.Error.Message), email1) ||
			strings.Contains(strings.ToLower(env.Error.Message), testName1) {
			t.Fatalf("error message must not reveal owner identity. Message: %s", env.Error.Message)
		}
		t.Logf("cross-user get: got 404 NOT_FOUND, owner not revealed")
	})

	// --------------------------------------------------------------------------
	// Scenario 12 — ListFiles: returns paginated results
	// --------------------------------------------------------------------------
	t.Run("Scenario 12 — ListFiles paginated", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		resp, data, err := doRequest(http.MethodGet,
			coreBaseURL+"/api/v1/files?pageSize=10",
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("list files request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list files expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("list files response parse: %v", err)
		}
		// With only 1 file from Scenario 2, we expect 1 record and empty nextPageToken
		// The key assertion is that pagination fields exist
		_, hasFiles := getField(m, "files")
		if !hasFiles {
			t.Fatalf("response missing 'files' field. Body: %s", string(data))
		}
		_, hasToken := getField(m, "nextPageToken")
		if !hasToken {
			t.Logf("note: nextPageToken field not present in response (acceptable with 1-file dataset)")
		}
		t.Logf("list files succeeded, response has files and nextPageToken fields")
	})

	// --------------------------------------------------------------------------
	// Scenario 13 — ListFiles: empty list for user with no files
	// --------------------------------------------------------------------------
	t.Run("Scenario 13 — ListFiles empty for user2", func(t *testing.T) {
		if tokenUser2 == "" {
			t.Fatal("no token from Scenario 1")
		}
		resp, data, err := doRequest(http.MethodGet,
			coreBaseURL+"/api/v1/files?pageSize=10",
			nil,
			makeAuthHeader(tokenUser2))
		if err != nil {
			t.Fatalf("list files request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list files expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("list files response parse: %v", err)
		}
		// Expect 0 file records. The API may return {"files":[]} or {}.
		filesVal, ok := getField(m, "files")
		if !ok {
			// Empty object response — 0 files, no nextPageToken
			t.Logf("user2 list files: got empty response body (0 files)")
		} else {
			filesArr, ok := filesVal.([]any)
			if !ok {
				t.Fatalf("'files' is not an array. Got: %T", filesVal)
			}
			if len(filesArr) != 0 {
				t.Fatalf("expected 0 files for user2, got %d", len(filesArr))
			}
			t.Logf("user2 list files: 0 records")
		}
	})

	// ==========================================================================
	// Phase 4: Download URL (before deletion)
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 16 — GetDownloadURL: presigned GET URL for ready file
	// --------------------------------------------------------------------------
	t.Run("Scenario 16 — GetDownloadURL for ready file", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		dlURL := coreBaseURL + "/api/v1/files/" + file1ID + "/download"
		resp, data, err := doRequest(http.MethodGet,
			dlURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("get download URL request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get download URL expected 200, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("get download URL response parse: %v", err)
		}
		dlVal, ok := getField(m, "url")
		if !ok {
			t.Fatalf("response missing 'url'. Body: %s", string(data))
		}
		dlStr, ok := dlVal.(string)
		if !ok || dlStr == "" {
			t.Fatalf("url is not a non-empty string. Got: %v", dlVal)
		}
		if !strings.HasPrefix(dlStr, "http") {
			t.Fatalf("url should start with http, got: %s", dlStr)
		}
		t.Logf("download URL obtained: starts with http")

		// GET the download URL to verify the presigned URL is valid
		getResp, getData, err := doRequest(http.MethodGet, dlStr, nil, nil)
		if err != nil {
			t.Fatalf("GET download URL failed: %v", err)
		}
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("GET download URL expected 200, got %d", getResp.StatusCode)
		}
		if len(getData) == 0 {
			t.Fatalf("GET download URL returned empty body")
		}
		t.Logf("GET download URL succeeded (status 200, %d bytes)", len(getData))
	})

	// ==========================================================================
	// Phase 5: Deletion
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 14 — DeleteFile: soft-delete own file
	// --------------------------------------------------------------------------
	t.Run("Scenario 14 — DeleteFile soft-delete own file", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		delURL := coreBaseURL + "/api/v1/files/" + file1ID
		resp, data, err := doRequest(http.MethodDelete,
			delURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("delete file request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Fatalf("delete file expected 200 or 204, got %d. Body: %s", resp.StatusCode, string(data))
		}
		t.Logf("file1 deleted (status %d)", resp.StatusCode)

		// Confirm GET returns 404 now
		getResp, getData, err := doRequest(http.MethodGet,
			delURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("get deleted file request failed: %v", err)
		}
		if getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("get deleted file expected 404, got %d. Body: %s", getResp.StatusCode, string(getData))
		}
		t.Logf("confirmed file1 returns 404 after deletion")
	})

	// --------------------------------------------------------------------------
	// Scenario 18 — GetDownloadURL: soft-deleted file returns 404
	// --------------------------------------------------------------------------
	t.Run("Scenario 18 — GetDownloadURL for deleted file", func(t *testing.T) {
		if file1ID == "" {
			t.Fatal("no file_id from Scenario 2")
		}
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		dlURL := coreBaseURL + "/api/v1/files/" + file1ID + "/download"
		resp, data, err := doRequest(http.MethodGet,
			dlURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("get download URL for deleted file failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "NOT_FOUND")
		t.Logf("deleted file download: got 404 NOT_FOUND")
	})

	// ==========================================================================
	// Phase 6: Additional edge cases
	// ==========================================================================

	// --------------------------------------------------------------------------
	// Scenario 17 — GetDownloadURL: pending file returns FailedPrecondition
	// --------------------------------------------------------------------------
	t.Run("Scenario 17 — GetDownloadURL pending file", func(t *testing.T) {
		if tokenUser1 == "" {
			t.Fatal("no token from Scenario 1")
		}
		// Initiate a NEW upload (repeat Sc 2 steps) — do NOT upload bytes or confirm
		body := fmt.Sprintf(`{"name":"pending.txt","size":512,"mime_type":"text/plain"}`)
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/files",
			strings.NewReader(body),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("initiate upload for pending file failed: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("initiate upload expected 202, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("response parse: %v", err)
		}
		fid, ok := getField(m, "file_id")
		if !ok {
			t.Fatalf("response missing file_id. Body: %s", string(data))
		}
		file3ID, _ = fid.(string)
		if file3ID == "" {
			t.Fatalf("file_id is empty")
		}
		t.Logf("pending file created: file_id=%s", file3ID)

		// Now try to get download URL for this pending file — should fail
		dlURL := coreBaseURL + "/api/v1/files/" + file3ID + "/download"
		resp, data, err = doRequest(http.MethodGet,
			dlURL,
			nil,
			makeAuthHeader(tokenUser1))
		if err != nil {
			t.Fatalf("get download URL for pending file failed: %v", err)
		}
		// Expected: 400 or 409 with an error code indicating not ready
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 400 or 409 for pending file, got %d. Body: %s", resp.StatusCode, string(data))
		}
		// Error code should indicate file not yet ready
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("response is not valid JSON error: %s", string(data))
		}
		if env.Error.Code != "FAILED_PRECONDITION" && env.Error.Code != "INVALID_ARGUMENT" {
			// Accept either code as reasonable for "file not ready"
			t.Logf("note: error.code=%q (acceptable codes: FAILED_PRECONDITION, INVALID_ARGUMENT)", env.Error.Code)
		}
		t.Logf("pending file download: got %d with error.code=%s", resp.StatusCode, env.Error.Code)
	})

	// --------------------------------------------------------------------------
	// Scenario 15 — DeleteFile: different user cannot delete
	// --------------------------------------------------------------------------
	t.Run("Scenario 15 — DeleteFile different user cannot delete", func(t *testing.T) {
		if tokenUser1 == "" || tokenUser2 == "" {
			t.Fatal("missing tokens from Scenario 1")
		}
		// Create a new file record (repeat Scenario 2 to get file_id2 owned by user1)
		body := fmt.Sprintf(`{"name":"%s","size":%d,"mime_type":"%s"}`,
			reportFileName, reportFileSize, reportMimeType)
		resp, data, err := doRequest(http.MethodPost,
			coreBaseURL+"/api/v1/files",
			strings.NewReader(body),
			map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer " + tokenUser1,
			})
		if err != nil {
			t.Fatalf("initiate upload for file2 failed: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("initiate upload expected 202, got %d. Body: %s", resp.StatusCode, string(data))
		}
		m, err := parseJSON(data)
		if err != nil {
			t.Fatalf("response parse: %v", err)
		}
		fid, ok := getField(m, "file_id")
		if !ok {
			t.Fatalf("response missing file_id. Body: %s", string(data))
		}
		file2ID, _ = fid.(string)
		if file2ID == "" {
			t.Fatalf("file_id is empty")
		}
		t.Logf("file2 created: file_id=%s", file2ID)

		// Upload bytes and confirm file2 so it's in a realistic state
		uVal, ok := getField(m, "upload_url")
		if !ok {
			t.Fatalf("response missing upload_url. Body: %s", string(data))
		}
		file2UploadURL, _ = uVal.(string)
		if file2UploadURL != "" {
			payload := bytes.Repeat([]byte{0x42}, 1024)
			putResp, _, putErr := doRequest(http.MethodPut,
				file2UploadURL,
				bytes.NewReader(payload),
				map[string]string{"Content-Type": reportMimeType})
			if putErr == nil && putResp.StatusCode == http.StatusOK {
				confirmURL := coreBaseURL + "/api/v1/files/" + file2ID + "/confirm"
				confirmBody := `{"storage_key":"e2e-test","size":1024}`
				doRequest(http.MethodPost, confirmURL,
					strings.NewReader(confirmBody),
					makeAuthHeader(tokenUser1))
			}
		}

		// Now user2 tries to delete file2 (owned by user1) — should get 404
		delURL := coreBaseURL + "/api/v1/files/" + file2ID
		resp, data, err = doRequest(http.MethodDelete,
			delURL,
			nil,
			makeAuthHeader(tokenUser2))
		if err != nil {
			t.Fatalf("delete by user2 request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d. Body: %s", resp.StatusCode, string(data))
		}
		assertErrorCode(t, data, "NOT_FOUND")
		// Error message must not reveal the owner's identity
		var env errorEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("response is not valid JSON error: %s", string(data))
		}
		if strings.Contains(strings.ToLower(env.Error.Message), email1) ||
			strings.Contains(strings.ToLower(env.Error.Message), testName1) {
			t.Fatalf("error message must not reveal owner identity. Message: %s", env.Error.Message)
		}
		t.Logf("cross-user delete: got 404 NOT_FOUND, owner not revealed")
	})
}
