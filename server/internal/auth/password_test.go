package auth

import "testing"

func TestHashAndComparePassword(t *testing.T) {
	hash, err := HashPassword("super-secret-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if !ComparePassword(hash, "super-secret-123") {
		t.Fatalf("password should match")
	}
	if ComparePassword(hash, "wrong-password") {
		t.Fatalf("password should not match")
	}
}
