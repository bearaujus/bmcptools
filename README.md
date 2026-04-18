# bmcptools

> An MCP server that exposes **69 developer tools** to any MCP-compatible LLM client — file I/O, shell execution, search, system info, and interactive user dialogs.

[![Go](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/bearaujus/bmcptools)](https://github.com/bearaujus/bmcptools/releases)

Communication happens over **stdio** using the [`mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) library.

### Why bmcptools?

- **69 tools, one binary** — covers file ops, search, shell, system, HTTP, clipboard, user interaction, and Binance USDT-M Futures trading
  Disable any tool group you don't need with `--disable=binance,user,...` (see [Disabling tool groups](#disabling-tool-groups)).
- **Binance USDT-M Futures** — market data, account/position queries, leverage/margin config, and gated trading (single + bracket orders) with browser-confirm dialogs and AI-supplied `reasoning`
- **Built-in server instructions** — the AI receives a categorized guide on when and how to use each tool
- **Interactive dialogs** — `ask_user` opens a browser dialog with choices, markdown, live updates, and typing indicators
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
# Drop Binance and the interactive user prompts
bmcptools --disable=binance,user

# Same via env (handy in MCP client configs)
BMCPTOOLS_DISABLE=binance bmcptools

# List valid group names
bmcptools --list-groups
```

Available groups: `user, file, multi, dir, search, exec, system, binance`. The server instructions sent to the LLM are auto-trimmed to match — disabled sections are removed so the model isn't told about tools it can't see.

For Claude Desktop / Cursor configs:

```json
{
  "mcpServers": {
    "bmcptools": {
      "command": "/absolute/path/to/bmcptools",
      "args": ["--disable=binance"]
    }
  }
}
```

---

## Tools

### File tools (14)

| Tool | Description |
|------|-------------|
| `read_file` | Read a file with encoding auto-detection. Supports `start_line`/`end_line`, multi-range (`ranges` param), `head`/`tail`, `show_line_numbers`, and byte limits. Binary → base64. |
| `write_file` | Create or overwrite a file. Auto-creates parent dirs. Returns a unified diff when overwriting (configurable via `show_diff`). |
| `append_to_file` | Append content to a file (creates if absent). Returns new file size. |
| `edit_file` | Surgical find-and-replace. Batch mode, Go regex, CRLF-transparent, near-miss probe, dry-run. **Preferred for editing.** |
| `delete_file` | Delete a single file. |
| `copy_file` | Copy a file. Auto-creates destination parent dirs. |
| `move_file` | Move or rename a file/directory. Cross-device safe. |
| `get_file_info` | Metadata: type, size, permissions, mod time, symlink target, line count. |
| `path_exists` | Lightweight existence check — faster than `read_file` or `get_file_info`. |
| `diff_files` | Unified diff between two files. Cross-platform. |
| `calculate_checksum` | MD5, SHA1, or SHA256 checksum. Batch-capable. Cross-platform. |
| `create_symlink` | Create a symbolic link. Cross-platform (may require elevated privileges on Windows). |
| `compress_files` | Compress files/directories into a zip or tar.gz archive. Auto-detects format from extension. |
| `extract_archive` | Extract a zip or tar.gz archive. Auto-detects format. Path-traversal protected. |

### Multi-file tools (5)

| Tool | Description |
|------|-------------|
| `read_multiple_files` | Read 2+ files in one call. More efficient than repeated `read_file`. |
| `write_multiple_files` | Write 2+ files in one call. `show_diff` defaults to false for performance. |
| `find_replace_in_files` | Find-and-replace across a directory tree. Regex, glob filter, dry-run, per-file diffs. |
| `path_exists_batch` | Check whether multiple paths exist in one call. Returns type and size for each. |
| `get_multiple_file_info` | Return metadata (type, size, modified, permissions, line count) for multiple paths. |

### Directory tools (4)

| Tool | Description |
|------|-------------|
| `list_directory` | Contents with sizes, timestamps. Recursion, glob filter, sort by name/size. |
| `directory_tree` | Visual tree view with sizes. Best for project structure overview. |
| `create_directory` | Create directory + parents (`mkdir -p`). Idempotent. |
| `delete_directory` | Delete directory. `force=true` for non-empty. |

### Search tools (2)

| Tool | Description |
|------|-------------|
| `search_files` | Find files by **name** (glob patterns: `*.go`, `**/*.json`, `{a,b}`). |
| `grep_files` | Find files by **content** (literal or regex). Auto/content/files/count output modes, pagination, multiline, glob filter. |

### User interaction tools (6)

| Tool | Platform | Description |
|------|----------|-------------|
| `ask_user` | macOS, Windows | Browser dialog with choices, markdown details, live updates, typing detection. Returns token → poll with `get_user_response`. |
| `get_user_response` | macOS, Windows | Long-poll for `ask_user`/`rest` response. Returns answer or detailed PENDING status (typing, idle, heartbeat). |
| `update_dialog` | macOS, Windows | Push live markdown updates into an open `ask_user` dialog. `replace_last` for streaming progress. |
| `cancel_ask_user` | macOS, Windows | Dismiss a pending dialog by token. |
| `notify_user` | **All platforms** | Fire-and-forget toast notification. `level`: info/warning/error. |
| `rest` | macOS, Windows | AI goes idle with a browser "resting" page. Wake-up button. Returns token → poll with `get_user_response`. |

### System tools (10)

| Tool | Description |
|------|-------------|
| `get_working_directory` | CWD, OS, hostname, and key env vars. **Call first** to orient. |
| `run_command` | Shell execution (`sh -c` / `cmd /C`). Timeout max 600 s. Supports detach, raw output, stdin, env vars, output capping. |
| `open_in_app` | Open file/dir/URL in default app. Cross-platform, non-blocking. |
| `http_request` | HTTP client (all methods). JSON auto-pretty-print. Response capped at 1 MB. Timeout max 300 s. |
| `list_processes` | Running processes with PID, name, CPU/memory. Filter and sort. |
| `get_system_info` | CPU, memory, and disk usage snapshot. |
| `get_env` | Read environment variables. Specific key, filter by name substring, or list all. |
| `clipboard_read` | Read system clipboard. |
| `clipboard_write` | Write to system clipboard. |
| `download_file` | Download a file from a URL to a local path. Streaming (no memory buffering). Auto-creates parent dirs. |

### Binance USDT-M Futures tools (28)

Lets an MCP-connected LLM analyze the market and execute leveraged trades on Binance USDT-M Futures (`fapi.binance.com`). Read-only market-data tools work without credentials; account/config/trading tools require API key + secret. Mutating tools open a browser confirm dialog (via `pkg/confirm`) and require the AI to pass a `reasoning` string. Set `BINANCE_SKIP_ASK_USER=true` for full autonomy.

**Environment variables**

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `BINANCE_API_KEY` | for auth tools | — | API key |
| `BINANCE_API_SECRET` | for auth tools | — | API secret (HMAC-SHA256 signing) |
| `BINANCE_FUTURES_BASE_URL` | no | `https://fapi.binance.com` | Override base URL (e.g. `https://testnet.binancefuture.com` for testnet) |
| `BINANCE_SKIP_ASK_USER` | no | `false` | Skip the confirm dialog on every mutating call |
| `BINANCE_RECV_WINDOW_MS` | no | `5000` | `recvWindow` for signed requests (max 60 000) |

**Market data — no auth (12)**

| Tool | Description |
|------|-------------|
| `binance_futures_ping` | Connectivity + server time skew check. |
| `binance_futures_exchange_info` | Full exchange filters (large; prefer `symbol_specs`). |
| `binance_futures_symbol_specs` | Compact per-symbol filters: `minNotional`, `minQty`, `stepSize`, `tickSize`, `maxLeverage`, allowed margin/order types. **Call before every order.** |
| `binance_futures_klines` | OHLCV candles (ascending order). Intervals `1m`..`1M`. |
| `binance_futures_ticker_price` | Latest price for one or all symbols. |
| `binance_futures_ticker_24hr` | 24h rolling stats (volume, change %). |
| `binance_futures_order_book` | Depth (bids/asks) for slippage estimation. |
| `binance_futures_mark_price` | Mark/index price + next funding rate + funding time. |
| `binance_futures_open_interest` | Current open interest; `history=true` for trend. |
| `binance_futures_long_short_ratio` | Top-trader / global sentiment. |
| `binance_futures_funding_rate_history` | Historical funding rate series for a symbol (bias / carry analysis). |
| `binance_futures_ta_snapshot` | One-shot technical-analysis snapshot: EMA(9/21/50), RSI(14), ATR(14), Bollinger(20,2), latest close — ready for the AI to reason over. |

**Account & positions — auth required (7)**

| Tool | Description |
|------|-------------|
| `binance_futures_account_info` | Wallet, available margin, total uPnL, position mode. |
| `binance_futures_position_risk` | Open positions: size, entry, mark, uPnL, leverage, liquidation. |
| `binance_futures_open_orders` | Currently working orders. |
| `binance_futures_order_history` | Historical orders (filled / canceled / expired). |
| `binance_futures_income_history` | Realized PnL, funding fees, commissions. |
| `binance_futures_commission_rate` | Per-symbol maker/taker commission rates (for accurate R:R math). |
| `binance_futures_position_overview` | Account + positions + commission + mark price stitched into one response (ideal for "where do I stand right now?"). |

**Account configuration — mutating, gated (3)**

| Tool | Description |
|------|-------------|
| `binance_futures_change_leverage` | Set leverage per symbol (1..maxLeverage). |
| `binance_futures_change_margin_type` | `ISOLATED` or `CROSSED` per symbol. |
| `binance_futures_change_position_mode` | Account-wide hedge vs one-way. |

**Trading — mutating, gated (6)**

| Tool | Description |
|------|-------------|
| `binance_futures_place_order` | Single order (`MARKET` / `LIMIT` / `STOP*` / `TAKE_PROFIT*` / `TRAILING_STOP_MARKET`). Idempotent via auto-generated `newClientOrderId`. Supports `dryRun=true` (uses `/order/test`). |
| `binance_futures_place_bracket_order` | **Preferred entry point** — entry + stop-loss + take-profit in one batch call. Validates SL/TP sides client-side. `onPartialFailure`: `rollback` (default) or `warn`. |
| `binance_futures_modify_order` | Amend price/quantity of a working LIMIT order (`PUT /fapi/v1/order`). |
| `binance_futures_close_position` | Flatten an open position by auto-detecting side + quantity from `position_risk`, then placing a reduceOnly MARKET (or LIMIT) order. |
| `binance_futures_cancel_order` | Cancel by `orderId` or `origClientOrderId`. |
| `binance_futures_cancel_all_open_orders` | Cancel all working orders for a symbol. |

**Safety model**

- Mutating tools build a markdown trade brief (mark price, free margin, current positions, est. notional, SL/TP %, R:R, AI `reasoning`) and require user **Approve** in a browser dialog.
- Auto time-sync against `/fapi/v1/time` (`-1021` triggers re-sync + retry).
- HTTP 503 *Unknown error* on order placement is **never blind-retried** — the tool reconciles via `origClientOrderId` before reporting success or asking the user to verify.
- Every response includes a `[rate-limit] used_weight_1m=… order_count_1m=…` footer so the AI can self-throttle.

**Testnet caveats**

- Base URL: `https://testnet.binancefuture.com`.
- `STOP_MARKET`, `TAKE_PROFIT_MARKET`, and `TRAILING_STOP_MARKET` are **not supported** on Futures testnet (returns `-4120`). As a result, `place_bracket_order` cannot fully execute on testnet — the entry leg places fine, but the SL / TP legs will fail and the tool will roll back. Use mainnet (with the smallest allowed notional) to validate brackets end-to-end, or simulate SL/TP with conditional `STOP` / `TAKE_PROFIT` *limit* orders on testnet.
- `/futures/data/*` (long/short ratio, open-interest history) returns HTTP 202 with empty body on testnet — this is a testnet limitation, not a tool bug.

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

On Linux/macOS/Windows, `make build` auto-detects the version from `git describe`. Pass `VERSION=<tag>` explicitly to override it, or build without git tags and it will default to `"dev"`.

### Repository structure

The codebase follows a modular layout. Public APIs live under `pkg/` (importable by external repos); internal implementation lives under `internal/`.

```
bmcptools/                     ← public API (server.go, registrar.go, toolnames.go)
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
        ├── user/              ← interactive UI tools (ask, notify, rest)
        └── binance/           ← Binance USDT-M Futures (market data + signed trading)
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
// Skip Binance + interactive user-dialog tools entirely.
// ServerInstructionsExcludingGroups trims the AI prompt to match.
s := server.NewMCPServer(bmcptools.ServerName, bmcptools.Version,
    server.WithInstructions(bmcptools.ServerInstructionsExcludingGroups(
        bmcptools.GroupBinance, bmcptools.GroupUser,
    )),
)

bmcptools.Register(s, bmcptools.WithDisableGroups(
    bmcptools.GroupBinance,
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

## Package stability

All packages under `pkg/` are considered **stable public API**:

| Package | Stability |
|---------|----------|
| `pkg/browser` | Stable — `Serve`, `Open`, `ServeAndOpen`, `OpenFn` |
| `pkg/confirm` | Stable — `Ask`, `AskWithHTML`, `ShowHTML`, `WithHTML`, `WithTimeout` |
| `pkg/connector` | Stable — `Connector` interface |
| `pkg/dialog` | Stable — `DialogTemplate`, `RestTemplate` and their constructors |
| `pkg/toolname` | Stable — tool name constants |
| `pkg/toolreg` | Stable — `ToolRegistrar` interface |

Root-level exports in `server.go`, `registrar.go`, and `toolnames.go` are also stable.
Packages under `internal/` are **not** part of the public API and may change without notice.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on adding new tools, project conventions, and known MCP SDK limitations.
