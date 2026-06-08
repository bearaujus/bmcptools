# bmcptools

> An MCP server that exposes **44 developer tools** to any MCP-compatible LLM client — file I/O, shell execution, search, system info, and interactive user dialogs.

[![Go](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/bearaujus/bmcptools)](https://github.com/bearaujus/bmcptools/releases)
[![SafeSkill 100/100](https://img.shields.io/badge/SafeSkill-100%2F100_Verified%20Safe-brightgreen)](https://safeskill.dev/scan?pkg=bearaujus%2Fbmcptools)

Communication happens over **stdio** using the [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) library.

### Why bmcptools?

- **44 tools, one binary** — covers file ops, search, shell, system, HTTP, clipboard, and user interaction.
  Disable whole groups with `--disable=...` or exact tools with `--exclude-tools=...` (see [Filter tools at launch](#filter-tools-at-launch)).
- **Built-in server instructions** — the AI receives a categorized guide on when and how to use each tool, trimmed to match the exposed tool set.
- **Interactive dialogs** — `ask_user` opens a browser dialog with choices, markdown, pasted/dropped images, live updates, and typing indicators.
- **Cross-platform** — macOS, Windows, and Linux (browser dialogs require macOS/Windows; `notify_user` works everywhere).
- **Surgical editing** — `edit_file` with near-miss probe, CRLF transparency, batch mode, and regex support.
- **Performance-first** — batch tools (`read_multiple_files`, `write_multiple_files`, `path_exists_batch`, `get_multiple_file_info`, `edit_file` batch mode) reduce round-trips.

---

## Quick Start

### Installation

Pick one of these install paths:

| Method | When to use it | Command / action |
|--------|----------------|------------------|
| Prebuilt release binary | Recommended for most users | Download the right artifact from [Releases](https://github.com/bearaujus/bmcptools/releases) and place it somewhere stable on your machine |
| `go install` | You already use Go and want the latest tagged CLI | `go install github.com/bearaujus/bmcptools/cmd/bmcptools@latest` |
| Build from source | You want to hack on the repo or pin to local changes | `make build` |

After installation, verify the binary works:

```bash
bmcptools --version
bmcptools --list-groups
bmcptools --list-tools
```

If you use a downloaded Windows binary, keep the `.exe` filename in your MCP config. If you use `go install`, make sure your `GOBIN` or Go bin directory is on `PATH`.

### Integrate with your MCP client

#### Codex

Official OpenAI Codex docs say MCP configuration is shared between the Codex CLI and IDE extension through `~/.codex/config.toml`. You can add `bmcptools` from the CLI:

```bash
# Expose all tools
codex mcp add bmcptools -- /absolute/path/to/bmcptools

# Or disable one or more whole groups at registration time
codex mcp add bmcptools -- /absolute/path/to/bmcptools --disable=user,system

# Verify it is registered
codex mcp list
```

Equivalent `~/.codex/config.toml` entry:

```toml
[mcp_servers.bmcptools]
command = "/absolute/path/to/bmcptools"
args = ["--disable=user,system"]
```

On Windows:

```toml
[mcp_servers.bmcptools]
command = 'C:\\tools\\bmcptools.exe'
args = ["--disable=user"]
```

#### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "bmcptools": {
      "command": "/absolute/path/to/bmcptools"
    }
  }
}
```

#### Cursor / Copilot / other clients

Point the MCP `command` field at the same binary path and pass whichever `args` you want for `--disable` or `--exclude-tools`.

### Filter tools at launch

Use group-level filtering when you want broad removal, and tool-level filtering when you want to keep a group but drop a few exact tools. Flags win over env vars when both are set.

| Option | Env var | Accepts | Use for |
|--------|---------|---------|---------|
| `--disable=...` | `BMCPTOOLS_DISABLE` | Comma-separated group names | Remove whole groups such as `user` or `system` |
| `--exclude-tools=...` | `BMCPTOOLS_EXCLUDE_TOOLS` | Comma-separated tool names | Remove exact tools such as `ask_user` while keeping the rest of the group |
| `--list-groups` | — | none | Print valid group names |
| `--list-tools` | — | none | Print valid tool names, grouped by tool group |

```bash
# Drop the whole interactive user group
bmcptools --disable=user

# Drop only browser question dialogs, but keep notify/rest
bmcptools --exclude-tools=ask_user,update_dialog,cancel_ask_user

# Same via env vars (handy in MCP client configs)
BMCPTOOLS_DISABLE=user bmcptools
BMCPTOOLS_EXCLUDE_TOOLS=ask_user,update_dialog,cancel_ask_user bmcptools

# Print valid values
bmcptools --list-groups
bmcptools --list-tools
```

The server instructions sent to the LLM are auto-trimmed to match the active tool set, so excluded groups and excluded tools are removed from the prompt as well as from registration.

#### Valid group names for `--disable`

| Group | Tool count | Contains |
|-------|------------|----------|
| `user` | 6 | Interactive dialogs, notifications, rest/wake flow |
| `file` | 14 | Single-file operations and archive helpers |
| `multi` | 8 | Batch file operations |
| `dir` | 4 | Directory listing/tree/create/delete |
| `search` | 2 | Path search and content grep |
| `exec` | 4 | Shell execution, app open, environment access |
| `system` | 6 | HTTP, clipboard, process list, system info, downloads |

Tool names accepted by `--exclude-tools` are the exact MCP tool names shown in the [Tools](#tools) tables below.

For Claude Desktop / Cursor configs:

```json
{
  "mcpServers": {
    "bmcptools": {
      "command": "/absolute/path/to/bmcptools",
      "args": ["--exclude-tools=ask_user,update_dialog,cancel_ask_user"]
    }
  }
}
```

For Codex, the equivalent registration examples are:

```bash
# Disable whole groups
codex mcp add bmcptools -- /absolute/path/to/bmcptools --disable=user

# Keep the user group, but drop only selected tools
codex mcp add bmcptools -- /absolute/path/to/bmcptools --exclude-tools=ask_user,update_dialog,cancel_ask_user
```

---

## Tools

These exact tool names are the values accepted by `--exclude-tools`, `BMCPTOOLS_EXCLUDE_TOOLS`, and `WithExcludeTools(...)`.

### `user` tools (6)

`get_user_response` is shared by both `ask_user` and `rest`. If you exclude `ask_user` but keep `rest`, leave `get_user_response` enabled.

| Tool | Platform | Description |
|------|----------|-------------|
| `ask_user` | macOS, Windows | Browser dialog with choices, required markdown details, pasted/dropped images, live updates, typing detection. Returns a token for follow-up polling. |
| `get_user_response` | macOS, Windows | Long-poll for a token returned by another user-interaction tool. Returns full answer text by default, plus local paths for attached images, or detailed PENDING status. |
| `update_dialog` | macOS, Windows | Push live markdown updates into an open decision dialog. `replace_last` for streaming progress. |
| `cancel_ask_user` | macOS, Windows | Dismiss a pending question dialog by token. |
| `notify_user` | **All platforms** | Fire-and-forget toast notification. Returns concise delivery metadata. `level`: info/warning/error. |
| `rest` | macOS, Windows | AI goes idle with a browser "resting" page. Wake-up button. Returns a token for follow-up polling. |

### `file` tools (14)

| Tool | Description |
|------|-------------|
| `read_file` | Read a file with encoding auto-detection. Supports `start_line`/`end_line`, multi-range (`ranges`), `head`/`tail`, `show_line_numbers`, and byte limits. Binary files return summaries unless `include_base64=true`. |
| `write_file` | Create or overwrite a file. Auto-creates parent dirs. Returns a capped unified diff only when `show_diff=true`. |
| `append_to_file` | Append content to a file and create it if absent. |
| `edit_file` | Surgical find-and-replace. Batch mode, Go regex with backreferences, CRLF-transparent, capped diff, near-miss probe, dry-run. |
| `delete_file` | Delete a single file or symlink. |
| `copy_file` | Copy one file or one directory tree. Auto-creates destination parent dirs. |
| `move_file` | Move or rename one file or one directory tree. Cross-device fallback copies then deletes the source when needed. |
| `get_file_info` | Compact metadata: type, size, permissions, mod time, symlink target, optional line count. `output_mode=details` for expanded fields. |
| `path_exists` | Lightweight existence/type check for one path. |
| `diff_files` | Unified diff between two text files with large-input guards and capped diff output. |
| `calculate_checksum` | MD5, SHA1, or SHA256 checksum for one or more files. |
| `create_symlink` | Create a symbolic link. May require elevated privileges on Windows. |
| `compress_files` | Compress files/directories into `zip`, `tar`, or `tar.gz`. |
| `extract_archive` | Extract `zip`, `tar`, or `tar.gz` archives with path-traversal protection. |

### `multi` tools (8)

| Tool | Description |
|------|-------------|
| `read_multiple_files` | Read 2+ files in one call. More efficient than repeated `read_file` for related small/medium files. Compact path/size headers; defaults to 128 KB per file and a 512 KB total output cap; binary summaries by default. For large repos, prefer `grep_files` first and then narrow with `ranges`, `head`, or `tail`. |
| `write_multiple_files` | Write 2+ files in one call. `show_diff` defaults to false for performance. |
| `find_replace_in_files` | Find-and-replace across a directory tree. Regex, glob/exclude filters, dry-run, per-file diffs, compact unmodified-file summary, and default oversized-file guard. |
| `path_exists_batch` | Check whether multiple paths exist in one call. Returns type and size with a summary and limit control. |
| `get_multiple_file_info` | Return compact metadata for multiple paths with limit control. `count_lines=true` opts into line counts. `output_mode=details` for expanded fields. |
| `delete_files` | Delete multiple files or symlinks in one call. |
| `copy_paths` | Copy multiple files or directory trees in one call. |
| `move_paths` | Move multiple files or directory trees in one call. |

### `dir` tools (4)

| Tool | Description |
|------|-------------|
| `list_directory` | Contents with sizes, timestamps. Recursion, glob/exclude filters, sort by name/size, and `max_entries` cap. |
| `directory_tree` | Visual tree view with sizes. Best for project structure overview. Supports glob/exclude filters and `max_entries` cap. |
| `create_directory` | Create directory + parents (`mkdir -p`). Idempotent. |
| `delete_directory` | Delete directory. `force=true` for non-empty. |

### `search` tools (2)

| Tool | Description |
|------|-------------|
| `search_files` | Find files/dirs by **name** (glob patterns or Go regex with `use_regex`). Concise relative paths by default, with details/absolute modes and case-insensitive matching. |
| `grep_files` | Find files by **content** (literal or regex). Auto/content/files/count output modes, pagination, multiline, glob/exclude filters, relative paths by default. |

### `exec` tools (4)

| Tool | Description |
|------|-------------|
| `get_working_directory` | CWD, OS, hostname, and compact key env summary. **Call first** to orient. |
| `run_command` | Non-interactive shell execution with selectable shell (`default`, `sh`, `bash`, `cmd`, `powershell`, `pwsh`). Timeout max 600 s with process-tree termination, fractional seconds, detach for long-running services, raw output, stdin, env vars, heredoc/here-string friendly command bodies, and default 256 KB output capping. No PTY/TUI support. |
| `open_in_app` | Open file/dir/URL in default app. Cross-platform, non-blocking. |
| `get_env` | Read environment variables. Specific key, filter by name substring, or list names by default with values capped/redacted. |

### `system` tools (6)

| Tool | Description |
|------|-------------|
| `http_request` | HTTP client (all methods). JSON compacts by default, `json_format="pretty"` opt-in, response body defaults to a 256 KB cap, and `body_filter` can return only literal/regex matches, lines, or counts from large responses. Timeout max 300 s. |
| `download_file` | Download a file from a URL to a local path. Streaming (no memory buffering). Auto-creates parent dirs. |
| `clipboard_read` | Read system clipboard with a default 256 KB output cap. |
| `clipboard_write` | Write to system clipboard. |
| `list_processes` | Running processes with PID, name, CPU/memory. Filter, sort, tune limit and command width. |
| `get_system_info` | CPU, memory, and disk usage snapshot. |

## Development

**Requirements:** Go 1.23+

```bash
# Build binary into ./bin/
make build

# Build and stamp with a version tag
make build VERSION=v1.2.3

# Install into $GOBIN
make install VERSION=v1.2.3

# Run tests (root + all internal sub-packages)
make test
# or directly:
go test ./...

# Format Go packages in this repo without passing mixed file types to gofmt
make fmt

# Run linter (requires golangci-lint)
make lint
```

On Linux/macOS/Windows, `make build` auto-detects the version from `git describe`. Pass `VERSION=<tag>` explicitly to override it, or build without git tags and it will default to `"dev"`.

### Repository structure

The codebase follows a modular layout. Public APIs live under `pkg/` (importable by external repos); internal implementation lives under `internal/`.

```
bmcptools/                     ← public API (server.go, registrar.go, toolnames.go, inventory.go)
├── safeskill.manifest.json    ← transparency/permission manifest
├── package.json + index.d.ts  ← npm metadata + typed tool/group inventory
├── cmd/bmcptools/             ← package main (entry point)
├── scripts/preview/           ← browser UI preview helper
├── pkg/
│   ├── browser/               ← shared HTTP server + browser-open infrastructure
│   ├── confirm/               ← standalone blocking confirm/cancel dialog
│   ├── connector/             ← Connector interface for external tool groups
│   ├── dialog/                ← HTML template contract types (DialogTemplate, RestTemplate)
│   ├── toolname/              ← public tool name constants (importable by connectors)
│   └── toolreg/               ← public ToolRegistrar interface
└── internal/
    ├── asset/                 ← embedded JSON descriptions + HTML/CSS/JS templates
    │   └── descriptions/      ← tool descriptions (JSON) + server instructions (TXT)
    ├── helper/                ← shared utilities (fs, read, diff, edit, mime, glob, checksum)
    └── tool/
        ├── dir/               ← directory tools
        ├── exec/              ← exec / process tools
        ├── file/              ← file read/write/edit tools
        ├── multi/             ← multi-file tools
        ├── search/            ← search & grep tools
        ├── system/            ← system info, HTTP, clipboard, processes
        └── user/              ← interactive UI tools (ask, notify, rest)
```

### Embedding bmcptools in another MCP server

#### Register all tools at once

```go
import bmcptools "github.com/bearaujus/bmcptools"

s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructions()),
)

bmcptools.Register(s) // all tool groups, default HTML
bmcptools.Register(s, bmcptools.WithUserOptions(
    bmcptools.UserWithDialogTemplate(myDialogTmpl), // custom ask_user HTML
))
```

#### Exclude specific tools

```go
import (
    bmcptools "github.com/bearaujus/bmcptools"
    "github.com/bearaujus/bmcptools/pkg/toolname"
)

// Keep registration and AI instructions aligned.
s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructionsExcludingTools(
        toolname.AskUser,
        toolname.UpdateDialog,
        toolname.CancelAskUser,
    )),
)

