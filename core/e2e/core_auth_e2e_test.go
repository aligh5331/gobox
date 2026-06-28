package e2e

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ---------------------------------------------------------------------------
// Shared state across scenarios
// ---------------------------------------------------------------------------

type suiteState struct {
	baseURL string // e.g. "http://localhost:3000"
	authURL string // e.g. "http://localhost:8082"

	// From Scenario 2 (Register)
	userID string

	// From Scenario 4 (Login)
	accessToken  string
	refreshToken string
	sessionID    string

	// From Scenario 6 (Refresh)
	newAccessToken  string
	newRefreshToken string

	// From Scenario 11 (crafted expired JWT)
	craftedExpiredToken string

	// Track whether password was changed (Scenario 15)
	passwordChanged bool
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// doRequest sends an HTTP request and returns status code, body bytes, and error.
func doRequest(method, url, bodyJSON, token string) (int, []byte, error) {
	var reqBody io.Reader
	if bodyJSON != "" {
		reqBody = bytes.NewBufferString(bodyJSON)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	if bodyJSON != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, body, nil
}

// parseJSON parses a JSON byte slice into a map.
func parseJSON(b []byte) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return m, nil
}

// getString safely extracts a string from a nested JSON map by key path.
// e.g. getString(m, "user", "email")
func getString(m map[string]interface{}, keys ...string) string {
	current := m
	for i, k := range keys {
		if i == len(keys)-1 {
			v, ok := current[k]
			if !ok {
				return ""
			}
			s, ok := v.(string)
			if !ok {
				return fmt.Sprintf("%v", v)
			}
			return s
		}
		v, ok := current[k]
		if !ok {
			return ""
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

// looksLikeUUID checks if a string looks like a UUID (e.g. "f47ac10b-58cc-4372-a567-0e02b2c3d479").
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return false
	}
	return true
}

// isJWT checks if a string looks like a JWT (three dot-separated base64 segments).
func isJWT(s string) bool {
	parts := strings.Split(s, ".")
	return len(parts) == 3
}

// getErrorCode extracts the error code from the standard error envelope.
func getErrorCode(m map[string]interface{}) string {
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}

func getErrorMessage(m map[string]interface{}) string {
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		return ""
	}
	msg, _ := errObj["message"].(string)
	return msg
}

// readPrivateKey reads an RSA private key from a PEM file.
// Supports both PKCS1 ("RSA PRIVATE KEY") and PKCS8 ("PRIVATE KEY") formats.
func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("key is not RSA (got %T)", key)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %q", block.Type)
	}
}

// craftExpiredJWT creates a JWT with an expired `exp` claim using the auth
// private key. Returns the signed token string.
func craftExpiredJWT(key *rsa.PrivateKey, userID, email, name string) string {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"name":  name,
		"iat":   now.Add(-1 * time.Hour).Unix(),
		"exp":   now.Add(-1 * time.Minute).Unix(), // expired
		"jti":   "e2e-expired-jti-uuid",
		"sid":   "e2e-expired-sid-uuid",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJleHBpcmVkIn0.dummy"
	}
	return signed
}

// generateUUID generates a random UUID v4 string using crypto/rand.
func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// findPrivateKey tries to locate the auth private key PEM file.
func findPrivateKey() (string, error) {
	candidates := []string{
		"../auth/keys/private.pem",
		"../../auth/keys/private.pem",
		"auth/keys/private.pem",
		"/home/ali/gobox/auth/keys/private.pem",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("private key not found in any candidate path")
}

// ---------------------------------------------------------------------------
// Scenario 1 — Health check
// ---------------------------------------------------------------------------

func (s *suiteState) testHealth(t *testing.T) {
	t.Log("Scenario 1: Core API health endpoint")
	code, body, err := doRequest("GET", s.baseURL+"/health", "", "")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse health response: %v", err)
	}
	if getString(m, "status") != "ok" {
		t.Fatalf("expected status=ok, got: %v", m)
	}
	t.Log("  PASS: health endpoint returns 200 OK")
}

// ---------------------------------------------------------------------------
// Scenario 2 — Register a new user successfully
// ---------------------------------------------------------------------------

