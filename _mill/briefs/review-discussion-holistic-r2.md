**If you find issues, REPORT them — do NOT fix them.**

You are an independent discussion reviewer for **codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)**. Round **2**. Reviewer model: **sonnetmax**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

---

## Task

Read the discussion at `/home/knatte/Code/loomyard/wts/codeintel-v1/_mill/discussion.md`. The discussion file is the authoritative scope. Read files referenced in `## Technical Context` to verify claims.

Constraints:
# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly machine-enforced (named test, fails `go test` / CI) and partly a review obligation. Fuller design/how-to lives in godoc and `docs/`, not here — this file is the index.

## Hub Geometry Invariant

`internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.

- All cwd / worktree-root queries go through `hubgeometry.Getwd()` / `Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`.
- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern` — are owned solely by `internal/hubgeometry`. No other package may use them in a path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`). Whole-token match; production files only; comparisons and git-pathspec slice literals are not path construction and stay allowed.
- `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through `hubgeometry.LyxDirName` / `ConfigDir(base)` / `ConfigFile(base, module)` — **in test code too** (a config-layout migration once broke a hardcoded test fixture).
- Geometry is structural, never config/env-overridable (the board dir is `--board-path` flag > `hubgeometry.BoardDir(l.Hub)`, not a config key).
- **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`. API and helpers: godoc for `internal/hubgeometry`.

## lyxtest Leaf Invariant

`internal/lyxtest` stays a leaf: it imports only the standard library and `internal/hubgeometry` — never `internal/configreg` or any feature package (`boardengine`/`boardcli`, …).

- A `lyxtest → configreg → feature` edge closes a test-build cycle under `-tags integration`. Tests needing real config call `lyxtest.SeedConfig(tb, dir, map[string]string{...})`; the `configreg`→map conversion happens at the test site, in a package that may legally import `configreg`.
- **Enforced by** `internal/lyxtest/leaf_enforcement_test.go` on every `go test`.

## Modelspec Leaf Invariant

`internal/modelspec` production code imports only stdlib, `internal/hubgeometry`, and `gopkg.in/yaml.v3` — so every future consumer (builder, perch/burler/loom configs) can import it without cycles.

- `configreg` → `modelspec` is the allowed direction (for `modelspec.ConfigTemplate`); the reverse import (`modelspec` → `configreg` or any feature package) is never allowed.
- **Enforced by** `internal/modelspec/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## Treadle Runner-Seam Invariant

`internal/treadleengine` never imports `internal/burlerengine` or any `internal/*cli` package; round runners adapt onto treadle's `RoundRunner` vocabulary in their own packages.

