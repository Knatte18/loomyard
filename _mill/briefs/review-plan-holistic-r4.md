**If you find issues, REPORT them — do NOT fix them.**

You are an independent plan reviewer for **webster: stop re-rendering already-inherited context into fork prompts**. You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **sonnetxhigh**. Round **4**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Constraints
# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly machine-enforced (named test, fails `go test`/CI) and partly a review obligation. This file states rules only — no rationale, no incident narratives, no historical justification. Fuller design/how-to lives in godoc and `docs/`.

## Hub Geometry Invariant

`internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.

- All cwd/worktree-root queries go through `hubgeometry.Getwd()`/`Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`.
- Geometry tokens (`_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern`) are owned solely by `internal/hubgeometry`; no other package may use them in a path-construction context (`filepath.Join` arg, `+` operand, or string `const`).
- `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through `hubgeometry.LyxDirName`/`ConfigDir(base)`/`ConfigFile(base, module)` — in test code too.
- Geometry is structural, never config/env-overridable.
- The weft-backed junction name-set is injected from fabric config, not enumerated in `hubgeometry`; `hubgeometry` exposes it via `HubReservedNames() []string` and takes name-sets as explicit `[]string` params (`HostJunctions`, `HostJunctionsHere`, `IsReservedHubName`).
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

