package auth

import (
	"context"
	"fmt"
	"net/http"

	oauthserver "github.com/giantswarm/mcp-oauth/server"
	oauthstorage "github.com/giantswarm/mcp-oauth/storage"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
)

func TokenVerifier(server *oauthserver.Server) mcpauth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*mcpauth.TokenInfo, error) {
		userInfo, err := server.ValidateToken(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
		}
		meta, err := tokenMetadata(server, token)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", mcpauth.ErrInvalidToken, err)
		}
		return &mcpauth.TokenInfo{
			Scopes:     meta.Scopes,
			Expiration: meta.ExpiresAt,
			UserID:     userInfo.ID,
			Extra: map[string]any{
				"client_id": meta.ClientID,
				"audience":  meta.Audience,
			},
		}, nil
	}
}

func tokenMetadata(server *oauthserver.Server, token string) (*oauthstorage.TokenMetadata, error) {
	store, ok := server.TokenStore().(oauthstorage.TokenMetadataGetter)
	if !ok {
		return nil, fmt.Errorf("oauth token store does not expose metadata")
	}
	meta, err := store.GetTokenMetadata(token)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("oauth token metadata missing")
	}
	return meta, nil
}