- The generalized round-loop engine's whole purpose is decoupling the loop from any one round-runner's types (`burlerengine` today, a future behavior-review runner); importing a runner's package back into treadle would defeat the seam it exists to provide. A type genuinely needed by both is extracted out of burler into shared ground, never imported downward — `internal/perchengine`'s `buildRoundProfile`/adapter own the burler-specific mapping instead.
- Import allowlist: stdlib, `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/shuttleengine`, and `gopkg.in/yaml.v3` — deliberately NOT `internal/hubgeometry`: the engine is geometry-blind (caller-supplied absolute `runDir`/`GateDir`), matching the Hub Geometry Invariant's carve-out for packages that never construct `_lyx` paths themselves.
- **Enforced by** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`) on every `go test`.

## Tokenvocab Leaf Invariant

`internal/tokenvocab` production code imports only stdlib, `internal/hubgeometry`, and `internal/stencil` — so every future consumer (reed's header pipeline, loom's prompt templates) can import it without cycles.

- The reverse import (`tokenvocab` → `reed`, `tokenvocab` → `loom`, or any other feature package) is never allowed.
- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## Codeintelengine Leaf Invariant

`internal/codeintelengine` production code imports only stdlib, `internal/hubgeometry`, and `gopkg.in/yaml.v3` — no `internal/output`, no `cobra`, no `internal/*cli` package — so the engine stays a cycle-free leaf importable by builder/webster later, exactly like `internal/modelspec`'s leaf excludes `output`. The engine returns typed Go errors and typed result values (`(T, error)`) and never touches `io.Writer`, exit codes, or the `output.Ok`/`output.Err` envelope; `internal/codeintelcli` is the sole layer that maps engine errors/results into that envelope.

- `codeintelcli` → `codeintelengine` is the only allowed direction; the reverse import (`codeintelengine` → `codeintelcli`, or `codeintelengine` → any other feature package) is never allowed.
- **Enforced by** `internal/codeintelengine/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## Pattern Leaf Invariant

`internal/pattern` production code imports only stdlib and `internal/hubgeometry` — never `builderengine`, `websterengine`, `burlerengine`, `loomengine`, or any other feature package — so every one of those four consumers can import it without cycles; the reverse import (`pattern` → any feature package) is never allowed.

- **Enforced by** `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.** Each module exposes `Command() *cobra.Command` and a thin `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`. Tests and root both drive the module through this seam.
- **Registration.** A new module is wired into `newRoot()`: import, `root.AddCommand(...)`, and append the module name to the root `Long` module-list. Unregistered ⇒ invisible to `--help`.
- **`Short` on every command** (parent + sub), non-empty. Self-discoverable commands also carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.** Presence of `Short` is machine-checked; prose-vs-behaviour is not. When a change alters observable behaviour, the reviewer must re-read every affected `Short`/`Long` and confirm it matches the code as changed — stale help is a review-blocking defect. Prefer generating mechanical help facts from source (e.g. configcli's `Known modules:` from `configreg.Names()`).
- **Errors are JSON.** Results and errors go through the `internal/output` envelope (`output.Ok` / `output.Err`), one JSON object per line, via the `clihelp.Execute` / root seam (`SilenceErrors = true`). No bare plain-text error paths. Parent groups set `RunE = clihelp.GroupRunE` to reject unknown subcommands.
- **Interactive-handoff exception (narrow, per-command).** A subcommand whose whole job is to hand the operator's stdio to another interactive program and block (`ide menu`'s stdin picker; `reed attach`'s `tmux attach`), or to self-display and then keep a pane alive forever (`reed header --blocking`, the header pane's own print-then-block keepalive tail), cannot emit the JSON envelope on that terminal-handover/keepalive tail. The exception is scoped tightly: everything that can fail runs **pre-flight and stays on the envelope** (`output.Err`, non-zero exit); only the post-handoff/keepalive tail is exempt, and on success it emits no JSON. `reed attach` follows the pre-existing `ide menu` precedent; `reed header --blocking` is a narrower sub-case still — the command's own default mode (no `--blocking`) stays fully on the envelope, and only the flag-gated tail is exempt. See the `internal/reedcli` attach/header commands' godoc/`Long` and [docs/overview.md#modules](docs/overview.md#modules) for the full rationale.
- **Package naming.** A Cobra-registered package is `<module>cli`; its extracted domain kernel is `<module>engine`. cli imports engine; engine never imports cli or cobra. Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine. Skip the engine only for trivial wrappers (`configcli`) or a throwaway proof-of-concept meant to be deleted once it proves its point (e.g. `muxpoccli`, which followed exactly that path — deleted once `reed` shipped); "no consumer today" is not a skip reason. `initcli`/`initengine` follows the standard split (no longer exempt — `lyx init --undo` grew enough core logic that mixing it into the cli package was rot, not simplicity).
- **Enforced by** `cmd/lyx/drift_test.go` (every command has `Short`), `helptree_test.go` (root names every module, module names every subcommand), `registration_test.go` (exists ⇒ registered), `longlist_test.go` (registered ⇒ in `root.Long`). Update the pinned sets in the same commit when adding a module/subcommand.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- CLI flags, the `settings.json` hook schema, TUI startup/trust markers, and pane key choreography are all Claude-specific and stay inside `internal/shuttleengine/claudeengine`. `internal/shuttleengine` and `internal/reedengine` stay provider-invariant: they define the `Engine` interface (and, for reed, the opaque `cmd`/`resumeCmd`/strand contract) and never import or reference Claude specifics.
- `internal/shuttleengine` never imports `internal/shuttleengine/claudeengine` — the reverse import only. Wiring a concrete engine into the run loop happens in `internal/shuttlecli`, which imports both.
- **Enforced by** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) on every `go test`, for the import-graph half of the rule. The semantic half — no Claude marker strings, hook payload shapes, or TUI grammar leaking outside `claudeengine` — is a review obligation, not machine-checked.

## Shell Mechanics Seam

Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.

- `internal/shell` defines the provider-invariant `Shell` interface (`Quote`/`Invoke`/ `ReadFile`) with a `pwsh` and a `posix` implementation, selected at runtime by `shell.ForGOOS()` (or exercised directly via `shell.Pwsh()`/`shell.Posix()`). `internal/shell` stays stdlib-only and provider-invariant: no Claude flags, marker strings, or hook shapes may appear in it, complementing the Shuttle Provider-Seam Invariant above (which keeps those Claude specifics confined to `claudeengine`).
- `internal/shuttleengine/claudeengine` (and any future provider engine) never emits raw pwsh/posix shell syntax directly — it composes its launch/resume command lines only by calling into `internal/shell`.
- **Enforced by** review obligation today: a grep-guard test — e.g. asserting the `Get-Content -Raw` idiom appears only under `internal/shell` — is a cheap future machine-check, deferred per YAGNI.

## Weft Git Invariant

Every git operation on the weft repo goes through `internal/fabricengine` in Go, driven by the orchestration layer in-process — never raw git, and never an LLM agent.

- **Module ownership.** Weft-internal git (`commit`/`push`/`pull`/`sync`) goes through `internal/fabricengine`; coordinated host↔weft topology (a checkout that moves both and re-points junctions, dual-worktree add/remove/clone) goes through `internal/fabricengine` too. No other package runs raw git against a weft worktree. The **host** repo is unrestricted — it is an ordinary project repo.
- **Orchestration, not agent.** The weft commit is Go calling the engine in-process (`fabricengine`'s `SyncWeft`/`CommitWeft`) at a round/phase boundary the loop owner (loom, or perch's CLI standalone) controls. An LLM agent never drives weft git — not raw git, not by shelling `lyx fabric`. Agents ride the file contract: they **write** overlay files (reviews, fixer-reports, status, raddle docs) into `_lyx`/`_raddle` via the junction; Go **reads and commits** them. Asymmetry: an agent **does** commit its own code to the **host** repo (commit-per-fix, normal dev git) — the weft, never.
- **Why.** A weft commit is an orchestration act (persist round/phase state at the right boundary, coordinate host↔weft) — the deterministic Go responsibility that is the whole lyx thesis. An agent-run weft commit reintroduces the non-deterministic, untestable, mis-ordered LLM orchestration lyx exists to remove.
- **Anchored exclusions.** A caller that passes `CommitWeft` a pathspec with `:(exclude)` entries must **anchor every exclusion under the same scoped `_lyx` base the positive entry names**, forward-slash spelled — `":(exclude)" + base + "/*.lock"`, never `":(exclude)*.lock"`. Git classifies a leading-`*` pattern with no further wildcard as a one-star pathspec, which false-positive-matches the intermediate directories it must descend through to reach a multi-segment positive pathspec: at a `layout.RelPath` of two or more segments the whole subtree is pruned, `git add` stages nothing, and the weft commit becomes a **silent no-op with no error**. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both anchored, with real-git depth coverage in each package's `weft_integration_test.go`) and `internal/perchcli`'s block-exit commit (**still unanchored** — carries this bug, owned by perch; the Cross-module exclusions bullet's git-exclude backstop happens to keep perch's commit from tracking the *standard* machine-local artifact set anyway, but perchcli's own pathspec is still wrong and should be fixed on its own terms, not relied on to stay correct only by that backstop). A slice-shape unit test cannot see this; only a real-git assertion can.
- **Cross-module exclusions.** The `_lyx` tree is **shared** by every round-loop module in a worktree, so a module's weft commit stages whatever the *other* modules happen to have left on disk. A caller's exclusion set must therefore name **every** module's machine-local artifacts, not just its own — today that is builder's and webster's pause flags plus webster's rendered fork prompts (`_lyx/webster/prompts/*`); the `<base>/*.lock` entry is already cross-module because a git pathspec `*` crosses `/`. Excluding only your own is not a cosmetic miss: once another module's flag is tracked, **its owner can never stage its deletion** (that module's own exclusion hides the path from `git add`), so it is pinned in weft `HEAD`, pushed, and materialized by every other machine's weft pull as a pause request nobody made. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both cross-module, with real-git coverage in each package's `weft_integration_test.go`). **Fixed at the git-exclude layer, not the pathspec layer:** `fabricengine.seedWeftArtifactExcludes` (called from `CommitWeft`'s `ensureWeftLockDir`, the single choke point every weft-git verb passes through) seeds `crossModuleMachineLocalExcludes` — gitignore-syntax patterns (`**/_lyx/*/*.lock`, `**/_lyx/*/pause`, `**/_lyx/*/prompts/`, module name wildcarded) — into the weft repo's `.git/info/exclude`. This makes **every** committer correct by construction, including `lyx fabric commit|push|sync`'s own pathspec (`internal/fabriccli/weft_verbs.go`, still positive-entries-only) and `internal/perchcli`'s still-unanchored block-exit commit (see the Anchored exclusions bullet above) — neither needs its own exclusion logic, since git itself now refuses to stage these paths regardless of what pathspec asked for them. Because gitignore glob semantics differ from git pathspec magic (a bare `*` here does **not** cross `/`), the `**/` prefix alone reaches every `RelPath` depth with no per-caller anchoring needed — the anchoring problem the "Anchored exclusions" bullet above describes is a pathspec-only failure mode, and does not apply to this exclude-file mechanism. **Known limitation:** this stops new pollution but does not untrack an artifact a pre-fix `lyx fabric sync` already committed on an existing hub — `.git/info/exclude` only affects untracked status. `git rm --cached <path>` (or a fresh `lyx fabric sync` after manually removing the tracked entries) is the manual remedy on an already-polluted hub; no automated migration tool was added.
- **Enforced by** review obligation: agent prompt templates never instruct a weft git op, and weft git stays inside `internal/fabricengine`. The module-ownership half is a candidate for a future import/grep guard; not machine-checked today. The agent half is partly machine-checked for webster runs by `websterengine`'s `weftReferencePattern` (a fork or Master Bash command matching `lyx fabric` — or a weft worktree path — is a hard, round-failing `weft-reference` violation).

## Review Round Invariant

One review+fix round (burler now, hardener later) follows the round discipline: A-before-B (the review is fully written to disk before any target file is touched); every recorded finding is fixed in B, all severities including LOW/NIT; no self-grading (round N's fix is judged by round N+1's fresh review, never its own); commit-per-fix on host source, never push. In a cluster round, the fork reports, the handler's own holistic review, and the consolidation into one review file are ALL part of A — the consolidated review is fully on disk before B touches a single target file, exactly as in a solo round — and fork reviewers are read-only: no writes, no git, mechanically enforced by the fork audit (never advisory).

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline` for the template's sequencing and fix-everything statements, `TestTemplate_StatesClusterForkDiscipline` for the cluster sequencing/read-only statements). The rest — no self-grading, commit-per-fix discipline — is a review obligation on prompt templates, not machine-checked.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be explicitly excluded with a reason.