bmcptools.Register(s, bmcptools.WithExcludeTools(
    toolname.AskUser,
    toolname.UpdateDialog,
    toolname.CancelAskUser,
))
```

#### Disable whole tool groups

```go
// Skip interactive user-dialog tools entirely.
// ServerInstructionsExcludingGroups trims the AI prompt to match.
s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructionsExcludingGroups(
        bmcptools.GroupUser,
    )),
)

bmcptools.Register(s, bmcptools.WithDisableGroups(
    bmcptools.GroupUser,
))
```

#### Combine group and tool exclusions

```go
s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructionsWithExclusions(
        []string{bmcptools.GroupSystem},
        []string{bmcptools.ToolAskUser},
    )),
)

bmcptools.Register(s,
    bmcptools.WithDisableGroups(bmcptools.GroupSystem),
    bmcptools.WithExcludeTools(bmcptools.ToolAskUser),
)
```

#### Register individual tool groups

```go
// Register only the groups you need, with scoped instructions
s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructionsForGroups("file", "search")),
)

bmcptools.RegisterFile(s)
bmcptools.RegisterSearch(s)
```

#### Register external connectors (Lark, Slack, etc.)

```go
import (
    bmcptools "github.com/bearaujus/bmcptools"
    "github.com/bearaujus/bmcptools/pkg/connector"
)

