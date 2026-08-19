package store

import (
	"context"
	"errors"
	"testing"

	"pokernode/internal/auth"
)

func TestMCPKeyLifecycle(t *testing.T) {
	database := openTestStore(t)
	ctx := context.Background()
	user, err := database.CreateRegisteredUser(ctx, "mcp_player", "MCP Player", "test-hash")
	if err != nil {
		t.Fatal(err)
	}
	_, firstHash, firstLast4, err := auth.GenerateMCPKey()
	if err != nil {
		t.Fatal(err)
	}
	created, err := database.UpsertMCPKey(ctx, user.ID, firstHash, firstLast4)
	if err != nil {
		t.Fatal(err)
	}
	if created.UserID != user.ID || created.Last4 != firstLast4 {
		t.Fatalf("unexpected key status: %#v", created)
	}
	resolved, err := database.UserByMCPKeyHash(ctx, firstHash)
	if err != nil || resolved.ID != user.ID {
		t.Fatalf("resolve first key: user=%#v err=%v", resolved, err)
	}

	_, secondHash, secondLast4, err := auth.GenerateMCPKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.UpsertMCPKey(ctx, user.ID, secondHash, secondLast4); err != nil {
		t.Fatal(err)
	}
	if _, err := database.UserByMCPKeyHash(ctx, firstHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rotated key remained valid: %v", err)
	}
	if err := database.DeleteMCPKey(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.MCPKeyForUser(ctx, user.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted key status returned %v", err)
	}
}
