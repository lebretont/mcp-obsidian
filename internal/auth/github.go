package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	oauthproviders "github.com/giantswarm/mcp-oauth/providers"
	"golang.org/x/oauth2"
)

const githubUserURL = "https://api.github.com/user"

type GitHubAllowlistProvider struct {
	base         oauthproviders.Provider
	allowedUsers []string
	httpClient   *http.Client
}

func NewGitHubAllowlistProvider(base oauthproviders.Provider, allowedUsers []string) (*GitHubAllowlistProvider, error) {
	if base == nil {
		return nil, fmt.Errorf("github provider is required")
	}
	allowed := NormalizeUsers(allowedUsers)
	if len(allowed) == 0 {
		return nil, fmt.Errorf("at least one allowed GitHub user is required")
	}
	return &GitHubAllowlistProvider{
		base:         base,
		allowedUsers: allowed,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *GitHubAllowlistProvider) Name() string { return p.base.Name() }

func (p *GitHubAllowlistProvider) DefaultScopes() []string {
	return p.base.DefaultScopes()
}

func (p *GitHubAllowlistProvider) AuthorizationURL(state, codeChallenge, codeChallengeMethod string, _ []string, opts *oauthproviders.AuthorizationURLOptions) string {
	return p.base.AuthorizationURL(state, codeChallenge, codeChallengeMethod, nil, opts)
}

func (p *GitHubAllowlistProvider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	token, err := p.base.ExchangeCode(ctx, code, codeVerifier)
	if err != nil {
		return nil, err
	}
	return p.githubRefreshCompatibleToken(token), nil
}

func (p *GitHubAllowlistProvider) ValidateToken(ctx context.Context, accessToken string) (*oauthproviders.UserInfo, error) {
	userInfo, err := p.base.ValidateToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	login, err := p.fetchLogin(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(p.allowedUsers, strings.ToLower(login)) {
		return nil, fmt.Errorf("github user %q is not allowed", login)
	}
	return userInfo, nil
}

func (p *GitHubAllowlistProvider) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("github access token is required for refresh")
	}
	if _, err := p.ValidateToken(ctx, refreshToken); err != nil {
		return nil, err
	}
	return p.githubRefreshCompatibleToken(&oauth2.Token{
		AccessToken: refreshToken,
		TokenType:   "Bearer",
	}), nil
}

func (p *GitHubAllowlistProvider) RevokeToken(ctx context.Context, token string) error {
	return p.base.RevokeToken(ctx, token)
}

func (p *GitHubAllowlistProvider) HealthCheck(ctx context.Context) error {
	return p.base.HealthCheck(ctx)
}

func (p *GitHubAllowlistProvider) githubRefreshCompatibleToken(token *oauth2.Token) *oauth2.Token {
	if token == nil || token.RefreshToken != "" || token.AccessToken == "" {
		return token
	}
	cloned := *token
	cloned.RefreshToken = token.AccessToken
	return &cloned
}

func (p *GitHubAllowlistProvider) fetchLogin(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github user lookup failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Login == "" {
		return "", fmt.Errorf("github user lookup did not return login")
	}
	return payload.Login, nil
}

func NormalizeUsers(users []string) []string {
	out := make([]string, 0, len(users))
	seen := make(map[string]bool, len(users))
	for _, user := range users {
		value := strings.ToLower(strings.TrimSpace(user))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