func (s *suiteState) testRegister(t *testing.T) {
	t.Log("Scenario 2: Register a new user successfully")
	bodyJSON := `{"email":"ali@example.com","password":"correctPass1!","name":"Ali"}`
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/register", bodyJSON, "")
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse register response: %v", err)
	}

	// Assert user object (snake_case keys)
	userEmail := getString(m, "user", "email")
	if userEmail != "ali@example.com" {
		t.Fatalf("expected user.email=ali@example.com, got %q", userEmail)
	}
	userName := getString(m, "user", "name")
	if userName != "Ali" {
		t.Fatalf("expected user.name=Ali, got %q", userName)
	}
	userID := getString(m, "user", "id")
	if userID == "" || !looksLikeUUID(userID) {
		t.Fatalf("expected user.id to be a valid UUID, got %q", userID)
	}
	if getString(m, "user", "created_at") == "" {
		t.Fatal("expected user.created_at to be non-empty")
	}
	if getString(m, "user", "updated_at") == "" {
		t.Fatal("expected user.updated_at to be non-empty")
	}

	// Assert tokens object (snake_case keys)
	accessToken := getString(m, "tokens", "access_token")
	if accessToken == "" || !isJWT(accessToken) {
		t.Fatalf("expected tokens.access_token to be a valid JWT, got %q", accessToken)
	}
	refreshToken := getString(m, "tokens", "refresh_token")
	if refreshToken == "" {
		t.Fatal("expected tokens.refresh_token to be non-empty")
	}
	expiresIn := getString(m, "tokens", "expires_in")
	if expiresIn == "" {
		t.Fatal("expected tokens.expires_in to be non-empty")
	}

	// Assert session object (snake_case keys)
	sessionID := getString(m, "session", "id")
	if sessionID == "" || !looksLikeUUID(sessionID) {
		t.Fatalf("expected session.id to be a valid UUID, got %q", sessionID)
	}
	sessionUserID := getString(m, "session", "user_id")
	if sessionUserID != userID {
		t.Fatalf("expected session.user_id (%s) to match user.id (%s)", sessionUserID, userID)
	}

	// Save state
	s.userID = userID
	s.accessToken = accessToken
	s.refreshToken = refreshToken
	s.sessionID = sessionID

	t.Logf("  PASS: registered user %s (id=%s)", userEmail, userID)
}

// ---------------------------------------------------------------------------
// Scenario 3 — Register with an already-registered email
// ---------------------------------------------------------------------------

