# Contributing to bmcptools

## Adding a new tool

Every tool follows the same pattern:

```go
// register.go
s.AddTool(mcp.NewTool(toolname.MyTool,
    mcp.WithDescription(asset.ToolDesc(toolname.MyTool)),
    mcp.WithString("param", mcp.Required(), mcp.Description(asset.ParamDesc(toolname.MyTool, "param"))),
), myToolHandler)

// handler.go
func myToolHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { ... }
```

### Step-by-step checklist

1. **Add a tool name constant** — add a constant to `pkg/toolname/toolname.go` and re-export it in `toolnames.go`.

2. **Add the description** — add your tool entry to the corresponding `internal/asset/descriptions/*.json` file. Use `asset.ToolDesc(toolname.X)` for the tool description and `asset.ParamDesc(toolname.X, "param")` for parameter descriptions. Never hardcode descriptions inline in the registration call.

3. **Write the handler** — add the handler function to `handler.go` in the matching package (`file`, `dir`, `exec`, `search`, `system`, `user`, `multi`). Use `req.GetString` / `req.GetBool` / `req.GetFloat` / `req.GetStringSlice` for parameters; never reach into `req.GetArguments()` for scalar types.

4. **Return results** — use `mcp.NewToolResultText(s)` for success and `mcp.NewToolResultError(s)` for user-facing errors. Return `(nil, err)` only for unexpected infrastructure failures.

5. **Register** — call `s.AddTool(...)` inside the `Register` function in `register.go` of the matching package.

6. **Test** — add test cases to the matching `handler_test.go` file.

7. **Update documentation** — update these files:
   - `internal/asset/descriptions/server_instructions.txt` — add your tool to the appropriate section
   - `README.md` — add to the tool table + update the tool count

## Architecture overview

### Embedded asset system

Tool descriptions are embedded at compile time via `go:embed` in `internal/asset/asset.go`. The flow:

```
internal/asset/descriptions/*.json  →  go:embed  →  asset.ToolDesc() / asset.ParamDesc()
```

This means descriptions are part of the binary — no external files needed at runtime. When you add a description entry to a JSON file, it's automatically available after rebuilding.

### Package structure

| Package | Contents |
|---------|----------|
| `pkg/toolname/` | Canonical tool name constants (importable by external connectors) |
| `toolnames.go` | Root-level re-export of all tool name constants |
| `internal/asset/` | Embedded JSON descriptions + HTML/CSS/JS templates |
| `internal/helper/` | Shared utilities (fs, read, diff, edit, mime, glob, checksum) |
| `internal/tool/<name>/` | Each tool group: `register.go`, `handler.go`, `testutil_test.go`, `handler_test.go` |

## Project conventions

- All tool handler signatures must accept `context.Context` as the first argument (even if unused — propagation matters for future cancellation support).
- Parameter descriptions live exclusively in `internal/asset/descriptions/*.json` — not inline.
- Use `helper.AtomicWriteFile` for all file writes to prevent partial-write corruption.
- Use `helper.LockFile` / defer unlock whenever the same file path could be written concurrently.
- Use `helper.HumanizeBytes` for human-readable file sizes in output.

## MCP SDK — known limitations & workarounds

The following were observed while building this project and may be useful context for contributors or upstream maintainers:

1. **No typed getter for object/map params.**
   Getting a `map` parameter requires reaching into `req.GetArguments()` directly and doing a manual type-assertion:
   ```go
   if hmap, ok := rawHeaders.(map[string]any); ok { ... }
   ```
   A helper like `req.GetMap("headers") → map[string]any` would be cleaner.

2. **No streaming / incremental results.**
   All output is buffered and returned at once. For long-running commands there is no way to stream partial results to the caller. A streaming variant would improve perceived latency on slow operations.

3. **No structured result type.**
   Only text results are available. A `mcp.NewToolResultJSON(v any)` helper would let callers reason over structured data without parsing text.

4. **`mcp.WithObject` schema is not validated.**
   Object parameters accept any shape — there is no way to declare that values must be strings or follow a specific schema. JSON Schema support would catch caller mistakes early.

## Running tests

```sh
make test
```

### Test helpers (per-package)

Every `internal/tool/<name>/` package has a `testutil_test.go` file that provides shared test utilities. When writing new tests, use these helpers — do not duplicate them:

| Helper | Signature | Purpose |
|--------|-----------|---------|
| `newTestRequest` | `(args map[string]any) mcp.CallToolRequest` | Build a `mcp.CallToolRequest` with the given argument map — the standard way to invoke a handler in tests. |
| `isResultError` | `(r *mcp.CallToolResult) bool` | Return `r.IsError` — used to assert that a handler returned an error result. |
| `resultText` | `(r *mcp.CallToolResult) string` | Extract the first `TextContent` string from a result — used to inspect handler output. |

**Finding them:** `grep -r "func newTestRequest" internal/` — each package has its own copy in `testutil_test.go`.

**Adding tests:** add cases to the existing `handler_test.go` in the matching package. Check for name collisions first (`grep "^func Test" internal/tool/<pkg>/handler_test.go`). Always add a comment above each test explaining **why** the case is needed.

### ask_user / get_user_response polling in tests

`askUserHandler` is asynchronous — it spawns a goroutine and immediately returns a JSON token. To test the response-receiving side without browser interaction, pre-load state with `storePendingDialog(token, state)` where `state.responseCh` is a buffered channel pre-seeded with the answer. See `internal/tool/user/handler_test.go` for full examples.

## Linting

```sh
make lint
```

## Building

```sh
make build VERSION=v1.2.3
```
