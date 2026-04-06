# bmcptools — LLM Tools

bmcptools is mcp server that exposes a rich set of developer tools to any MCP-compatible LLM client (e.g. Claude Desktop, Cursor, Copilot).

Communication happens over **stdio** using the `mark3labs/mcp-go` library.

---

## Tools

### File tools (`tools_file.go`)

| Tool | Description |
|------|-------------|
| `read_file` | Read a file's contents. Auto-detects encoding (UTF-8/UTF-16 ± BOM). Binary files are returned as base64. Supports `start_line`/`end_line` for large files. **`head: N`/`tail: N` shorthand.** **`show_line_numbers: true`** prefixes every line with its number in **`%6d|line` padded format** (matches the system inline-line-numbers convention). Full-file reads include a **`[filename — N lines]` header**. Truncation message includes **total line count** for navigation. |
| `write_file` | Write (or overwrite) a file. Creates parent directories by default. **`show_diff: true`** returns a unified diff of what changed when overwriting an existing file. |
| `append_to_file` | Append content to a file, creating it if absent. |
| `edit_file` | Find-and-replace inside a file. **Batch mode:** pass an `edits` array to apply multiple changes in one call — far more efficient than repeated single-edit calls. Supports plain-text and Go regex; returns a unified diff. **Reports unmatched patterns** in batch mode. **`dry_run: true`** previews the diff without modifying the file. **`context_lines`** controls how many unchanged lines surround each changed region in the diff (default: 3, max: 50). |
| `delete_file` | Delete a single file. |
| `copy_file` | Copy a file to a new location. |
| `move_file` | Move or rename a file (cross-device aware). **`overwrite=true`** explicitly allows replacing an existing destination — safe by default (errors if dst exists). |
| `get_file_info` | Return metadata: type, size, permissions, modification time, symlink target. **Includes line count for text files** — eliminates a follow-up `read_file` call just to know file length before using `start_line`/`end_line`. |
| `path_exists` | **Lightweight existence check** — returns `"true — <path> is a file/directory/symlink (N bytes)"` or `"false — <path> does not exist"` without reading the file. Use before read/write operations to branch on whether a path already exists. |

### Multi-file tools (`tools_multi.go`)

| Tool | Description |
|------|-------------|
| `read_multiple_files` | Read multiple files in one call. Each file header includes **file size and line count**. Partial failures are reported inline. Binary files are returned as base64. |
| `write_multiple_files` | Write multiple files in a single call — efficient for bulk refactors. Returns a per-file success/failure summary. |
| `find_replace_in_files` | Search and replace across an entire directory tree. Supports literal strings and Go regex, glob file filtering, `dry_run` preview mode, **per-file diff output**, and **reports skipped binary files**. **Always replaces all occurrences** (equivalent to `replace_all=true`). **Reports total files scanned** so you know if a glob was too restrictive. Skips hidden files/dirs by default (`show_hidden` toggles this). |

### Directory tools (`tools_dir.go`)

| Tool | Description |
|------|-------------|
| `list_directory` | List directory contents with sizes and timestamps. Supports recursion, hidden-file toggling, **`sort_by` (name or size)**, and **`glob` filter** to show only matching files (e.g. `*.go`, `*.{ts,tsx}` — **alternation `{a,b}` supported**). Summary includes total directory size. |
| `create_directory` | Create a directory (and all missing parents, like `mkdir -p`). |
| `delete_directory` | Delete a directory. Requires `force=true` for non-empty directories. |
| `directory_tree` | Get a visual `tree`-style recursive view of a directory. Shows file sizes, supports `max_depth`, `show_hidden`, `exclude_patterns`, and **`glob` filter** (same `{a,b}` alternation support as `list_directory`). **Summary includes total directory size.** |

### Search tools (`tools_search.go`)

