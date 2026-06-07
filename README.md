# bmcptools

> An MCP server that exposes **44 developer tools** to any MCP-compatible LLM client — file I/O, shell execution, search, system info, and interactive user dialogs.

[![Go](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/bearaujus/bmcptools)](https://github.com/bearaujus/bmcptools/releases)
[![SafeSkill 100/100](https://img.shields.io/badge/SafeSkill-100%2F100_Verified%20Safe-brightgreen)](https://safeskill.dev/scan?pkg=bearaujus%2Fbmcptools)

Communication happens over **stdio** using the [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) library.

### Why bmcptools?

- **44 tools, one binary** — covers file ops, search, shell, system, HTTP, clipboard, and user interaction
  Disable any tool group you don't need with `--disable=user,system,...` (see [Disabling tool groups](#disabling-tool-groups)).
- **Built-in server instructions** — the AI receives a categorized guide on when and how to use each tool
- **Interactive dialogs** — `ask_user` opens a browser dialog with choices, markdown, pasted/dropped images, live updates, and typing indicators
- **Cross-platform** — macOS, Windows, and Linux (user dialogs require macOS/Windows; `notify_user` works everywhere)
- **Surgical editing** — `edit_file` with near-miss probe, CRLF transparency, batch mode, and regex support
- **Performance-first** — batch tools (`read_multiple_files`, `write_multiple_files`, `path_exists_batch`, `get_multiple_file_info`, `edit_file` batch mode) reduce round-trips

---

## Quick Start

### Download

Grab a pre-built binary from the [Releases](https://github.com/bearaujus/bmcptools/releases) page.

### Integrate with your MCP client

**Claude Desktop** — add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "bmcptools": {
      "command": "/absolute/path/to/bmcptools"
    }
  }
}
```

**Cursor / Copilot / other clients** — point the `command` field at the same binary path.

### Disabling tool groups

Don't need every group? Pass `--disable=GROUPS` (CSV) when launching the binary, or set the `BMCPTOOLS_DISABLE` env var. The flag wins when both are set.

```bash
# Drop the interactive user prompts
bmcptools --disable=user

# Same via env (handy in MCP client configs)
BMCPTOOLS_DISABLE=user bmcptools

# List valid group names
bmcptools --list-groups
```

Available groups: `user, file, multi, dir, search, exec, system`. The server instructions sent to the LLM are auto-trimmed to match — disabled sections are removed so the model isn't told about tools it can't see.

For Claude Desktop / Cursor configs:

```json
{
  "mcpServers": {
    "bmcptools": {
      "command": "/absolute/path/to/bmcptools",
      "args": ["--disable=user"]
    }
  }
}
```

---

## Tools

### File tools (14)

| Tool | Description |
|------|-------------|
| `read_file` | Read a file with encoding auto-detection. Defaults to a 256 KB context cap. Supports `start_line`/`end_line`, multi-range (`ranges` param), `head`/`tail`, `show_line_numbers`, and byte limits. Binary files return summaries unless `include_base64=true`. |
| `write_file` | Create or overwrite a file. Auto-creates parent dirs. Returns a capped unified diff only when `show_diff=true`. |
| `append_to_file` | Append content to a file (creates if absent). Returns new file size. |
| `edit_file` | Surgical find-and-replace. Batch mode, Go regex with backreferences, CRLF-transparent, capped diff, near-miss probe, dry-run. **Preferred for editing.** |
| `delete_file` | Delete a single file. |
| `copy_file` | Copy a file or directory tree. Auto-creates destination parent dirs. |
| `move_file` | Move or rename a file/directory. Cross-device fallback copies then deletes the source when needed. |
| `get_file_info` | Compact metadata: type, size, permissions, mod time, symlink target, optional line count. `output_mode=details` for expanded fields. |
| `path_exists` | Lightweight existence check — faster than `read_file` or `get_file_info`. |
| `diff_files` | Unified diff between two files. Guards large inputs and caps diff output by default. Cross-platform. |
| `calculate_checksum` | MD5, SHA1, or SHA256 checksum. Batch-capable. Cross-platform. |
| `create_symlink` | Create a symbolic link. Cross-platform (may require elevated privileges on Windows). |
| `compress_files` | Compress files/directories into a zip or tar.gz archive. Auto-detects format from extension. |
| `extract_archive` | Extract a zip or tar.gz archive. Auto-detects format. Path-traversal protected. |

### Multi-file tools (8)

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

### Directory tools (4)

| Tool | Description |
|------|-------------|
| `list_directory` | Contents with sizes, timestamps. Recursion, glob/exclude filters, sort by name/size, and `max_entries` cap. |
| `directory_tree` | Visual tree view with sizes. Best for project structure overview. Supports glob/exclude filters and `max_entries` cap. |
| `create_directory` | Create directory + parents (`mkdir -p`). Idempotent. |
| `delete_directory` | Delete directory. `force=true` for non-empty. |

### Search tools (2)

| Tool | Description |
|------|-------------|
| `search_files` | Find files/dirs by **name** (glob patterns or Go regex with `use_regex`). Concise relative paths by default, with details/absolute modes and case-insensitive matching. |
| `grep_files` | Find files by **content** (literal or regex). Auto/content/files/count output modes, pagination, multiline, glob/exclude filters, relative paths by default. |

### User interaction tools (6)

| Tool | Platform | Description |
|------|----------|-------------|
| `ask_user` | macOS, Windows | Browser dialog with choices, required markdown details, pasted/dropped images, live updates, typing detection. Returns token → poll with `get_user_response`. |
| `get_user_response` | macOS, Windows | Long-poll for `ask_user`/`rest` response. Returns full answer text by default, plus local paths for attached images, or detailed PENDING status (typing, idle, heartbeat). |
| `update_dialog` | macOS, Windows | Push live markdown updates into an open `ask_user` dialog. `replace_last` for streaming progress. |
| `cancel_ask_user` | macOS, Windows | Dismiss a pending dialog by token. |
| `notify_user` | **All platforms** | Fire-and-forget toast notification. Returns concise delivery metadata. `level`: info/warning/error. |
| `rest` | macOS, Windows | AI goes idle with a browser "resting" page. Wake-up button. Returns token → poll with `get_user_response`. |

### System tools (10)

| Tool | Description |
|------|-------------|
| `get_working_directory` | CWD, OS, hostname, and compact key env summary. **Call first** to orient. |
| `run_command` | Non-interactive shell execution with selectable shell (`default`, `sh`, `bash`, `cmd`, `powershell`, `pwsh`). Timeout max 600 s with process-tree termination, fractional seconds, detach for long-running services, raw output, stdin, env vars, heredoc/here-string friendly command bodies, and default 256 KB output capping. No PTY/TUI support. |
| `open_in_app` | Open file/dir/URL in default app. Cross-platform, non-blocking. |
| `http_request` | HTTP client (all methods). JSON compacts by default, `json_format="pretty"` opt-in, response body defaults to a 256 KB cap, and `body_filter` can return only literal/regex matches, lines, or counts from large responses. Timeout max 300 s. |
| `list_processes` | Running processes with PID, name, CPU/memory. Filter, sort, tune limit and command width. |
| `get_system_info` | CPU, memory, and disk usage snapshot. |
| `get_env` | Read environment variables. Specific key, filter by name substring, or list names by default with values capped/redacted. |
| `clipboard_read` | Read system clipboard with a default 256 KB output cap. |
| `clipboard_write` | Write to system clipboard. |
| `download_file` | Download a file from a URL to a local path. Streaming (no memory buffering). Auto-creates parent dirs. |

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
bmcptools/                     ← public API (server.go, registrar.go, toolnames.go)
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

bmcptools.Register(s)                         // all tool groups, default HTML
bmcptools.Register(s, bmcptools.WithUserOptions(
    bmcptools.UserWithDialogTemplate(myDialogTmpl),  // custom ask_user HTML
))
```

#### Exclude specific tools

```go
import "github.com/bearaujus/bmcptools/pkg/toolname"

bmcptools.Register(s, bmcptools.WithExcludeTools(
    toolname.CompressFiles,
    toolname.ExtractArchive,
    toolname.CreateSymlink,
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

Root-level exports in `server.go`, `registrar.go`, and `toolnames.go` are also stable.
Packages under `internal/` are **not** part of the public API and may change without notice.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on adding new tools, project conventions, and known MCP SDK limitations.
