package auth_test

import (
	"testing"
	"time"

	"github.com/JadedPigeon/Chirpy/internal/auth"
	"github.com/google/uuid"
)

func TestMakeAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "super-secret"
	expiresIn := time.Hour

	token, err := auth.MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("unexpected error making JWT: %v", err)
	}

	parsedID, err := auth.ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("unexpected error validating JWT: %v", err)
	}

	if parsedID != userID {
		t.Errorf("expected userID %v but got %v", userID, parsedID)
	}
}

func TestExpiredJWT(t *testing.T) {
	userID := uuid.New()
	secret := "super-secret"
	expiresIn := -1 * time.Second // already expired

	token, err := auth.MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("unexpected error making JWT: %v", err)
	}

	_, err = auth.ValidateJWT(token, secret)
	if err == nil {
		t.Error("expected error validating expired JWT, got nil")
	}
}

func TestInvalidSecretJWT(t *testing.T) {
	userID := uuid.New()
	goodSecret := "correct-secret"
	badSecret := "wrong-secret"

	token, err := auth.MakeJWT(userID, goodSecret, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error making JWT: %v", err)
	}

	_, err = auth.ValidateJWT(token, badSecret)
	if err == nil {
		t.Error("expected error validating JWT with wrong secret, got nil")
	}
}
