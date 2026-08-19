package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	MCPKeyPrefix = "pnmcp_"
	mcpKeyBytes  = 32
)

var ErrInvalidMCPKey = errors.New("invalid MCP key")

func GenerateMCPKey() (token string, hash []byte, last4 string, err error) {
	random := make([]byte, mcpKeyBytes)
	if _, err := rand.Read(random); err != nil {
		return "", nil, "", fmt.Errorf("generate MCP key: %w", err)
	}
	token = MCPKeyPrefix + base64.RawURLEncoding.EncodeToString(random)
	hash, err = HashMCPKey(token)
	if err != nil {
		return "", nil, "", err
	}
	return token, hash, token[len(token)-4:], nil
}

func HashMCPKey(token string) ([]byte, error) {
	if !strings.HasPrefix(token, MCPKeyPrefix) {
		return nil, ErrInvalidMCPKey
	}
	encoded := strings.TrimPrefix(token, MCPKeyPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != mcpKeyBytes {
		return nil, ErrInvalidMCPKey
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:], nil
}
