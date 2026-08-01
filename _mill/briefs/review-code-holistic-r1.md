**If you find issues, REPORT them — do NOT fix them.**

You are an independent code reviewer for **fabric: audit and migrate all remaining direct git mutations onto Fabric**. You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**. Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Prior non-blocking items

The following items were judged non-blocking in a prior round. Do NOT escalate any of them to BLOCKING unless NEW information justifies it -- a new diff, a real reproducible failure, or a concrete in-repo convention. If you escalate, you MUST state the new information explicitly.

Prefer the convention already used by analogous code in the provided source files over a stricter alternative.

(none)

## Constraints
# Constraints

Short, authoritative list of the repo's structural invariants. Each is partly machine-enforced (named test, fails `go test` / CI) and partly a review obligation. Fuller design/how-to lives in godoc and `docs/`, not here — this file is the index.

## Hub Geometry Invariant

`internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.

- All cwd / worktree-root queries go through `hubgeometry.Getwd()` / `Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`.
- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern` — are owned solely by `internal/hubgeometry`. No other package may use them in a path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`). Whole-token match; production files only; comparisons and git-pathspec slice literals are not path construction and stay allowed.
- `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through `hubgeometry.LyxDirName` / `ConfigDir(base)` / `ConfigFile(base, module)` — **in test code too** (a config-layout migration once broke a hardcoded test fixture).
- Geometry is structural, never config/env-overridable (the board dir is `--board-path` flag > `hubgeometry.BoardDir(l.Hub)`, not a config key).
- The weft-backed junction *name-set* (which directories get a host↔weft junction — `_lyx`, `_pattern`, and any future fabric-config-listed name) is injected from fabric config (`pathspec`), NOT enumerated inside `hubgeometry`. `hubgeometry` still owns all path *construction* and the hub-structural tokens (`_board`, `_portals`, `_launchers`, `_raddle`) — it exposes that hub-structural reserved set via the exported `HubReservedNames() []string` accessor, the sole source both `IsReservedHubName` and fabricengine's wiring-guard filter consume. Its junction methods take the name-set as an explicit `[]string` parameter and hold no `_lyx`/`_pattern` junction-record literals themselves: `HostJunctions(slug string, names []string)`, `HostJunctionsHere(names []string)`, and `IsReservedHubName(name string, junctionNames []string)`. `enforcement_test.go`'s machine check stays green under this split because no new geometry-token literal is introduced anywhere outside `hubgeometry`; note for future readers that a `[]string{"_lyx","_pattern"}` slice literal in non-`hubgeometry` Go is NOT one of the three contexts `TestEnforcement_GeometryLiterals` catches (a `filepath.Join` arg, a binary-`+` operand, or a `const`-value `BasicLit`), so the "no config-migrated names hardcoded in fabric" rule rests on review discipline, not the machine check.
- **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`. API and helpers: godoc for `internal/hubgeometry`.
- `RelPath` is resolved from the recorded `.fabric-anchor` marker (read from `BoardDir(Hub)` via `readRecordedAnchor`), not positionally from cwd: the record wins when present, cwd is demoted to a validated at-or-below gate (`Resolve` returns `ErrCwdOutsideAnchor` when cwd is not at or below the anchored subtree), and cwd-derived `RelPath` remains the fallback only when the marker is absent (mid-clone, lyxtest synthetic hubs, non-fabric repos). The gate applies only to the entry `Resolve(cwd)`; `ResolveWorktree(worktreeRoot)` and `SiblingLayout(worktreeRoot)` read the same recorded anchor but apply no cwd gate, since both derive another worktree's geometry from its root, which sits above any subpath anchor. The marker is a structural geometry artifact — a fixed per-repo anchor recorded once at create/clone — never a config/env override, so the existing "geometry is structural, never config/env-overridable" rule still holds; `hubgeometry` stays YAML-free, since the marker is a plain single-line file read with `os.ReadFile`+`strings.TrimSpace`.

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
- Import allowlist: stdlib, `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/shuttleengine`, and `gopkg.in/yaml.v3` — deliberately NOT `internal/hubgeometry` as a direct import: the engine is geometry-blind (caller-supplied absolute `runDir`/`GateDir`), matching the Hub Geometry Invariant's carve-out for packages that never construct `_lyx` paths themselves. The allowlisted `internal/logger` pulls in `internal/hubgeometry` transitively (for its durable-sink directory resolution); this invariant polices treadle's own direct imports only, not the transitive closure.
- **Enforced by** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`) on every `go test`.