func (s *suiteState) testRegisterDuplicate(t *testing.T) {
	t.Log("Scenario 3: Register with an already-registered email")
	bodyJSON := `{"email":"ali@example.com","password":"SecurePass123!","name":"Ali Again"}`
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/register", bodyJSON, "")
	if err != nil {
		t.Fatalf("register duplicate request failed: %v", err)
	}
	if code != http.StatusConflict {
		t.Fatalf("expected 409, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "CONFLICT" {
		t.Fatalf("expected error.code=CONFLICT, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: duplicate email correctly rejected with 409 CONFLICT")
}

// ---------------------------------------------------------------------------
// Scenario 4 — Login with valid credentials
// ---------------------------------------------------------------------------

func (s *suiteState) testLoginValid(t *testing.T) {
	t.Log("Scenario 4: Login with valid credentials")
	bodyJSON := `{"email":"ali@example.com","password":"correctPass1!"}`
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/login", bodyJSON, "")
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse login response: %v", err)
	}

	// Assert user object
	if id := getString(m, "user", "id"); id != s.userID {
		t.Fatalf("expected user.id=%s, got %s", s.userID, id)
	}
	if email := getString(m, "user", "email"); email != "ali@example.com" {
		t.Fatalf("expected user.email=ali@example.com, got %q", email)
	}
	if name := getString(m, "user", "name"); name != "Ali" {
		t.Fatalf("expected user.name=Ali, got %q", name)
	}

	// Assert tokens (snake_case)
	accessToken := getString(m, "tokens", "access_token")
	if accessToken == "" || !isJWT(accessToken) {
		t.Fatalf("expected tokens.access_token to be a valid JWT, got %q", accessToken)
	}
	refreshToken := getString(m, "tokens", "refresh_token")
	if refreshToken == "" {
		t.Fatal("expected tokens.refresh_token to be non-empty")
	}
	expiresIn := getString(m, "tokens", "expires_in")
	if expiresIn == "" {
		t.Fatal("expected tokens.expires_in to be non-empty")
	}

	// Assert session (snake_case)
	sessionID := getString(m, "session", "id")
	if sessionID == "" || !looksLikeUUID(sessionID) {
		t.Fatalf("expected session.id to be a valid UUID, got %q", sessionID)
	}
	sessionUserID := getString(m, "session", "user_id")
	if sessionUserID != s.userID {
		t.Fatalf("expected session.user_id (%s) to match user.id (%s)", sessionUserID, s.userID)
	}

	// Save state (overwrite tokens from registration)
	s.accessToken = accessToken
	s.refreshToken = refreshToken
	s.sessionID = sessionID

	t.Log("  PASS: login returns valid tokens and session")
}

// ---------------------------------------------------------------------------
// Scenario 5 — Login with wrong password
// ---------------------------------------------------------------------------

func (s *suiteState) testLoginWrongPassword(t *testing.T) {
	t.Log("Scenario 5: Login with wrong password")
	bodyJSON := `{"email":"ali@example.com","password":"wrongPassword99!"}`
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/login", bodyJSON, "")
	if err != nil {
		t.Fatalf("login wrong password request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: wrong password correctly rejected with 401 UNAUTHORIZED")
}

// ---------------------------------------------------------------------------
// Scenario 6 — Refresh tokens with a valid refresh token
// ---------------------------------------------------------------------------

func (s *suiteState) testRefreshValid(t *testing.T) {
	t.Log("Scenario 6: Refresh tokens with a valid refresh token")
	if s.refreshToken == "" {
		t.Fatal("no refreshToken available from Scenario 4")
	}
	bodyJSON := fmt.Sprintf(`{"refresh_token":"%s"}`, s.refreshToken)
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/refresh", bodyJSON, "")
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse refresh response: %v", err)
	}

	newAccessToken := getString(m, "tokens", "access_token")
	if newAccessToken == "" || !isJWT(newAccessToken) {
		t.Fatalf("expected tokens.access_token to be a valid JWT, got %q", newAccessToken)
	}
	if newAccessToken == s.accessToken {
		t.Fatal("expected new access_token to differ from old one (token rotation)")
	}

	newRefreshToken := getString(m, "tokens", "refresh_token")
	if newRefreshToken == "" {
		t.Fatal("expected tokens.refresh_token to be non-empty")
	}
	if newRefreshToken == s.refreshToken {
		t.Fatal("expected new refresh_token to differ from old one (token rotation)")
	}

	expiresIn := getString(m, "tokens", "expires_in")
	if expiresIn == "" {
		t.Fatal("expected tokens.expires_in to be non-empty")
	}

	// Save new tokens
	s.newAccessToken = newAccessToken
	s.newRefreshToken = newRefreshToken

	t.Log("  PASS: refresh returns rotated tokens")
}

// ---------------------------------------------------------------------------
// Scenario 7 — Refresh with an expired/stale refresh token
// ---------------------------------------------------------------------------

func (s *suiteState) testRefreshExpired(t *testing.T) {
	t.Log("Scenario 7: Refresh with an expired/stale refresh token")
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/refresh",
		`{"refresh_token":"expired-refresh-token-xyz"}`, "")
	if err != nil {
		t.Fatalf("refresh expired request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: expired refresh token correctly rejected with 401 UNAUTHORIZED")
}

// ---------------------------------------------------------------------------
// Scenario 8 — Logout with a valid session
// ---------------------------------------------------------------------------

func (s *suiteState) testLogoutValid(t *testing.T) {
	t.Log("Scenario 8: Logout with a valid session")
	// Scenario 6 (refresh) rotated the session, so the old sessionID is stale.
	// Re-login to get a fresh session for logout.
	t.Log("  Re-logging in to get a fresh session...")
	code, respBody, err := doRequest("POST", s.baseURL+"/api/v1/auth/login",
		`{"email":"ali@example.com","password":"correctPass1!"}`, "")
	if err != nil {
		t.Fatalf("relogin request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200 on relogin, got %d", code)
	}
	lm, err := parseJSON(respBody)
	if err != nil {
		t.Fatalf("parse relogin response: %v", err)
	}
	freshSessionID := getString(lm, "session", "id")
	freshAccessToken := getString(lm, "tokens", "access_token")
	if freshSessionID == "" || freshAccessToken == "" {
		t.Fatal("fresh login did not return session.id or access_token")
	}
	s.sessionID = freshSessionID
	s.accessToken = freshAccessToken

	t.Logf("  Logging out session %s...", freshSessionID)
	bodyJSON := fmt.Sprintf(`{"session_id":"%s"}`, freshSessionID)
	code, respBody, err = doRequest("DELETE", s.baseURL+"/api/v1/auth/logout", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("logout request failed: %v", err)
	}
	if code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d. body: %s", code, string(respBody))
	}
	if len(respBody) != 0 {
		t.Fatalf("expected empty body, got: %s", string(respBody))
	}
	t.Log("  PASS: logout returns 204 No Content")
}

