package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	setRequiredOAuthEnv(t)
	t.Setenv("OBSIDIAN_VAULT_PATH", "")
	t.Setenv("S3_BUCKET", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VaultPath != "/vault" {
		t.Fatalf("unexpected vault path: %q", cfg.VaultPath)
	}
	if cfg.S3.Enabled {
		t.Fatal("S3 should be disabled without S3_BUCKET")
	}
	if cfg.AllowDelete {
		t.Fatal("delete should default to false")
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Fatalf("unexpected HTTP addr: %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.PublicBaseURL != "https://obsidian.example.com" {
		t.Fatalf("unexpected public base URL: %q", cfg.HTTP.PublicBaseURL)
	}
	if cfg.OAuth.SQLitePath != "/data/oauth.db" {
		t.Fatalf("unexpected OAuth SQLite path: %q", cfg.OAuth.SQLitePath)
	}
	if cfg.OAuth.RegistrationAccessToken != "" {
		t.Fatal("registration access token should default to empty")
	}
	if cfg.OAuth.AllowPublicClientRegistration {
		t.Fatal("public client registration should default to false")
	}
	if len(cfg.OAuth.GitHubAllowedUsers) != 1 || cfg.OAuth.GitHubAllowedUsers[0] != "dibou" {
		t.Fatalf("unexpected allowed users: %#v", cfg.OAuth.GitHubAllowedUsers)
	}
}

func TestOAuthRegistrationConfig(t *testing.T) {
	setRequiredOAuthEnv(t)
	t.Setenv("OAUTH_REGISTRATION_ACCESS_TOKEN", "registration-secret")
	t.Setenv("OAUTH_ALLOW_PUBLIC_CLIENT_REGISTRATION", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuth.RegistrationAccessToken != "registration-secret" {
		t.Fatalf("unexpected registration access token: %q", cfg.OAuth.RegistrationAccessToken)
	}
	if !cfg.OAuth.AllowPublicClientRegistration {
		t.Fatal("expected public client registration to be enabled")
	}
}

func TestS3ImplicitEnable(t *testing.T) {
	setRequiredOAuthEnv(t)
	t.Setenv("S3_BUCKET", "bucket")
	t.Setenv("S3_PREFIX", "/vault/prefix/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.S3.Enabled {
		t.Fatal("S3 should be enabled when S3_BUCKET is set")
	}
	if cfg.S3.Prefix != "vault/prefix/" {
		t.Fatalf("unexpected prefix: %q", cfg.S3.Prefix)
	}
	if cfg.S3.DeleteRemote {
		t.Fatal("remote deletes should be opt-in")
	}
}

func TestLoadRequiresAllowedUsers(t *testing.T) {
	t.Setenv("PUBLIC_BASE_URL", "https://obsidian.example.com")
	t.Setenv("OAUTH_GITHUB_CLIENT_ID", "client")
	t.Setenv("OAUTH_GITHUB_CLIENT_SECRET", "secret")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing allowed users to fail")
	}
}

func TestLoadRejectsInsecurePublicURL(t *testing.T) {
	setRequiredOAuthEnv(t)
	t.Setenv("PUBLIC_BASE_URL", "http://obsidian.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure PUBLIC_BASE_URL to fail")
	}
}

func setRequiredOAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PUBLIC_BASE_URL", "https://obsidian.example.com/")
	t.Setenv("OAUTH_GITHUB_CLIENT_ID", "client")
	t.Setenv("OAUTH_GITHUB_CLIENT_SECRET", "secret")
	t.Setenv("OAUTH_GITHUB_ALLOWED_USERS", "dibou")
}
