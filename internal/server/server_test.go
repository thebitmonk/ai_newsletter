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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thebitmonk/ai_newsletter/internal/firebaseauth"
	"github.com/thebitmonk/ai_newsletter/internal/server"
)

var (
	testPool     *pgxpool.Pool
	testVerifier *firebaseauth.FakeVerifier
)

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
	testVerifier = firebaseauth.NewFakeVerifier()

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
		`truncate candidates, issues, sources, publications, account_members, users, accounts cascade`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// newServer builds a server.Engine wired with the shared FakeVerifier so
// tokens minted by signupAs are accepted. Forwards any extra options the
// caller wants (e.g. WithCurateTrigger).
func newServer(t *testing.T, opts ...server.Option) *gin.Engine {
	t.Helper()
	opts = append(opts, server.WithTokenVerifier(testVerifier))
	return server.New(testPool, opts...)
}

// signupAs is a test helper: mints a synthetic Firebase ID token + UID,
// registers them in the FakeVerifier, then calls /whoami on the supplied
// router to trigger the user/account upsert. Returns (token, accountID).
//
// Each call produces a fresh UID so concurrent tests don't collide.
func signupAs(t *testing.T, r http.Handler, email string) (token, accountID string) {
	t.Helper()
	uid := "test-uid-" + uuid.NewString()
	token = "test-token-" + uuid.NewString()
	testVerifier.Register(token, &firebaseauth.Claims{
		UID:            uid,
		Email:          email,
		EmailVerified:  true,
		SignInProvider: "password",
	})
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("signupAs %s upsert failed (%d): %v", email, w.Code, body)
	}
	acc, _ := body["account_id"].(string)
	if acc == "" {
		t.Fatalf("signupAs %s: no account_id in /whoami response: %v", email, body)
	}
	return token, acc
}

func parseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return u
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
	r := newServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/healthz", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["status"] != "ok" || body["db"] != "ok" {
		t.Fatalf("unexpected body: %v", body)
	}
}

// -----------------------------------------------------------------------------
// Firebase auth → user/account upsert (replaces the old local signup/login tests)
// -----------------------------------------------------------------------------

func TestFirstAuth_CreatesUserAccountMembershipAtomically(t *testing.T) {
	truncate(t)
	r := newServer(t)

	signupAs(t, r, "alice@example.com")

	var users, accounts, members int
	_ = testPool.QueryRow(context.Background(), `select count(*) from users`).Scan(&users)
	_ = testPool.QueryRow(context.Background(), `select count(*) from accounts`).Scan(&accounts)
	_ = testPool.QueryRow(context.Background(), `select count(*) from account_members`).Scan(&members)
	if users != 1 || accounts != 1 || members != 1 {
		t.Fatalf("expected 1 of each, got users=%d accounts=%d members=%d",
			users, accounts, members)
	}
}

func TestSecondAuth_SameUID_IsIdempotent(t *testing.T) {
	truncate(t)
	r := newServer(t)

	// Register one synthetic token, hit /whoami twice — the user/account
	// rows should be created only once.
	token := "test-token-" + uuid.NewString()
	uid := "test-uid-" + uuid.NewString()
	testVerifier.Register(token, &firebaseauth.Claims{
		UID: uid, Email: "alice@example.com", EmailVerified: true,
	})

	for i := range 3 {
		w, _ := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: want 200, got %d", i, w.Code)
		}
	}

	var users, accounts int
	_ = testPool.QueryRow(context.Background(), `select count(*) from users`).Scan(&users)
	_ = testPool.QueryRow(context.Background(), `select count(*) from accounts`).Scan(&accounts)
	if users != 1 || accounts != 1 {
		t.Fatalf("expected upsert to be idempotent — got users=%d accounts=%d", users, accounts)
	}
}

func TestAuth_EmailUpdatedFromClaims(t *testing.T) {
	truncate(t)
	r := newServer(t)

	token := "test-token-" + uuid.NewString()
	uid := "test-uid-" + uuid.NewString()
	testVerifier.Register(token, &firebaseauth.Claims{
		UID: uid, Email: "old@example.com", EmailVerified: false,
	})
	_, _ = doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)

	// Same UID, new email + verified.
	testVerifier.Register(token, &firebaseauth.Claims{
		UID: uid, Email: "new@example.com", EmailVerified: true,
	})
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["email"] != "new@example.com" {
		t.Errorf("email not refreshed from claims: %v", body["email"])
	}
	if body["email_verified"] != true {
		t.Errorf("email_verified not refreshed from claims: %v", body["email_verified"])
	}
}

// -----------------------------------------------------------------------------
// middleware / scope
// -----------------------------------------------------------------------------

func TestBearer_MissingToken(t *testing.T) {
	r := newServer(t)
	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %v", w.Code, body)
	}
}

func TestBearer_MalformedHeader(t *testing.T) {
	r := newServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestBearer_BogusToken(t *testing.T) {
	r := newServer(t)
	w, _ := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, "not-a-real-token-at-all")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestBearer_RevokedToken_Rejected(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, _ := signupAs(t, r, "eve@example.com")

	// Simulate revocation by forgetting the token on the verifier side
	// (e.g. user deleted their account on the Firebase side).
	testVerifier.Forget(tokenA)

	w, _ := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, tokenA)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 after revocation, got %d", w.Code)
	}
}

func TestWhoami_ReturnsCorrectIdentity(t *testing.T) {
	truncate(t)
	r := newServer(t)
	token, accountID := signupAs(t, r, "frank@example.com")

	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, token)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %v", w.Code, body)
	}
	if body["account_id"] != accountID {
		t.Fatalf("account_id mismatch: %v vs %v", body["account_id"], accountID)
	}
	if body["email"] != "frank@example.com" {
		t.Fatalf("email mismatch: %v", body["email"])
	}
}

func TestCrossAccount_IsolatedSessions(t *testing.T) {
	truncate(t)
	r := newServer(t)
	tokenA, accountA := signupAs(t, r, "a@example.com")
	tokenB, accountB := signupAs(t, r, "b@example.com")

	if accountA == accountB {
		t.Fatalf("two signups produced the same account_id")
	}

	w, body := doJSON(t, r, http.MethodGet, "/api/v1/whoami", nil, tokenB)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if body["account_id"] != accountB {
		t.Fatalf("whoami leaked across accounts: got %v want %v", body["account_id"], accountB)
	}
	_ = tokenA // suppress unused — useful for inspection during debugging
}
