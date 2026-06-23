**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent plan reviewer for **weft producers: _lyx/config, lyx config, codeguide**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **opushigh**. Round **2**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints
# Constraints

## Path Invariant

All worktree and hub geometry must be resolved through `internal/paths`, not raw primitives. This invariant is enforced at build time.

### Rule

- All cwd and worktree root queries MUST go through `internal/paths.Getwd()` and `internal/paths.Resolve()`.
- Raw `os.Getwd` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- The ban is enforced at `go test` / CI time by `internal/paths/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found.

### For New Code

If you need a cwd or worktree root:
- Call `paths.Getwd()` to get the current working directory.
- Call `paths.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, hub, relative path, etc.).
- Use the `Layout` methods to derive paths: `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`.

## Documentation Lifecycle

For the convention governing which docs are kept and which are deleted (mechanical per-module docs vs. durable design docs), see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=28)

- C:\Code\loomyard\wts\weft-producers\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\weft-producers\_mill\plan\01-module-config-templates.md
- C:\Code\loomyard\wts\weft-producers\_mill\plan\02-paths-host-junctions.md
- C:\Code\loomyard\wts\weft-producers\_mill\plan\03-config-edit-machinery.md
- C:\Code\loomyard\wts\weft-producers\_mill\plan\04-lyx-config-command.md
- C:\Code\loomyard\wts\weft-producers\internal\board\config.go
- C:\Code\loomyard\wts\weft-producers\internal\board\init.go
- C:\Code\loomyard\wts\weft-producers\internal\worktree\config.go
- C:\Code\loomyard\wts\weft-producers\internal\weft\config.go
- C:\Code\loomyard\wts\weft-producers\internal\board\init_test.go
- C:\Code\loomyard\wts\weft-producers\internal\paths\codeguide_guard_test.go
- C:\Code\loomyard\wts\weft-producers\internal\paths\paths.go
- C:\Code\loomyard\wts\weft-producers\CONSTRAINTS.md
- C:\Code\loomyard\wts\weft-producers\docs\overview.md
- C:\Code\loomyard\wts\weft-producers\internal\worktree\add.go
- C:\Code\loomyard\wts\weft-producers\internal\worktree\weft.go
- C:\Code\loomyard\wts\weft-producers\internal\worktree\weft_test.go
- C:\Code\loomyard\wts\weft-producers\internal\paths\weft_test.go
- C:\Code\loomyard\wts\weft-producers\internal\lyxtest\lyxtest.go
- C:\Code\loomyard\wts\weft-producers\internal\config\config.go
- C:\Code\loomyard\wts\weft-producers\internal\config\config_test.go
- C:\Code\loomyard\wts\weft-producers\internal\output\output.go
- C:\Code\loomyard\wts\weft-producers\internal\weft\cli.go
- C:\Code\loomyard\wts\weft-producers\internal\ide\menu.go
- C:\Code\loomyard\wts\weft-producers\cmd\lyx\main.go
- C:\Code\loomyard\wts\weft-producers\docs\roadmap.md
- C:\Code\loomyard\wts\weft-producers\internal\ide\menu_test.go
- C:\Code\loomyard\wts\weft-producers\cmd\lyx\main_test.go