- **Tagging.** A scenario in **any** suite file matching `tools/sandbox/*SUITE.md` (today: `SANDBOX-BUILDER-SUITE.md`, `SANDBOX-BURLER-SUITE.md`, `SANDBOX-CORE-SUITE.md`, `SANDBOX-FABRIC-SUITE.md`, `SANDBOX-PERCH-SUITE.md`, `SANDBOX-REED-SUITE.md`, `SANDBOX-SHUTTLE-SUITE.md`, `SANDBOX-WEBSTER-SUITE.md`) that drives a specific module declares it with a `**Covers:** <module>[, <module>...]` line, in the same bold-label style as the scenario's `**Goal:**`/`**Watch:**`/`**Verdict:**` lines. The guard unions tags across all matched files. Coverage is checked at module granularity against the live cobra root (`newRoot().Commands()`, skipping `help`/`completion`) — the same enumeration `longlist_test.go` already uses, never a separately hand-maintained list. The guard fails fast if the glob matches fewer than two files (vacuous-glob protection).
- **Allowlist.** Modules that are intentionally never sandbox-exercised across any suite file are named on the test's `excludedModules` allowlist with a one-line reason: `ide` (side-effect heavy: `spawn` opens a real VS Code window, `menu` is an interactive stdin picker), `selfreport` (`create` files a real GitHub issue).
- **Exists ⇒ covered or excluded.** Adding a new registered module requires either a scenario in some suite file tagged with that module's `**Covers:**` or a new allowlist entry with a reason — the same "exists ⇒ registered" discipline as the CLI/Cobra Invariant's registration guard.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go` (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) on every `go test`.

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init` / `git worktree add` / fixture-tree copies; Tier 1 stays offline and fast.

