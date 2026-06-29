# mcp-obsidian

Go MCP server for an Obsidian vault stored locally, with optional Markdown-only S3-compatible sync.

## Features

- MCP stdio transport.
- Tools: `list_notes`, `read_note`, `search_notes`, `create_note`, `update_note`, `append_note`, `delete_note`, `sync_status`, `sync_now`.
- Local vault is canonical.
- Optional S3-compatible sync using AWS SDK for Go.
- Blocking pull before MCP operations when the last pull is older than `S3_SYNC_INTERVAL_MINUTES`.
- Push after local writes; if push fails, the local write still succeeds and the tool returns a warning.
- Markdown-only sync for `**/*.md`.
- Strict path validation: relative UTF-8 paths only, no absolute paths, no `..`, no backslashes, no control characters, `.md` required.
- Atomic local writes.
- Docker image designed for a persistent `/vault` volume.

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
| `S3_SYNC_DELETE` | `true` | Delete S3 objects when manifested local Markdown files are deleted. Use S3 versioning. |

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

## Development

```bash
go test ./...
go run ./cmd/mcp-obsidian
```

Logs are written to stderr so they do not pollute MCP stdio.
