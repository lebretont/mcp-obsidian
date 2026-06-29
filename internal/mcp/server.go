package mcp

import (
	"context"
	"fmt"

	"github.com/dibou/mcp-obsidian/internal/config"
	syncapi "github.com/dibou/mcp-obsidian/internal/sync"
	"github.com/dibou/mcp-obsidian/internal/vault"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Dependencies struct {
	Config config.Config
	Vault  *vault.Service
	Sync   syncapi.Syncer
}

type listNotesParams struct {
	PathPrefix string `json:"path_prefix,omitempty" jsonschema:"Optional path prefix filter."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum number of notes to return."`
}

type readNoteParams struct {
	Path string `json:"path" jsonschema:"Relative UTF-8 .md path inside the vault."`
}

type noteContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type searchNotesParams struct {
	Query         string `json:"query" jsonschema:"Text to search for."`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	PathPrefix    string `json:"path_prefix,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type writeNoteParams struct {
	Path    string `json:"path" jsonschema:"Relative UTF-8 .md path inside the vault."`
	Content string `json:"content"`
}

type pathParams struct {
	Path string `json:"path" jsonschema:"Relative UTF-8 .md path inside the vault."`
}

type writeResult struct {
	OK      bool   `json:"ok"`
	Warning string `json:"warning,omitempty"`
}

type emptyParams struct{}

func Register(server *mcpsdk.Server, deps Dependencies) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "list_notes", Description: "List Markdown notes in the Obsidian vault."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params listNotesParams) (*mcpsdk.CallToolResult, []vault.NoteInfo, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, nil, err
		}
		notes, err := deps.Vault.List(params.PathPrefix, params.Limit)
		return nil, notes, err
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "read_note", Description: "Read a Markdown note by relative path."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params readNoteParams) (*mcpsdk.CallToolResult, noteContent, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, noteContent{}, err
		}
		content, err := deps.Vault.Read(params.Path)
		if err != nil {
			return nil, noteContent{}, err
		}
		return nil, noteContent{Path: params.Path, Content: content}, nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "search_notes", Description: "Search Markdown notes with a simple line scan."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params searchNotesParams) (*mcpsdk.CallToolResult, []vault.SearchResult, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, nil, err
		}
		results, err := deps.Vault.Search(params.Query, params.PathPrefix, params.CaseSensitive, params.Limit)
		return nil, results, err
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "create_note", Description: "Create a new Markdown note; fails if it already exists."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params writeNoteParams) (*mcpsdk.CallToolResult, writeResult, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, writeResult{}, err
		}
		if err := deps.Vault.Create(params.Path, params.Content); err != nil {
			return nil, writeResult{}, err
		}
		return nil, writeOutput(ctx, deps.Sync), nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "update_note", Description: "Replace a Markdown note's complete content atomically."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params writeNoteParams) (*mcpsdk.CallToolResult, writeResult, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, writeResult{}, err
		}
		if err := deps.Vault.Update(params.Path, params.Content); err != nil {
			return nil, writeResult{}, err
		}
		return nil, writeOutput(ctx, deps.Sync), nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "append_note", Description: "Append content to an existing Markdown note atomically."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params writeNoteParams) (*mcpsdk.CallToolResult, writeResult, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, writeResult{}, err
		}
		if err := deps.Vault.Append(params.Path, params.Content); err != nil {
			return nil, writeResult{}, err
		}
		return nil, writeOutput(ctx, deps.Sync), nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "delete_note", Description: "Delete a Markdown note when ALLOW_DELETE=true."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, params pathParams) (*mcpsdk.CallToolResult, writeResult, error) {
		if err := deps.Sync.EnsureFresh(ctx); err != nil {
			return nil, writeResult{}, err
		}
		if err := deps.Vault.Delete(params.Path); err != nil {
			return nil, writeResult{}, err
		}
		return nil, writeOutput(ctx, deps.Sync), nil
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "sync_status", Description: "Return S3 sync status and last error."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyParams) (*mcpsdk.CallToolResult, syncapi.Status, error) {
		status, err := deps.Sync.Status(ctx)
		return nil, status, err
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{Name: "sync_now", Description: "Force a blocking S3 pull followed by a push."}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ emptyParams) (*mcpsdk.CallToolResult, syncapi.Status, error) {
		if err := deps.Sync.Pull(ctx); err != nil {
			return nil, syncapi.Status{}, err
		}
		if err := deps.Sync.Push(ctx); err != nil {
			return nil, syncapi.Status{}, err
		}
		status, err := deps.Sync.Status(ctx)
		return nil, status, err
	})
}

func writeOutput(ctx context.Context, s syncapi.Syncer) writeResult {
	if err := s.Push(ctx); err != nil {
		return writeResult{OK: true, Warning: fmt.Sprintf("local write succeeded but S3 push failed: %v", err)}
	}
	return writeResult{OK: true}
}
