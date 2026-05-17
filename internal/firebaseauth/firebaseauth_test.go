package firebaseauth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebitmonk/ai_newsletter/internal/firebaseauth"
)

func TestFakeVerifier_RegisteredToken_ReturnsClaims(t *testing.T) {
	fv := firebaseauth.NewFakeVerifier()
	want := &firebaseauth.Claims{UID: "u-1", Email: "a@example.com", EmailVerified: true}
	fv.Register("tok", want)

	got, err := fv.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != want {
		t.Errorf("returned different *Claims: %+v vs %+v", got, want)
	}
}

func TestFakeVerifier_UnregisteredToken_ErrInvalidToken(t *testing.T) {
	fv := firebaseauth.NewFakeVerifier()
	_, err := fv.Verify(context.Background(), "nope")
	if !errors.Is(err, firebaseauth.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestFakeVerifier_ForgetMakesTokenInvalid(t *testing.T) {
	fv := firebaseauth.NewFakeVerifier()
	fv.Register("tok", &firebaseauth.Claims{UID: "u"})
	fv.Forget("tok")
	_, err := fv.Verify(context.Background(), "tok")
	if !errors.Is(err, firebaseauth.ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken after Forget, got %v", err)
	}
}