| Tool | Description |
|------|-------------|
| `search_files` | Find files/directories by glob pattern. Supports `*`, `**`, `?`, and `{a,b}` alternation — **including multiple alternation groups** (e.g. `{src,lib}/**/*.{ts,tsx}`). Results include file size and modification time. **`show_hidden` toggle** (default: false). |
| `grep_files` | Search file contents for a literal string or Go regex. **`glob` parameter restricts which files are searched (e.g. `*.go`).** Supports `output_mode`: `"content"` / `"files_with_matches"` / `"count"`. **`show_hidden` toggle** (default: false). **`multiline: true`** enables cross-line pattern matching (dot-all regex). **`context_lines` capped at 50** (before and after each match). **`offset` for pagination** — skip the first N matches and display the next page. **`max_file_size`** skips files larger than N bytes — useful for generated/minified files. **Output header always reports files searched, binary files skipped, and total match count.** `count`/`files_with_matches` modes scan all eligible files for accurate totals. |

### User interaction (`tools_user.go`)

| Tool | Description |
|------|-------------|
| `notify_user` | **Non-blocking** fire-and-forget notification. Shows a balloon tip (Windows), system notification (macOS), or `notify-send` popup (Linux). Always falls back to stderr. Ideal for progress updates: `"Starting analysis…"`, `"Build complete."`. **`level`** parameter (`"info"` / `"warning"` / `"error"`) controls the notification icon and urgency on supported platforms. **`duration_seconds`** controls how long the toast stays visible (default: 5 s, max: 60 s). Returns immediately — never blocks the AI. |
| `ask_user` | Pop up a dialog (or fall back to console) to ask the user a question and capture their reply. **`choices` array** for multiple-choice picker dialogs with **live search/filter** (Windows WPF search box, macOS `choose from list`, Linux `zenity --list`). **`timeout_seconds`** configures how long to wait (default: 120 s, max: 3600 s). **Keep timeout below your MCP client's own request timeout** — most clients enforce 30–120 s. |

### Shell execution (`tools_exec.go`)

| Tool | Description |
|------|-------------|
| `get_working_directory` | Return the server's current working directory, OS, hostname, and **key environment variables** (`HOME`, `GOPATH`, `GOROOT`, `PATH` summary). Use as your first call when you need to orient yourself in the filesystem. |
| `run_command` | Execute any shell command (`cmd /C` on Windows, `sh -c` elsewhere). Returns combined stdout+stderr, exit code, elapsed time, and **resolved working directory** (always shown). Configurable timeout (default 60 s, max 600 s) and working directory. Supports injecting environment variables via the `env` parameter. **`max_output_bytes`** truncates large outputs to keep LLM context manageable. **`stdin`** passes content to the command's standard input (e.g. piping a script or answering prompts). Non-zero exits are surfaced as tool errors. |

---

## Requirements

- Go 1.23+

## Build

```sh
go build -o bmcptools .
```

## Run

```sh
./bmcptools
```

The server reads JSON-RPC messages from **stdin** and writes responses to **stdout** (MCP stdio transport).

## Test

```sh
go test ./...
```

## Integration with Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "file-tools": {
      "command": "/absolute/path/to/bmcptools"
    }
  }
}
```

---

## Project structure

```
main.go            — Entry point; wires up all tools and starts stdio server
helpers.go         — Shared utilities: humanizeBytes, stripBOM, isBinaryContent,
                     sniffAndOpen, readBinaryFile, readFullText, applyEdit,
                     entryWithInfo, pluralize (go-pluralize), generateDiff (Myers/go-diff)