- **Module ownership.** Weft-internal git (`commit`/`push`/`pull`/`sync`) goes through `internal/fabricengine`; coordinated host↔weft topology (a checkout that moves both and re-points junctions, dual-worktree add/remove/clone) goes through `internal/fabricengine` too. No other package runs raw git against a weft worktree — **and the same holds for warp**: the host/warp repo is unrestricted only for external actors (a human, or any tool outside LYX, has an ordinary project repo to work in); no LYX package other than `internal/fabricengine` runs raw git against warp either, regardless of purpose (verification, bisect, etc.). Read-only verbs (e.g. reading current SHA, `git status --porcelain`) are exempt from this — only *mutating* warp git needs to dispatch through fabric, per `fabric-unified-view.md`'s "Scope boundary" section. **Gap closed:** both previously-tracked instances — `internal/websterengine`'s bisect/verify path (`CheckoutDetached`/`RestoreBranch` in `integration.go`) and `internal/builderengine`'s `ResetHard` chain-rollback path (`RestartChain` in `chain.go`) — now dispatch through `internal/fabricengine`'s warp-only methods (`Fabric.CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard`) via each package's own narrow consumer-side interface (`WarpBisector`, `WarpResetter`). Both packages' mutating-git non-bypass is now machine-checked (see the Enforced-by bullet below).
- **Orchestration, not agent.** The weft commit is Go calling the engine in-process (`fabricengine`'s `SyncWeft`/`CommitWeft`) at a round/phase boundary the loop owner (loom, or perch's CLI standalone) controls. An LLM agent never drives weft git — not raw git, not by shelling `lyx fabric`. Agents ride the file contract: they **write** overlay files (reviews, fixer-reports, status, raddle docs) into `_lyx`/`_raddle` via the junction; Go **reads and commits** them. Asymmetry: an agent **does** commit its own code to the **host** repo (commit-per-fix, normal dev git) — the weft, never. **Board carve-out.** `internal/boardengine`'s writes to `weft:main` are the one deliberate exception to timing control living with the loop owner: any LLM session, in any worktree, may trigger a board write (via `lyx board <verb>`) at any time — that is the whole point of board's shared-visibility model. Module ownership still holds without exception (board's git flows through `fabricengine.CommitWeftAt`/`PushWeftAt`, never raw git); only the *timing*-control half is scoped away from board. `Fabric.Commit` (`internal/fabricengine/commit.go`) has landed as a Go API called by the orchestration layer — a classify-and-dispatch two-sided commit, never invoked by an LLM agent. That is deliberate policy, not a code guard: unlike the old weft-write path, where an agent's `git add <weft-file>` simply failed because the weft file wasn't tracked in warp, `Fabric.Commit` makes weft writes clean, so the accidental guardrail this bullet used to describe no longer exists — nothing in `Fabric.Commit` itself refuses an agent-driven call, and the old accidental "`git add` fails" guardrail is intentionally not reintroduced. The invariant holds anyway because nothing in the orchestration stack (loom, perch's CLI standalone) ever hands an LLM agent a path to `Fabric.Commit`; module ownership is satisfied because the weft side of `Fabric.Commit` routes through the same `commitWeftLocked`/`CommitWeft` machinery every other weft-git verb uses, under the fabric-layer write lock, never raw git.
- **Why.** A weft commit is an orchestration act (persist round/phase state at the right boundary, coordinate host↔weft) — the deterministic Go responsibility that is the whole lyx thesis. An agent-run weft commit reintroduces the non-deterministic, untestable, mis-ordered LLM orchestration lyx exists to remove.
- **Anchored exclusions.** A caller that passes `CommitWeft` a pathspec with `:(exclude)` entries must **anchor every exclusion under the same scoped `_lyx` base the positive entry names**, forward-slash spelled — `":(exclude)" + base + "/*.lock"`, never `":(exclude)*.lock"`. Git classifies a leading-`*` pattern with no further wildcard as a one-star pathspec, which false-positive-matches the intermediate directories it must descend through to reach a multi-segment positive pathspec: at a `layout.RelPath` of two or more segments the whole subtree is pruned, `git add` stages nothing, and the weft commit becomes a **silent no-op with no error**. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both anchored, with real-git depth coverage in each package's `weft_integration_test.go`) and `internal/perchcli`'s block-exit commit (**still unanchored** — carries this bug, owned by perch; the Cross-module exclusions bullet's git-exclude backstop happens to keep perch's commit from tracking the *standard* machine-local artifact set anyway, but perchcli's own pathspec is still wrong and should be fixed on its own terms, not relied on to stay correct only by that backstop). A slice-shape unit test cannot see this; only a real-git assertion can.
- **Cross-module exclusions.** The `_lyx` tree is **shared** by every round-loop module in a worktree, so a module's weft commit stages whatever the *other* modules happen to have left on disk. A caller's exclusion set must therefore name **every** module's machine-local artifacts, not just its own — today that is builder's and webster's pause flags plus webster's rendered fork prompts (`_lyx/webster/prompts/*`); the `<base>/*.lock` entry is already cross-module because a git pathspec `*` crosses `/`. Excluding only your own is not a cosmetic miss: once another module's flag is tracked, **its owner can never stage its deletion** (that module's own exclusion hides the path from `git add`), so it is pinned in weft `HEAD`, pushed, and materialized by every other machine's weft pull as a pause request nobody made. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both cross-module, with real-git coverage in each package's `weft_integration_test.go`). **Fixed at the git-exclude layer, not the pathspec layer:** `fabricengine.seedWeftArtifactExcludes` (called from `CommitWeft`'s `ensureWeftLockDir`, the single choke point every weft-git verb passes through) seeds `crossModuleMachineLocalExcludes` — gitignore-syntax patterns (`**/_lyx/*/*.lock`, `**/_lyx/*/pause`, `**/_lyx/*/prompts/`, module name wildcarded) — into the weft repo's `.git/info/exclude`. This makes **every** committer correct by construction, including `lyx fabric commit|push|sync`'s own pathspec (`internal/fabriccli/weft_verbs.go`, still positive-entries-only) and `internal/perchcli`'s still-unanchored block-exit commit (see the Anchored exclusions bullet above) — neither needs its own exclusion logic, since git itself now refuses to stage these paths regardless of what pathspec asked for them. Because gitignore glob semantics differ from git pathspec magic (a bare `*` here does **not** cross `/`), the `**/` prefix alone reaches every `RelPath` depth with no per-caller anchoring needed — the anchoring problem the "Anchored exclusions" bullet above describes is a pathspec-only failure mode, and does not apply to this exclude-file mechanism. **Known limitation:** this stops new pollution but does not untrack an artifact a pre-fix `lyx fabric sync` already committed on an existing hub — `.git/info/exclude` only affects untracked status. `git rm --cached <path>` (or a fresh `lyx fabric sync` after manually removing the tracked entries) is the manual remedy on an already-polluted hub; no automated migration tool was added.
- **Enforced by** review obligation: agent prompt templates never instruct a weft git op, and weft git stays inside `internal/fabricengine`. The module-ownership half is machine-checked for `internal/boardengine` specifically by `cmd/lyx/boardguard_test.go` (no raw `gitrepo`/`gitexec` import or shell-out); it is also machine-checked for `internal/websterengine`/`internal/builderengine` specifically by `cmd/lyx/rawgitmutation_test.go` (`TestNoRawGitMutation_WebsterBuilderProductionSource`), which bans `gitrepo.New(`/`gitexec.RunGit(` in both packages' production source, file-allowlisting `gitwrap.go`'s and `gitquery.go`'s grandfathered read-only exemptions; the general case (every OTHER `fabricengine` caller) remains a review obligation, not machine-checked today. The agent half is partly machine-checked for webster runs by `websterengine`'s `weftReferencePattern` (a fork or Master Bash command matching `lyx fabric` — or a weft worktree path — is a hard, round-failing `weft-reference` violation).

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

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=18)

- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/00-overview.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/01-planparser-card-source-identity.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/02-webster-prompt-split.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/hubgeometry/hubgeometry.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/CONSTRAINTS.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/plan.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse_test.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/fork-template.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/integration-template.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/master-template.md
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/stencil/stencil.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/pattern/pattern.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/render.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/recoverbatch.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/beginbatch.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/doc.go
- /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/template_test.go

## Plan files to review
- Overview: `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/00-overview.md`
- Batches:
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/01-planparser-card-source-identity.md`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/plan/02-webster-prompt-split.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/hubgeometry/hubgeometry.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/plan.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/planparser/parse_test.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/fork-template.md`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/integration-template.md`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/master-template.md`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/stencil/stencil.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/pattern/pattern.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/render.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/recoverbatch.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/beginbatch.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/doc.go`
- `/home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/internal/websterengine/template_test.go`

## Intentionally deleted (N=1)

- internal/websterengine/fork-template.md

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
# Review: webster: stop re-rendering already-inherited context into fork prompts — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnetxhigh
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

Write your full report to this file: /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/briefs/review-plan-holistic-r4.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/briefs/review-plan-holistic-r4.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/webster-fork-context-hygiene/_mill/briefs/review-plan-holistic-r4.out.md`