// Your connector implements connector.Connector:
//   Name() string
//   Register(s toolreg.ToolRegistrar)
bmcptools.RegisterConnectors(s, larkConnector, slackConnector)
```

#### Override dialog HTML

```go
import "github.com/bearaujus/bmcptools/pkg/dialog"

tmpl, err := dialog.NewDialogTemplate(myHTML) // validated at construction
if err != nil { ... }

bmcptools.RegisterUser(s, bmcptools.UserWithDialogTemplate(tmpl))
```

For wrappers or config UIs, use `AllGroups()`, `AllTools()`, `ToolsForGroup()`, and `ValidateToolNames()` instead of hardcoding accepted values.

Each `internal/tool/<name>/` package exports a single `Register(s toolreg.ToolRegistrar)` function. The root `Register(s ToolRegistrar)` delegates to all of them.

### Previewing browser UI templates (macOS/Windows)

The browser-based tools (`ask_user`, `rest`) and the destructive-operation `confirm` dialog use embedded HTML templates under `internal/asset/html/`. To preview them locally without running the full MCP server:

```sh
# Preview all pages (opens in your default browser)
go run ./scripts/preview

# Preview a specific page
go run ./scripts/preview dialog
go run ./scripts/preview rest
go run ./scripts/preview confirm
```

### CI/CD

Pushing a tag (`v*`) triggers the release workflow, which cross-compiles for all platforms and publishes a GitHub release:

| Artifact | Platform |
|----------|----------|
| `bmcptools-mac-amd64` | macOS Intel |
| `bmcptools-mac-arm64` | macOS Apple Silicon |
| `bmcptools-linux-amd64` | Linux x86-64 |
| `bmcptools-linux-arm64` | Linux ARM64 |
| `bmcptools.exe` | Windows x86-64 |
| `bmcptools-win-arm64.exe` | Windows ARM64 |
| `checksums.txt` | SHA256 checksums for all binaries |

---

## Troubleshooting

### "Unknown publisher" / "cannot be opened" warning

The release binaries are not code-signed, so your OS may show a security warning on first run. This is normal for open-source CLI tools distributed as standalone binaries.

**Verify integrity first** — each release includes a `checksums.txt` file with SHA256 hashes:

```bash
# macOS / Linux
sha256sum -c checksums.txt

