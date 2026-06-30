package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	oauthproviders "github.com/giantswarm/mcp-oauth/providers"
	"golang.org/x/oauth2"
)

func TestNormalizeUsers(t *testing.T) {
	users := NormalizeUsers([]string{" Dibou ", "dibou", "", "Other"})
	if len(users) != 2 {
		t.Fatalf("unexpected user count: %d", len(users))
	}
	if users[0] != "dibou" || users[1] != "other" {
		t.Fatalf("unexpected normalized users: %#v", users)
	}
}

func TestGitHubAllowlistProviderRefreshUsesGitHubAccessToken(t *testing.T) {
	base := &mockGitHubProvider{
		token: &oauth2.Token{
			AccessToken: "github-access-token",
			TokenType:   "Bearer",
		},
		userInfo: &oauthproviders.UserInfo{ID: "1"},
	}
	provider, err := NewGitHubAllowlistProvider(base, []string{"dibou"})
	if err != nil {
		t.Fatalf("NewGitHubAllowlistProvider() error = %v", err)
	}
	provider.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"login":"dibou"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	token, err := provider.ExchangeCode(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if token.RefreshToken != "github-access-token" {
		t.Fatalf("ExchangeCode() refresh token = %q, want GitHub access token", token.RefreshToken)
	}

	refreshed, err := provider.RefreshToken(context.Background(), token.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if refreshed.AccessToken != "github-access-token" || refreshed.RefreshToken != "github-access-token" {
		t.Fatalf("RefreshToken() = %#v, want access token reused", refreshed)
	}
}

type mockGitHubProvider struct {
	token    *oauth2.Token
	userInfo *oauthproviders.UserInfo
}

func (m *mockGitHubProvider) Name() string { return "github" }

func (m *mockGitHubProvider) DefaultScopes() []string { return []string{"read:user"} }

func (m *mockGitHubProvider) AuthorizationURL(string, string, string, []string, *oauthproviders.AuthorizationURLOptions) string {
	return "https://github.example/authorize"
}

func (m *mockGitHubProvider) ExchangeCode(context.Context, string, string) (*oauth2.Token, error) {
	return m.token, nil
}

func (m *mockGitHubProvider) ValidateToken(context.Context, string) (*oauthproviders.UserInfo, error) {
	return m.userInfo, nil
}

func (m *mockGitHubProvider) RefreshToken(context.Context, string) (*oauth2.Token, error) {
	return nil, fmt.Errorf("github oauth apps do not support token refresh")
}

func (m *mockGitHubProvider) RevokeToken(context.Context, string) error { return nil }

func (m *mockGitHubProvider) HealthCheck(context.Context) error { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