- **Statement.** A test file whose first non-empty line is not a `//go:build` constraint mentioning `integration` or `smoke` is "untagged" and runs on every plain `go test`. Untagged files must not spawn: no `gitexec.RunGit`, no `exec.Command`/`exec.CommandContext`, no `lyxtest.Copy*` fixture-tree copy. A platform-only constraint (e.g. `//go:build windows`) still counts as untagged — it still runs in Tier 1 on that platform, so its spawns still count. This is deliberately narrower than "spawn no processes": an untagged test that reaches `hubgeometry.Resolve` on an error path still spawns one cheap failing `git rev-parse`, which the guard does not ban.
- **Mechanics.** The guard walks every `*_test.go` file under the module root (resolved via `go env GOMOD`, cwd-independent) and checks each untagged file's source for a banned token as a **raw substring** — `gitexec.RunGit`, `exec.Command` (which also matches `exec.CommandContext`), or `lyxtest.Copy` (prefix-matches `CopyPaired`, `CopyPairedLocal`, `CopyHostHub`, `CopyWeft`, and any future `Copy*` fixture). Raw substring matching is deliberate: a comment or string-literal mention in an untagged file trips the guard too (rename the mention or tag the file).
- **Allowlist.** Exists ⇒ tagged or allowlisted with a reason. A file or directory-path prefix that must legitimately spawn in an untagged file is named on the guard's `allowedSpawners` map with a one-line reason: `internal/proc` (process control is the package's subject — its tests must spawn) and `cmd/lyx/tierpurity_test.go` itself (contains the banned token strings as its own test data).
- **Enforced by** `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`) on every `go test`.

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git — directly or via the lyxtest fixture helpers — runs under the hermetic git test environment, so no test behaviour depends on the operator's `~/.gitconfig` or the system gitconfig.

