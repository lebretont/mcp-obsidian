package vault

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateNotePath(t *testing.T) {
	valid := []string{
		"note.md",
		"Projects/été 2026/idées.md",
		"with spaces/file.md",
	}
	for _, input := range valid {
		if _, err := ValidateNotePath(input); err != nil {
			t.Fatalf("expected %q to be valid: %v", input, err)
		}
	}

	invalid := []string{
		"",
		"note.txt",
		"../note.md",
		"/note.md",
		`dir\note.md`,
		"dir/\x00note.md",
	}
	for _, input := range invalid {
		if _, err := ValidateNotePath(input); err == nil {
			t.Fatalf("expected %q to be invalid", input)
		}
	}
}

func TestVaultOperationsAndSearch(t *testing.T) {
	root := t.TempDir()
	svc, err := New(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("folder/a.md", "hello\nworld\n"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("folder/b.md", "another hello\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	notes, err := svc.List("folder", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}

	content, err := svc.Read("folder/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello\nworld\n" {
		t.Fatalf("unexpected content: %q", content)
	}

	results, err := svc.Search("HELLO", "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 search results, got %d", len(results))
	}
	for _, result := range results {
		if result.MatchType != "content" {
			t.Fatalf("expected content match, got %q", result.MatchType)
		}
		if result.Score <= 0 {
			t.Fatalf("expected positive search score, got %d", result.Score)
		}
	}

	if err := svc.Append("folder/a.md", "again\n"); err != nil {
		t.Fatal(err)
	}
	content, err = svc.Read("folder/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello\nworld\nagain\n" {
		t.Fatalf("unexpected appended content: %q", content)
	}

	if err := svc.Delete("folder/b.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Read("folder/b.md"); err == nil {
		t.Fatal("expected deleted note to be unreadable")
	}
}

func TestSearchMatchesFileTitleAndSimplePlural(t *testing.T) {
	svc, err := New(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("Recettes/Glaces.md", "# Desserts\n"); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search("recette de glace", "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d: %#v", len(results), results)
	}
	result := results[0]
	if result.Path != "Recettes/Glaces.md" {
		t.Fatalf("expected Glaces.md result, got %q", result.Path)
	}
	if result.Line != 0 {
		t.Fatalf("expected title match line 0, got %d", result.Line)
	}
	if result.MatchType != "title" {
		t.Fatalf("expected title match, got %q", result.MatchType)
	}

	results, err = svc.Search("glace", "", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected case-sensitive search not to match Glaces.md, got %#v", results)
	}
}

func TestSearchNoResultsReturnsEmptyJSONList(t *testing.T) {
	svc, err := New(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("note.md", "hello\n"); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search("missing", "", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if results == nil {
		t.Fatal("expected empty result slice, got nil")
	}
	data, err := json.Marshal(struct {
		Results []SearchResult `json:"results"`
	}{Results: results})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"results":[]}` {
		t.Fatalf("expected empty JSON list, got %s", data)
	}
}

func TestDeleteDisabled(t *testing.T) {
	svc, err := New(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Create("a.md", "x"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete("a.md"); err == nil {
		t.Fatal("expected delete to be disabled")
	}
}

func TestRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	svc, err := New(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := svc.Create("linked/escape.md", "nope"); err == nil {
		t.Fatal("expected symlink escape write to be rejected")
	}
	if err := os.WriteFile(filepath.Join(outside, "escape.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Read("linked/escape.md"); err == nil {
		t.Fatal("expected symlink escape read to be rejected")
	}
}
