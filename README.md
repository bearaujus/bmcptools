# bmcptools

> An MCP server that exposes a rich set of developer tools to any MCP-compatible LLM client.

[![Go](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/bearaujus/bmcptools)](https://github.com/bearaujus/bmcptools/releases)

Communication happens over **stdio** using the [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) library.

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

---

## Tools

### File tools

| Tool | Description |
|------|-------------|
| `read_file` | Read a file's contents. Auto-detects encoding (UTF-8/UTF-16 ± BOM). Binary files are returned as base64. Supports `start_line`/`end_line` for large files. **`head: N`/`tail: N` shorthand.** **`show_line_numbers: true`** prefixes every line with its number. Full-file reads include a `[filename — N lines]` header. |
| `write_file` | Write (or overwrite) a file. Creates parent directories by default. **`show_diff: true`** returns a unified diff of what changed when overwriting. |
| `append_to_file` | Append content to a file, creating it if absent. |
| `edit_file` | Find-and-replace inside a file. **Batch mode:** pass an `edits` array to apply multiple changes in one call. Supports plain-text and Go regex; returns a unified diff. **`dry_run: true`** previews without modifying. **`context_lines`** controls diff context (default: 3, max: 50). |
| `delete_file` | Delete a single file. |
| `copy_file` | Copy a file to a new location. |
| `move_file` | Move or rename a file (cross-device aware). **`overwrite=true`** required to replace an existing destination. |
| `get_file_info` | Return metadata: type, size, permissions, modification time, symlink target, and **line count** for text files. |
| `path_exists` | Lightweight existence check — returns type and size, or `"false"` if the path does not exist. |
| `diff_files` | Compare two files and return a **unified diff**. Returns empty when files are identical. **`context_lines`** controls surrounding context (default: 3). |
| `calculate_checksum` | Calculate MD5, SHA1, or SHA256 (default) checksum of one or more files. Cross-platform. |

### Multi-file tools

| Tool | Description |
|------|-------------|
| `read_multiple_files` | Read multiple files in one call. Each header includes file size and line count. Binary files returned as base64. |
| `write_multiple_files` | Write multiple files in a single call — efficient for bulk refactors. |
| `find_replace_in_files` | Search and replace across a directory tree. Supports literal strings and Go regex, glob filtering, `dry_run`, per-file diff output, and binary-file skipping. Reports total files scanned. |

### Directory tools

| Tool | Description |
|------|-------------|
| `list_directory` | List directory contents with sizes and timestamps. Supports recursion, `sort_by`, and **`glob` filter** (with `{a,b}` alternation). |
| `create_directory` | Create a directory and all missing parents (`mkdir -p`). |
| `delete_directory` | Delete a directory. Requires `force=true` for non-empty directories. |
| `directory_tree` | Visual `tree`-style recursive view. Supports `max_depth`, `show_hidden`, `exclude_patterns`, and **`glob` filter**. |

### Search tools

| Tool | Description |
|------|-------------|
| `search_files` | Find files by glob pattern. Supports `*`, `**`, `?`, and `{a,b}` alternation. Results include size and modification time. |
| `grep_files` | Search file contents for a literal string or Go regex. Supports `output_mode`, `glob`, `multiline`, `context_lines`, `offset` for pagination, and `max_file_size`. Output header always reports files searched, binary files skipped, and total match count. |

### User interaction tools

| Tool | Description |
|------|-------------|
| `notify_user` | **Non-blocking** fire-and-forget notification. Supports Windows, macOS, and Linux (falls back to stderr). **`level`** (`info`/`warning`/`error`) and **`duration_seconds`** control appearance. |
| `ask_user` | Pop up a browser-based dialog (macOS and Windows) to ask the user a question and capture their reply. Supports `details` (markdown context shown below the question — e.g. change summary, findings), `choices` chips, `subtitle`, `allow_freeform`, `timeout_seconds`, and `non_blocking`. Not supported on Linux. |
| `get_user_response` | Poll for a pending `ask_user` response after `non_blocking=true`. Waits up to `wait_seconds` (default: 55 s) per call. Returns the answer or a `PENDING` status. |
| `update_dialog` | Push a live message into an open `ask_user` dialog (non-blocking mode, macOS and Windows). |
| `open_chat` | Open a persistent two-way chat window in the browser (macOS and Windows). Returns a `chat_id`. Not supported on Linux. |
| `send_chat_message` | Send a message from the AI into an open chat window. |
| `get_chat_messages` | Poll for user messages from an open chat window. Returns `PENDING` or `CLOSED`. |
| `close_chat` | Close an open chat window and free the port. |
| `rest` | Let the AI go AFK with a browser "resting" page and a wake-up button. macOS and Windows only. |

### System tools

| Tool | Description |
|------|-------------|
| `get_working_directory` | Return CWD, OS, hostname, and key environment variables. Use as your first call to orient in the filesystem. |
| `run_command` | Execute any shell command. Returns stdout+stderr, exit code, elapsed time, and working directory. Supports `timeout_seconds`, `cwd`, `env`, `stdin`, `max_output_bytes`, and `raw_output`. |
| `open_in_app` | Open a file, directory, or URL in the default system app. Cross-platform, non-blocking. |
| `get_system_info` | Return CPU, memory, and disk usage snapshot. |
| `list_processes` | List running processes with PID, name, CPU%, memory%, and command. Optional `filter` and `sort_by`. |
| `http_request` | Make an HTTP request. Returns status code, response body, and timing. Supports all methods, headers, auth, redirects, and timeout. |
| `clipboard_read` | Read text from the system clipboard (macOS/Linux/Windows). |
| `clipboard_write` | Write text to the system clipboard (macOS/Linux/Windows). |

---

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

# Run linter (requires golangci-lint)
make lint
```

On Linux/macOS, `make build` auto-detects the version from `git describe`. On Windows, pass `VERSION=<tag>` explicitly or let it default to `"dev"` for local builds.

### Repository structure

The codebase follows a modular `internal/` layout — no business logic lives in the root package:

```
bmcptools/                     ← public API (server.go, registrar.go, toolnames.go)
├── cmd/bmcptools/             ← package main (entry point)
├── scripts/preview/           ← browser UI preview helper
└── internal/
    ├── asset/                 ← embedded JSON descriptions + HTML/CSS/JS templates
    ├── helper/                ← shared utilities (fs, read, diff, edit, mime, glob, checksum)
    ├── toolname/              ← all tool name string constants
    ├── toolreg/               ← ToolRegistrar interface
    └── tool/
        ├── dir/               ← directory tools
        ├── exec/              ← exec / process tools
        ├── file/              ← file read/write/edit tools
        ├── multi/             ← multi-file tools
        ├── search/            ← search & grep tools
        ├── system/            ← system info, HTTP, clipboard, processes
        └── user/              ← interactive UI tools (ask, chat, notify, rest)
```

Each `internal/tool/<name>/` package exports a single `Register(s toolreg.ToolRegistrar)` function. The root `Register(s ToolRegistrar)` delegates to all of them.

### Previewing browser UI templates (macOS/Windows)

The browser-based tools (`ask_user`, `open_chat`, `rest`) use embedded HTML templates under `assets/`. To preview them locally without running the full MCP server:

```sh
# Preview all pages (opens in your default browser)
go run ./scripts/preview

# Preview a specific page
go run ./scripts/preview dialog
go run ./scripts/preview chat
go run ./scripts/preview rest
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

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on adding new tools, project conventions, and known MCP SDK limitations.