- **Statement.** A package is "git-spawning" when any of its `*_test.go` files spawns git directly (`gitexec.RunGit`, `exec.Command`/`exec.CommandContext`) or indirectly through a lyxtest fixture helper (`lyxtest.Copy*`, `lyxtest.MustRun`, `lyxtest.SeedConfig`). Every such package must contain a `TestMain` that calls `lyxtest.HermeticGitEnv()` before `m.Run()`, or be named on the allowlist below with a reason. The concrete failure this kills: a global `core.fsmonitor=true` in the operator's gitconfig spawning hundreds of `fsmonitor--daemon` processes per integration run — see `docs/benchmarks/fixture-copy.md` for measured numbers.
- **Mechanics.** The guard walks every `*_test.go` file under the module root (resolved via `go env GOMOD`, cwd-independent) and checks each file's source for the bare, unqualified `HermeticGitEnv` substring — matching both the qualified `lyxtest.HermeticGitEnv()` call form (other packages) and the unqualified `HermeticGitEnv()` form `internal/lyxtest`'s own tests use. Unlike the Test Tier Purity Invariant's guard, this one scans **every** test file regardless of build constraint: the git-spawning set is almost exactly the integration-tagged set, so skipping tagged files would make the guard vacuous. This proves presence only — the mechanical half of the check. The semantic half (a real `TestMain` that calls the helper before `m.Run()`) is a review obligation, exactly like the repo's other grep-guards (the Shell Mechanics Seam and Provider-Seam entries above).
- **Allowlist.** Exists ⇒ hermetic or allowlisted with a reason. A package directory-path prefix that spawns non-git processes for which a git-hermetic `TestMain` would be meaningless is named on the guard's `allowedNonHermetic` map with a one-line reason: `internal/proc` (process control is the package's subject — its tests must spawn, just not git). The guard's own file, `cmd/lyx/hermeticenv_test.go`, carries the tokens (including the bare `HermeticGitEnv` presence token) as its own test data; it is a **per-file scan exclusion**, not a package-level exemption — `cmd/lyx` itself genuinely spawns git in its e2e tests and satisfies the requirement through its own real `TestMain`.
- **Enforced by** `cmd/lyx/hermeticenv_test.go` (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) on every `go test`.

