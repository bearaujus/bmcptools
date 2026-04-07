# Contributing to bmcptools

## Adding a new tool

Every tool follows the same pattern:

```
registerXTools(s) → s.AddTool(mcp.NewTool(...), handlerFunc)
→ func handlerFunc(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
```

1. **Choose the right file** — pick the `handler.go` (for handler logic) and `register.go` (for tool registration) in the package that best matches the domain
   (`file`, `dir`, `exec`, `search`, `system`, `user`, `multi`). Create a new package directory if needed.
2. **Add the description** — add your tool entry to the corresponding
   `assets/descriptions/*.json` file. Use `td("tool_name")` for the tool description
   and `pd("tool_name", "param_name")` for parameter descriptions. Never hardcode
   descriptions inline in the registration call.
3. **Write the handler** — use `req.GetString` / `req.GetBool` / `req.GetFloat` for
   parameters; never reach into `req.GetArguments()` for scalar types.
4. **Return results** — use `mcp.NewToolResultText(s)` for success and
   `mcp.NewToolResultError(s)` for failure. Return `(nil, err)` only for unexpected
   infrastructure failures; prefer returning an error result text for user-facing errors.
5. **Register** — call `s.AddTool(...)` inside the matching `registerXTools` function.
6. **Test** — add test cases to the matching `tools_*_test.go` file.

## Project conventions

- All tool handler signatures must accept `context.Context` as the first argument
  (even if the current implementation doesn't use it — propagation matters for future
  cancellation support).
- Parameter descriptions live exclusively in `assets/descriptions/*.json` — not inline.
- Use `atomicWriteFile` for all file writes to prevent partial-write corruption.
- Use `lockFile` / defer unlock whenever the same file path could be written concurrently.

## MCP SDK — known limitations & workarounds

The following were observed while building this project and may be useful context
for contributors or upstream maintainers:

1. **No typed getter for object/map params.**
   Getting a `map` parameter requires reaching into `req.GetArguments()` directly
   and doing a manual type-assertion:
   ```go
   if hmap, ok := rawHeaders.(map[string]any); ok { ... }
   ```
   A helper like `req.GetMap("headers") → map[string]any` would be cleaner.

2. **No streaming / incremental results.**
   All output is buffered and returned at once. For long-running commands there is
   no way to stream partial results to the caller. A streaming variant would improve
   perceived latency on slow operations.

3. **No structured result type.**
   Only text results are available. A `mcp.NewToolResultJSON(v any)` helper would
   let callers reason over structured data without parsing text.

4. **`mcp.WithObject` schema is not validated.**
   Object parameters accept any shape — there is no way to declare that values must
   be strings or follow a specific schema. JSON Schema support would catch caller
   mistakes early.

## Running tests

```sh
make test
```

## Linting

```sh
make lint
```

## Building

```sh
make build VERSION=v1.2.3
```
