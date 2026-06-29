package state

import (
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	now := time.Now().UTC().Truncate(time.Second)
	in := Status{
		LastPull: now,
		Entries: map[string]FileEntry{
			"a.md": {Path: "a.md", Size: 1, LocalSize: 1, LocalModTime: now},
		},
	}
	if err := store.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !out.LastPull.Equal(now) {
		t.Fatalf("unexpected last pull: %v", out.LastPull)
	}
	if out.Entries["a.md"].Path != "a.md" {
		t.Fatalf("entry not loaded: %#v", out.Entries)
	}
}
