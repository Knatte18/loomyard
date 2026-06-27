**You are a READ-ONLY reviewer. You MUST NOT call Edit, Write, Bash, or any
tool that modifies files or runs commands. You MUST NOT make git commits.
Your sole output is the review file in the format below. If you find issues,
REPORT them — do NOT fix them.**

You are an independent plan reviewer for **Built-in CLI help: lyx self-documents modules & commands**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **opushigh**. Round **1**.

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


## Files included (N=55)

- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\00-overview.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\01-clihelp-foundation.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\02-flat-and-config-modules.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\03-ide-and-weft.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\04-board.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\05-muxpoc-and-warp.md
- C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\06-root-assembly-and-tests.md
- C:\Code\loomyard\wts\builtin-cli-help\go.mod
- C:\Code\loomyard\wts\builtin-cli-help\go.sum
- C:\Code\loomyard\wts\builtin-cli-help\internal\output\output.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\initcli\initcli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\update\update.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\menu.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\configcli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\initcli\initcli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\update\update_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\configcli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\ide\menu.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\ide\spawn.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\paths\paths.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\ide\cli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\spawn.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\sync.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\status.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\config.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\weft.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\cli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\ide\cli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\weft\cli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\board\board.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\board\cli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\board\spawn.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\board\cli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cmd.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\up.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\review.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\attach.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\down.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\status.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\daemon.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cli.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\remove.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\prune.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\cleanup.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\list.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\status.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\add.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\clone.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\checkout.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\reconcile.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\warp.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cli_test.go
- C:\Code\loomyard\wts\builtin-cli-help\internal\warp\warp_test.go
- C:\Code\loomyard\wts\builtin-cli-help\cmd\lyx\main.go
- C:\Code\loomyard\wts\builtin-cli-help\cmd\lyx\main_test.go

## Plan files to review
- Overview: `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\00-overview.md`
- Batches:
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\01-clihelp-foundation.md`
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\02-flat-and-config-modules.md`
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\03-ide-and-weft.md`
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\04-board.md`
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\05-muxpoc-and-warp.md`
- `C:\Code\loomyard\wts\builtin-cli-help\_mill\plan\06-root-assembly-and-tests.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `C:\Code\loomyard\wts\builtin-cli-help\go.mod`
- `C:\Code\loomyard\wts\builtin-cli-help\go.sum`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\output\output.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\initcli\initcli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\update\update.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\menu.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\configcli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\initcli\initcli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\update\update_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\configcli\configcli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\ide\menu.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\ide\spawn.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\paths\paths.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\ide\cli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\spawn.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\sync.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\status.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\config.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\weft.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\cli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\ide\cli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\weft\cli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\board\board.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\board\cli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\board\spawn.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\board\cli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cmd.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\up.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\review.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\attach.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\down.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\status.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\daemon.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cli.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\remove.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\prune.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\cleanup.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\list.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\status.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\add.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\clone.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\checkout.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\reconcile.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\warp.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\muxpoc\cli_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\internal\warp\warp_test.go`
- `C:\Code\loomyard\wts\builtin-cli-help\cmd\lyx\main.go`
- `C:\Code\loomyard\wts\builtin-cli-help\cmd\lyx\main_test.go`

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
# Review: Built-in CLI help: lyx self-documents modules & commands — holistic

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