## Plan files to review
- Overview: `C:\Code\loomyard\wts\weft-producers\_mill\plan\00-overview.md`
- Batches:
- `C:\Code\loomyard\wts\weft-producers\_mill\plan\01-module-config-templates.md`
- `C:\Code\loomyard\wts\weft-producers\_mill\plan\02-paths-host-junctions.md`
- `C:\Code\loomyard\wts\weft-producers\_mill\plan\03-config-edit-machinery.md`
- `C:\Code\loomyard\wts\weft-producers\_mill\plan\04-lyx-config-command.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `C:\Code\loomyard\wts\weft-producers\internal\board\config.go`
- `C:\Code\loomyard\wts\weft-producers\internal\board\init.go`
- `C:\Code\loomyard\wts\weft-producers\internal\worktree\config.go`
- `C:\Code\loomyard\wts\weft-producers\internal\weft\config.go`
- `C:\Code\loomyard\wts\weft-producers\internal\board\init_test.go`
- `C:\Code\loomyard\wts\weft-producers\internal\paths\codeguide_guard_test.go`
- `C:\Code\loomyard\wts\weft-producers\internal\paths\paths.go`
- `C:\Code\loomyard\wts\weft-producers\CONSTRAINTS.md`
- `C:\Code\loomyard\wts\weft-producers\docs\overview.md`
- `C:\Code\loomyard\wts\weft-producers\internal\worktree\add.go`
- `C:\Code\loomyard\wts\weft-producers\internal\worktree\weft.go`
- `C:\Code\loomyard\wts\weft-producers\internal\worktree\weft_test.go`
- `C:\Code\loomyard\wts\weft-producers\internal\paths\weft_test.go`
- `C:\Code\loomyard\wts\weft-producers\internal\lyxtest\lyxtest.go`
- `C:\Code\loomyard\wts\weft-producers\internal\config\config.go`
- `C:\Code\loomyard\wts\weft-producers\internal\config\config_test.go`
- `C:\Code\loomyard\wts\weft-producers\internal\output\output.go`
- `C:\Code\loomyard\wts\weft-producers\internal\weft\cli.go`
- `C:\Code\loomyard\wts\weft-producers\internal\ide\menu.go`
- `C:\Code\loomyard\wts\weft-producers\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\weft-producers\docs\roadmap.md`
- `C:\Code\loomyard\wts\weft-producers\internal\ide\menu_test.go`
- `C:\Code\loomyard\wts\weft-producers\cmd\lyx\main_test.go`

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the plan as a whole)

- **Constraint violations** — BLOCKING.
- **Alignment** — plan covers all task requirements.
- **Decision alignment** — every `### Decision:` in `## Shared Decisions` faithfully implemented.
- **Completeness** — every card has `Creates`/`Edits`, `Context`, `Requirements`, `Commit`.
- **Sequencing + batch dependencies** — correct order within and across batches; `batch-depends` accurate; no forward deps.
- **Batch Index DAG integrity** — BLOCKING if the `batches:` block in `00-overview.md` has a cycle, references a batch name not declared, or names a `file:` not present in the plan directory.
- **Edge cases + risks** — failures, empty states, boundaries addressed.
- **Over-engineering** — unneeded abstractions or unrequested features.
- **Codebase consistency** — follows patterns in the source files provided.
- **Test coverage** — error paths + edges.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).
- **Integration test reachability** — BLOCKING if integration tests added but `verify:` doesn't run them.
- **Explore targets** — purpose-driven; subset of `Context:`.
- **Step granularity + atomicity** — each card small and self-contained.
- **Requirements specificity** — BLOCKING if `Requirements:` uses vague prose ("refactor X", "update to use helper") without naming the specific function, class, or constant being changed. Stable identifiers are required.
- **Context field** — non-empty per card; Edits: files are implicitly read.
- **Context completeness** — BLOCKING if `Requirements:` mentions a function, class, or constant from a file not listed in `Context:` or `Edits:`. The implementer may only read files in `Context:`; a missing entry means cold-start exploration.
- **Global step numbering** — unique, sequential, no gaps across batches.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. The consumer has full context of the plan; do NOT explain background. Cite the batch/card, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE, ~600–1200 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1500 tokens, compress.

```
MILL_REVIEW_BEGIN
# Review: weft producers: _lyx/config, lyx config, codeguide — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: opushigh
reviewed_file: plan/
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING] <short title, <60 chars>
**Location:** <batch / card number>
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT] <short title>
**Location:** <batch / card>
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity / verdict rules match review-plan-batch.md.

Omit `## Findings` if zero findings. Never invent findings to pad.
