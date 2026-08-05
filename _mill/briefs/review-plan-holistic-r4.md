**If you find issues, REPORT them — do NOT fix them.**

You are an independent plan reviewer for **fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **fablehigh**. Round **4**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints
# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly machine-enforced (named test, fails `go test`/CI) and partly a review obligation. This file states rules only — no rationale, no incident narratives, no historical justification. Fuller design/how-to lives in godoc and `docs/`.

## Hub Geometry Invariant

`internal/hubgeometry` owns only cwd/worktree-root/anchor resolution — never weft, never any per-module path.

- **`root` always means the git worktree/repo root; the current working directory is `cwd`.** Never name a parameter, field, or local variable `root` for a value that is actually `cwd`, or vice versa.
- All cwd/worktree-root queries go through `hubgeometry.Getwd()`/`Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`.
- `hubgeometry.Resolve` exposes only `Cwd`, the worktree root, `Hub`, and (from the recorded anchor) `RelPath`. It never resolves or exposes a weft path, a junction path, or any per-module subdirectory — those are not geometry `hubgeometry` owns.
- A module's own durable-storage subdirectory (e.g. `_lyx/plan`, `_lyx/webster`) is that module's own private relative-path constant, joined onto `cwd` directly — never a `hubgeometry` function call. Adding a module's own subdirectory is never a `hubgeometry` change.
- Weft-sibling paths and junction construction belong to `internal/fabricengine`, never `hubgeometry`: `WeftWorktree`/`WeftRepoRoot`/`HostLyxLink`/`HostJunctions`/portal and launcher paths, and the `Prime`/sibling-worktree-list lookup they're built from, are `fabricengine`-private. `hubgeometry` never mentions weft.
- `_board` (a real `weft:main` worktree at `<Hub>/_board`, not a junction) is the one hub-structural token `hubgeometry` still owns directly, because it needs the name itself to read `<Hub>/_board/.fabric-anchor`. Every other geometry token (`_lyx`, `_pattern`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`) is owned by `fabricengine`, not `hubgeometry`.
- Geometry is structural, never config/env-overridable.
- The weft-backed junction name-set is injected from fabric config (`fabric.yaml`'s `pathspec`, read at `<Hub>/_board/_lyx/config/fabric.yaml`) — `fabricengine`'s concern, not `hubgeometry`'s.
- `RelPath` resolves from the recorded `.fabric-anchor` marker, not positionally from cwd; cwd is a validated at-or-below gate (`ErrCwdOutsideAnchor` if violated), falling back to cwd-derived `RelPath` only when the marker is absent. `ResolveWorktree`/`SiblingLayout` read the same anchor with no cwd gate.
- **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`).

## lyxtest Leaf Invariant

`internal/lyxtest` stays a leaf: imports only stdlib and `internal/hubgeometry` — never `internal/configreg` or any feature package.

- Tests needing real config call `lyxtest.SeedConfig(tb, dir, map[string]string{...})`.
- **Enforced by** `internal/lyxtest/leaf_enforcement_test.go`.

## Modelspec Leaf Invariant

`internal/modelspec` production code imports only stdlib, `internal/hubgeometry`, and `gopkg.in/yaml.v3`.