tools_file.go      — File-manipulation tool handlers
tools_multi.go     — Multi-file tools (read_multiple_files, write_multiple_files, find_replace_in_files)
tools_dir.go       — Directory tool handlers (list_directory, create_directory, delete_directory, directory_tree)
tools_search.go    — File search and grep tool handlers (search_files, grep_files)
tools_exec.go      — Shell command execution tool handler
tools_user.go      — User-interaction (ask_user) tool handler
*_test.go          — Unit and integration tests (100+ tests)
```

---

## Changelog

### 2.2.0
- **[BUG FIX]** `directory_tree` with `glob` — directories whose subtrees contain no matching files are now **pruned** from the output entirely. Previously all directories were always shown even when they contributed no matching files, wasting AI context window and causing confusion. Connectors (`├──` / `└──`) are recomputed correctly after pruning.
- **[FEAT]** `edit_file` — new **`context_lines`** parameter (default 3, max 50) controls how many unchanged lines appear around each changed region in the returned diff. Set to 0 for a minimal diff; increase to 10+ for more surrounding context when reviewing large edits.
- **[FEAT]** `grep_files` — new **`max_file_size`** parameter skips files larger than N bytes. Ideal for excluding large generated files (minified bundles, `go.sum`, vendor lockfiles). Skipped oversized files are reported separately from binary files in the output header: `"skipped 2 files (oversized)"`.
- **[BUG FIX]** `sanitizePSHereString` — leading-whitespace `'@` lines (e.g. `  '@`) were not sanitised because `strings.TrimRight` only strips trailing whitespace. Fixed by using `strings.TrimSpace` so both leading and trailing whitespace is removed before the comparison.

### 2.1.0
- **[FEAT]** `path_exists` — new lightweight existence-check tool. Returns `"true — <path> is a file/directory (N bytes)"` or `"false — <path> does not exist"` without reading any content. Eliminates the need to call `get_file_info` just to branch on whether a path exists.
- **[FEAT]** `directory_tree` — added **`glob` filter** parameter, consistent with `list_directory`. Supports `{a,b}` alternation.
- **[BUG FIX]** `move_file` — added explicit **`overwrite` flag** (default `false`). Previously `os.Rename` silently clobbered the destination on Unix and unexpectedly failed on Windows. Now the behaviour is consistent and predictable across all platforms.
- **[BUG FIX]** `ask_user` — default `timeout_seconds` reduced from **300 s → 120 s**. Most MCP clients enforce a transport-level timeout of 30–120 s, making the old 300 s default reliably cause `MCP error -32001: Request timed out` before the user could even see the dialog. The description now explicitly warns about the MCP client timeout.
- **[FEAT]** `notify_user` — **`duration_seconds`** parameter (default 5 s, max 60 s) now controls how long the Windows WPF toast stays on screen. Previously hardcoded to 5 s with no override.
- **[FEAT]** `ask_user` choices dialog (Windows WPF) — now includes a **live search TextBox** above the list. Typing filters the list as you type; `↓` moves focus to the list. Ideal for long choice lists (e.g. file names, enum values).
- **[ARCH]** `write_file` — `show_diff` description updated to explicitly recommend setting it to `true` when editing existing files.

### 2.0.0
- **[FEAT]** `grep_files` — output header now always reports **total files searched**, binary files skipped, and eligible files. Example: `"Found 23 matches for "foo" (searched 42 files, skipped 3 binary files):"`. Eliminates the guesswork of "did my glob filter actually hit anything?"
- **[FEAT]** `grep_files` — **`count` and `files_with_matches` modes now scan all eligible files** without an artificial match limit. Previously, both modes silently truncated at `offset + max_results` matches, making per-file counts incomplete for large codebases. The `max_results` parameter now means "max files to show in the output" for these modes.
- **[FEAT]** `grep_files` — **accurate total match count in header** for `content` mode. When results are complete: `"Found 37 matches"`. When truncated by limit: `"Found 50+ matches"` — so the AI always knows whether the page is complete or partial.
- **[FEAT]** `grep_files` — **improved pagination footer**: now shows `"Showing matches N..M of K+ total"` instead of just the next offset hint. Precise enough to plan the next page request.
- **[FEAT]** `grep_files` — `count` and `files_with_matches` output now ends with a **total summary line**: `"Total: 87 matches across 5 files."` for instant confirmation.
- **[FEAT]** `get_file_info` — now includes **line count** for text files (e.g. `Lines: 1 042 lines`). Avoids a follow-up `read_file` call just to learn file length before deciding on `start_line`/`end_line` pagination.
- **[ARCH]** `errBinaryFile` — replaced anonymous `fmt.Errorf("binary file")` strings in `grepFile` and `grepFileMultiline` with a typed sentinel `errors.New("binary file")`. The handler now uses `errors.Is()` for reliable binary-skip detection.
- **[FEAT]** `tools_user_test.go` — new `TestAskUserToolPainPointSurvey`: 7-question interactive survey covering most-used tools, pain points, diff clarity, pagination UX, notification preferences, missing features, and overall description clarity. Run with `go test -run TestAskUserToolPainPointSurvey` to gather real feedback.

### 1.9.0
- **[FEAT]** `notify_user` — **`level` parameter** (`"info"` / `"warning"` / `"error"`). Controls the notification icon (Windows: `ToolTipIcon::Info/Warning/Error`) and urgency (Linux: `notify-send --urgency=low/normal/critical`). The stderr fallback now includes the level: `[AI NOTIFY][WARNING]`.
- **[FEAT]** `get_working_directory` — now includes **key environment variables** (`HOME`, `USERPROFILE`, `GOPATH`, `GOROOT`, and a condensed `PATH` showing the first 3 entries). Eliminates the need to run a separate `run_command` just to know the Go environment.
- **[FEAT]** `directory_tree` — summary now includes **total directory size** (e.g. `42 files, 7 directories (1.2 MB)`), consistent with `list_directory`.
- **[FEAT]** `append_to_file` — response now includes the **new total file size** after the append (e.g. `Appended 512 B to foo.txt (new size: 2.3 KB)`). Eliminates a follow-up `get_file_info` call to verify.
- **[BUG FIX]** `list_directory` glob filter — now supports **`{a,b}` alternation** (e.g. `*.{ts,tsx}`) using the same `expandAlternation` already used by `search_files`. Previously `filepath.Match` was called directly, silently failing for patterns with `{}`.
- **[BUG FIX]** `find_replace_in_files` glob filter — same `{a,b}` fix applied to `collectFiles` in `helpers.go` which `find_replace_in_files` uses internally.
- **[FEAT]** `read_file` `show_line_numbers` — format changed from `N\tline` to `%6d|line` (right-aligned 6-digit padded number). Matches the `LINE_NUMBER|LINE_CONTENT` convention used in the MCP client system prompt, so line references are immediately copy-pasteable.
- **[ARCH]** `applyReplaceToFile` moved from `tools_multi.go` to `helpers.go` — centralises all file-mutation utilities. Removes the `net/http` import from `tools_multi.go`.
- **[ARCH]** `grepFilesHandler` — replaced duplicated walk logic with a call to the shared `collectFiles` helper. Automatically inherits the `{a,b}` glob fix above.

### 1.8.0
- **[FEAT]** `notify_user` — **new tool**. Non-blocking fire-and-forget notification. Shows a balloon tip (Windows), `display notification` (macOS), or `notify-send` (Linux). Always prints to stderr as fallback. Returns immediately — the AI no longer needs to use `ask_user` just to inform the user of progress. Major AI UX gap closed.
- **[FEAT]** `grep_files` — **`offset` parameter** for pagination. With `max_results=50, offset=50` you get matches 51–100. The result now includes a helpful message: `[Showing matches N..M — use offset=X to see the next page]`. Previously, results beyond `max_results` were silently lost.
- **[FEAT]** `list_directory` — **`glob` filter** parameter. Pass `glob=*.go` to show only Go files. Directories are always shown when `recursive=true`. Avoids the need to call `search_files` just to see files of a specific type.
- **[FEAT]** `run_command` — **`stdin` parameter**. Pass content to the command's standard input. Enables piping scripts, answering prompts, and using commands like `cat - | wc -l` without a shell pipe.
- **[FEAT]** `write_file` — **`show_diff: true`** parameter. When overwriting an existing file, returns a unified diff of exactly what changed. Consistent with `edit_file` feedback. Helps the AI immediately verify the write was correct.
- **[BUG FIX]** `ask_user` (Windows) — `promptWindows` now normalises line endings (`\r\n` → `\n`, `\r` → `\n`) before escaping single quotes. Prevents malformed PowerShell scripts when the question contains carriage returns. `promptChoiceWindows` flattens newlines to spaces for `Out-GridView -Title` (single-line field).
- **[FEAT]** `tools_user_test.go` — expanded `TestAskUserSampleQuestions` with realistic AI use-cases: destructive action confirmation, multi-line questions (tests newline rendering), error-recovery choices, deployment target selection, secret handling. New `notify_user` test suite: `TestNotifyUserMissingMessage`, `TestNotifyUserWhitespaceMessage`, `TestNotifyUserReturnsImmediately` (verifies < 500 ms non-blocking), `TestNotifyUserDefaultTitle`.

### 1.7.0
- **[FEAT]** `ask_user` — **`timeout_seconds`** parameter (default 300 s, max 3600 s). Previously hardcoded at 5 min with no way for the AI to know or adjust how long it would block waiting for the user.
- **[BUG FIX]** `ask_user` — Windows `Out-GridView` picker now uses `-OutputMode Single` instead of `-PassThru`, preventing multi-select (Shift+Click would previously return multiple lines as a single concatenated string).
- **[BUG FIX]** `edit_file` — dry_run mode now correctly prefixes `[DRY RUN]` even when the pattern is not found. Previously the "Pattern not found" early-return path bypassed the dry_run check.
- **[FLOW]** `run_command` — **resolved working directory is now always shown** in output (even when `cwd` param is omitted). Previously the `cwd:` line only appeared when the caller explicitly set a working directory, making it ambiguous which directory the command ran in.
- **[FLOW]** `find_replace_in_files` — output now reports **total files scanned** alongside changed files. Helps diagnose overly restrictive glob patterns that result in "No matches" with zero insight into how many files were checked.
- **[FLOW]** `grep_files` — `context_lines` cap increased from 10 → **50**, and the cap is now documented in the tool description. Previously silently truncated without feedback.
- **[ARCH]** `readOneFileAsText`, `copyFileData`, `copyFileDataN`, `countLines`, `collectFiles` moved from `tools_file.go` / `tools_multi.go` to `helpers.go` — all shared utilities are now co-located. Tool files are narrower and easier to navigate.
- **[FEAT]** `tools_user_test.go` — new test file with sample `ask_user` questions demonstrating real AI use-cases, timeout clamping validation, and headless-safe skip logic (`-short` flag or `CI=true`).

### 1.6.0
- **[FEAT]** `edit_file` — **`dry_run: true`** parameter: previews a unified diff of what would change without modifying the file. Major AI UX improvement — lets the model verify uniqueness and correctness of `old_str` before committing.
- **[FEAT]** `grep_files` — **`multiline: true`** parameter: loads the full file content and matches patterns that span multiple lines using a dot-all (`(?s)`) regex. Equivalent to `ripgrep -U`. Works with both `use_regex=true` and `use_regex=false` (literal strings).
- **[BUG FIX]** `generateDiff` — hunk headers now use the standard **`@@ -a,b +c,d @@`** format instead of the non-standard `@@ -N @@`. Both line counts (original and new) are now included, making diffs compatible with standard diff tooling.
- **[ARCH]** `countContentLines` moved from `tools_file.go` to `helpers.go` — it was already used in both `tools_file.go` and `tools_multi.go`; now centralised alongside other shared string utilities.
- **[CLEAN]** `tools_user.go` — `promptConsole` and `promptChoiceConsole` now share a single `printConsolePromptHeader` helper; eliminated duplicated border-drawing code.
- **[CLEAN]** `find_replace_in_files` — per-file diff output is no longer indented 4 spaces; diff lines are written directly, making `@@`, `+`, `-` markers immediately visible.
- **[CLEAN]** `go.mod` — `go-pluralize` and `go-diff` promoted from `// indirect` to direct dependencies (both are imported directly in `helpers.go`).