## Dev/Prod Binary Separation

The sandbox tooling resolves the dev binary from the derived `.dev-bin` (falling back to PATH) through `resolveLyx`, never a bare-PATH `lyx` lookup that could silently resolve prod.

- **Statement.** `resolveLyx` (`tools/sandbox/resolve.go`) is the single allowlisted resolution site: it checks the derived `.dev-bin/lyx` first and falls back to `lookPath("lyx")`, returning `(path, source)` with `source ∈ {dev, prod}`. This covers **both** `lookPath("lyx")` **and** the separator-free `exec.Command("lyx", …)` / `exec.CommandContext("lyx", …)` form — Go's `exec.Command` LookPath's a name with no path separator, so it is the same footgun as a direct `lookPath` call.
- **Never installed to prod.** The dev binary (`tools/deploy -dev`) is built into `<repoRoot>/.dev-bin`, never the production install location `deploy.cmd`/`deploy` target; `.dev-bin/` is gitignored.
- **Agent-only PATH prepend.** `.dev-bin` is prepended only to the agent child-process PATH (`launchAgent`), never the operator's own PATH — a bare `lyx` in an operator shell always resolves prod.
- **Enforced by** `tools/sandbox/pathresolve_guard_test.go` (`TestPathResolveGuard_NoBarePathLyxOutsideResolve`) for the mechanical half — no non-test file outside `resolve.go` may contain a banned bare-PATH `lyx` literal. The semantic half (agent-only PATH prepend, dev binary never installed to prod) is review discipline, not machine-checked.

## Planparser Sole-Parser Invariant

`internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`).

