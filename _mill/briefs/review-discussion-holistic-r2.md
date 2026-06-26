**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent discussion reviewer for **Speed up internal/warp integration tests**. Round **2**. Reviewer model: **opushigh**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: Do NOT use Write, Edit, or run git/bash. Return review as text.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

---

## Task

Read the discussion at `C:\Code\loomyard\wts\warp-test-speedup\_mill\discussion.md`. The discussion file is the authoritative scope. Read files referenced in `## Technical Context` to verify claims.

Constraints:
# Constraints

## Path Invariant

All worktree and hub geometry must be resolved through `internal/paths`, not raw primitives. This invariant is enforced at build time.

### Rule

- All cwd and worktree root queries MUST go through `internal/paths.Getwd()` and `internal/paths.Resolve()`.
- Raw `os.Getwd` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
- The ban is enforced at `go test` / CI time by `internal/paths/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found.

### `_lyx` and config-file paths

- The `_lyx` directory name, its `config/` subdirectory, and any `<module>.yaml` config file MUST be resolved through `internal/paths` helpers — never built from string literals like `filepath.Join(base, "_lyx", "config")` or `"board.yaml"`.
  - `paths.LyxDirName` — the `_lyx` directory name constant (use `filepath.Join(base, paths.LyxDirName)` for a bare `_lyx` dir).
  - `paths.ConfigDir(base)` — the `<base>/_lyx/config` directory.
  - `paths.ConfigFile(base, module)` — the `<base>/_lyx/config/<module>.yaml` file (e.g. `module` = `"board"`, `"worktree"`, `"weft"`). For a relative path, pass `"."` as `base`.
- **This rule applies to test code too.** A migration of the config layout (PR #20 moved configs from `_lyx/<module>.yaml` to `_lyx/config/<module>.yaml`) silently broke a hardcoded test fixture (`internal/worktree/cli_test.go`) because its literal write path drifted from the loader's read path. Routing every path through the helpers makes such migrations track automatically. The two genuine exceptions are `internal/paths/*_test.go` (those literals *are* the spec under test) and `_lyx` used as link-target geometry or string-content assertions — neither resolves a config path.
- This case is **not** caught by `internal/paths/enforcement_test.go` (which only bans `os.Getwd` / `git rev-parse`); it is a code-review and planning-discipline rule.

### For New Code

If you need a cwd or worktree root:
- Call `paths.Getwd()` to get the current working directory.
- Call `paths.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, hub, relative path, etc.).
- Use the `Layout` methods to derive paths: `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.

If you need an `_lyx` / config path (in production or test code), use `paths.LyxDirName`, `paths.ConfigDir(base)`, and `paths.ConfigFile(base, module)` as above.

## lyxtest Leaf Invariant

`internal/lyxtest` must remain a leaf package importing only the standard library and `internal/paths`. It must not import `internal/configreg` or any feature package (`board`, `worktree`, `weft`).

### Rule

- `internal/lyxtest` must not import `internal/configreg` or any feature package.
- Tests that need real configuration must seed it themselves via `SeedConfig`, passing a configreg-free `map[string]string` (module name to YAML content).
- The `configreg.Modules()` to map conversion happens at the test site, in a package that may legally import `configreg`.
- Feature packages' internal tests import `lyxtest`; a `lyxtest → configreg → feature` import would close a test-build cycle (the trap that motivated this task).

### Rationale

The cycle closes silently when `lyxtest` imports `configreg` and `configreg` imports feature packages, but only under `-tags integration` (feature-internal tests are integration-tagged). An untagged import-scan test (`internal/lyxtest/leaf_enforcement_test.go`) catches a reintroduced edge on every `go test ./...` with a clear message, instead of waiting for an integration suite run.

### For New Tests

If a test needs real config:
- Obtain each module's template from the module's own `ConfigTemplate()` function (e.g., `weft.ConfigTemplate()`).
- Use the unqualified name (`ConfigTemplate()`) when calling from a file in that same package.
- Use the qualified form (e.g., `weft.ConfigTemplate()`) from a different package, adding the feature import as needed to the test file.
- Pass the templates to `lyxtest.SeedConfig(tb, repoDir, map[string]string{...})` **never** pass `configreg.Module` types or call `configreg` from inside lyxtest.

The enforcement test (`internal/lyxtest/leaf_enforcement_test.go`) is run on every `go test ./...` and fails the build if any of the banned imports appear in lyxtest source files.

## Documentation Lifecycle

For the convention governing which docs are kept and which are deleted (mechanical per-module docs vs. durable design docs), see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Source-grounding rule

Never fabricate file contents or code behaviour you have not actually read. You are in tool-use mode — if you need a file to verify a claim in the discussion, open it with Read/Grep/Glob. Do not infer from filenames or positions.

## Criteria (apply briefly to each)

- **Undecided items** — TBDs, unresolved options, multiple alternatives without a choice.
- **Scope** — what's in/out; could a plan writer disagree?
- **Constraint coverage** — CONSTRAINTS.md items acknowledged; implicit perf/compat constraints stated.
- **Failure modes** — empty states, concurrency, invalid input, partial failures addressed.
- **Testing** — strategy named (unit/integration/e2e); absence or non-commital language flagged.
- **Ambiguity** — requirements needing interpretation ("fast", "handle errors").
- **Feasibility** — technical obstacles not addressed, based on source files read.
- **Decisions** — each `### Decision:` has rationale + rejected alternatives; implicit decisions surfaced.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** No "I reviewed..." sentences. No narrative intro.

Per finding: 3–5 lines total, short and factual. The consumer has full context of the discussion; do NOT explain background. Cite the section, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE (just verdict + brief summary), ~600–900 tokens for GAPS_FOUND (one finding block per issue). If you produce more than ~1200 tokens, you are being verbose — compress.

```
MILL_REVIEW_BEGIN
# Review: Speed up internal/warp integration tests

```yaml
verdict: APPROVE | GAPS_FOUND
reviewer_model: opushigh
reviewed_file: <artefact reference>
date: <UTC YYYY-MM-DD>
```

## Findings

### [GAP] <short title, <60 chars>
**Section:** <§ or heading>
**Issue:** <one sentence — what's missing or ambiguous>
**Fix:** <one sentence — what to clarify or add>

### [NOTE] <short title>
**Section:** <§>
**Issue:** <one sentence>
**Fix:** <one sentence>

## Verdict

<APPROVE | GAPS_FOUND>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity rules (discussion-specific, per v1 convention):
- `GAP` — must resolve before plan writing can proceed.
- `NOTE` — record but do not block.

Verdict rules:
- `APPROVE` — zero GAPs. NOTEs fine.
- `GAPS_FOUND` — one or more GAPs.

Note: plan and code reviews use `BLOCKING` / `NIT` + `REQUEST_CHANGES`. Discussion review uses `GAP` / `NOTE` + `GAPS_FOUND` because the semantics differ — a discussion "gap" is missing information, not a must-fix defect.

Omit the `## Findings` section entirely if there are zero findings. Never invent findings to pad the review.