### 1.5.0
- **[BUG FIX]** `grep_files` — `grepFile` previously loaded the entire file into a `[]string` slice before matching, risking OOM on large files. Replaced with a streaming ring-buffer implementation: before-context is kept in a circular buffer of at most `context_lines` entries, and after-context is accumulated lazily as the scanner advances. Memory usage is now O(context_lines + matches), not O(file size).
- **[FEAT]** `read_file` — `head=N`, `tail=N`, and `start_line/end_line` range reads now include the **total line count** in their header, e.g. `[file.go — lines 1..5 of 1000 lines]`. Previously the header omitted the total, forcing a separate call to determine file size.
- **[FEAT]** `read_multiple_files` — output now ends with a trailing **summary line**: `--- Summary: N files read, M failed (X KB total) ---`. Makes it easy to verify batch read results at a glance.
- **[FLOW]** `find_replace_in_files` — binary-file detection in `applyReplaceToFile` now checks only the **first 512 bytes** (matching `sniffAndOpen`), instead of scanning the entire file content for null bytes. Consistent heuristic, better performance on large text files.
- **[CLEAN]** `serverName` corrected to `"bmcptools"` — was `"bmcp-tools"`, inconsistent with the binary name and README.

### 1.4.0
- **[FEAT]** `get_working_directory` — new tool that returns CWD, OS/arch, and hostname. Ideal as the first call in any session to orient the LLM in the filesystem.
- **[FEAT]** `read_file` — full-file reads now include a `[filename — N lines]` header. Truncation notice now shows total line count. `show_line_numbers` now correctly applies to `head`/`tail`/`start_line`/`end_line` range reads. Both `show_line_numbers` and plain full-reads include consistent headers with line counts.
- **[FEAT]** `find_replace_in_files` — added `show_hidden` parameter (default `false`). Hidden files and dot-directories are now skipped by default, preventing accidental modification of `.git/`, `.env`, etc.
- **[FEAT]** `edit_file` — batch mode now reports which `old_str` patterns had zero matches. Single-edit "not found" message now quotes the missing pattern.
- **[BUG FIX]** `read_multiple_files` — line count in file headers was overcounted for files ending with a newline.
- **[BUG FIX]** `read_file` — truncated large-file reads no longer show a wrong line count in the header (previously computed from truncated portion; now the header is only shown for non-truncated reads).
- **[ARCH]** `readFileHandler` refactored to use the shared `sniffAndOpen` helper — eliminates duplicated binary-sniff logic.
- **[CLEAN]** Removed dead `defaultMaxOutputBytes` constant from `tools_exec.go`.