- No other package parses `00-overview.md`/`NN-<card-slug>.md`; consumers (webster's `RenderForkPrompt`, the integration fork) read plan-level sections only from the `planparser.Plan` model a caller hands in, never by re-deriving the grammar themselves.
- Composes with the Hub Geometry Invariant above: `planparser` resolves `_lyx/plan/` via `hubgeometry`, never string literals.
- **Enforced by** review obligation today (candidate future import/grep guard).

## Batcher Registry+Config Invariant

webster's execution unit is the batchifier-derived batch, not the raw plan card.

- Batching is selected by `internal/batcher`'s name-keyed registry plus the `batcher:` webster.yaml config key (default `identity`) — no plan-supplied batching exists and no batch grouping is expressed in the plan format itself.
- **Enforced by** review obligation.

## GitHub Auth Invariant

All GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`.

- **Statement.** Token resolution, token caching, and construction of an authenticated `*github.Client` live solely in `internal/githubclient`. No other production (non-test) package invokes `gh` — directly via `exec.Command`/`exec.CommandContext` or indirectly via a bare `LookPath("gh")` — or otherwise builds its own GitHub credential path.
- **Leaf property.** `internal/githubclient`'s production imports are allowlisted to stdlib, `go-github`, `golang.org/x/sys`, and `internal/proc`. `internal/proc` is on that list because the `gh auth token` fallback shell-out needs `proc.HideWindow` to suppress a console window on Windows, and `internal/proc` is itself stdlib-only — allowlisting it does not widen the leaf's real dependency surface or weaken the leaf property.
- **Enforced by** `cmd/lyx/ghguard_test.go` (`TestGHGuard_NoShellOutOutsideGithubclient`, the shell-out half) and `internal/githubclient/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`, the leaf-import half).

## gitrepo Client Boundary Invariant

`internal/gitrepo` splits local-vs-remote by client: go-git owns local object and ref access, `gitexec` owns anything that authenticates to a remote or mutates the working tree.

- **Statement.** go-git handles reads that resolve state already on disk — commit/tree/blob lookups and ref reads. `gitexec` stays the only path to the git CLI, used for `StageAndCommit`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `Pull`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `SetSnapshotSHA`'s push, `SnapshotSHA`'s fetch, and `hasUnpushed` (measured and reverted from a go-git ancestry walk per this entry's own reversal criterion) — the CLI-bound set named exhaustively here, not just in the package doc, because this entry is what a reviewer checks a new call against. Any new `gitexec` call added inside `internal/gitrepo` must come with an updated entry here justifying it in the same commit; widening the CLI-bound set without editing this list is itself a violation.
- **Known blind spot.** The guard's method-set check is set-equality on method names, so it cannot see a new `r.run` call added inside a method that is already on the pinned list (e.g. a third, illegitimate call slipped into `SnapshotSHA`, which legitimately mixes a migrated read with a CLI-bound fetch). The per-call review obligation stands for those already-pinned methods — the guard narrows what a reviewer must check by hand, it does not replace the check.
- **Enforced by** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`).

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Source-grounding rule

Never fabricate file contents or code behaviour you have not actually read. Do not infer from filenames or positions.

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
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: APPROVE | GAPS_FOUND
reviewer_model: sonnetmax
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

**Severity vocabulary is closed.** Use ONLY `GAP` or `NOTE` as the bracketed label in a finding heading -- never invent another word. If a finding's severity feels ambiguous, default to `GAP`, never `NOTE`.

Verdict rules:
- `APPROVE` — zero GAPs. NOTEs fine.
- `GAPS_FOUND` — one or more GAPs.

Note: plan and code reviews use `BLOCKING` / `NIT` + `REQUEST_CHANGES`. Discussion review uses `GAP` / `NOTE` + `GAPS_FOUND` because the semantics differ — a discussion "gap" is missing information, not a must-fix defect.

Omit the `## Findings` section entirely if there are zero findings. Never invent findings to pad the review.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/codeintel-v1/_mill/briefs/review-discussion-holistic-r2.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/codeintel-v1/_mill/briefs/review-discussion-holistic-r2.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/codeintel-v1/_mill/briefs/review-discussion-holistic-r2.out.md`
