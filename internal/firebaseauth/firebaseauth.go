// Package firebaseauth validates Firebase ID tokens — the credential format
// the backend trusts for every authed request after ADR-0016. The TokenVerifier
// interface is the seam tests stub through FakeVerifier; production wires the
// firebase.google.com/go/v4 Admin SDK implementation.
package firebaseauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// Claims are the parts of a verified Firebase ID token the rest of the
// backend uses. Reduced from the full token to the four fields anyone needs.
type Claims struct {
	UID           string
	Email         string // empty if the user signed in with a UID-only provider
	EmailVerified bool
	SignInProvider string // "password", "google.com", etc. — from the firebase claim
}

// TokenVerifier turns a raw bearer token into Claims, or a typed error if the
// token is missing/expired/wrong-issuer/wrong-audience.
type TokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (*Claims, error)
}

// ErrInvalidToken is returned for any verification failure that should map to
// HTTP 401. Wrap with %w so callers can errors.Is against it.
var ErrInvalidToken = errors.New("invalid id token")

// ---------------------------------------------------------------------------
// Real verifier (firebase.google.com/go/v4)
// ---------------------------------------------------------------------------

type real struct {
	client *fbauth.Client
}

// NewFromEnv constructs a real TokenVerifier using FIREBASE_PROJECT_ID
// (required) and GOOGLE_APPLICATION_CREDENTIALS (path to service-account JSON;
// tilde-expanded — the SDK itself does not expand `~`, but we do so callers
// can keep a friendly path in .env). When FIREBASE_AUTH_EMULATOR_HOST is set,
// the Admin SDK routes calls to the emulator instead of real Firebase and the
// credentials path becomes optional.
func NewFromEnv(ctx context.Context) (TokenVerifier, error) {
	projectID := os.Getenv("FIREBASE_PROJECT_ID")
	if projectID == "" {
		return nil, errors.New("firebaseauth: FIREBASE_PROJECT_ID is required")
	}

	// Normalise the credentials path env var in place — the firebase SDK
	// auto-loads it via Application Default Credentials, so once we've
	// expanded the tilde here, no option needs to be passed.
	if credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credsPath != "" {
		expanded, err := expandTilde(credsPath)
		if err != nil {
			return nil, fmt.Errorf("firebaseauth: expand creds path: %w", err)
		}
		if expanded != credsPath {
			if err := os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", expanded); err != nil {
				return nil, fmt.Errorf("firebaseauth: set creds env: %w", err)
			}
		}
	}

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, fmt.Errorf("firebaseauth: new app: %w", err)
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebaseauth: auth client: %w", err)
	}
	return &real{client: client}, nil
}

func (r *real) Verify(ctx context.Context, rawIDToken string) (*Claims, error) {
	tok, err := r.client.VerifyIDToken(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return claimsFromToken(tok), nil
}

func claimsFromToken(tok *fbauth.Token) *Claims {
	email, _ := tok.Claims["email"].(string)
	verified, _ := tok.Claims["email_verified"].(bool)
	provider := ""
	if fb, ok := tok.Claims["firebase"].(map[string]any); ok {
		provider, _ = fb["sign_in_provider"].(string)
	}
	return &Claims{
		UID:            tok.UID,
		Email:          email,
		EmailVerified:  verified,
		SignInProvider: provider,
	}
}

// ---------------------------------------------------------------------------
// Fake verifier (tests)
// ---------------------------------------------------------------------------

// FakeVerifier maps registered tokens to Claims. Tests register tokens at
// setup, hand them to the request as Bearer values, and the FakeVerifier
// returns the registered Claims (or ErrInvalidToken when unregistered).
type FakeVerifier struct {
	mu    sync.RWMutex
	table map[string]*Claims
}

func NewFakeVerifier() *FakeVerifier {
	return &FakeVerifier{table: make(map[string]*Claims)}
}

// Register makes a token → Claims mapping available to Verify.
func (f *FakeVerifier) Register(token string, claims *Claims) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.table[token] = claims
}

// Forget removes a token mapping (for test-side delete simulation).
func (f *FakeVerifier) Forget(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.table, token)
}

func (f *FakeVerifier) Verify(_ context.Context, raw string) (*Claims, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	c, ok := f.table[raw]
	if !ok {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// ---------------------------------------------------------------------------

func expandTilde(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}