- `configreg` → `modelspec` is allowed (for `modelspec.ConfigTemplate`); the reverse is never allowed.
- **Enforced by** `internal/modelspec/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Treadle Runner-Seam Invariant

`internal/treadleengine` never imports `internal/burlerengine` or any `internal/*cli` package; round runners adapt onto treadle's `RoundRunner` vocabulary in their own packages.

- Import allowlist: stdlib, `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/shuttleengine`, `gopkg.in/yaml.v3` — not `internal/hubgeometry` directly. Policed on direct imports only, not the transitive closure.
- **Enforced by** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`).

## Tokenvocab Leaf Invariant

`internal/tokenvocab` production code imports only stdlib, `internal/hubgeometry`, and `internal/stencil`.

- Reverse import (`tokenvocab` → `reed`/`loom`/any feature package) is never allowed.
- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Scoutengine Leaf Invariant

`internal/scoutengine` production code imports only stdlib, `internal/hubgeometry`, `internal/lock`, `internal/proc`, `internal/logger`, and `gopkg.in/yaml.v3` — no `internal/output`, `cobra`, or `internal/*cli`. Returns typed `(T, error)`, never touches `io.Writer`/exit codes/the output envelope; `internal/scoutcli` maps engine results into that envelope.

- `scoutcli` → `scoutengine` is the only allowed direction.
- **Enforced by** `internal/scoutengine/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Pattern Leaf Invariant

`internal/pattern` production code imports only stdlib and `internal/hubgeometry` — never `builderengine`, `websterengine`, `burlerengine`, `loomengine`, or any other feature package. Reverse import never allowed.

- **Enforced by** `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.** Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
- **Registration.** A new module is wired into `newRoot()`: import, `root.AddCommand(...)`, and appended to the root `Long` module-list. Unregistered ⇒ invisible to `--help`.
- **`Short` on every command** (parent + sub), non-empty. Self-discoverable commands also carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.** When a change alters observable behaviour, the reviewer must re-check every affected `Short`/`Long`.
- **Errors are JSON**, via the `internal/output` envelope (`output.Ok`/`output.Err`), one JSON object per line, through `clihelp.Execute`/root seam. No bare plain-text error paths. Parent groups set `RunE = clihelp.GroupRunE`.
- **Interactive-handoff exception (narrow, per-command).** A subcommand that hands stdio to another interactive program and blocks, or self-displays and then blocks forever, is exempt from the envelope only on that terminal-handover/keepalive tail — everything that can fail stays pre-flight, on the envelope.
- **Package naming.** A cobra-registered package is `<module>cli`; its domain kernel is `<module>engine`. cli imports engine; engine never imports cli or cobra. Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine. Skip the engine only for trivial wrappers or a throwaway proof-of-concept meant to be deleted.
- **Enforced by** `cmd/lyx/drift_test.go`, `helptree_test.go`, `registration_test.go`, `longlist_test.go`.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- `internal/shuttleengine` and `internal/reedengine` stay provider-invariant: they define the `Engine` interface (and, for reed, the opaque `cmd`/`resumeCmd`/strand contract) and never reference Claude specifics.
- `internal/shuttleengine` never imports `internal/shuttleengine/claudeengine` — the reverse import only. Wiring a concrete engine happens in `internal/shuttlecli`.
- **Enforced by** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) for the import-graph half; no Claude-specific leakage outside `claudeengine` is a review obligation.

## Shell Mechanics Seam

Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.

- `internal/shell` defines the provider-invariant `Shell` interface (`Quote`/`Invoke`/`ReadFile`) with `pwsh` and `posix` implementations, selected via `shell.ForGOOS()`. Stdlib-only, no Claude specifics.
- `internal/shuttleengine/claudeengine` (and any future provider engine) never emits raw pwsh/posix shell syntax directly — only via `internal/shell`.
- **Enforced by** review obligation today (candidate future grep guard).

## Fabric Git Invariant (warp + weft)

Every git operation that LYX/LoomYard's own code performs — on **either** the weft repo or the warp/host repo — goes through `internal/fabricengine` in Go, in-process, never raw git and never an LLM agent. This binds LYX's own code only; a human or any tool outside LYX keeps ordinary git in their warp worktree, untouched.

- **Module ownership.** Weft-internal git (`commit`/`push`/`pull`/`sync`) and coordinated host↔weft topology (checkout, dual-worktree add/remove/clone) both go through `internal/fabricengine`. The same holds for warp: no LYX package other than `internal/fabricengine` runs raw git against warp. Read-only verbs (current SHA, `git status --porcelain`) are exempt — only *mutating* warp git must dispatch through fabric; see `fabric-unified-view.md`'s "Scope boundary" section for the current warp-mutation call sites.
- **Orchestration, not agent.** The weft commit is Go calling the engine in-process at a round/phase boundary the loop owner (loom, or perch's CLI standalone) controls — never an LLM agent, not raw git, not by shelling `lyx fabric`. Agents ride the file contract: they **write** overlay files into `_lyx`/`_raddle` via the junction; Go **reads and commits** them. An agent does commit its own code to the **host** repo (commit-per-fix) — the weft, never. **Board carve-out:** `internal/boardengine`'s writes to `weft:main` are the one exception to timing control — any LLM session, in any worktree, may trigger a board write at any time — but module ownership still holds (board's git flows through `Bolt`, never raw git); only the *timing*-control half is scoped away.
- **Cross-module exclusions.** The `_lyx` tree is shared by every round-loop module, so every weft-commit caller passes a **positive-only** file list — no `:(exclude)` pathspec magic — built via `fabricengine.ScopedPathspec`. Machine-local artifacts (pause flags, rendered fork prompts, every module's `*.lock` files) are excluded **solely at the git-exclude layer** (`fabricengine.seedWeftArtifactExcludes`), never per-call. **Known limitation:** this stops new pollution but does not untrack an artifact a pre-fix sync already committed — `git rm --cached <path>` is the manual remedy.
- **Enforced by** review obligation: agent prompt templates never instruct a weft git op. Module ownership is machine-checked for `internal/boardengine` (`cmd/lyx/boardguard_test.go`) and for `internal/websterengine`/`internal/builderengine` (`cmd/lyx/rawgitmutation_test.go`, `TestNoRawGitMutation_WebsterBuilderProductionSource`); every other `fabricengine` caller remains a review obligation. The agent half is partly machine-checked for webster runs by `websterengine`'s `weftReferencePattern` (a fork or Master Bash command matching `lyx fabric` is a hard, round-failing violation).

## Review Round Invariant

One review+fix round (burler now, hardener later) follows: A-before-B (review fully written to disk before any target file is touched); every recorded finding is fixed in B, all severities including LOW/NIT; no self-grading (round N's fix is judged by round N+1's fresh review, never its own); commit-per-fix on host source, never push. In a cluster round, fork reports, the handler's own holistic review, and the consolidation into one review file are ALL part of A; fork reviewers are read-only (no writes, no git), mechanically enforced by the fork audit.

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`). No-self-grading and commit-per-fix discipline are review obligations, not machine-checked.

## Live-Substrate Spawn Observability

Any code path that starts a real OS process on behalf of a round/strand/session (a tmux server, a provider session, any subprocess) logs the spawn and its teardown via `internal/logger` — `logger.Info` for normal spawn/teardown events, `logger.Warn` for a retry or a teardown that did not confirm clean. The durable Info+ trace-file sink captures these regardless of verbosity or env-var configuration (under `go test`, gated by `LYX_TRACE=1`).

- A new spawn point for a live-substrate module must add its own `logger.Info`/`Warn` call in the same change — review obligation, not machine-enforced.
- A spawned pane/child must never re-exec `os.Executable()` while running under `go test`: a Go test binary invoked with positional arguments does not error on them, so the arguments are silently ignored and the full suite runs unfiltered. Guarded by `reedengine`'s `headerLaunchLine` (suppresses header re-exec when `testing.Testing()`) and `lyxtest.HermeticGitEnv` (`refuseCLIReexec` refuses any test binary invoked with a leading positional argument).
- A retry loop around a real process spawn must cap attempt COUNT, not only elapsed time — a fast-failing spawn burns a time-only budget in far more attempts than it was sized for. `maxBootAttempts` in `internal/reedengine/lifecycle.go` is the pattern: track an attempt counter, exit on whichever of (time, count) is hit first.
- Known instrumented call sites: `internal/reedengine/lifecycle.go`, `internal/shuttleengine/run.go`, `internal/burlerengine/engine.go`, `internal/scoutengine/ensureserver.go`.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be explicitly excluded with a reason.

- **Tagging.** A scenario in any suite file (`tools/sandbox/*SUITE.md`) that drives a specific module declares it with a `**Covers:** <module>[, <module>...]` line. Coverage is checked at module granularity against the live cobra root (`newRoot().Commands()`, skipping `help`/`completion`).
- **Allowlist.** Modules intentionally never sandbox-exercised are named on `excludedModules` with a one-line reason: `ide`, `selfreport`, `scout` today.
- **Exists ⇒ covered or excluded.** A new registered module needs either a `**Covers:**` scenario or a new allowlist entry with a reason.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go` (`TestSandboxCoverage_AllModulesCoveredOrExcluded`).

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init`/`git worktree add`/fixture-tree copies; Tier 1 stays offline and fast.

- A test file whose first non-empty line is not a `//go:build` constraint mentioning `integration`, `smoke`, or `scout` is "untagged" and must not call `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*`. Raw substring match — a comment or string-literal mention also trips it.
- Substrate definition (real git/tmux/filesystem/cross-compile/external-binary spawn) lives in `docs/benchmarks/running-tests.md`'s "## The two tiers" section.
- Allowlist: `internal/proc` (its tests must spawn), `cmd/lyx/tierpurity_test.go` itself (carries the banned tokens as test data).
- Additive real-time-wait guard: an untagged file's `time.Sleep(...)` with a compile-time-constant duration ≥ 1s is flagged unless allowlisted (`allowedLongSleepers` in `cmd/lyx/tiersleep_test.go`); an unresolvable duration expression is conservatively flagged too.
- **Enforced by** `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git — directly or via lyxtest fixture helpers — runs under the hermetic git test environment, so no test behaviour depends on the operator's `~/.gitconfig` or the system gitconfig.

- A package is "git-spawning" when any `*_test.go` file spawns git directly (`gitexec.RunGit`, `exec.Command`/`exec.CommandContext`) or indirectly via a lyxtest fixture helper (`lyxtest.Copy*`, `lyxtest.MustRun`, `lyxtest.SeedConfig`). Every such package must have a `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`, or be allowlisted.
- Allowlist: `internal/proc` (spawns non-git processes).
- **Enforced by** `cmd/lyx/hermeticenv_test.go` (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) — proves presence of the call only; a real, correctly-ordered `TestMain` is a review obligation.

## Dev/Prod Binary Separation

The sandbox tooling resolves the dev binary from the derived `.dev-bin` (falling back to PATH) through `resolveLyx`, never a bare-PATH `lyx` lookup that could silently resolve prod.

- `resolveLyx` (`tools/sandbox/resolve.go`) is the single allowlisted resolution site: checks `.dev-bin/lyx` first, falls back to `lookPath("lyx")`. Covers both `lookPath("lyx")` and the separator-free `exec.Command("lyx", …)`/`exec.CommandContext("lyx", …)` form.
- The dev binary (`tools/deploy -dev`) builds into `<repoRoot>/.dev-bin` (gitignored), never the production install location.
- `.dev-bin` is prepended only to the agent child-process PATH (`launchAgent`), never the operator's own PATH.
- **Enforced by** `tools/sandbox/pathresolve_guard_test.go` (`TestPathResolveGuard_NoBarePathLyxOutsideResolve`) for the mechanical half; agent-only PATH prepend and never-installed-to-prod are review obligations.

## Planparser Sole-Parser Invariant

`internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`).

- No other package parses `00-overview.md`/`NN-<card-slug>.md`; consumers read plan-level sections only from the `planparser.Plan` model a caller hands in.
- Resolves `_lyx/plan/` via `hubgeometry`, never string literals.
- **Enforced by** review obligation today (candidate future import/grep guard).

## Batcher Registry+Config Invariant

webster's execution unit is the batchifier-derived batch, not the raw plan card.

- Batching is selected by `internal/batcher`'s name-keyed registry plus the `batcher:` webster.yaml config key (default `identity`) — no plan-supplied batching, no batch grouping in the plan format itself.
- **Enforced by** review obligation.

## GitHub Auth Invariant

All GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`.

- Token resolution, token caching, and construction of an authenticated `*github.Client` live solely in `internal/githubclient`. No other production package invokes `gh` (via `exec.Command`/`exec.CommandContext` or bare `LookPath("gh")`) or otherwise builds its own GitHub credential path.
- `internal/githubclient` production imports are allowlisted to stdlib, `go-github`, `golang.org/x/sys`, and `internal/proc`.
- **Enforced by** `cmd/lyx/ghguard_test.go` (`TestGHGuard_NoShellOutOutsideGithubclient`) and `internal/githubclient/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## gitrepo Client Boundary Invariant

`internal/gitrepo` splits local-vs-remote by client: go-git owns local object and ref access, `gitexec` owns anything that authenticates to a remote or mutates the working tree.

- go-git handles reads that resolve state already on disk — commit/tree/blob lookups and ref reads. `gitexec` is the only path to the git CLI, used for `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `Fetch`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `IsAncestor`, `HasUnpushed`. Any new `gitexec` call added inside `internal/gitrepo` must update this list in the same commit.
- Known guard blind spot: the check is set-equality on method names, so a new `r.run` call slipped inside an already-pinned method is not caught — per-call review still applies to those methods.
- **Enforced by** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`).

## Never Force-Add Invariant

Fabric/gitrepo never runs `git add -f`.

Transients are kept out of the index by each repo's own `.git/info/exclude` (warp: `seedGitExclude`; weft: `seedWeftArtifactExcludes`), never by force-adding past them and never by per-call `:(exclude)` pathspec magic.

This is enforced structurally — `gitrepo.StageAndCommit` has no `-f` code path at all — plus a machine-checked grep guard against its reintroduction.

- **Enforced by** `internal/gitrepo/noforceadd_test.go` (`TestNoForceAdd_GitrepoSourceHasNoForceAddBranch`).

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=281)

- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/00-overview.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/01-pre-moves.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/02-rename-and-reshape.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/03-production-sweep.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/04-test-sweep.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/05-module-owned-constructors.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/06-fabric-owns-the-illusion.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/07-board-junction.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/08-guard-and-docs.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/CONSTRAINTS.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/leaf_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/fabric.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/branchname.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/branchname_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/clone.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/clone_adopt_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/geometry_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/lyxtest.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/audit.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/audit_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/configengine.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/envsource.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/exitcode_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/main_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/boardtest/bench_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/smoke_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/menu.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/reconcile_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/edit.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/edit_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/set.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/set_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configsync/configsync.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configsync/configsync_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/envsource/envsource.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/commit_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config_driven_junctions_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_pattern_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile_stale_registration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile_stale_removal_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/unwire.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/unwire_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftgit.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/menu.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/menu_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/preflight_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/lyxtest_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/leaf_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/load.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/load_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/modelspec.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/template_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/run.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/run_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/contract_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/leaf_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/load.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/load_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/weft.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/weft_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/config_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelist.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/fabric.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/topology.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_rollback_adopt_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout_index_refresh_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout_rollback_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/cleanup.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/list.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/prune.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/status.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftwiring.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry_unit_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/idecli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/spawn.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/spawn_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/preflight.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/tokenvocab.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/tokenvocab_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/vscode/color.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/vscode/color_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junctionnames.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hook_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hostlayout.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/remove.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/pattern_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/weft_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/siblinglayout_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/discussion.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hook.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftgit_exclude_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/clone.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/unwire.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/weft_verbs.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/commit.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/drift.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hostclean.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/launchers.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/portals.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/pull.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/beginbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/recordbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/recoverbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/run.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/validate.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/beginbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recordbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recoverbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/render.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/runlevel.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/poll.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/run.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/spawnbatch.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/validate.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/spawn.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlercli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/engine.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/discussion.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/plan.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/planparser/parse.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/sink.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/pattern.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/engine.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lock.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutcli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/cli.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/rundir.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/wait.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_branch_exists_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_repoint_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junctionnames_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/pull_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/remove_junctions_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/warpforward_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftwiring_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/smoke_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/verbs_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/beginbatch_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recordbatch_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recoverbatch_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/runlevel_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/template_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/pause_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/poll_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/run_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/spawnbatch_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/status_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/testdata_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/spawn_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/engine_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/smoke_cluster_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/smoke_round_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/discussion_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/plan_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/smoke_judge_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/cli_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/run_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/cli_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/smoke_lifecycle_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/header_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lock_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/mouse_boot_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/refs_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_scout_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/smoke_interrupt_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run_inject_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/rundir_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/wait_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/notes_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/leaf_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/pattern_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/leaf_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/overview.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/fabric-unified-view.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/registration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/unknown_subcommand_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/boardtest/bench_cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/plan.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/state.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/prompt.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/reconcile_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/snapshot_integration_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/gitrepo/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/idecli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/retention.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/sink_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/identity_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/server.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutcli/cli_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/toolchain.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/engine.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/seam_enforcement_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/state.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/report.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/daemonstate.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/logger.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lifecycle.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/doc.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/spawn.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/spawn_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/strand.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/strand_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/config.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/template_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fslink/fslink.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/main.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/README.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/plan-format.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/builder-contract.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/discussion-format.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/status-schema.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/model-spec.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/loom.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/pattern.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/roadmap.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/webstergeom_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/scoutdaemon_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelogs_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/raddle_guard_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/discussionpath_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/loomstatus_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/anchor.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/anchor_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/hubgeometry.md
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/testmain_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/planpath_test.go
- /home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelist_test.go

## Plan files to review
- Overview: `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/00-overview.md`
- Batches:
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/01-pre-moves.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/02-rename-and-reshape.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/03-production-sweep.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/04-test-sweep.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/05-module-owned-constructors.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/06-fabric-owns-the-illusion.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/07-board-junction.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/plan/08-guard-and-docs.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/leaf_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/fabric.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/branchname.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/branchname_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/clone.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/clone_adopt_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/geometry_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/lyxtest.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/audit.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/audit_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/configengine.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/envsource.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/exitcode_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/main_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/boardtest/bench_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/smoke_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/configcli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/menu.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/reconcile_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/edit.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/edit_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/set.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configengine/set_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configsync/configsync.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configsync/configsync_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/envsource/envsource.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/commit_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config_driven_junctions_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_pattern_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile_stale_registration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/reconcile_stale_removal_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/unwire.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/unwire_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftgit.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/menu.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/menu_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/preflight_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/lyxtest_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/leaf_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/load.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/load_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/modelspec.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/modelspec/template_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/run.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/run_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/contract_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/leaf_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/load.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/load_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/weft.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/weft_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/config_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelist.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/fabric.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/topology.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_rollback_adopt_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout_index_refresh_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout_rollback_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/cleanup.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/list.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/prune.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/status.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftwiring.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/hubgeometry_unit_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/idecli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/spawn.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/ideengine/spawn_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/preflight.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/tokenvocab.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/tokenvocab_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/vscode/color.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/vscode/color_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junctionnames.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hook_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hostlayout.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/remove.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/pattern_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/weft_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/siblinglayout_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/discussion.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hook.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftgit_exclude_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/clone.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/unwire.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabriccli/weft_verbs.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/checkout.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/commit.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/drift.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/hostclean.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/launchers.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/portals.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/pull.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/beginbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/recordbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/recoverbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/run.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/validate.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/beginbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recordbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recoverbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/render.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/runlevel.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/poll.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/run.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/spawnbatch.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/validate.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/spawn.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlercli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/engine.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/discussion.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/plan.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/planparser/parse.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/sink.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/pattern.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/engine.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lock.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutcli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/cli.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/rundir.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/wait.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/add_branch_exists_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_repoint_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junction_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/junctionnames_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/pull_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/remove_junctions_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/warpforward_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/weftwiring_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/smoke_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/webstercli/verbs_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/beginbatch_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recordbatch_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/recoverbatch_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/runlevel_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/template_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/pause_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/poll_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/run_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/spawnbatch_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/status_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/testdata_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/buildercli/weft_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/spawn_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/engine_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/smoke_cluster_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/smoke_round_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/discussion_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/plan_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/smoke_judge_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchcli/cli_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/run_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/cli_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedcli/smoke_lifecycle_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/header_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lock_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/mouse_boot_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/ensureserver_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/refs_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_scout_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/supervised_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttlecli/smoke_interrupt_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run_inject_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/run_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/rundir_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/shuttleengine/wait_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardcli/notes_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/leaf_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/pattern_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/leaf_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/overview.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/fabric-unified-view.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/registration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/unknown_subcommand_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/boardtest/bench_cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/plan.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/state.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/prompt.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/configcli/reconcile_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/snapshot_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/gitrepo/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/idecli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/retention.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/sink_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/lyxtest/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/perchengine/identity_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/server.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutcli/cli_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/toolchain.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/tokenvocab/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/engine.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/treadleengine/seam_enforcement_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/state.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/loomengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/builderengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/websterengine/report.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/scoutengine/daemonstate.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/pattern/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/logger/logger.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/lifecycle.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/burlerengine/doc.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/spawn.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/spawn_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/strand.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/reedengine/strand_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fabricengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/config.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/boardengine/template_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/fslink/fslink.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/cmd/lyx/main.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/README.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/plan-format.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/builder-contract.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/discussion-format.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/status-schema.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/reference/model-spec.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/loom.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/designs/pattern.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/manifest/roadmap.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/webstergeom_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/scoutdaemon_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelogs_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/raddle_guard_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/discussionpath_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/loomstatus_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/anchor.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/anchor_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/docs/shared-libs/hubgeometry.md`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/testmain_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/planpath_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-illusion-core/internal/hubgeometry/worktreelist_test.go`

## Intentionally deleted (N=1)

- internal/hubgeometry/siblinglayout_test.go

## Source-grounding rule

**Never guess.** A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt. Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list. If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan; do not emit NEED_CONTEXT for files in the manifest. Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path). The orchestrator will re-fire the review with those files added. Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the plan as a whole)

- **Constraint violations** — BLOCKING.
- **Alignment** — plan covers all task requirements.
- **Decision alignment** — every `### Decision:` in `## Shared Decisions` faithfully implemented.
- **Completeness** — every card has `Creates`/`Edits`, `Context`, `Moves`, `Requirements`, `Commit`.
- **Moves well-formed** — each `Moves:` sub-bullet is an `` `old` -> `new` `` pair (backtick-wrapped paths, ASCII ` -> ` arrow); bare `none` on the label line is valid; any other format is a finding.
- **Rename mechanic present** — any batch whose cards contain a non-empty `Moves:` must include a `## Rename mechanic` section describing the `git mv` + surgical-edit approach; absence is a finding.
- **No full-file rewrites of relocated files** — prescribing a write-from-scratch for a file that appears in `Moves:` (rather than `git mv` + surgical edits) is a finding.
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
- **All Files Touched scope** — the overview's `## All Files Touched` section lists the union of `Edits:`/`Creates:`/Move-target paths across all batches; `Deletes:` tokens and Move-source paths are excluded by convention. A Deletes-only or Move-source-only path missing from that list is correct, not a finding.
- **Platform-behavior-claim verification** — BLOCKING if a plan or discussion claim describes Claude Code's own platform/harness behavior (e.g. agent auto-discovery, plugin manifest semantics) and a manifest or doc file that could confirm or refute the claim is present in your context, bulked or Read-able, but the claim was accepted without checking that file. Tool-use-mode reviewers may Read `plugin.json`/platform docs directly even when not bulked.

Independently state, in the `reviewer_self_id:` field below, what model/version you believe yourself to be — this is your own best-effort assessment, distinct from the `reviewer_model:` value already dictated to you above.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** Per finding: 3–5 lines, short and factual. The consumer has full context of the plan; do NOT explain background. Cite the batch/card, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE, ~600–1200 tokens for REQUEST_CHANGES across multiple batches. If you produce more than ~1500 tokens, compress.

```
MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7) — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: fablehigh
reviewer_self_id: <your own model self-identification, if known>
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

**Severity vocabulary is closed.** Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word (e.g. `MAJOR`, `MINOR`, `CRITICAL`, `MEDIUM`, `HIGH`). If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT` -- an over-cautious BLOCKING can be pushed back on by the orchestrator; a mislabeled NIT (or an unrecognized label) can silently skip review entirely.

Omit `## Findings` if zero findings. Never invent findings to pad.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-plan-holistic-r4.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-plan-holistic-r4.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-plan-holistic-r4.out.md`
