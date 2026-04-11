# Instructions

1. Read the **full contents** of:
    - `./README.md`
    - `./Makefile`
    - `./CONTRIBUTING.md`
2. Understand and follow the rules, patterns, and conventions based on those files.
3. At the end of a conversation or implementation, update these files and their references if required:
    - `./README.md`
    - `./Makefile`
    - `./CONTRIBUTING.md`
---

## Before Completing Any Task

- Run `go build ./...` to verify compilation.
- Run `go test ./...` to verify all tests pass.
- If you added or modified a tool, also update:
    - `internal/asset/descriptions/server_instructions.txt`
    - `README.md` (tool tables and counts)
- Do **not** self-declare done — confirm with the user first.

---

## If `bmcptools` is available on your MCP, do:

> **ABSOLUTE RULE: Every question, confirmation, and clarification MUST use the `bmcptools-ask_user` MCP tool — never plain chat text. If it fails, retry; if still failing, fall back to the built-in `ask_user`.**

### How to Start

**User opens with a greeting or no task (e.g. "Hi", "Hello"):**
→ Call `bmcptools-ask_user` to ask what their task is, then follow the **After Every Task** loop below.

**User opens with a task already stated (e.g. "Please add X feature"):**
→ Implement the task directly, then follow the **After Every Task** loop below.

### After Every Task — Repeat This Loop

1. Call `bmcptools-ask_user` to confirm the user is satisfied — **do not self-declare done**.
2. Call `bmcptools-ask_user` to ask if there are more tasks.
3. **If yes** → implement the new task, return to step 1.
4. **If no** → ask once more: *"Are you sure there's nothing else?"*
5. **If confirmed again** → stop. Only stop after **two consecutive "nothing left"** answers.

```text
Note: If the user says "Okay, I'm done" but then reveals a new task, the counter resets.
You need 2 clean, consecutive "nothing left" answers before stopping.
```

### Mid-Task Rules

- For any ambiguity (scope, design choices, unexpected blockers) — call `bmcptools-ask_user` before proceeding. Do not assume.
- Prefer `choices` arrays for known options; the UI adds a freeform input automatically.
- Always use the `details` parameter to show relevant context alongside your question — e.g. a summary of changes made, current findings, or background info. Keep the `question` itself short; put the substance in `details`.

### Sub-Agent Rules (task tool)

When launching sub-agents via the `task` tool, **ALWAYS** include this instruction in the prompt:

> **IMPORTANT: Do NOT use `bmcptools-ask_user`, `ask_user`, or any interactive confirmation tools. You are a sub-agent — work autonomously, make reasonable decisions, and complete the task without asking the user anything. If something is ambiguous, pick the most reasonable option and proceed.**

The `bmcptools-ask_user` rule applies **only to the main agent** (you). Sub-agents must never pop up dialogs or ask the user questions — they should work silently and return results.

---
