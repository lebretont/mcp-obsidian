# mcp-obsidian

Go MCP server for an Obsidian vault stored locally, with optional Markdown-only S3-compatible sync.

The local vault is the source of truth. S3 is used as an optional mirror so a container can bootstrap from a bucket, refresh before tool calls, and push changes after local writes.

## Features

- MCP stdio transport.
- Tools: `list_notes`, `read_note`, `search_notes`, `create_note`, `update_note`, `append_note`, `delete_note`, `sync_status`, `sync_now`.
- Local vault is canonical.
- Optional S3-compatible sync using AWS SDK for Go.
- Blocking pull before MCP operations when the last pull is older than `S3_SYNC_INTERVAL_MINUTES`.
- Push after local writes; if push fails, the local write still succeeds and the tool returns a warning.
- Markdown-only sync for `**/*.md`.
- Strict path validation: relative UTF-8 paths only, no absolute paths, no `..`, no backslashes, no control characters, `.md` required.
- Symlink resolution for local file operations so note paths cannot escape the vault.
- Atomic local writes.
- Docker image designed for a persistent `/vault` volume.

## Tools

| Tool | Description |
| --- | --- |
| `list_notes` | List Markdown notes. Supports `path_prefix` and `limit`. |
| `read_note` | Read a note by relative `.md` path. |
| `search_notes` | Scan notes line by line. Supports `query`, `case_sensitive`, `path_prefix`, and `limit`. |
| `create_note` | Create a new note. Fails if the path already exists. |
| `update_note` | Replace a note's full content atomically. |
| `append_note` | Append content to an existing note atomically. |
| `delete_note` | Delete a note only when `ALLOW_DELETE=true`. |
| `sync_status` | Return S3 sync status, sync age, and the last sync error. |
| `sync_now` | Force a blocking pull followed by a push. |

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `OBSIDIAN_VAULT_PATH` | `/vault` | Local vault path. |
| `ALLOW_DELETE` | `false` | Enables `delete_note`. |
| `S3_ENABLED` | implicit | If unset, S3 is enabled when `S3_BUCKET` is set. |
| `AWS_ACCESS_KEY_ID` | | S3 access key. |
| `AWS_SECRET_ACCESS_KEY` | | S3 secret key. |
| `AWS_SESSION_TOKEN` | | Optional session token. |
| `AWS_REGION` | `us-east-1` | AWS region. |
| `S3_BUCKET` | | Bucket name. |
| `S3_PREFIX` | | Optional key prefix. |
| `S3_ENDPOINT` | | Optional endpoint for MinIO, R2, Scaleway, etc. |
| `S3_FORCE_PATH_STYLE` | `false` | Use path-style addressing. |
| `S3_SYNC_INTERVAL_MINUTES` | `10` | Pull interval threshold. `0` means pull before every operation. |
| `S3_SYNC_DELETE` | `false` | Delete S3 objects when manifested local Markdown files are deleted. Enable only with S3 versioning. |

S3 sync is enabled implicitly when `S3_BUCKET` is set. Set `S3_ENABLED=false` to force local-only mode even if S3 variables are present.

For S3-compatible providers, set `S3_ENDPOINT`. MinIO often also needs `S3_FORCE_PATH_STYLE=true`.

## Docker

```bash
docker run --rm -i \
  -v obsidian-vault:/vault \
  -e S3_BUCKET=my-notes \
  -e AWS_REGION=eu-west-3 \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  ghcr.io/OWNER/REPO:latest
```

The image does not contain notes. Mount a persistent volume at `/vault`. If the vault has no Markdown notes and S3 is configured, the first MCP operation performs an initial pull.

## MCP client example

Use stdio. A typical client configuration looks like this:

```json
{
  "mcpServers": {
    "obsidian": {
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "-i",
        "-v",
        "obsidian-vault:/vault",
        "-e",
        "S3_BUCKET=my-notes",
        "-e",
        "AWS_REGION=eu-west-3",
        "-e",
        "AWS_ACCESS_KEY_ID",
        "-e",
        "AWS_SECRET_ACCESS_KEY",
        "ghcr.io/OWNER/REPO:latest"
      ]
    }
  }
}
```

## Sync model

Only `**/*.md` files are synchronized. Paths are stored in S3 with `/` separators and must be relative UTF-8 Markdown paths. Absolute paths, `..`, backslashes, control characters, and non-`.md` paths are rejected. Existing symlinks are resolved and rejected when they escape the vault root.

Before each tool call, the server performs a blocking S3 pull when the vault is empty, or when a previous pull exists and is older than `S3_SYNC_INTERVAL_MINUTES`. A non-empty local vault without a sync manifest is treated as canonical and is not overwritten by an implicit first pull. Use `sync_now` if you explicitly want to reconcile it. After successful local writes, the server pushes changed Markdown files. If that push fails, the tool still reports the local write as successful and includes a warning; `sync_status` keeps the last error.

Remote deletion is opt-in with `S3_SYNC_DELETE=true`. Keep bucket versioning enabled before turning it on.

## Development

```bash
go test ./...
go run ./cmd/mcp-obsidian
```

Logs are written to stderr so they do not pollute MCP stdio.
