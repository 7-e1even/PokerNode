package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const playScope = "poker:play"

type HTTPIdentity struct {
	UserID string
	Client *APIClient
}

type HTTPIdentityResolver func(context.Context, string, *http.Request) (*HTTPIdentity, error)

func NewHTTPHandler(resolve HTTPIdentityResolver, logger *slog.Logger) http.Handler {
	schemaCache := mcp.NewSchemaCache()
	waits := newWaitRegistry()
	streamable := mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		tokenInfo := mcpauth.TokenInfoFromContext(request.Context())
		if tokenInfo == nil || tokenInfo.Extra == nil {
			return nil
		}
		identity, _ := tokenInfo.Extra["pokernode_identity"].(*HTTPIdentity)
		if identity == nil || identity.Client == nil {
			return nil
		}
		return newServerForIdentity(identity.Client, schemaCache, waits, identity.UserID)
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		Logger:                       logger,
		MaxRequestBodyBytes:          1 << 20,
		PropagateRequestCancellation: true,
	})
	verifier := func(ctx context.Context, token string, request *http.Request) (*mcpauth.TokenInfo, error) {
		identity, err := resolve(ctx, token, request)
		if err != nil || identity == nil || identity.UserID == "" || identity.Client == nil {
			if err != nil && logger != nil {
				logger.Warn("reject MCP bearer token", "error", err)
			}
			return nil, fmt.Errorf("%w: PokerNode MCP key was rejected", mcpauth.ErrInvalidToken)
		}
		return &mcpauth.TokenInfo{
			UserID: identity.UserID,
			Scopes: []string{playScope},
			Extra:  map[string]any{"pokernode_identity": identity},
		}, nil
	}
	authenticated := mcpauth.RequireBearerToken(verifier, &mcpauth.RequireBearerTokenOptions{
		Scopes:                 []string{playScope},
		AllowMissingExpiration: true,
	})(streamable)
	return http.NewCrossOriginProtection().Handler(authenticated)
}
