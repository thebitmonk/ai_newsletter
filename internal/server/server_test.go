package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/server"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://ai_newsletter:ai_newsletter@localhost:5433/ai_newsletter?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot connect to test postgres at %s: %v\n", dbURL, err)
		os.Exit(0)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "SKIP: cannot ping test postgres: %v\n", err)
		pool.Close()
		os.Exit(0)
	}
	testPool = pool

	if err := migrateUp(dbURL); err != nil {
		fmt.Fprintf(os.Stderr, "migrate up: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func migrateUp(dbURL string) error {
	_, file, _, _ := runtime.Caller(0)
	migrationsPath := "file://" + filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")

	m, err := migrate.New(migrationsPath, dbURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func truncate(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`truncate publications, sessions, account_members, users, accounts cascade`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// signupAs is a test helper: creates a new account+user, returns the token.
func signupAs(t *testing.T, r http.Handler, email string) (token, accountID string) {
	t.Helper()
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": email, "password": "supersecret"}, "")
	tok, _ := body["token"].(string)
	acc, _ := body["account_id"].(string)
	if tok == "" || acc == "" {
		t.Fatalf("signupAs %s failed: %v", email, body)
	}
	return tok, acc
}

func doJSON(t *testing.T, r http.Handler, method, path string, body any, bearer string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Logf("body: %s", w.Body.String())
		}
	}
	return w, resp
}

// -----------------------------------------------------------------------------
// /healthz
// -----------------------------------------------------------------------------

func TestHealthz_OK(t *testing.T) {
	r := server.New(testPool)
	w, body := doJSON(t, r, http.MethodGet, "/healthz", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["status"] != "ok" || body["db"] != "ok" {
		t.Fatalf("unexpected body: %v", body)
	}
}

// -----------------------------------------------------------------------------
// signup / login
// -----------------------------------------------------------------------------

func TestSignup_HappyPath_CreatesAtomically(t *testing.T) {
	truncate(t)
	r := server.New(testPool)

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "alice@example.com", "password": "supersecret"}, "")

	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %v", w.Code, body)
	}
	if body["token"] == nil || body["user_id"] == nil || body["account_id"] == nil {
		t.Fatalf("missing fields: %v", body)
	}

	// Verify all three rows exist
	var users, accounts, members int
	_ = testPool.QueryRow(context.Background(), `select count(*) from users`).Scan(&users)
	_ = testPool.QueryRow(context.Background(), `select count(*) from accounts`).Scan(&accounts)
	_ = testPool.QueryRow(context.Background(), `select count(*) from account_members`).Scan(&members)
	if users != 1 || accounts != 1 || members != 1 {
		t.Fatalf("expected 1 of each, got users=%d accounts=%d members=%d", users, accounts, members)
	}
}

func TestSignup_DuplicateEmail_RollsBackAccount(t *testing.T) {
	truncate(t)
	r := server.New(testPool)

	_, _ = doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "bob@example.com", "password": "supersecret"}, "")

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "BOB@example.com", "password": "supersecret"}, "")

	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody["code"] != "email_taken" {
		t.Fatalf("want email_taken, got %v", errBody)
	}

	// Should still be exactly one account (the failed signup's tx rolled back)
	var accounts int
	_ = testPool.QueryRow(context.Background(), `select count(*) from accounts`).Scan(&accounts)
	if accounts != 1 {
		t.Fatalf("want 1 account after failed dup, got %d", accounts)
	}
}

func TestSignup_PasswordTooShort(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "x@y.com", "password": "short"}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestLogin_HappyPath(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	_, _ = doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "carol@example.com", "password": "supersecret"}, "")

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "carol@example.com", "password": "supersecret"}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["token"] == nil {
		t.Fatalf("no token in response: %v", body)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	_, _ = doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "dave@example.com", "password": "supersecret"}, "")

	w, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "dave@example.com", "password": "wrong-one"}, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %v", w.Code, body)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	w, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"email": "ghost@example.com", "password": "supersecret"}, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// -----------------------------------------------------------------------------
// middleware / scope
// -----------------------------------------------------------------------------

func TestBearer_MissingToken(t *testing.T) {
	r := server.New(testPool)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %v", w.Code, body)
	}
}

func TestBearer_MalformedHeader(t *testing.T) {
	r := server.New(testPool)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestBearer_BogusToken(t *testing.T) {
	r := server.New(testPool)
	w, _ := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, "not-a-real-token-at-all")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestBearer_ExpiredToken(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	_, body := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "eve@example.com", "password": "supersecret"}, "")
	token := body["token"].(string)

	// Manually expire the session.
	_, err := testPool.Exec(context.Background(),
		`update sessions set expires_at = now() - interval '1 hour'`)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}

	w, _ := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 after expiry, got %d", w.Code)
	}
}

func TestWhoami_ReturnsCorrectIdentity(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	_, signup := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "frank@example.com", "password": "supersecret"}, "")
	token := signup["token"].(string)

	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["user_id"] != signup["user_id"] {
		t.Fatalf("user_id mismatch: %v vs %v", body["user_id"], signup["user_id"])
	}
	if body["account_id"] != signup["account_id"] {
		t.Fatalf("account_id mismatch: %v vs %v", body["account_id"], signup["account_id"])
	}
}

func TestCrossAccount_IsolatedSessions(t *testing.T) {
	truncate(t)
	r := server.New(testPool)
	_, a := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "a@example.com", "password": "supersecret"}, "")
	_, b := doJSON(t, r, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"email": "b@example.com", "password": "supersecret"}, "")

	if a["account_id"] == b["account_id"] {
		t.Fatalf("two signups produced the same account_id")
	}

	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, b["token"].(string))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["account_id"] != b["account_id"] {
		t.Fatalf("whoami leaked across accounts: got %v want %v", body["account_id"], b["account_id"])
	}
}