// ---------------------------------------------------------------------------
// Scenario 9 — Logout without an Authorization header (missing token)
// ---------------------------------------------------------------------------

func (s *suiteState) testLogoutNoAuth(t *testing.T) {
	t.Log("Scenario 9: Logout without an Authorization header (missing token)")
	code, body, err := doRequest("DELETE", s.baseURL+"/api/v1/auth/logout",
		`{"session_id":"a47ac10b-58cc-4372-a567-0e02b2c3d479"}`, "")
	if err != nil {
		t.Fatalf("logout no auth request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: logout without token correctly returns 401 UNAUTHORIZED")
}

// ---------------------------------------------------------------------------
// Scenario 10 — Get own profile with a valid token
// ---------------------------------------------------------------------------

func (s *suiteState) testGetMeValid(t *testing.T) {
	t.Log("Scenario 10: Get own profile with a valid token")
	if s.accessToken == "" {
		t.Fatal("no accessToken available from Scenario 4")
	}
	code, body, err := doRequest("GET", s.baseURL+"/api/v1/me", "", s.accessToken)
	if err != nil {
		t.Fatalf("get /me request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse /me response: %v", err)
	}
	// GET /me returns fields directly (not wrapped in "user")
	if id := getString(m, "id"); id != s.userID {
		t.Fatalf("expected id=%s, got %s", s.userID, id)
	}
	if email := getString(m, "email"); email != "ali@example.com" {
		t.Fatalf("expected email=ali@example.com, got %q", email)
	}
	if name := getString(m, "name"); name != "Ali" {
		t.Fatalf("expected name=Ali, got %q", name)
	}
	if getString(m, "created_at") == "" {
		t.Fatal("expected created_at to be non-empty")
	}
	if getString(m, "updated_at") == "" {
		t.Fatal("expected updated_at to be non-empty")
	}
	t.Log("  PASS: /me returns user profile")
}

// ---------------------------------------------------------------------------
// Scenario 11 — Get own profile with an expired token
// ---------------------------------------------------------------------------

func (s *suiteState) testGetMeExpiredToken(t *testing.T) {
	t.Log("Scenario 11: Get own profile with an expired token")

	// Try to read the private key and craft an expired JWT
	keyPath, err := findPrivateKey()
	if err != nil {
		t.Logf("  Note: private key not found (%v). Using dummy token.", err)
		s.craftedExpiredToken = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJleHBpcmVkIiwiZXhwIjoxfQ.dummy"
	} else {
		key, err := readPrivateKey(keyPath)
		if err != nil {
			t.Fatalf("read private key: %v", err)
		}
		s.craftedExpiredToken = craftExpiredJWT(key, s.userID, "ali@example.com", "Ali")
		t.Log("  Crafted RS256-signed expired JWT")
	}

	code, body, err := doRequest("GET", s.baseURL+"/api/v1/me", "", s.craftedExpiredToken)
	if err != nil {
		t.Fatalf("get /me with expired token request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: expired token correctly rejected with 401 UNAUTHORIZED")
}

// ---------------------------------------------------------------------------
// Scenario 12 — Update own profile with valid fields
// ---------------------------------------------------------------------------

func (s *suiteState) testUpdateMeValid(t *testing.T) {
	t.Log("Scenario 12: Update own profile with valid fields")
	if s.accessToken == "" {
		t.Fatal("no accessToken available from Scenario 4")
	}
	bodyJSON := `{"name":"Ali Reza"}`
	code, body, err := doRequest("PUT", s.baseURL+"/api/v1/me", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("update /me request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse /me update response: %v", err)
	}
	// PUT /me returns fields inside a "user" wrapper
	if id := getString(m, "user", "id"); id != s.userID {
		t.Fatalf("expected user.id=%s, got %s", s.userID, id)
	}
	if email := getString(m, "user", "email"); email != "ali@example.com" {
		t.Fatalf("expected user.email=ali@example.com, got %q", email)
	}
	if name := getString(m, "user", "name"); name != "Ali Reza" {
		t.Fatalf("expected user.name='Ali Reza', got %q", name)
	}
	if getString(m, "user", "updated_at") == "" {
		t.Fatal("expected user.updated_at to be non-empty")
	}
	t.Log("  PASS: /me update returns updated profile")
}

// ---------------------------------------------------------------------------
// Scenario 13 — Update own profile with empty name
// ---------------------------------------------------------------------------

func (s *suiteState) testUpdateMeEmptyName(t *testing.T) {
	t.Log("Scenario 13: Update own profile with empty name")
	if s.accessToken == "" {
		t.Fatal("no accessToken available from Scenario 4")
	}
	bodyJSON := `{"name":""}`
	code, body, err := doRequest("PUT", s.baseURL+"/api/v1/me", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("update /me empty name request failed: %v", err)
	}
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "BAD_REQUEST" {
		t.Fatalf("expected error.code=BAD_REQUEST, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: empty name correctly rejected with 400 BAD_REQUEST")
}

// ---------------------------------------------------------------------------
// Scenario 14 — Update own profile with missing name field
// ---------------------------------------------------------------------------

func (s *suiteState) testUpdateMeMissingName(t *testing.T) {
	t.Log("Scenario 14: Update own profile with missing name field")
	if s.accessToken == "" {
		t.Fatal("no accessToken available from Scenario 4")
	}
	bodyJSON := `{}`
	code, body, err := doRequest("PUT", s.baseURL+"/api/v1/me", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("update /me missing name request failed: %v", err)
	}
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "BAD_REQUEST" {
		t.Fatalf("expected error.code=BAD_REQUEST, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: missing name field correctly rejected with 400 BAD_REQUEST")
}

// ---------------------------------------------------------------------------
// Scenario 15 — Change password with correct old password
// ---------------------------------------------------------------------------

func (s *suiteState) testChangePasswordCorrect(t *testing.T) {
	t.Log("Scenario 15: Change password with correct old password")
	if s.accessToken == "" {
		t.Fatal("no accessToken available from Scenario 4")
	}
	bodyJSON := `{"old_password":"correctPass1!","new_password":"newSecurePass99!"}`
	code, body, err := doRequest("PUT", s.baseURL+"/api/v1/me/password", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("change password request failed: %v", err)
	}
	if code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d. body: %s", code, string(body))
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body, got: %s", string(body))
	}
	s.passwordChanged = true
	t.Log("  PASS: change password returns 204 No Content")
}

// ---------------------------------------------------------------------------
// Scenario 16 — Change password with wrong old password
// ---------------------------------------------------------------------------

func (s *suiteState) testChangePasswordWrong(t *testing.T) {
	t.Log("Scenario 16: Change password with wrong old password")
	// Need a fresh login since Scenario 15 changed the password.
	// Relogin with the new password.
	t.Log("  Re-logging in with new password...")
	bodyJSON := `{"email":"ali@example.com","password":"newSecurePass99!"}`
	code, body, err := doRequest("POST", s.baseURL+"/api/v1/auth/login", bodyJSON, "")
	if err != nil {
		t.Fatalf("relogin request failed: %v", err)
	}
	if code != http.StatusOK {
		t.Fatalf("expected 200 on relogin, got %d. body: %s", code, string(body))
	}
	m2, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse relogin response: %v", err)
	}
	s.accessToken = getString(m2, "tokens", "access_token")
	if s.accessToken == "" {
		t.Fatal("no access_token from relogin")
	}

	t.Log("  Trying wrong old password...")
	bodyJSON = `{"old_password":"wrongOldPass!","new_password":"newSecurePass99!"}`
	code, body, err = doRequest("PUT", s.baseURL+"/api/v1/me/password", bodyJSON, s.accessToken)
	if err != nil {
		t.Fatalf("change password wrong old request failed: %v", err)
	}
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "FORBIDDEN" {
		t.Fatalf("expected error.code=FORBIDDEN, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: wrong old password correctly rejected with 403 FORBIDDEN")
}

// ---------------------------------------------------------------------------
// Auth boundary scenarios
// ---------------------------------------------------------------------------

func (s *suiteState) testBoundaryNoToken(t *testing.T) {
	t.Log("Boundary 1: No token on authenticated endpoint")
	code, body, err := doRequest("GET", s.baseURL+"/api/v1/me", "", "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: no token returns 401 UNAUTHORIZED")
}

func (s *suiteState) testBoundaryMalformedToken(t *testing.T) {
	t.Log("Boundary 2: Malformed token on authenticated endpoint")
	code, body, err := doRequest("GET", s.baseURL+"/api/v1/me", "", "obviouslyinvalid")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: malformed token returns 401 UNAUTHORIZED")
}

// Boundary 3 — Empty token. Note: sending a Bearer header with empty value may
// be treated the same as no token by the middleware.
func (s *suiteState) testBoundaryEmptyToken(t *testing.T) {
	t.Log("Boundary 3: Empty token on authenticated endpoint")
	code, body, err := doRequest("GET", s.baseURL+"/api/v1/me", "", "")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d. body: %s", code, string(body))
	}
	m, err := parseJSON(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if code := getErrorCode(m); code != "UNAUTHORIZED" {
		t.Fatalf("expected error.code=UNAUTHORIZED, got %q", code)
	}
	if msg := getErrorMessage(m); msg == "" {
		t.Fatal("expected error.message to be non-empty")
	}
	t.Log("  PASS: empty token returns 401 UNAUTHORIZED")
}

// ---------------------------------------------------------------------------
// Main E2E test — runs all scenarios in order
// ---------------------------------------------------------------------------

func TestCoreAuthE2E(t *testing.T) {
	s := &suiteState{
		baseURL: "http://localhost:3000",
		authURL: "http://localhost:8082",
	}

	t.Run("Scenario 1 — Health check", s.testHealth)
	t.Run("Scenario 2 — Register new user", s.testRegister)
	t.Run("Scenario 3 — Register duplicate email", s.testRegisterDuplicate)
	t.Run("Scenario 4 — Login valid credentials", s.testLoginValid)
	t.Run("Scenario 5 — Login wrong password", s.testLoginWrongPassword)
	t.Run("Scenario 6 — Refresh valid token", s.testRefreshValid)
	t.Run("Scenario 7 — Refresh expired token", s.testRefreshExpired)
	t.Run("Scenario 8 — Logout valid session", s.testLogoutValid)
	t.Run("Scenario 9 — Logout no auth", s.testLogoutNoAuth)
	t.Run("Scenario 10 — Get /me valid token", s.testGetMeValid)
	t.Run("Scenario 11 — Get /me expired token", s.testGetMeExpiredToken)
	t.Run("Scenario 12 — Update /me valid fields", s.testUpdateMeValid)
	t.Run("Scenario 13 — Update /me empty name", s.testUpdateMeEmptyName)
	t.Run("Scenario 14 — Update /me missing name", s.testUpdateMeMissingName)
	t.Run("Scenario 15 — Change password correct", s.testChangePasswordCorrect)
	t.Run("Scenario 16 — Change password wrong", s.testChangePasswordWrong)

	t.Run("Boundary 1 — No token", s.testBoundaryNoToken)
	t.Run("Boundary 2 — Malformed token", s.testBoundaryMalformedToken)
	t.Run("Boundary 3 — Empty token", s.testBoundaryEmptyToken)
}