## Tokenvocab Leaf Invariant

`internal/tokenvocab` production code imports only stdlib, `internal/hubgeometry`, and `internal/stencil` — so every future consumer (reed's header pipeline, loom's prompt templates) can import it without cycles.

- The reverse import (`tokenvocab` → `reed`, `tokenvocab` → `loom`, or any other feature package) is never allowed.
- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

## Scoutengine Leaf Invariant

`internal/scoutengine` production code imports only stdlib, `internal/hubgeometry`, `internal/lock`, `internal/proc`, `internal/logger`, and `gopkg.in/yaml.v3` — no `internal/output`, no `cobra`, no `internal/*cli` package — so the engine stays a cycle-free leaf importable by builder/webster later, exactly like `internal/modelspec`'s leaf excludes `output`. The engine returns typed Go errors and typed result values (`(T, error)`) and never touches `io.Writer`, exit codes, or the `output.Ok`/`output.Err` envelope; `internal/scoutcli` is the sole layer that maps engine errors/results into that envelope.

- `scoutcli` → `scoutengine` is the only allowed direction; the reverse import (`scoutengine` → `scoutcli`, or `scoutengine` → any other feature package) is never allowed.
- `internal/lock` fences the toolchain-install race (the Go toolchain manager's `resolveGoToolchain`) and the daemon spawn-race (the supervised-strategy daemon state file) — both genuinely cross-process coordination problems the leaf needs to solve itself, and `internal/lock` is already the repo's one primitive for exactly that, so allowlisting it is reuse rather than a new dependency class.
- `internal/proc` supplies the cross-platform `IsAlive` PID-liveness primitive the daemon state file's staleness check needs and the `Detach`/`DetachBreakaway` spawn primitive the `supervised` strategy needs — and, mirroring the **GitHub Auth Invariant** entry's own justification for the identical allowlist question, `internal/proc`'s own production imports are `os/exec` and `syscall` only, so allowlisting it does not widen the leaf's real transitive dependency surface.
- `internal/logger` is allowlisted because `EnsureServer`'s `supervised` strategy spawns a detached, session-long LSP daemon — making scoutengine a live-substrate spawn point that this document's own Live-Substrate Spawn Observability entry already requires to log through `internal/logger`. The widening costs nothing in real dependency surface: scoutengine already allows `internal/hubgeometry`, `internal/logger`'s only new transitive import, so the leaf's transitive set does not grow.
- **Enforced by** `internal/scoutengine/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.

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
- **Package naming.** A Cobra-registered package is `<module>cli`; its extracted domain kernel is `<module>engine`. cli imports engine; engine never imports cli or cobra. Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine. Skip the engine only for trivial wrappers (`configcli`) or a throwaway proof-of-concept meant to be deleted once it proves its point (e.g. `muxpoccli`, which followed exactly that path — deleted once `reed` shipped); "no consumer today" is not a skip reason. The `init` module is removed: its wiring responsibilities moved into `fabric clone`/`fabric add` (eager wiring) and its `--undo` teardown into `lyx fabric unwire`, all inside the existing `fabriccli`/`fabricengine` split.
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

## Fabric Git Invariant (warp + weft)

Every git operation that LYX/LoomYard's own code performs — on **either** the weft repo or the warp/host repo — goes through `internal/fabricengine` in Go, driven by the orchestration layer in-process — never raw git, and never an LLM agent. This is an invariant on LYX's own code, not a gate on external actors: a human (or any tool outside LYX) keeps ordinary git in their warp worktree — untouched and untouchable — and fabric's job is to stay correct under that arbitrary external activity, not to prevent it. See `manifest/designs/fabric-unified-view.md`'s "Warp stays ordinary git" section and the `fabric-rebase-reconcile` task (slice 6), whose job is reconciling exactly this case (warp moved by raw git outside fabric).

- **Module ownership.** Weft-internal git (`commit`/`push`/`pull`/`sync`) goes through `internal/fabricengine`; coordinated host↔weft topology (a checkout that moves both and re-points junctions, dual-worktree add/remove/clone) goes through `internal/fabricengine` too. No other package runs raw git against a weft worktree — **and the same holds for warp**: the host/warp repo is unrestricted only for external actors (a human, or any tool outside LYX, has an ordinary project repo to work in); no LYX package other than `internal/fabricengine` runs raw git against warp either, regardless of purpose (verification, bisect, etc.). Read-only verbs (e.g. reading current SHA, `git status --porcelain`) are exempt from this — only *mutating* warp git needs to dispatch through fabric, per `fabric-unified-view.md`'s "Scope boundary" section. **Gap closed:** both previously-tracked instances — `internal/websterengine`'s bisect/verify path (`CheckoutDetached`/`RestoreBranch` in `integration.go`) and `internal/builderengine`'s `ResetHard` chain-rollback path (`RestartChain` in `chain.go`) — now dispatch through `internal/fabricengine`'s warp-only methods (`Fabric.CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard`) via each package's own narrow consumer-side interface (`WarpBisector`, `WarpResetter`). Both packages' mutating-git non-bypass is now machine-checked (see the Enforced-by bullet below).
- **Orchestration, not agent.** The weft commit is Go calling the engine in-process (`fabricengine`'s `SyncWeft`/`CommitWeft`) at a round/phase boundary the loop owner (loom, or perch's CLI standalone) controls. An LLM agent never drives weft git — not raw git, not by shelling `lyx fabric`. Agents ride the file contract: they **write** overlay files (reviews, fixer-reports, status, raddle docs) into `_lyx`/`_raddle` via the junction; Go **reads and commits** them. Asymmetry: an agent **does** commit its own code to the **host** repo (commit-per-fix, normal dev git) — the weft, never. **Board carve-out.** `internal/boardengine`'s writes to `weft:main` are the one deliberate exception to timing control living with the loop owner: any LLM session, in any worktree, may trigger a board write (via `lyx board <verb>`) at any time — that is the whole point of board's shared-visibility model. Module ownership still holds without exception (board's git flows through `fabricengine.CommitWeftAt`/`PushWeftAt`, never raw git); only the *timing*-control half is scoped away from board. `Fabric.Commit` (`internal/fabricengine/commit.go`) has landed as a Go API called by the orchestration layer — a classify-and-dispatch two-sided commit, never invoked by an LLM agent. That is deliberate policy, not a code guard: unlike the old weft-write path, where an agent's `git add <weft-file>` simply failed because the weft file wasn't tracked in warp, `Fabric.Commit` makes weft writes clean, so the accidental guardrail this bullet used to describe no longer exists — nothing in `Fabric.Commit` itself refuses an agent-driven call, and the old accidental "`git add` fails" guardrail is intentionally not reintroduced. The invariant holds anyway because nothing in the orchestration stack (loom, perch's CLI standalone) ever hands an LLM agent a path to `Fabric.Commit`; module ownership is satisfied because the weft side of `Fabric.Commit` routes through the same `commitWeftLocked`/`CommitWeft` machinery every other weft-git verb uses, under the fabric-layer write lock, never raw git.
- **Why.** A weft commit is an orchestration act (persist round/phase state at the right boundary, coordinate host↔weft) — the deterministic Go responsibility that is the whole lyx thesis. An agent-run weft commit reintroduces the non-deterministic, untestable, mis-ordered LLM orchestration lyx exists to remove.
- **Anchored exclusions.** A caller that passes `CommitWeft` a pathspec with `:(exclude)` entries must **anchor every exclusion under the same scoped `_lyx` base the positive entry names**, forward-slash spelled — `":(exclude)" + base + "/*.lock"`, never `":(exclude)*.lock"`. Git classifies a leading-`*` pattern with no further wildcard as a one-star pathspec, which false-positive-matches the intermediate directories it must descend through to reach a multi-segment positive pathspec: at a `layout.RelPath` of two or more segments the whole subtree is pruned, `git add` stages nothing, and the weft commit becomes a **silent no-op with no error**. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both anchored, with real-git depth coverage in each package's `weft_integration_test.go`) and `internal/perchcli`'s block-exit commit (**still unanchored** — carries this bug, owned by perch; the Cross-module exclusions bullet's git-exclude backstop happens to keep perch's commit from tracking the *standard* machine-local artifact set anyway, but perchcli's own pathspec is still wrong and should be fixed on its own terms, not relied on to stay correct only by that backstop). A slice-shape unit test cannot see this; only a real-git assertion can.
- **Cross-module exclusions.** The `_lyx` tree is **shared** by every round-loop module in a worktree, so a module's weft commit stages whatever the *other* modules happen to have left on disk. A caller's exclusion set must therefore name **every** module's machine-local artifacts, not just its own — today that is builder's and webster's pause flags plus webster's rendered fork prompts (`_lyx/webster/prompts/*`); the `<base>/*.lock` entry is already cross-module because a git pathspec `*` crosses `/`. Excluding only your own is not a cosmetic miss: once another module's flag is tracked, **its owner can never stage its deletion** (that module's own exclusion hides the path from `git add`), so it is pinned in weft `HEAD`, pushed, and materialized by every other machine's weft pull as a pause request nobody made. Live callers today are `internal/buildercli`'s and `internal/webstercli`'s `weftCommit` (both cross-module, with real-git coverage in each package's `weft_integration_test.go`). **Fixed at the git-exclude layer, not the pathspec layer:** `fabricengine.seedWeftArtifactExcludes` (called from `CommitWeft`'s `ensureWeftLockDir`, the single choke point every weft-git verb passes through) seeds `crossModuleMachineLocalExcludes` — gitignore-syntax patterns (`**/_lyx/*/*.lock`, `**/_lyx/*/pause`, `**/_lyx/*/prompts/`, module name wildcarded) — into the weft repo's `.git/info/exclude`. This makes **every** committer correct by construction, including `lyx fabric commit|push|sync`'s own pathspec (`internal/fabriccli/weft_verbs.go`, still positive-entries-only) and `internal/perchcli`'s still-unanchored block-exit commit (see the Anchored exclusions bullet above) — neither needs its own exclusion logic, since git itself now refuses to stage these paths regardless of what pathspec asked for them. Because gitignore glob semantics differ from git pathspec magic (a bare `*` here does **not** cross `/`), the `**/` prefix alone reaches every `RelPath` depth with no per-caller anchoring needed — the anchoring problem the "Anchored exclusions" bullet above describes is a pathspec-only failure mode, and does not apply to this exclude-file mechanism. **Known limitation:** this stops new pollution but does not untrack an artifact a pre-fix `lyx fabric sync` already committed on an existing hub — `.git/info/exclude` only affects untracked status. `git rm --cached <path>` (or a fresh `lyx fabric sync` after manually removing the tracked entries) is the manual remedy on an already-polluted hub; no automated migration tool was added.
- **Enforced by** review obligation: agent prompt templates never instruct a weft git op, and weft git stays inside `internal/fabricengine`. The module-ownership half is machine-checked for `internal/boardengine` specifically by `cmd/lyx/boardguard_test.go` (no raw `gitrepo`/`gitexec` import or shell-out); it is also machine-checked for `internal/websterengine`/`internal/builderengine` specifically by `cmd/lyx/rawgitmutation_test.go` (`TestNoRawGitMutation_WebsterBuilderProductionSource`), which bans `gitrepo.New(`/`gitexec.RunGit(` in both packages' production source, file-allowlisting `gitwrap.go`'s and `gitquery.go`'s grandfathered read-only exemptions; the general case (every OTHER `fabricengine` caller) remains a review obligation, not machine-checked today. The agent half is partly machine-checked for webster runs by `websterengine`'s `weftReferencePattern` (a fork or Master Bash command matching `lyx fabric` — or a weft worktree path — is a hard, round-failing `weft-reference` violation).

## Review Round Invariant

One review+fix round (burler now, hardener later) follows the round discipline: A-before-B (the review is fully written to disk before any target file is touched); every recorded finding is fixed in B, all severities including LOW/NIT; no self-grading (round N's fix is judged by round N+1's fresh review, never its own); commit-per-fix on host source, never push. In a cluster round, the fork reports, the handler's own holistic review, and the consolidation into one review file are ALL part of A — the consolidated review is fully on disk before B touches a single target file, exactly as in a solo round — and fork reviewers are read-only: no writes, no git, mechanically enforced by the fork audit (never advisory).

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline` for the orchestrator's sequencing statements and instruction 3's fix-everything/never-push statements, `TestTemplate_StatesClusterForkDiscipline` for instruction 2's cluster sequencing/read-only statements via a cluster-profile `composePrompt` render, and `TestTemplate_OrchestratorExcludesDownstreamBodies` guarding that the inline orchestrator does not carry the downstream instruction bodies — the lazy-read separation). The rest — no self-grading, commit-per-fix discipline — is a review obligation on prompt templates, not machine-checked.

## Live-Substrate Spawn Observability

Any code path that starts a real OS process on behalf of a round/strand/session (a tmux server, a `claude`/provider session, any subprocess a live-substrate test can multiply) logs the spawn and its teardown via `internal/logger` — `logger.Info` for the normal spawn/teardown events (session/socket/PID/round identifiers), `logger.Warn` for a retry or a teardown that did not confirm clean. On stderr this is silent by default (matching `internal/logger`'s own Warn-threshold default) but is switched on for a `go test`-only entry point — which never reaches `cmd/lyx/main.go`'s `-v`/`-vv` flag parsing — via the `LYX_LOG_LEVEL`/`LYX_LOG_FILE` environment variables `internal/logger` reads at init. The durable Info+ trace-file sink removes that precondition for these spawn/teardown events specifically: they land in the durable trace file regardless of verbosity or env-var configuration (subject to the `LYX_TRACE=1` test-entry-activation gate under `go test`), while `LYX_LOG_LEVEL`/`LYX_LOG_FILE` remain exactly as described above for the stderr-only half. See `internal/logger`'s package doc ("Level policy" section) for the Warn/Info/Debug definitions and the loop-body hard rule — not restated here. This exists because a RAM-exhaustion incident (crucible round on burler, 2026-07-30) left no scout of what had actually spawned or how many times — only `ps` forensics after the fact could reconstruct any of it. Known instrumented call sites today: `internal/reedengine/lifecycle.go` (tmux server spawn/boot-retry/teardown), `internal/shuttleengine/run.go` (`Start`, one line per run naming role/round/fork-authorization), `internal/burlerengine/engine.go` (`Run`, naming the cluster fan and resolved fork count before the round starts), `internal/scoutengine/ensureserver.go` (`EnsureServer`'s supervised-daemon spawn, naming lang/pid/socket).

- **A new spawn point for a live-substrate module must add its own `logger.Info`/`Warn` call in the same change** — this is a review obligation, not machine-enforced.
- **A spawned pane/child must never re-exec `os.Executable()` while running under `go test`.** Under `go test`, `os.Executable()` is the TEST BINARY, and a Go test binary invoked with positional arguments does not error — flag parsing stops at the first non-flag argument, so the arguments are silently ignored and the FULL suite runs with no `-run` filter. Confirmed live (2026-07-30, the root cause of that day's four RAM-exhaustion incidents): reedengine's header pane ran `<test-binary> reed header --blocking`, which re-ran the entire `-tags smoke` suite from inside the pane, each smoke test booting a fresh substrate whose own header pane repeated the re-exec — unbounded recursion, ~30 matched (tmux server, claude) pairs from one correctly-scoped single-test invocation. Two enforcement layers, both landed with this entry: `reedengine`'s `headerLaunchLine` (headerpane.go) suppresses the header re-exec when `testing.Testing()` is true, leaving the pane a bare blocking shell (unit-pinned by `TestHeaderLaunchLine`); and `lyxtest.HermeticGitEnv` refuses to run any test binary invoked with a leading positional argument (`refuseCLIReexec`, exit 2, loud diagnostic) — defense-in-depth against any future spawn point making the same mistake, wired into every git-spawning package automatically via the Hermetic Git Test Environment Invariant below.
- **A retry loop around a real process spawn must cap attempt COUNT, not only elapsed time.** A time-only budget (e.g. "retry for up to 90s") assumes each failed attempt costs real wall-clock time; a spawn that fails FAST (resource exhaustion, a rejected fork, a cgroup limit) burns through that budget in far more attempts than the timeout was sized for — confirmed live (2026-07-30, `internal/reedengine/lifecycle.go`'s tmux boot loop): a fast-failure spiral reached 30-90+ real tmux-server spawns inside a single `bootOverallTimeout` window before the loop ever exited. `maxBootAttempts` in that file is the pattern to copy: track an attempt counter and exit on whichever of (time, count) is hit first.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be explicitly excluded with a reason.

- **Tagging.** A scenario in **any** suite file matching `tools/sandbox/*SUITE.md` (today: `SANDBOX-BUILDER-SUITE.md`, `SANDBOX-BURLER-SUITE.md`, `SANDBOX-CORE-SUITE.md`, `SANDBOX-FABRIC-SUITE.md`, `SANDBOX-PERCH-SUITE.md`, `SANDBOX-REED-SUITE.md`, `SANDBOX-SHUTTLE-SUITE.md`, `SANDBOX-WEBSTER-SUITE.md`) that drives a specific module declares it with a `**Covers:** <module>[, <module>...]` line, in the same bold-label style as the scenario's `**Goal:**`/`**Watch:**`/`**Verdict:**` lines. The guard unions tags across all matched files. Coverage is checked at module granularity against the live cobra root (`newRoot().Commands()`, skipping `help`/`completion`) — the same enumeration `longlist_test.go` already uses, never a separately hand-maintained list. The guard fails fast if the glob matches fewer than two files (vacuous-glob protection).
- **Allowlist.** Modules that are intentionally never sandbox-exercised across any suite file are named on the test's `excludedModules` allowlist with a one-line reason: `ide` (side-effect heavy: `spawn` opens a real VS Code window, `menu` is an interactive stdin picker), `selfreport` (`create` files a real GitHub issue), `scout` (requires an external language-server binary (gopls/pyright/csharp-ls) on $PATH; exercised by //go:build scout tests, not the black-box sandbox suite).
- **Exists ⇒ covered or excluded.** Adding a new registered module requires either a scenario in some suite file tagged with that module's `**Covers:**` or a new allowlist entry with a reason — the same "exists ⇒ registered" discipline as the CLI/Cobra Invariant's registration guard.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go` (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) on every `go test`.

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init` / `git worktree add` / fixture-tree copies; Tier 1 stays offline and fast.

- **Statement.** A test file whose first non-empty line is not a `//go:build` constraint mentioning any tag in `cmd/lyx/tierpurity_test.go`'s known-tags list (`integration`, `smoke`, `scout`) is "untagged" and runs on every plain `go test`. Untagged files must not spawn: no `gitexec.RunGit`, no `exec.Command`/`exec.CommandContext`, no `lyxtest.Copy*` fixture-tree copy. A platform-only constraint (e.g. `//go:build windows`) still counts as untagged — it still runs in Tier 1 on that platform, so its spawns still count. This is deliberately narrower than "spawn no processes": an untagged test that reaches `hubgeometry.Resolve` on an error path still spawns one cheap failing `git rev-parse`, which the guard does not ban.
- **Substrate definition.** The full substrate definition this invariant enforces — real `git` subprocess spawning, real filesystem junctions/symlinks, real `tmux` sessions, real cross-compilation, and real external-binary spawn (the `scout` tag's category) — lives in `docs/benchmarks/running-tests.md`'s "## The two tiers" section; this entry is the terse index pointer, not a duplicate of the explanation.
- **Mechanics.** The guard walks every `*_test.go` file under the module root (resolved via `go env GOMOD`, cwd-independent) and checks each untagged file's source for a banned token as a **raw substring** — `gitexec.RunGit`, `exec.Command` (which also matches `exec.CommandContext`), or `lyxtest.Copy` (prefix-matches `CopyPaired`, `CopyPairedLocal`, `CopyHostHub`, `CopyWeft`, and any future `Copy*` fixture). Raw substring matching is deliberate: a comment or string-literal mention in an untagged file trips the guard too (rename the mention or tag the file).
- **Allowlist.** Exists ⇒ tagged or allowlisted with a reason. A file or directory-path prefix that must legitimately spawn in an untagged file is named on the guard's `allowedSpawners` map with a one-line reason: `internal/proc` (process control is the package's subject — its tests must spawn) and `cmd/lyx/tierpurity_test.go` itself (contains the banned token strings as its own test data).
- **Real-time-wait guard (additive).** An untagged test file containing a literal `time.Sleep(...)` call whose duration argument is a compile-time constant ≥ 1 second is also flagged, allowlisted the same way as the banned-spawn-token check (`allowedLongSleepers` in `cmd/lyx/tiersleep_test.go`); an argument shape the guard cannot resolve to a known constant (an unrecognized selector, an identifier with no in-file declaration, or a malformed numeric literal) is conservatively flagged too, forcing an explicit allowlist entry or a rename rather than silently passing; this check does not inspect `context.WithTimeout` ceilings or short bounded-retry constants, and is additive only — it would not have caught the historic `githubclient`/`webstercli` real-time-wait regressions (those were production-side hardcoded `const` timeouts a test could not override, not a test-side `Sleep` call).
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

- **Statement.** go-git handles reads that resolve state already on disk — commit/tree/blob lookups and ref reads. `gitexec` stays the only path to the git CLI, used for `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `Fetch`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `IsAncestor`, and `HasUnpushed` (measured and reverted from a go-git ancestry walk per this entry's own reversal criterion) — the CLI-bound set named exhaustively here, not just in the package doc, because this entry is what a reviewer checks a new call against. `Fetch` refreshes this checkout's remote-tracking refs without merging, so a caller can inspect what changed upstream before deciding how to reconcile; `IsAncestor` classifies fast-forward-vs-rewrite and drives the anchor-walk reachability checks. Any new `gitexec` call added inside `internal/gitrepo` must come with an updated entry here justifying it in the same commit; widening the CLI-bound set without editing this list is itself a violation. `CommitEmpty` mutates the repository's history, which is squarely on the `gitexec` side of the split, and go-git offers no equivalent that respects the pre-check.
- **Known blind spot.** The guard's method-set check is set-equality on method names, so it cannot see a new `r.run` call added inside a method that is already on the pinned list (e.g. a fourth, illegitimate call slipped into `StageAndCommit`, which legitimately mixes three CLI-bound calls — add, `diff --cached`, commit — with a migrated go-git `CurrentSHA` read; `CommitEmpty` is a second worked example, legitimately mixing two CLI-bound calls — the dirty-index pre-check and the commit — with a migrated go-git `CurrentSHA` read on both its entry and its return path). The per-call review obligation stands for those already-pinned methods — the guard narrows what a reviewer must check by hand, it does not replace the check.
- **Enforced by** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`).

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=34)

- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/00-overview.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/01-fabric-warp-methods.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/02-webster-bisect-migrate.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/03-builder-resethard-migrate.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/04-regression-guard-and-constraints.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/fabric.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/manifest/designs/fabric-unified-view.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/warpforward.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/warpforward_integration_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/integration.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/runlevel.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/runlevel_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/integration_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/chain.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/gitquery.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/gitquery_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/spawn.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/chain_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/spawn_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/tierpurity_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/CONSTRAINTS.md
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/rawgitmutation_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/gitrepo/gitrepo.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/gitrepo/reset.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/checkout_rollback_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/reconcile_stale_registration_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/testmain_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/lyxtest
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/hubgeometry/hubgeometry.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/webstercli/weft.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/buildercli/weft.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/gitrepoboundary_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/hermeticenv_test.go
- /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/gitwrap.go

## Plan + source files to review
- Overview: `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/00-overview.md`
- Batch file(s):
  - `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/01-fabric-warp-methods.md`
  - `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/02-webster-bisect-migrate.md`
  - `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/03-builder-resethard-migrate.md`
  - `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/plan/04-regression-guard-and-constraints.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/fabric.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/manifest/designs/fabric-unified-view.md`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/warpforward.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/warpforward_integration_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/integration.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/runlevel.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/runlevel_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/integration_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/chain.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/gitquery.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/gitquery_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/spawn.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/chain_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/builderengine/spawn_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/tierpurity_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/rawgitmutation_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/gitrepo/gitrepo.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/gitrepo/reset.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/checkout_rollback_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/reconcile_stale_registration_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/fabricengine/testmain_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/lyxtest`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/hubgeometry/hubgeometry.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/webstercli/weft.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/buildercli/weft.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/gitrepoboundary_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/cmd/lyx/hermeticenv_test.go`
- `/home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/internal/websterengine/gitwrap.go`

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
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric — holistic

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

**Severity vocabulary is closed.** Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word (e.g. `MAJOR`, `MINOR`, `CRITICAL`, `MEDIUM`, `HIGH`). If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT` -- an over-cautious BLOCKING can be pushed back on by the orchestrator; a mislabeled NIT (or an unrecognized label) can silently skip review entirely.

Omit `## Findings` if zero findings. Never invent findings to pad.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/briefs/review-code-holistic-r1.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/briefs/review-code-holistic-r1.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/webster-bisect-fabric-migrate/_mill/briefs/review-code-holistic-r1.out.md`
