package auth

import (
	"bytes"
	"testing"
)

func TestGenerateAndHashMCPKey(t *testing.T) {
	t.Parallel()
	token, generatedHash, last4, err := GenerateMCPKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != len(MCPKeyPrefix)+43 || token[:len(MCPKeyPrefix)] != MCPKeyPrefix {
		t.Fatalf("unexpected MCP key format: %q", token)
	}
	if last4 != token[len(token)-4:] {
		t.Fatalf("unexpected last4 %q", last4)
	}
	recomputed, err := HashMCPKey(token)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generatedHash, recomputed) {
		t.Fatal("generated and recomputed hashes differ")
	}
	if _, err := HashMCPKey("not-a-pokernode-key"); err == nil {
		t.Fatal("invalid key was accepted")
	}
}
