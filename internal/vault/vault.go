package vault

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dibou/mcp-obsidian/internal/state"
)

var ErrInvalidPath = errors.New("invalid note path")

type Service struct {
	root        string
	allowDelete bool
}

type NoteInfo struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type SearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

func New(root string, allowDelete bool) (*Service, error) {
	if root == "" {
		return nil, fmt.Errorf("vault path cannot be empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Service{root: abs, allowDelete: allowDelete}, nil
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) IsMarkdownEmpty() (bool, error) {
	notes, err := s.List("", 1)
	if err != nil {
		return false, err
	}
	return len(notes) == 0, nil
}

func ValidateNotePath(input string) (string, error) {
	if input == "" || !utf8.ValidString(input) {
		return "", ErrInvalidPath
	}
	if strings.Contains(input, "\\") || strings.ContainsRune(input, '\x00') {
		return "", ErrInvalidPath
	}
	for _, r := range input {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidPath
		}
	}
	if path.IsAbs(input) || filepath.IsAbs(input) {
		return "", ErrInvalidPath
	}
	clean := path.Clean(input)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", ErrInvalidPath
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func ValidatePathPrefix(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if !utf8.ValidString(input) || strings.Contains(input, "\\") || strings.ContainsRune(input, '\x00') {
		return "", ErrInvalidPath
	}
	for _, r := range input {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidPath
		}
	}
	if path.IsAbs(input) || filepath.IsAbs(input) {
		return "", ErrInvalidPath
	}
	clean := path.Clean(input)
	if clean == "." {
		return "", nil
	}
	if strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", ErrInvalidPath
	}
	return strings.TrimSuffix(clean, "/") + "/", nil
}

func (s *Service) fullPath(rel string) string {
	return filepath.Join(s.root, filepath.FromSlash(rel))
}

func (s *Service) List(prefix string, limit int) ([]NoteInfo, error) {
	cleanPrefix, err := ValidatePathPrefix(prefix)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 1000
	}
	var notes []NoteInfo
	err = filepath.WalkDir(s.root, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == state.DirName || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(s.root, filePath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(strings.ToLower(rel), ".md") || !strings.HasPrefix(rel, cleanPrefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		notes = append(notes, NoteInfo{Path: rel, Size: info.Size(), ModTime: info.ModTime().UTC()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	if limit > 0 && len(notes) > limit {
		notes = notes[:limit]
	}
	return notes, nil
}

func (s *Service) Read(rel string) (string, error) {
	clean, err := ValidateNotePath(rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(s.fullPath(clean))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) Create(rel, content string) error {
	clean, err := ValidateNotePath(rel)
	if err != nil {
		return err
	}
	final := s.fullPath(clean)
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("note already exists: %s", clean)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return atomicWrite(final, []byte(content), 0o644)
}

func (s *Service) Update(rel, content string) error {
	clean, err := ValidateNotePath(rel)
	if err != nil {
		return err
	}
	return atomicWrite(s.fullPath(clean), []byte(content), 0o644)
}

func (s *Service) Append(rel, content string) error {
	current, err := s.Read(rel)
	if errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err != nil {
		return err
	}
	return s.Update(rel, current+content)
}

func (s *Service) Delete(rel string) error {
	if !s.allowDelete {
		return fmt.Errorf("delete disabled; set ALLOW_DELETE=true")
	}
	clean, err := ValidateNotePath(rel)
	if err != nil {
		return err
	}
	return os.Remove(s.fullPath(clean))
}

func (s *Service) Search(query, prefix string, caseSensitive bool, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 20
	}
	notes, err := s.List(prefix, -1)
	if err != nil {
		return nil, err
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	var results []SearchResult
	for _, note := range notes {
		file, err := os.Open(s.fullPath(note.Path))
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			haystack := line
			if !caseSensitive {
				haystack = strings.ToLower(haystack)
			}
			if strings.Contains(haystack, needle) {
				results = append(results, SearchResult{Path: note.Path, Line: lineNo, Snippet: trimSnippet(line)})
				if len(results) >= limit {
					_ = file.Close()
					return results, nil
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func atomicWrite(final string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), ".tmp-*.md")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, final)
}

func trimSnippet(line string) string {
	line = strings.TrimSpace(line)
	const max = 240
	if len(line) <= max {
		return line
	}
	return line[:max] + "..."
}

func CopyFile(dst string, src io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".download-*.md")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