### 1.3.0
- **[FEAT]** `edit_file` — **batch edits** via `edits: [{old_str, new_str, use_regex?, replace_all?}]` array. Apply multiple changes in a single call; far more efficient than repeated individual calls.
- **[FEAT]** `read_file` — `show_line_numbers: true` prefixes every line with its number for precision editing.
- **[FEAT]** `ask_user` — `choices: string[]` parameter for multiple-choice picker dialogs (Out-GridView / osascript / zenity / console fallback).
- **[FEAT]** `run_command` — `max_output_bytes` parameter to truncate large outputs and protect LLM context.
- **[FEAT]** `search_files` + `grep_files` — `show_hidden` toggle (default: `false`), consistent with `list_directory` and `directory_tree`.
- **[FEAT]** `find_replace_in_files` — per-file **diff output** (`show_diff: true` by default) and **binary-file skip reporting**.
- **[BUG FIX]** `search_files` — `expandAlternation` now fully recursive, correctly handling multiple `{}` groups (e.g. `{src,lib}/**/*.{ts,tsx}`).
- **[ARCH]** `readBinaryFile`, `readFullText`, `applyEdit`, `entryWithInfo` moved from `tools_file.go` to `helpers.go` — shared across all tool files.
- **[ARCH]** `sniffAndOpen` helper in `helpers.go` eliminates duplicated binary-sniff + seek logic.
- **[CLEAN]** `plural()` replaced with `github.com/gertd/go-pluralize` — production-grade English pluralisation with irregular forms.
- **[CLEAN]** `generateDiff` replaced with `github.com/sergi/go-diff` (Myers algorithm) — no line-count limit, faster, higher-quality diffs.

