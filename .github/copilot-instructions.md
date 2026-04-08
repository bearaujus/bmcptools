# Instructions

1. Read `README.md`.
2. Read `Makefile`.
3. At the end of conversation, update `README.md` & `Makefile` if required.

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

---
