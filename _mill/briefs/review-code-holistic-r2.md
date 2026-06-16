**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent code reviewer for **Rename mhgo to Loomyard (lyx)**. You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**. Round **2**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints
# Constraints

## Path Invariant

All worktree and container geometry must be resolved through `internal/paths`, not raw primitives. This invariant is enforced at build time.

### Rule

- All cwd and worktree root queries MUST go through `internal/paths.Getwd()` and `internal/paths.Resolve()`.
- Raw `os.Getwd` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- The ban is enforced at `go test` / CI time by `internal/paths/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found.

### For New Code

If you need a cwd or worktree root:
- Call `paths.Getwd()` to get the current working directory.
- Call `paths.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, container, relative path, etc.).
- Use the `Layout` methods to derive paths: `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `PortalLink(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `HubName()`.


## Files included (N=99)

- C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\01-code-rename.md
- C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\02-docs-and-config.md
- C:\Code\loomyard\wts\rename-to-loomyard\go.mod
- C:\Code\loomyard\wts\rename-to-loomyard\.gitignore
- C:\Code\loomyard\wts\rename-to-loomyard\cmd\lyx\main.go
- C:\Code\loomyard\wts\rename-to-loomyard\cmd\lyx\main_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\paths.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\paths_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\enforcement_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\worktreelist.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\worktreelist_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\git\git_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\lock\lock.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\lock\lock_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\output\output_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\config\config.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\config\config_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\gitignore\gitignore.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\gitignore\gitignore_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\board.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\cli.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\config.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\git.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\init.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\store.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\sync.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\spawn_other.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\spawn_windows.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\board_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\cli_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\config_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\git_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\init_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\layer_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\render_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\store_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\sync_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\task_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\doc.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\bench_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\bench_git_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\concurrency_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\integration_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\cli.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\color.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\color_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\menu.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\menu_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\spawn.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\spawn_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\vscode.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\attach.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\cli.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\daemon.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\down.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\review.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\state.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\status.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\up.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\state_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\muxpoc_smoke_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\add.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\add_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\cli.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\cli_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\config.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\config_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\launchers.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\launchers_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\list.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\list_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\portals.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\portals_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\remove.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\remove_test.go
- C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\worktree.go
- C:\Code\loomyard\wts\rename-to-loomyard\_mill\discussion.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\overview.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\roadmap.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\psmux-tui-behavior.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\benchmarks\board-performance.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\benchmarks\test-suite-timing.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\board.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\ide.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-exploration.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-hooks-exploration.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-proposal.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\muxpoc.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\worktree.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\README.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\config.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\gitignore.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\lock.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\paths.md
- C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\state.md
- C:\Code\loomyard\wts\rename-to-loomyard\CONSTRAINTS.md
- C:\Code\loomyard\wts\rename-to-loomyard\mill-config.yaml

## Plan + source files to review
- Overview: `C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\00-overview.md`
- Batch file(s):
  - `C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\01-code-rename.md`
  - `C:\Code\loomyard\wts\rename-to-loomyard\_mill\plan\02-docs-and-config.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `C:\Code\loomyard\wts\rename-to-loomyard\go.mod`
- `C:\Code\loomyard\wts\rename-to-loomyard\.gitignore`
- `C:\Code\loomyard\wts\rename-to-loomyard\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\cmd\lyx\main_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\paths.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\paths_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\enforcement_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\worktreelist.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\paths\worktreelist_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\git\git_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\lock\lock.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\lock\lock_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\output\output_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\config\config.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\config\config_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\gitignore\gitignore.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\gitignore\gitignore_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\board.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\cli.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\config.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\git.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\init.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\store.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\sync.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\spawn_other.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\spawn_windows.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\board_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\cli_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\config_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\git_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\init_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\layer_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\render_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\store_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\sync_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\task_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\doc.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\bench_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\bench_git_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\concurrency_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\board\boardtest\integration_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\cli.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\color.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\color_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\menu.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\menu_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\spawn.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\spawn_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\ide\vscode.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\attach.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\cli.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\daemon.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\down.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\review.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\state.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\status.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\up.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\state_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\muxpoc\muxpoc_smoke_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\add.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\add_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\cli.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\cli_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\config.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\config_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\launchers.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\launchers_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\list.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\list_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\portals.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\portals_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\remove.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\remove_test.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\internal\worktree\worktree.go`
- `C:\Code\loomyard\wts\rename-to-loomyard\_mill\discussion.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\overview.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\roadmap.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\psmux-tui-behavior.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\benchmarks\board-performance.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\benchmarks\test-suite-timing.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\board.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\ide.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-exploration.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-hooks-exploration.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\mux-proposal.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\muxpoc.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\modules\worktree.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\README.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\config.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\gitignore.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\lock.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\paths.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\docs\shared-libs\state.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\CONSTRAINTS.md`
- `C:\Code\loomyard\wts\rename-to-loomyard\mill-config.yaml`

## Intentionally deleted (N=2)

- cmd/mhgo/main.go
- cmd/mhgo/main_test.go

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the implementation as a whole)

- **End-to-end plan alignment** — every batch's cards are realised; every file listed across all batches' `Context:`/`Edits:`/`Creates:` is present in the source files provided.
- **Shared-decisions alignment** — the `## Shared Decisions` subsections are applied consistently across all batches; deviation is BLOCKING.
- **Out-of-plan files** — BLOCKING if any source file is present that is not accounted for in any batch's reference lists. If the implementer added it, the batch file must have been updated first; a review with surprise files means that discipline was skipped somewhere.
- **Cross-batch contracts** — interfaces produced by one batch and consumed by another are compatible. Dependency order implied by `depends-on:` is reflected in the code (consumers don't assume behaviour the producer doesn't guarantee).
- **Integration correctness** — the pieces work together, not just per-batch. Call sites match signatures; shared state is consistently managed; error surfaces compose.
- **Global utility duplication** — BLOCKING if two batches independently reimplement the same helper. Consolidate into a shared module.
- **Test coverage across the whole surface** — happy paths + errors for every batch's entry point. Integration tests reach across batch boundaries where appropriate.
- **Constraint violations** — BLOCKING.
- **Codebase consistency** — naming, error handling, imports, and style match the conventions visible in the source files provided.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. Cite file and line, state the issue, propose the fix.

Target length: ~400 tokens for APPROVE, ~800–1500 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1800 tokens, compress.

~~~markdown
MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx) — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** `path/to/file.py:42` (or `:42-58`)
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** `path/to/file.py:N`
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
~~~

Severity / verdict rules match review-code-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
