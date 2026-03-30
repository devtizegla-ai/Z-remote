package auth

import (
	"testing"
	"time"
)

func TestTokenManagerGenerateAndParse(t *testing.T) {
	manager := NewTokenManager("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)
	token, err := manager.GenerateAccessToken("usr_123", "user@example.com")
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "usr_123" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
	if claims.Type != "access" {
		t.Fatalf("unexpected token type: %s", claims.Type)
	}
}
