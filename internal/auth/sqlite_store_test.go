package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	oauthstorage "github.com/giantswarm/mcp-oauth/storage"
	"golang.org/x/oauth2"
)

func TestSQLiteStoreAtomicGetAndDeleteRefreshTokenBackfillsGitHubRefreshToken(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "oauth.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Stop()

	ctx := context.Background()
	userID := "user-1"
	clientID := "client-1"
	refreshToken := "mcp-refresh-token"
	githubAccessToken := "github-access-token"

	if err := store.SaveToken(ctx, userID, &oauth2.Token{
		AccessToken: githubAccessToken,
		TokenType:   "Bearer",
	}); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}
	if err := store.SaveRefreshToken(ctx, refreshToken, userID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshToken() error = %v", err)
	}
	if err := store.SaveTokenMetadata(ctx, refreshToken, oauthstorage.TokenMetadata{
		UserID:    userID,
		ClientID:  clientID,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveTokenMetadata() error = %v", err)
	}

	gotUserID, gotClientID, providerToken, err := store.AtomicGetAndDeleteRefreshToken(ctx, refreshToken)
	if err != nil {
		t.Fatalf("AtomicGetAndDeleteRefreshToken() error = %v", err)
	}
	if gotUserID != userID || gotClientID != clientID {
		t.Fatalf("AtomicGetAndDeleteRefreshToken() ids = %q/%q, want %q/%q", gotUserID, gotClientID, userID, clientID)
	}
	if providerToken.RefreshToken != githubAccessToken {
		t.Fatalf("provider refresh token = %q, want GitHub access token", providerToken.RefreshToken)
	}
}
