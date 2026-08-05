**If you find issues, REPORT them — do NOT fix them.**

You are an independent discussion reviewer for **fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)**. Round **2**. Reviewer model: **opushigh**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

---

## Task

Read the discussion at `/home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/discussion.md`. The discussion file is the authoritative scope. Read files referenced in `## Technical Context` to verify claims.

Constraints:
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

Independently state, in the `reviewer_self_id:` field below, what model/version you believe yourself to be — this is your own best-effort assessment, distinct from the `reviewer_model:` value already dictated to you above.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line. Everything outside these markers is ignored by the backend. **No preamble inside the markers.** No "I reviewed..." sentences. No narrative intro.

Per finding: 3–5 lines total, short and factual. The consumer has full context of the discussion; do NOT explain background. Cite the section, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE (just verdict + brief summary), ~600–900 tokens for GAPS_FOUND (one finding block per issue). If you produce more than ~1200 tokens, you are being verbose — compress.

```
MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: APPROVE | GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: <your own model self-identification, if known>
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

Write your full report to this file: /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-discussion-holistic-r2.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-discussion-holistic-r2.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/briefs/review-discussion-holistic-r2.out.md`
