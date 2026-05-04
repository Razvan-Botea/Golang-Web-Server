package internal

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJWT(t *testing.T) {
	userID := uuid.New()
	secret := "super-secret-key"

	// Test 1: Creating and Validating a good token
	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("Failed to create JWT: %v", err)
	}

	parsedID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("Failed to validate JWT: %v", err)
	}
	if parsedID != userID {
		t.Fatalf("Expected userID %v, got %v", userID, parsedID)
	}

	// Test 2: Validating with the wrong secret
	_, err = ValidateJWT(token, "wrong-secret")
	if err == nil {
		t.Fatal("Expected error when validating with wrong secret, got none")
	}

	// Test 3: Expired Token
	expiredToken, _ := MakeJWT(userID, secret, -time.Hour)
	_, err = ValidateJWT(expiredToken, secret)
	if err == nil {
		t.Fatal("Expected error when validating expired token, got none")
	}
}