# Windows (PowerShell)
Get-FileHash bmcptools.exe -Algorithm SHA256
# Compare the output with the hash in checksums.txt
```

**Then unblock the binary:**

| OS | How to unblock |
|----|----------------|
| **macOS** | Run `xattr -d com.apple.quarantine bmcptools-mac-*` in Terminal, or right-click the binary → Open → Open |
| **Windows** | Right-click the `.exe` → Properties → check **Unblock** → OK. Or in PowerShell: `Unblock-File bmcptools.exe` |
| **Linux** | `chmod +x bmcptools-linux-*` (no signing issues, just needs execute permission) |

---

## Security & transparency

bmcptools is scanned by [SafeSkill](https://safeskill.dev/scan?pkg=bearaujus%2Fbmcptools), which performs AST-based taint tracking and prompt-injection analysis on the source.

For machine-readable transparency, the repo root ships:

- **[`safeskill.manifest.json`](safeskill.manifest.json)** — declares the server's permissions (filesystem, shell, network, environment, clipboard, process), the full tool inventory, and confirms there is **no telemetry, no analytics, no hardcoded outbound endpoints, and no install scripts**.
- **`package.json` / `index.d.ts` / `index.js`** — npm-ecosystem metadata (repository link, typed tool/group inventory). The package ships static metadata only; the server itself is the standalone Go binary.

By design, bmcptools never collects or transmits user data on its own. Every network call is caller-initiated and SSRF-guarded (private/loopback/link-local targets are blocked unless explicitly allowed), `get_env` lists names only and redacts secret-like values by default, and destructive operations are gated and annotated.

---

## Package stability

All packages under `pkg/` are considered **stable public API**:

| Package | Stability |
|---------|----------|
| `pkg/browser` | Stable — `Serve`, `Open`, `ServeAndOpen`, `OpenFn` |
| `pkg/confirm` | Stable — `Ask`, `AskWithHTML`, `ShowHTML`, `WithHTML`, `WithTimeout`, `WithEditableParams`, `EditableParam` |
| `pkg/connector` | Stable — `Connector` interface |
| `pkg/dialog` | Stable — `DialogTemplate`, `RestTemplate` and their constructors |
| `pkg/toolname` | Stable — tool name constants |
| `pkg/toolreg` | Stable — `ToolRegistrar` interface |

Root-level exports in `server.go`, `registrar.go`, `toolnames.go`, and `inventory.go` are also stable.
Packages under `internal/` are **not** part of the public API and may change without notice.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on adding new tools, project conventions, and known MCP SDK limitations.