### 1.2.0
- **[NEW]** `directory_tree` — visual recursive tree view (like `tree` command)
- **[NEW]** `read_file` — `head: N` / `tail: N` shorthand parameters
- **[NEW]** `grep_files` — `glob` parameter to restrict search to specific file types
- **[NEW]** `list_directory` — `sort_by` parameter: `"name"` (default) or `"size"` (largest first)
- **[IMPROVED]** `search_files` — patterns with `/` or `**` now match against the full relative path
- **[BUG FIX]** `write_file` — line count in response was wrong for empty files and files ending with a newline
- **[BUG FIX]** `grep_files` / `find_replace_in_files` — walk errors previously swallowed silently are now surfaced

### 1.1.0
- **[BUG FIX]** `read_multiple_files` now correctly handles binary files
- **[NEW]** `write_multiple_files` — write many files in one MCP call
- **[NEW]** `run_command` — `env` parameter to inject environment variables
- **[NEW]** `edit_file` — response now includes a unified diff
- **[NEW]** `grep_files` — `output_mode` parameter
- **[IMPROVED]** `read_multiple_files` — file headers now include size and line count
- **[IMPROVED]** `search_files` — results now include file size and modification time
- **[IMPROVED]** `list_directory` — summary now includes total directory size

---

## Version

`2.0.0`
