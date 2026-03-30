package ws

import "testing"

func TestParseAuthFromSubprotocols(t *testing.T) {
	token, key, did := parseAuthFromSubprotocols([]string{
		"ra.v1",
		"did.dev_123",
		"access.aaa.bbb.ccc",
		"dkey.dkey_abc123",
	})

	if token != "aaa.bbb.ccc" {
		t.Fatalf("unexpected token: %q", token)
	}
	if key != "dkey_abc123" {
		t.Fatalf("unexpected device key: %q", key)
	}
	if did != "dev_123" {
		t.Fatalf("unexpected device id: %q", did)
	}
}
