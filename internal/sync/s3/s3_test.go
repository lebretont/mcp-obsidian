package s3

import (
	"testing"

	"github.com/dibou/mcp-obsidian/internal/config"
)

func TestS3KeyAndRelativePathUseExactPrefix(t *testing.T) {
	syncer := &Syncer{cfg: config.S3Config{Prefix: "vault/prefix/"}}

	if got := syncer.key("folder/note.md"); got != "vault/prefix/folder/note.md" {
		t.Fatalf("unexpected key: %q", got)
	}
	if got := syncer.relFromKey("vault/prefix/folder/note.md"); got != "folder/note.md" {
		t.Fatalf("unexpected relative path: %q", got)
	}
	if got := syncer.relFromKey("vault/prefixology/note.md"); got != "vault/prefixology/note.md" {
		t.Fatalf("prefix without slash should not be stripped: %q", got)
	}
}

func TestS3KeyWithoutPrefix(t *testing.T) {
	syncer := &Syncer{}

	if got := syncer.key("folder/note.md"); got != "folder/note.md" {
		t.Fatalf("unexpected key: %q", got)
	}
	if got := syncer.relFromKey("folder/note.md"); got != "folder/note.md" {
		t.Fatalf("unexpected relative path: %q", got)
	}
}
