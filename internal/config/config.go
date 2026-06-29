package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	VaultPath   string
	AllowDelete bool
	S3          S3Config
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
			DeleteRemote:    getenvBool("S3_SYNC_DELETE", true),
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
