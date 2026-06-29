package config

import "testing"

func TestLoadDefaults(t *testing.T) {
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
}

func TestS3ImplicitEnable(t *testing.T) {
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
