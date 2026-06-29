package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	VaultPath   string
	AllowDelete bool
	HTTP        HTTPConfig
	OAuth       OAuthConfig
	S3          S3Config
}

type HTTPConfig struct {
	Addr          string
	PublicBaseURL string
}

type OAuthConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	GitHubAllowedUsers []string
	SQLitePath         string
	Scopes             []string
}

type S3Config struct {
	Enabled         bool
	Bucket          string
	Prefix          string
	Region          string
	Endpoint        string
	ForcePathStyle  bool
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	SyncInterval    time.Duration
	DeleteRemote    bool
}

func Load() (Config, error) {
	vaultPath := getenv("OBSIDIAN_VAULT_PATH", "/vault")
	if strings.TrimSpace(vaultPath) == "" {
		return Config{}, fmt.Errorf("OBSIDIAN_VAULT_PATH cannot be empty")
	}
	publicBaseURL, err := cleanBaseURL(os.Getenv("PUBLIC_BASE_URL"))
	if err != nil {
		return Config{}, err
	}
	githubClientID := strings.TrimSpace(os.Getenv("OAUTH_GITHUB_CLIENT_ID"))
	if githubClientID == "" {
		githubClientID = strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID"))
	}
	if githubClientID == "" {
		return Config{}, fmt.Errorf("OAUTH_GITHUB_CLIENT_ID is required")
	}
	githubClientSecret := strings.TrimSpace(os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"))
	if githubClientSecret == "" {
		githubClientSecret = strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET"))
	}
	if githubClientSecret == "" {
		return Config{}, fmt.Errorf("OAUTH_GITHUB_CLIENT_SECRET is required")
	}
	allowedUsers := splitList(os.Getenv("OAUTH_GITHUB_ALLOWED_USERS"))
	if len(allowedUsers) == 0 {
		return Config{}, fmt.Errorf("OAUTH_GITHUB_ALLOWED_USERS is required in HTTP/OAuth mode")
	}

	allowDelete := getenvBool("ALLOW_DELETE", false)
	bucket := strings.TrimSpace(os.Getenv("S3_BUCKET"))
	explicitS3 := strings.TrimSpace(os.Getenv("S3_ENABLED"))
	s3Enabled := bucket != ""
	if explicitS3 != "" {
		s3Enabled = getenvBool("S3_ENABLED", false)
	}

	intervalMinutes := getenvInt("S3_SYNC_INTERVAL_MINUTES", 10)
	if intervalMinutes < 0 {
		return Config{}, fmt.Errorf("S3_SYNC_INTERVAL_MINUTES must be >= 0")
	}

	cfg := Config{
		VaultPath:   vaultPath,
		AllowDelete: allowDelete,
		HTTP: HTTPConfig{
			Addr:          getenv("MCP_HTTP_ADDR", ":8080"),
			PublicBaseURL: publicBaseURL,
		},
		OAuth: OAuthConfig{
			GitHubClientID:     githubClientID,
			GitHubClientSecret: githubClientSecret,
			GitHubAllowedUsers: allowedUsers,
			SQLitePath:         getenv("OAUTH_SQLITE_PATH", "/data/oauth.db"),
			Scopes:             []string{"notes:read", "notes:write", "notes:delete", "sync:run"},
		},
		S3: S3Config{
			Enabled:         s3Enabled,
			Bucket:          bucket,
			Prefix:          cleanPrefix(os.Getenv("S3_PREFIX")),
			Region:          getenv("AWS_REGION", "us-east-1"),
			Endpoint:        strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
			ForcePathStyle:  getenvBool("S3_FORCE_PATH_STYLE", false),
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
			SyncInterval:    time.Duration(intervalMinutes) * time.Minute,
			DeleteRemote:    getenvBool("S3_SYNC_DELETE", false),
		},
	}
	if cfg.S3.Enabled && cfg.S3.Bucket == "" {
		return Config{}, fmt.Errorf("S3_BUCKET is required when S3_ENABLED=true")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/"))
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	return prefix + "/"
}

func cleanBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL is required in HTTP/OAuth mode")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("PUBLIC_BASE_URL must be an absolute URL")
	}
	if u.Scheme != "https" && !isLocalHTTP(u) {
		return "", fmt.Errorf("PUBLIC_BASE_URL must use https outside localhost")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func isLocalHTTP(u *url.URL) bool {
	if u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func splitList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		value := strings.ToLower(strings.TrimSpace(field))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
