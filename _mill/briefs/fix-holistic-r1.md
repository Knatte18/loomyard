# Holistic Fixer Brief — Reconcile stale design docs (stateless + weft model)

You are a dedicated holistic fixer for the mill-v2 orchestrator. This is a cold-start session with no prior context. You have access to the entire worktree and may touch any file mentioned in any finding. You must read the review file and the plan to understand and apply the fixes.

## Inputs

- **Holistic review file:** `C:\Code\loomyard\wts\reconcile-stale-docs\_mill\reviews\20260619-112418-code-review-r1.md`
- **Plan overview:** `C:\Code\loomyard\wts\reconcile-stale-docs\_mill\plan\00-overview.md`
- **Worktree cwd (use for git and verify):** `C:\Code\loomyard\wts\reconcile-stale-docs`
- **Wiki path:** `C:\Code\loomyard\wiki`
- Round: **1**

Batch plan files (for `verify:` commands):

```
C:\Code\loomyard\wts\reconcile-stale-docs\_mill\plan\01-docs-durable-core.md
C:\Code\loomyard\wts\reconcile-stale-docs\_mill\plan\02-docs-sweep-and-moves.md
C:\Code\loomyard\wts\reconcile-stale-docs\_mill\plan\03-code-headers.md
```

## Before reading any finding

Load the **mill-receiving-review** skill before reading any finding in `C:\Code\loomyard\wts\reconcile-stale-docs\_mill\reviews\20260619-112418-code-review-r1.md`. This is non-negotiable.

## Fix discipline

1. Apply findings in the order the review lists them.
2. After each fix, commit using the `git-commit` skill (so lint and `codeguide-update` run per commit). Do not call raw `git commit`.
3. For each finding routed to FIX: edit the relevant file(s) and commit.
4. When a finding describes a repeating or systemic pattern — the same violation class appearing across multiple files — do NOT fix only the cited exemplars. Instead, grep or search the whole worktree for that pattern and fix every occurrence in one pass. For any newly-touched files discovered during the sweep, add them to the relevant batch plan file's `Edits:` or `Creates:` list before editing (you cannot edit files outside a batch's declared scope without updating the plan first). Include in the commit message a note of the sweep (e.g. "swept all occurrences of <pattern>") so that review can see the scope of what was fixed.
5. For each finding routed to PUSH BACK: note your rebuttal; do not modify code.
6. If a fix requires touching a file not mentioned in any batch plan file:
   - Add the file to the relevant batch file first.
   - Commit the plan edit (`plan: extend <batch-name> refs for <short reason>`).
   - Then make the code change.
7. If a finding cannot be fixed without revising the plan, report `{"status":"stuck","stuck_type":"logic","reason":"plan conflict: <finding title>"}` (note the exact prefix).

## Verify

After all fixes are committed, run every non-null `verify:` command from every batch plan file listed above, in the order listed. Run each from `C:\Code\loomyard\wts\reconcile-stale-docs` via Bash. If a verify command fails:

- Try to self-fix and retry.
- After **2** failing self-fix attempts for the same batch, stop and report `stuck`.

If all `verify:` commands are null, skip straight to Report.

## Report

Your last line of output (after all work and commits) MUST be a single JSON object:

```json
{"status":"success","commit_sha":"<last-HEAD-sha>","session_id":"6f904a7b-075e-4f77-81e2-689e60587f61"}
```
**Do not wrap the JSON in a code block. Output it as a bare line — no backticks, no fence. Anything other than a bare JSON line is treated as `stuck_type: logic`.**

**`session_id` MUST be exactly `6f904a7b-075e-4f77-81e2-689e60587f61` (the UUID shown in the example above — it was injected into this brief when it was rendered). Copy it verbatim.**

or, when stuck:

```json
{"status":"stuck","stuck_type":"transient|verify|logic","reason":"<one-line>","commit_sha":"<last-HEAD-sha>","session_id":"6f904a7b-075e-4f77-81e2-689e60587f61"}
```
**Do not wrap the JSON in a code block. Output it as a bare line — no backticks, no fence. Anything other than a bare JSON line is treated as `stuck_type: logic`.**

**`session_id` MUST be exactly `6f904a7b-075e-4f77-81e2-689e60587f61` (the UUID shown in the example above — it was injected into this brief when it was rendered). Copy it verbatim.**

`stuck_type` values:
- `transient` — tool/network failure that a retry might clear (quota, 5xx, timeout).
- `verify` — `verify:` still failing after 2 self-fix attempts.
- `logic` — plan is unclear, contradicts itself, or requires plan revision.

Anything other than this JSON on the last line is a protocol violation; mill-go treats that as `stuck_type: logic` with reason "no structured report".

**Long-session reminder:** if you have produced a lot of tool output earlier in this session (e.g. many `Bash` calls, large `Read` results), your final assistant turn's text output may be truncated by the orchestrator before the JSON line is captured. To protect against this, emit the JSON line as the **first** non-tool content of your final assistant turn, before any optional commentary or further tool calls. Re-emit the JSON line at the end of the same turn as well — duplicate JSON is fine, `_implementer_common._forward_output` reads the last match.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Banned: TodoWrite, WebFetch, WebSearch. Use `git -C C:\Code\loomyard\wts\reconcile-stale-docs` for commits; do not `cd`.

## Cross-worktree isolation

You run inside a task worktree. The parent worktree (the repo's main branch checkout) is a sibling directory — do NOT change directory into it.

- **Banned:** `cd <parent-worktree-path>` or any command that changes the process working directory to the parent. A single stray `cd` to the parent corrupts the shell cwd for every subsequent command in this session — the rest of the batch runs in the wrong directory with no error indicator.
- **Allowed:** `git -C <parent-path> log/status/show/diff/ls-files` for read-only queries. Never `git -C <parent-path> commit/push/add` — those would mutate the parent's state.
- **If you need a file from the parent:** use `git -C C:\Code\loomyard\wts\reconcile-stale-docs show <parent-branch>:<path>` to read it without changing cwd.
- **Never `cd` into a test fixture or scratch directory.** Fixtures under `.scratch/`, `unit_tests/fixtures/`, or any sub-tree may contain their own `.git/` — `cd <fixture>` corrupts every subsequent `git commit` in this session because git resolves the repo from cwd. To inspect a fixture, use the `Read` tool (for files) or `git -C <fixture> log/status` (for git queries). To run a test that exercises a fixture, run the test from the worktree root.
