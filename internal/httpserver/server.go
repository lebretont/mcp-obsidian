package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	obsauth "github.com/dibou/mcp-obsidian/internal/auth"
	"github.com/dibou/mcp-obsidian/internal/config"
	obsmcp "github.com/dibou/mcp-obsidian/internal/mcp"
	syncapi "github.com/dibou/mcp-obsidian/internal/sync"
	"github.com/dibou/mcp-obsidian/internal/vault"
	oauth "github.com/giantswarm/mcp-oauth"
	oauthhandler "github.com/giantswarm/mcp-oauth/handler"
	oauthproviders "github.com/giantswarm/mcp-oauth/providers"
	"github.com/giantswarm/mcp-oauth/providers/github"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Dependencies struct {
	Config config.Config
	Vault  *vault.Service
	Sync   syncapi.Syncer
	Logger *slog.Logger
	Version string
}

func New(ctx context.Context, deps Dependencies) (*http.Server, error) {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	store, err := obsauth.NewSQLiteStore(deps.Config.OAuth.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("open OAuth SQLite store: %w", err)
	}

	provider, err := newGitHubProvider(deps.Config)
	if err != nil {
		store.Stop()
		return nil, err
	}

	oauthServer, err := oauth.NewServerWithCombined(
		provider,
		store,
		oauthConfig(deps.Config),
		logger,
	)
	if err != nil {
		store.Stop()
		return nil, fmt.Errorf("create OAuth server: %w", err)
	}

	mcpServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "mcp-obsidian", Version: deps.Version}, nil)
	obsmcp.Register(mcpServer, obsmcp.Dependencies{
		Config: deps.Config,
		Vault:  deps.Vault,
		Sync:   deps.Sync,
	})

	mux := http.NewServeMux()
	oauthHandler := oauthhandler.New(oauthServer, logger)
	oauthHandler.RegisterOAuthRoutes(mux, oauthhandler.OAuthRoutesOptions{
		MCPPath:         "/mcp",
		IncludeMetadata: true,
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return mcpServer
	}, &mcpsdk.StreamableHTTPOptions{Logger: logger})
	protectedMCP := mcpauth.RequireBearerToken(obsauth.TokenVerifier(oauthServer), &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: deps.Config.HTTP.PublicBaseURL + "/.well-known/oauth-protected-resource/mcp",
	})(mcpHandler)
	mux.Handle("/mcp", protectedMCP)

	srv := &http.Server{
		Addr:    deps.Config.HTTP.Addr,
		Handler: mux,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
	return srv, nil
}

func newGitHubProvider(cfg config.Config) (oauthproviders.Provider, error) {
	gh, err := github.NewProvider(&github.Config{
		ClientID:     cfg.OAuth.GitHubClientID,
		ClientSecret: cfg.OAuth.GitHubClientSecret,
		RedirectURL:  cfg.HTTP.PublicBaseURL + "/oauth/callback",
		Scopes:       []string{"read:user", "user:email"},
	})
	if err != nil {
		return nil, fmt.Errorf("create GitHub provider: %w", err)
	}
	provider, err := obsauth.NewGitHubAllowlistProvider(gh, cfg.OAuth.GitHubAllowedUsers)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func oauthConfig(cfg config.Config) *oauth.ServerConfig {
	resource := cfg.HTTP.PublicBaseURL + "/mcp"
	return &oauth.ServerConfig{
		Issuer:                 cfg.HTTP.PublicBaseURL,
		AllowInsecureHTTP:      strings.HasPrefix(cfg.HTTP.PublicBaseURL, "http://localhost") || strings.HasPrefix(cfg.HTTP.PublicBaseURL, "http://127.0.0.1"),
		SupportedScopes:        cfg.OAuth.Scopes,
		DefaultChallengeScopes:  cfg.OAuth.Scopes,
		ResourceIdentifier:     resource,
		EnableRevocationEndpoint: true,
		ResourceMetadataByPath: map[string]oauthserver.ProtectedResourceConfig{
			"/mcp": {
				ScopesSupported:     cfg.OAuth.Scopes,
				ResourceIdentifier: resource,
			},
		},
	}
}
