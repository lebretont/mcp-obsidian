# mcp-obsidian

Remote MCP server for an Obsidian vault stored locally in a container, with optional Markdown-only S3-compatible sync.

The local vault is the source of truth. S3 is an optional mirror so the container can bootstrap from a bucket, refresh before tool calls, and push changes after local writes.

## Features

- MCP Streamable HTTP endpoint on `/mcp` using `github.com/modelcontextprotocol/go-sdk/mcp`.
- OAuth 2.1 flow for remote clients using `github.com/giantswarm/mcp-oauth`.
- GitHub OAuth provider with mandatory username allowlist.
- Tools: `list_notes`, `read_note`, `search_notes`, `create_note`, `update_note`, `append_note`, `delete_note`, `sync_status`, `sync_now`.
- Read-write mode for ChatGPT; `delete_note` still requires `ALLOW_DELETE=true`.
- Optional S3-compatible sync using AWS SDK for Go.
- Markdown-only sync for `**/*.md`.
- Strict path validation and atomic local writes.
- Docker image designed for persistent `/vault` and `/data` volumes.

## HTTP Endpoints

| Endpoint | Description |
| --- | --- |
| `/mcp` | Streamable HTTP MCP endpoint, protected by OAuth. |
| `/healthz` | Unauthenticated health probe. |
| `/oauth/authorize` | OAuth authorization endpoint. |
| `/oauth/callback` | GitHub OAuth callback endpoint. |
| `/oauth/token` | OAuth token endpoint. |
| `/oauth/revoke` | OAuth revocation endpoint. |
| `/oauth/register` | Dynamic client registration endpoint. |
| `/.well-known/oauth-protected-resource` | MCP protected resource metadata. |
| `/.well-known/oauth-authorization-server` | OAuth authorization server metadata. |

The service expects TLS to be handled by your reverse proxy. Do not expose the container directly without HTTPS in front of it.

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

### Server and OAuth

| Variable | Default | Description |
| --- | --- | --- |
| `MCP_HTTP_ADDR` | `:8080` | HTTP listen address inside the container. |
| `PUBLIC_BASE_URL` | required | Public HTTPS base URL, for example `https://obsidian-mcp.example.com`. |
| `OAUTH_GITHUB_CLIENT_ID` | required | GitHub OAuth App client ID. |
| `OAUTH_GITHUB_CLIENT_SECRET` | required | GitHub OAuth App client secret. |
| `OAUTH_GITHUB_ALLOWED_USERS` | required | Comma-separated GitHub usernames allowed to access the MCP. |
| `OAUTH_SQLITE_PATH` | `/data/oauth.db` | SQLite database used for OAuth clients, flows, tokens, and token metadata. |
| `OAUTH_REGISTRATION_ACCESS_TOKEN` | | Bearer token required by `/oauth/register` when public client registration is disabled. |
| `OAUTH_ALLOW_PUBLIC_CLIENT_REGISTRATION` | `false` | Allows clients such as ChatGPT to call `/oauth/register` without a registration token. Enable only if you accept public dynamic client registration. |
| `ALLOW_DELETE` | `false` | Enables `delete_note`. |

GitHub OAuth App settings:

- Homepage URL: `https://obsidian-mcp.example.com`
- Authorization callback URL: `https://obsidian-mcp.example.com/oauth/callback`

ChatGPT/OpenAI Apps SDK should be pointed at:

```text
https://obsidian-mcp.example.com/mcp
```

For ChatGPT client registration, either set `OAUTH_ALLOW_PUBLIC_CLIENT_REGISTRATION=true` so ChatGPT can obtain its own OAuth client ID from `/oauth/register`, or keep it `false` and provide `OAUTH_REGISTRATION_ACCESS_TOKEN` to trusted registration clients.

### Vault and S3

| Variable | Default | Description |
| --- | --- | --- |
| `OBSIDIAN_VAULT_PATH` | `/vault` | Local vault path. |
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

## Docker

```bash
docker run --rm \
  -p 127.0.0.1:8080:8080 \
  -v obsidian-vault:/vault \
  -v obsidian-mcp-data:/data \
  -e PUBLIC_BASE_URL=https://obsidian-mcp.example.com \
  -e OAUTH_GITHUB_CLIENT_ID=... \
  -e OAUTH_GITHUB_CLIENT_SECRET=... \
  -e OAUTH_GITHUB_ALLOWED_USERS=your-github-login \
  -e OAUTH_ALLOW_PUBLIC_CLIENT_REGISTRATION=true \
  -e ALLOW_DELETE=false \
  -e S3_BUCKET=my-notes \
  -e AWS_REGION=eu-west-3 \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  ghcr.io/OWNER/REPO:latest
```

The image does not contain notes. Mount a persistent volume at `/vault`. If the vault has no Markdown notes and S3 is configured, the first MCP operation performs an initial pull.

`/data` stores the OAuth SQLite database. Keep this volume persistent so ChatGPT does not need to repeat registration and authorization after each restart.

## Sync Model

Only `**/*.md` files are synchronized. Paths are stored in S3 with `/` separators and must be relative UTF-8 Markdown paths. Absolute paths, `..`, backslashes, control characters, and non-`.md` paths are rejected. Existing symlinks are resolved and rejected when they escape the vault root.

Before each tool call, the server performs a blocking S3 pull when the vault is empty, or when a previous pull exists and is older than `S3_SYNC_INTERVAL_MINUTES`. A non-empty local vault without a sync manifest is treated as canonical and is not overwritten by an implicit first pull. Use `sync_now` if you explicitly want to reconcile it. After successful local writes, the server pushes changed Markdown files. If that push fails, the tool still reports the local write as successful and includes a warning; `sync_status` keeps the last error.

Remote deletion is opt-in with `S3_SYNC_DELETE=true`. Keep bucket versioning enabled before turning it on.

## Security Notes

- Keep the service behind a TLS reverse proxy.
- Bind the container port to localhost or a private Docker network when possible.
- Set `OAUTH_GITHUB_ALLOWED_USERS` to your exact GitHub username; empty allowlists are rejected at startup.
- Start with `ALLOW_DELETE=false` and `S3_SYNC_DELETE=false`, then enable them deliberately.
- Keep S3 bucket versioning enabled before allowing deletes.
- Logs go to stderr and should not include note contents.

## Development

```bash
go test ./...
go run ./cmd/mcp-obsidian
```

Required local environment for development:

```bash
export PUBLIC_BASE_URL=http://localhost:8080
export OAUTH_GITHUB_CLIENT_ID=...
export OAUTH_GITHUB_CLIENT_SECRET=...
export OAUTH_GITHUB_ALLOWED_USERS=your-github-login
```
