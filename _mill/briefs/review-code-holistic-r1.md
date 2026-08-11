**If you find issues, REPORT them — do NOT fix them.**

You are an independent code reviewer for **fabric: one ownership-and-dirtiness gate for all destruction (slice 12)**.
You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**.
Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Prior non-blocking items

The following items were judged non-blocking in a prior round.
Do NOT escalate any of them to BLOCKING unless NEW information justifies it -- a new diff, a real reproducible failure, or a concrete in-repo convention.
If you escalate, you MUST state the new information explicitly.

Prefer the convention already used by analogous code in the provided source files over a stricter alternative.

(none)

## Constraints
# Constraints

Short, authoritative list of the repo's structural invariants.
Each is partly machine-enforced (named test, fails `go test`/CI) and partly a review obligation.
This file states rules only — no rationale, no incident narratives, no historical justification.
Fuller design/how-to lives in godoc and `docs/`.

## Cwd Resolution Invariant

`internal/lyxcwd` owns cwd resolution and nothing else — never weft, never a junction path, never any per-module subdirectory.

- **`root` always means the git worktree/repo root;
  the current working directory is `cwd`.**
  Never name a parameter, field, or local variable `root` for a value that is actually `cwd`, or vice versa.
- All cwd/worktree-root queries go through `lyxcwd.Getwd()`/`Resolve()`.
  Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/lyxcwd` and `cmd/lyx/main.go`.
- `lyxcwd.Resolve` exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`,
  and the two derived accessors (`WorktreePath()`, `AnchorPath()`) built from them.
  It never resolves or exposes a weft path, a junction path, or any per-module subdirectory — those are not geometry `lyxcwd` owns.
- cwd must equal `AnchorPath()` exactly;
  `Resolve` returns `ErrCwdOutsideAnchor` otherwise. `ResolveWithAnchor` and `ResolveWorktree` are ungated — `ResolveWithAnchor` is a documented bypass, used only by callers that legitimately stand somewhere the gate would reject (fabric's clone, lyxtest's synthetic hubs).
- A module's own durable-storage subdirectory (e.g. `_lyx/plan`, `_lyx/webster`) is that module's own private relative-path constant, joined onto `AnchorPath()` directly — never a `lyxcwd` function call.
  Adding a module's own subdirectory is never a `lyxcwd` change.
  Its ephemeral twin is the Durable-vs-Ephemeral State Invariant below.
- `internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — this is what keeps `fabricengine` → `logger` → `lyxcwd` acyclic.
- Weft-sibling paths and junction construction belong to `internal/fabricengine`, never `lyxcwd`: `WeftWorktree`/`WeftRepoRoot`/`WarpLyxLink`/`WarpJunctions`/portal and launcher paths,
  and the `Prime`/sibling-worktree-list lookup they're built from, are `fabricengine`-private. `lyxcwd` never mentions weft.
  See the Fabric Vocabulary Invariant below for the vocabulary rule this bullet is one instance of.
- Geometry is structural, never config/env-overridable.
- The weft-backed junction name-set is injected from fabric config (`fabric.yaml`'s `pathspec`, read at `<Hub>/_board/_lyx/config/fabric.yaml`) — `fabricengine`'s concern, not `lyxcwd`'s.
- `AnchorRel` resolves from the recorded `.lyx-anchor` marker, not positionally from cwd;
  cwd is a validated exact-equality gate (`ErrCwdOutsideAnchor` if violated), falling back to `"."` only when the marker is absent. `ResolveWorktree`/`ResolveWithAnchor` read the same anchor with no cwd gate.
- The `"."` fallback applies to an ABSENT anchor only, never a stale one: a board carrying the pre-rename `.lyx-anchor` spelling (`lyxcwd.StaleAnchorFileName`) with no renamed marker beside it returns `ErrStaleAnchorMarker` from every resolver.
  `lyxcwd` is the single declarer of both marker names;
  fabric's clone-time guard aliases them rather than re-declaring the literals.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) for the geometry-literal ban,
  and `internal/lyxcwd/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) for the import cap.

## Lyxdirs Single-Declarer Invariant

`internal/lyxdirs` is the sole declarer of the two lyx directory-name tokens, `_lyx` (`LyxDirName`) and `.lyx` (`DotLyxDirName`).

- `internal/lyxdirs` stays stdlib-only, a zero-import leaf, so every module that needs either token can import it without cycle risk.
- No other production file may name either literal in path-construction context (a `filepath.Join` argument, a `+` operand, or a string const declaration value) — every caller uses `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName` instead.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_GeometryLiterals`).

## Durable-vs-Ephemeral State Invariant

Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to. `_lyx` holds tracked content only.

- `_lyx` and `.lyx` are directory siblings under `AnchorPath()` — sole exception `reedengine.HubLogsDir` (hub-anchored).
- No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one.
- `_lyx`/`.lyx` are structural (`fabricengine`'s `structuralCommittedDirs`/`structuralNeverCommittedDirs`), never read from `fabric.yaml`'s `pathspec` key, which is reserved for optional, explicitly-named dirs only.
- `.lyx` is in the wired name-set (`WiredNames`/`RepoWiredNames`) but never in the pathspec/commit-routing set (`PathspecNames`).
- `<hub>/.lyx` is hub-level geometry alongside `<hub>/_board`, created by `fabricengine.CloneHub` — a real directory, never a junction, reserved so no worktree slug can claim the name.
- **Enforced by** `cmd/lyx/notransients_test.go`, `cmd/lyx/constructoranchoring_test.go`, `internal/fabricengine/structuraldirs_test.go`, `template_test.go`, `dotlyxjunction_integration_test.go`.
  A newly added transient's mirrored-subpath placement is a review obligation.

## lyxtest Leaf Invariant

`internal/lyxtest` production code imports only stdlib, `internal/lyxcwd`, `internal/weftname`, `internal/configengine`, and `internal/lyxdirs`.
`internal/configreg` and every feature package (`boardengine`/`boardcli`, `ideengine`/`idecli`, `selfreportengine`/`selfreportcli`, `fabricengine`/`fabriccli`) are excluded by construction — feature packages' own tests import lyxtest, so a reverse import would close a test-build cycle.

- Tests needing real config call `lyxtest.SeedConfig(tb, dir, map[string]string{...})`.
- **Enforced by** `internal/lyxtest/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Modelspec Leaf Invariant

`internal/modelspec` production code imports only stdlib, `internal/configengine`, and `gopkg.in/yaml.v3`.

- `configreg` → `modelspec` is allowed (for `modelspec.ConfigTemplate`);
  the reverse is never allowed.
- **Enforced by** `internal/modelspec/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Treadle Runner-Seam Invariant

`internal/treadleengine` never imports `internal/burlerengine` or any `internal/*cli` package;
round runners adapt onto treadle's `RoundRunner` vocabulary in their own packages.

- Import allowlist: stdlib, `internal/lock`, `internal/logger`, `internal/state`, `internal/stencil`, `internal/shuttleengine`, `gopkg.in/yaml.v3` — not `internal/lyxcwd` directly.
  Policed on direct imports only, not the transitive closure: `lyxcwd` is reachable through both `logger` and `shuttleengine`, so excluding it buys no isolation.
  What the exclusion enforces is that treadle is *told* its geometry and never derives it — `Engine.Run` takes a caller-supplied absolute `runDir`, a block's `Profile` carries a caller-supplied `GateDir`, and every path this package builds is joined onto one of those.
- **Enforced by** `internal/treadleengine/seam_enforcement_test.go` (`TestRunnerSeamInvariant_AllowlistOnly`).

## Tokenvocab Leaf Invariant

`internal/tokenvocab` production code imports only stdlib, `internal/lyxcwd`, and `internal/stencil`.

- Reverse import (`tokenvocab` → `reed`/`loom`/any feature package) is never allowed.
- **Enforced by** `internal/tokenvocab/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## Scout Engine-Seam Invariant

`internal/scoutengine` never imports `internal/output`, `cobra`, or any `internal/*cli` package.
It returns typed `(T, error)` and never touches `io.Writer`, exit codes, or the output envelope;
`internal/scoutcli` maps engine results into that envelope.

- `scoutcli` → `scoutengine` is the only allowed direction.
- No import allowlist.
  Scout draws on the shared-infrastructure layer as freely as `websterengine`, `perchengine`, and `loomengine` do.
  Policed as a banned list on direct imports only, never the transitive closure — a banned package reached through a permitted one is not caught, by design. `internal/clihelp` is named explicitly because it carries cobra without matching the `*cli` suffix.
- **Narrower file-scoped guard.** `internal/scoutengine/lspclient.go` imports stdlib plus `internal/logger` and nothing else, keeping the ported stdio LSP client liftable back out of lyx.
  The rule is that allowed set exactly. `internal/logger` itself imports `internal/lyxcwd` and `internal/proc`, so the file must never be described as stdlib-only or hermetic — it is neither.
- **Enforced by** `internal/scoutengine/seam_enforcement_test.go` (`TestEngineSeamInvariant_BannedImports`) for the banned list,
  and `internal/scoutengine/lspclient_guard_test.go` (`TestLSPClientGuard_StdlibAndLoggerOnly`) for the file-scoped guard.

## Pattern Leaf Invariant

`internal/pattern` production code imports only stdlib, `internal/lyxcwd`, and `internal/lyxdirs` — never `websterengine`, `burlerengine`, `loomengine`, or any other feature package.
Reverse import never allowed.
`internal/lyxdirs` is admissible because it is a stdlib-only zero-import leaf (its own Lyxdirs Single-Declarer Invariant), and therefore cannot participate in a cycle by construction.

- **Enforced by** `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- **Seam.**
  Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int` = `clihelp.Execute(Command(), out, args)`.
- **Registration.**
  A new module is wired into `newRoot()`: import, `root.AddCommand(...)`, and appended to the root `Long` module-list.
  Unregistered ⇒ invisible to `--help`.
- **`Short` on every command** (parent + sub), non-empty.
  Self-discoverable commands also carry a `Long` with concrete examples.
- **Help accuracy is a review obligation.**
  When a change alters observable behaviour, the reviewer must re-check every affected `Short`/`Long`.
- **Errors are JSON**, via the `internal/output` envelope (`output.Ok`/`output.Err`), one JSON object per line, through `clihelp.Execute`/root seam.
  No bare plain-text error paths.
  Parent groups set `RunE = clihelp.GroupRunE`.
- **Interactive-handoff exception (narrow, per-command).**
  A subcommand that hands stdio to another interactive program and blocks, or self-displays and then blocks forever, is exempt from the envelope only on that terminal-handover/keepalive tail — everything that can fail stays pre-flight, on the envelope.
- **Package naming.**
  A cobra-registered package is `<module>cli`;
  its domain kernel is `<module>engine`. cli imports engine;
  engine never imports cli or cobra.
  Litmus: returns `(T, error)` with no cobra/`io.Writer`/exit codes ⇒ engine.
  Skip the engine only for trivial wrappers or a throwaway proof-of-concept meant to be deleted.
- **Enforced by** `cmd/lyx/drift_test.go`, `helptree_test.go`, `registration_test.go`, `longlist_test.go`.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- `internal/shuttleengine` and `internal/reedengine` stay provider-invariant: they define the `Engine` interface (and, for reed, the opaque `cmd`/`resumeCmd`/strand contract) and never reference Claude specifics.
- `internal/shuttleengine` never imports `internal/shuttleengine/claudeengine` — the reverse import only.
  Wiring a concrete engine happens in `internal/shuttlecli`.
- **Enforced by** `internal/shuttleengine/seam_enforcement_test.go` (`TestProviderSeamImportRule`) for the import-graph half;
  no Claude-specific leakage outside `claudeengine` is a review obligation.

## Shell Mechanics Seam

Pane-shell command strings — argument quoting, the call operator, and the prompt-file read idiom — are built ONLY via `internal/shell`.

- `internal/shell` defines the provider-invariant `Shell` interface (`Quote`/`Invoke`/`ReadFile`) with `pwsh` and `posix` implementations, selected via `shell.ForGOOS()`.
  Stdlib-only, no Claude specifics.
- `internal/shuttleengine/claudeengine` (and any future provider engine) never emits raw pwsh/posix shell syntax directly — only via `internal/shell`.
- **Enforced by** review obligation today (candidate future grep guard).

## Fabric Vocabulary Invariant

**Fabric** (capital F) names the fully wired-up composite — the warp repo with junctions into weft inside it.
Any reader meaning *the repo as a whole* says Fabric.
**warp** and **weft** name the two sides and are used — including in CLI help text and user-visible messages — at exactly those points where the two sides genuinely must be told apart, e.g. `lyx fabric clone <weft-url> [<warp-url>]` and `fabric: warp/weft out of sync`.
"repo" alone is too vague to denote warp and is never a substitute for it.
**`host` is retired** and is never used in any of these senses, anywhere — including inside the owner set below.

The phrase predicate is the sense-discriminator, retained unchanged: `host` is policed via the fabric-sense phrase list (`host repo`, `host repository`, `host worktree`, `host working tree`, `host checkout`, `host branch`, `host junction`, `host path`, `host side`, `host HEAD`, any case, hyphenated or spaced) plus the policed geometry identifiers (`hostBranch`, `hostLayoutFor`, `hostReason`, `HostJunction`, `hostClean`), never as a bare word.
The bare word — the verb sense, the machine/OS sense, and the PowerShell `Write-Host` cmdlet — still passes untouched, because a whole-word ban would rewrite ordinary English in modules with no connection to fabric.
Keep these lists verbatim: they are the ban list, and renaming them would delete the rule.

- **Owner set carves out the bare weft/warp rule only, never the host rule.**
  Owner set: `internal/fabricengine`, `internal/fabriccli`, `internal/weftname`, `internal/lyxtest`, `internal/boardengine`, `internal/configsync` (string literals and comments, never identifiers).
  `tools/` and `sandbox/` are not in the owner set — they lie outside the enforcement walk entirely, since the Go walk covers `internal/` and `cmd/` only, so an owner-map row for them would be dead code that never matches.
  Vocabulary in `tools/` and `sandbox/` is a review obligation, not machine-checked.
- **Prose-doc split — review obligation, not machine-checked:** a doc explaining fabric's own mechanism keeps the vocabulary;
  a doc describing a consumer module's behaviour rewords to Fabric or drops the qualifier, because that module does not know weft exists.
  A token scan cannot express this distinction, so it is not covered by the enforcement test.
- This invariant binds every module, template, and doc that talks about fabric — `internal/lyxcwd` is merely one of the packages it binds, not its owner.
  The enforcement test's placement in `internal/lyxcwd/enforcement_test.go` is a file-layout convenience — it reuses that file's `filepath.WalkDir` helper — not an ownership claim.
- **What the machine check does and does not reach — stated honestly, not implying full coverage.**
  Production Go under `internal/` and `cmd/` is machine-guarded, plus an `internal/**/*.md` walk and the embedded agent prompt templates.
  `*_test.go` files are excluded from all three rules.
  `hostGeometryIdentifiers` is five exact lowercased names, so `HostJunctions`, `hostPath`, `hostBare`, `CopyHostHub`, and `HostFixture` are matched only by the phrase half, and only where they occur inside a policed phrase.
  Test files, documentation outside `internal/`, shell, and `tools/` remain a **review obligation**, not a machine check.
- **Enforced by** `internal/lyxcwd/enforcement_test.go` (`TestEnforcement_FabricVocabulary`), covering identifiers, string literals, and comments in production `.go` files under `internal/` and `cmd/`, plus an `internal/**/*.md` walk and the embedded agent prompt templates.
  The host rule is machine-checked everywhere this test reaches, including the owner dirs;
  the prose-doc split above is a review obligation the machine check does not cover.

## Fabric Git Invariant (warp + weft)

Every git operation that LYX/LoomYard's own code performs — on **either** the weft repo or the warp repo — goes through `internal/fabricengine` in Go, in-process, never raw git and never an LLM agent.
This binds LYX's own code only;
a human or any tool outside LYX keeps ordinary git in their warp worktree, untouched.

- **Module ownership.**
  Weft-internal git (`commit`/`push`/`pull`/`sync`) and coordinated warp↔weft topology (checkout, dual-worktree add/remove/clone) both go through `internal/fabricengine`.
  The same holds for warp: no LYX package other than `internal/fabricengine` runs raw git against warp.
  Read-only verbs (current SHA, `git status --porcelain`) are exempt — only *mutating* warp git must dispatch through fabric;
  see `fabric-unified-view.md`'s "Scope boundary" section for the current warp-mutation call sites.
- **Orchestration, not agent.**
  The weft commit is Go calling the engine in-process at a round/phase boundary the loop owner (loom, or perch's CLI standalone) controls — never an LLM agent, not raw git, not by shelling `lyx fabric`.
  Agents ride the file contract: they **write** overlay files into `_lyx` via the junction — raddle content lives at `_lyx/raddle/` and therefore arrives through this same junction;
  Go **reads and commits** them.
  An agent does commit its own code to the **warp** repo (commit-per-fix) — the weft, never.
  **Board carve-out:** `internal/boardengine`'s writes to `weft:main` are the one exception to timing control — any LLM session, in any worktree, may trigger a board write at any time — but module ownership still holds (board's git flows through `Bolt`, never raw git);
  only the *timing*-control half is scoped away.
- **Cross-module exclusions.**
  The `_lyx` tree is shared by every round-loop module, so every weft-commit caller passes a **positive-only** file list — no `:(exclude)` pathspec magic — built via `fabricengine.ScopedPathspec`.
  Machine-local artifacts (pause flags, fork prompts, module `*.lock` files) live under `.lyx` (see Durable-vs-Ephemeral State Invariant), never reaching a weft-commit pathspec.
  `fabricengine.seedWeftArtifactExcludes` covers the weft repo's never-tracked operational artifacts: fabric's own `.weft/` lock directory, gitrepo's push-lock file, `.lyx/`, and every module's `*.lock`/`*.swaplock` write- and swap-lock files.
  **Known limitation:** does not untrack an artifact a pre-fix sync already committed — `git rm --cached <path>` is the manual remedy.
- **Never-committed routing.** `structuralNeverCommittedDirs` membership makes a path uncommittable, filtered only where the pathspec is constructed (`ScopedPathspec` callers, via `pathspecNames`) — never in `Config.Dirs()`, `WiredNames`, or the slug-reservation union.
  `classifyPaths` routes such a path to a third bucket; `Commit` hard-errors on a non-empty third bucket rather than dropping silently.
  `weftPathspecFilter`'s `git ls-files` probe passes `--exclude-standard`.
- **Junction exclusion** goes through `.git/info/exclude` on both sides (warp: `WireJunctions`; weft: `seedWeftArtifactExcludes`), never a tracked `.gitignore`.
  That file lives in the repo's COMMON gitdir, so it is one repo-wide file, never per-worktree: an exclude entry is removed only once NO warp worktree in the hub still wires a junction of that name (`namesWiredInSiblingWorktrees`).
  Because it is repo-wide, every read-modify-write of it — warp and weft alike — goes through `fabricengine.mutateGitExclude`, which holds a repo-wide flock across read, rewrite and write and replaces the file by same-directory rename.
  No caller may read or write `info/exclude` directly: an unsynchronised `os.ReadFile`/`os.WriteFile` pair loses a sibling worktree's update, and `os.WriteFile`'s truncate-then-write lets a concurrent reader observe an empty file and write that emptiness back, destroying the operator's own exclude patterns along with fabric's junction exclusions.
- **Unwire** removes warp junctions and their warp `.git/info/exclude` entries only — weft-side `_lyx`/`.lyx` content is always preserved.
  Downgrade (a pre-fix binary's `applyStaleRemoval` against this change's output) is unsupported.
- **Enforced by** review obligation: agent prompt templates never mention the two-repo structure at all, per `templates-describe-one-repo` — stronger than merely never instructing a weft git op.
  Never-committed routing: `internal/fabricengine/classify_test.go`, `structuraldirs_test.go`, `internal/fabriccli/cli_test.go`.
  Junction exclusion / unwire: `internal/fabricengine/dotlyxjunction_integration_test.go`, `unwire_test.go`.
  Module ownership is machine-checked for `internal/boardengine` (`cmd/lyx/boardguard_test.go`) and for `internal/websterengine` (`cmd/lyx/rawgitmutation_test.go`, `TestNoRawGitMutation_WebsterProductionSource`);
  every other `fabricengine` caller remains a review obligation.
  The agent half is machine-checked for webster runs by `fabricengine.RefScanner` (a fork or Master Bash command matching a fabric-driving command spelling or the weft sibling worktree path is a hard, round-failing violation).

## Fabric Destruction Chokepoint Invariant

`internal/fabricengine/destroy.go` is the only file in `package fabricengine` permitted to perform a destructive primitive: `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D`, `fslink.Remove`, and a warp checkout's `ResetHard`.

- The banned bypass tokens are `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`, and `createdToken{`.
- Every destructive executor runs the gate's four checks first, always in this fixed order, stopping at the first failure: containment, ownership, dirtiness, force.
- `--force` answers dirtiness only.
  It never satisfies containment and never satisfies ownership.
- A gate refusal (`*destructiveRefusal`) is never discarded on a best-effort path — every such site wraps its executor call in `surfaceRefusal` (or, where the call site cannot return an error at all, logs the refusal via `logger.Warn`) rather than swallowing it.
- Every entry on the guard's per-file allowlist carries a reason.
- **Known guard blind spot:** the check is raw substring matching, so an alternative argument-slice spelling with different spacing, a dynamically built argument slice, and aliasing a raw repo handle to a local all evade it, and the allowlist is per-file, so a new raw call added inside an already-allowlisted file is not caught.
  A shared static-analysis-guard framework (issue #135) would close this class of blind spot repo-wide;
  this invariant does not resolve that question.
- **Enforced by** `cmd/lyx/destructiveguard_test.go` (`TestNoDestructiveBypass_FabricengineProductionSource`).

## Markdown Link Integrity

Every inline markdown link (`[text](target)`) in a `.md` file under `manifest/` or `docs/` resolves — both its file part and, for a `.md` target carrying one, its `#anchor`.

- **The root restriction is source-side only.**
  `manifest/` and `docs/` name which files are *scanned* for outgoing links;
  they do not restrict where those links may *point*.
  Every link target is resolved wherever it lands in the repo, and any `.md` target gets its `#anchor` resolved too, whether that target sits inside `manifest/`/`docs/` or not.
  Reading the root restriction as licence to skip anchor resolution for an out-of-root target would silently un-guard `finalize.md`'s `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft` link and the `../../internal/*/doc.go` targets this task creates.
- **A file-layout convenience, not an ownership claim.**
  The enforcing test lives in `internal/lyxcwd` (`docslink_test.go`'s `TestEnforcement_MarkdownLinks`), reusing that package's `repoRootForEnforcement` and `walkEnforcementRoots` helpers.
  That placement is a file-layout convenience, not an ownership claim on markdown links by `internal/lyxcwd` — the Cwd Resolution Invariant scopes that package to cwd resolution and nothing else, exactly the caveat the Fabric Vocabulary Invariant above already states for its own test.
- **What the machine check does and does not reach — stated honestly, not implying full coverage.**
  Not reached: external `http`/`https`/`mailto` URLs, never fetched;
  reference-style links (`[text][ref]`) and `<...>` autolinks, out of grammar by decision, not by oversight;
  link-shaped text inside fenced code blocks, deliberately skipped;
  prose mentions of a filename that are not markdown links — `manifest/roadmap.md:98`'s `scout-redesign.md` reference is a live example this task leaves standing;
  and `.md` files outside `manifest/` and `docs/` as **scan sources**, so `README.md`, `CLAUDE.md`, and `internal/**/*.md` have their own outgoing links checked by nobody.
- **The allowlist's self-expiring contract.**
  Keyed by `(file, target)`, never by line number, with every entry naming its owning task.
  An entry whose key is not matched by any break in a scan is reported as deletable — this covers both a link that was fixed and a keyed file that was renamed or deleted away, since neither case is ever visited by the scan again.
- **Enforced by** `internal/lyxcwd/docslink_test.go` (`TestEnforcement_MarkdownLinks`).

## Review Round Invariant

One review+fix round (burler now, hardener later) follows: A-before-B (review fully written to disk before any target file is touched);
every recorded finding is fixed in B, all severities including LOW/NIT;
no self-grading (round N's fix is judged by round N+1's fresh review, never its own);
commit-per-fix on warp source, never push.
In a cluster round, fork reports, the handler's own holistic review, and the consolidation into one review file are ALL part of A;
fork reviewers are read-only (no writes, no git), mechanically enforced by the fork audit.

- **Enforced by** `internal/burlerengine/template_test.go` (`TestTemplate_StatesRoundDiscipline`, `TestTemplate_StatesClusterForkDiscipline`, `TestTemplate_OrchestratorExcludesDownstreamBodies`).
  No-self-grading and commit-per-fix discipline are review obligations, not machine-checked.

## Live-Substrate Spawn Observability

Any code path that starts a real OS process on behalf of a round/strand/session (a tmux server, a provider session, any subprocess) logs the spawn and its teardown via `internal/logger` — `logger.Info` for normal spawn/teardown events, `logger.Warn` for a retry or a teardown that did not confirm clean.
The durable Info+ trace-file sink captures these regardless of verbosity or env-var configuration (under `go test`, gated by `LYX_TRACE=1`).

- A new spawn point for a live-substrate module must add its own `logger.Info`/`Warn` call in the same change — review obligation, not machine-enforced.
- A spawned pane/child must never re-exec `os.Executable()` while running under `go test`: a Go test binary invoked with positional arguments does not error on them, so the arguments are silently ignored and the full suite runs unfiltered.
  Guarded by `reedengine`'s `headerLaunchLine` (suppresses header re-exec when `testing.Testing()`) and `lyxtest.HermeticGitEnv` (`refuseCLIReexec` refuses any test binary invoked with a leading positional argument).
- A retry loop around a real process spawn must cap attempt COUNT, not only elapsed time — a fast-failing spawn burns a time-only budget in far more attempts than it was sized for. `maxBootAttempts` in `internal/reedengine/lifecycle.go` is the pattern: track an attempt counter, exit on whichever of (time, count) is hit first.
- Known instrumented call sites: `internal/reedengine/lifecycle.go`, `internal/shuttleengine/run.go`, `internal/burlerengine/engine.go`, `internal/scoutengine/ensureserver.go`.

## Sandbox Suite Coverage

Every registered lyx module must be exercised by the black-box sandbox suite or be explicitly excluded with a reason.

- **Tagging.**
  A scenario in any suite file (`tools/sandbox/*SUITE.md`) that drives a specific module declares it with a `**Covers:** <module>[, <module>...]` line.
  Coverage is checked at module granularity against the live cobra root (`newRoot().Commands()`, skipping `help`/`completion`).
- **Allowlist.**
  Modules intentionally never sandbox-exercised are named on `excludedModules` with a one-line reason: `ide`, `selfreport`, `scout` today.
- **Exists ⇒ covered or excluded.**
  A new registered module needs either a `**Covers:**` scenario or a new allowlist entry with a reason.
- **Enforced by** `cmd/lyx/sandbox_coverage_test.go` (`TestSandboxCoverage_AllModulesCoveredOrExcluded`).

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns — no `git init`/`git worktree add`/fixture-tree copies;
Tier 1 stays offline and fast.

- A test file whose first non-empty line is not a `//go:build` constraint mentioning `integration`, `smoke`, or `scout` is "untagged" and must not call `gitexec.RunGit`, `exec.Command`/`exec.CommandContext`, or `lyxtest.Copy*`.
  Raw substring match — a comment or string-literal mention also trips it.
- Substrate definition (real git/tmux/filesystem/cross-compile/external-binary spawn) lives in `docs/benchmarks/running-tests.md`'s "## The two tiers" section.
- Allowlist: `internal/proc` (its tests must spawn), `cmd/lyx/tierpurity_test.go` itself (carries the banned tokens as test data).
- Additive real-time-wait guard: an untagged file's `time.Sleep(...)` with a compile-time-constant duration ≥ 1s is flagged unless allowlisted (`allowedLongSleepers` in `cmd/lyx/tiersleep_test.go`);
  an unresolvable duration expression is conservatively flagged too.
- **Enforced by** `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`).

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git — directly or via lyxtest fixture helpers — runs under the hermetic git test environment, so no test behaviour depends on the operator's `~/.gitconfig` or the system gitconfig.

- A package is "git-spawning" when any `*_test.go` file spawns git directly (`gitexec.RunGit`, `exec.Command`/`exec.CommandContext`) or indirectly via a lyxtest fixture helper (`lyxtest.Copy*`, `lyxtest.MustRun`, `lyxtest.SeedConfig`).
  Every such package must have a `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`, or be allowlisted.
- Allowlist: `internal/proc` (spawns non-git processes).
- **Enforced by** `cmd/lyx/hermeticenv_test.go` (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) — proves presence of the call only;
  a real, correctly-ordered `TestMain` is a review obligation.

## Dev/Prod Binary Separation

The sandbox tooling resolves the dev binary from the derived `.dev-bin` (falling back to PATH) through `resolveLyx`, never a bare-PATH `lyx` lookup that could silently resolve prod.

- `resolveLyx` (`tools/sandbox/resolve.go`) is the single allowlisted resolution site: checks `.dev-bin/lyx` first, falls back to `lookPath("lyx")`.
  Covers both `lookPath("lyx")` and the separator-free `exec.Command("lyx", …)`/`exec.CommandContext("lyx", …)` form.
- The dev binary (`tools/deploy -dev`) builds into `<repoRoot>/.dev-bin` (gitignored), never the production install location.
- `.dev-bin` is prepended only to the agent child-process PATH (`launchAgent`), never the operator's own PATH.
- **Enforced by** `tools/sandbox/pathresolve_guard_test.go` (`TestPathResolveGuard_NoBarePathLyxOutsideResolve`) for the mechanical half;
  agent-only PATH prepend and never-installed-to-prod are review obligations.

## Planparser Sole-Parser Invariant

`internal/planparser` is the SOLE parser of the on-disk plan format (`_lyx/plan/`).

- No other package parses `00-overview.md`/`NN-<card-slug>.md`;
  consumers read plan-level sections only from the `planparser.Plan` model a caller hands in.
- Resolves `_lyx/plan/` via `lyxcwd`, never string literals.
- **Enforced by** review obligation today (candidate future import/grep guard).

## Batcher Registry+Config Invariant

webster's execution unit is the batchifier-derived batch, not the raw plan card.

- Batching is selected by `internal/batcher`'s name-keyed registry plus the `batcher:` webster.yaml config key (default `identity`) — no plan-supplied batching, no batch grouping in the plan format itself.
- **Enforced by** review obligation.

## GitHub Auth Invariant

All GitHub authentication goes through `internal/githubclient`;
no other production package shells out to `gh`.

- Token resolution, token caching, and construction of an authenticated `*github.Client` live solely in `internal/githubclient`.
  No other production package invokes `gh` (via `exec.Command`/`exec.CommandContext` or bare `LookPath("gh")`) or otherwise builds its own GitHub credential path.
- `internal/githubclient` production imports are allowlisted to stdlib, `go-github`, `golang.org/x/sys`, and `internal/proc`.
- **Enforced by** `cmd/lyx/ghguard_test.go` (`TestGHGuard_NoShellOutOutsideGithubclient`) and `internal/githubclient/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`).

## gitrepo Client Boundary Invariant

`internal/gitrepo` splits local-vs-remote by client: go-git owns local object and ref access, `gitexec` owns anything that authenticates to a remote or mutates the working tree.

- go-git handles reads that resolve state already on disk — commit/tree/blob lookups and ref reads. `gitexec` is the only path to the git CLI, used for `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `Fetch`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, `IsAncestor`, `HasUnpushed`.
  Any new `gitexec` call added inside `internal/gitrepo` must update this list in the same commit.
- Known guard blind spot: the check is set-equality on method names, so a new `r.run` call slipped inside an already-pinned method is not caught — per-call review still applies to those methods.
- **Enforced by** `cmd/lyx/gitrepoboundary_test.go` (`TestGitrepoBoundary_PinnedRunCallSites`).

## Never Force-Add Invariant

Fabric/gitrepo never runs `git add -f`.

Transients are kept out of the index by each repo's own `.git/info/exclude` (warp: `seedGitExclude`;
weft: `seedWeftArtifactExcludes`), never by force-adding past them and never by per-call `:(exclude)` pathspec magic.

This is enforced structurally — `gitrepo.StageAndCommit` has no `-f` code path at all — plus a machine-checked grep guard against its reintroduction.

- **Enforced by** `internal/gitrepo/noforceadd_test.go` (`TestNoForceAdd_GitrepoSourceHasNoForceAddBranch`).

## Documentation Lifecycle

Which docs are kept vs deleted (mechanical per-module docs vs durable design docs): see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Files included (N=53)

- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/00-overview.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/01-dirtiness-probe.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/02-the-gate.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/03-path-callsites.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/04-clone-callsites.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/05-branch-callsites.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/06-guard-and-docs.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/07-gap-integration-tests.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/dirtiness.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/add.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/checkout.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/prune.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/pull.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/remove.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpclean.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/reconcile.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destroy.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpforward.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destroy_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/launchers.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/portals.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fslink/fslink_linux.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fslink/fslink_windows.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/weftwiring.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/weftwiring_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/unwire.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junction.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junction_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/cleanup.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/destructiveguard_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/tierpurity_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/CONSTRAINTS.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/doc.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/manifest/designs/fabric-crucible-followups.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/manifest/roadmap.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/export_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destructivegaps_integration_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/discussion.md
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/slug.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/worktreelist.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/ancestors.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/fabric.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/open.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junctionnames.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/rawgitmutation_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpprobe.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/gitexclude.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/index.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/hook.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/prune_unowned_integration_test.go
- /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/testmain_test.go

## Plan + source files to review
- Overview: `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/00-overview.md`
- Batch file(s):
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/01-dirtiness-probe.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/02-the-gate.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/03-path-callsites.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/04-clone-callsites.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/05-branch-callsites.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/06-guard-and-docs.md`
  - `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/plan/07-gap-integration-tests.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/dirtiness.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/add.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/checkout.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/prune.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/pull.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/remove.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpclean.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/reconcile.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destroy.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpforward.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destroy_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/launchers.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/portals.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fslink/fslink_linux.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fslink/fslink_windows.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/weftwiring.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/weftwiring_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/unwire.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junction.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junction_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/cleanup.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/destructiveguard_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/tierpurity_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/CONSTRAINTS.md`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/doc.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/manifest/designs/fabric-crucible-followups.md`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/manifest/roadmap.md`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/export_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/destructivegaps_integration_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/discussion.md`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/slug.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/worktreelist.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/ancestors.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/fabric.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/open.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/clone_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/junctionnames.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/cmd/lyx/rawgitmutation_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/warpprobe.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/gitexclude.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/index.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/hook.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/prune_unowned_integration_test.go`
- `/home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/internal/fabricengine/testmain_test.go`

## Source-grounding rule

**Never guess.**
A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt.
Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list.
If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan;
do not emit NEED_CONTEXT for files in the manifest.
Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path).
The orchestrator will re-fire the review with those files added.
Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the implementation as a whole)

- **End-to-end plan alignment** — every batch's cards are realised;
  every file listed across all batches' `Context:`/`Edits:`/`Creates:` is present in the source files provided.
- **Shared-decisions alignment** — the `## Shared Decisions` subsections are applied consistently across all batches;
  deviation is BLOCKING.
- **Out-of-plan files** — BLOCKING if any source file is present that is not accounted for in any batch's reference lists.
  If the implementer added it, the batch file must have been updated first;
  a review with surprise files means that discipline was skipped somewhere.
- **Cross-batch contracts** — interfaces produced by one batch and consumed by another are compatible.
  Dependency order implied by `depends-on:` is reflected in the code (consumers don't assume behaviour the producer doesn't guarantee).
- **Integration correctness** — the pieces work together, not just per-batch.
  Call sites match signatures;
  shared state is consistently managed;
  error surfaces compose.
- **Global utility duplication** — BLOCKING if two batches independently reimplement the same helper.
  Consolidate into a shared module.
- **Test coverage across the whole surface** — happy paths + errors for every batch's entry point.
  Integration tests reach across batch boundaries where appropriate.
- **Constraint violations** — BLOCKING.
- **Codebase consistency** — naming, error handling, imports, and style match the conventions visible in the source files provided.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line.
Everything outside these markers is ignored by the backend.
**No preamble inside the markers.**
Per finding: 3–5 lines, short and factual.
Cite file and line, state the issue, propose the fix.

Target length: ~400 tokens for APPROVE, ~800–1500 tokens for REQUEST_CHANGES across multiple batches.
If you produce more than ~1800 tokens, compress.

~~~markdown
MILL_REVIEW_BEGIN
# Review: fabric: one ownership-and-dirtiness gate for all destruction (slice 12) — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING:design] <short title, <60 chars>
**Location:** `path/to/file.py:42` (or `:42-58`)
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT:consistency] <short title>
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

**Severity vocabulary is closed.**
Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word (e.g. `MAJOR`, `MINOR`, `CRITICAL`, `MEDIUM`, `HIGH`).
If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT` -- an over-cautious BLOCKING can be pushed back on by the orchestrator;
a mislabeled NIT (or an unrecognized label) can silently skip review entirely.

**Class is the second axis, encoded in the same bracket as severity, colon-separated, lowercase: `### [BLOCKING:design] <title>`.**
A finding with no class, or a class outside the four names below, is a reviewer defect.
The four recognised classes, identical in meaning across every review stage:

- `design` — a decision is missing, wrong, or rests on a false premise.
  Example: the implementation fixes the symptom at one call site but never resolves which layer owns the validation.
- `scope` — the work inventory is incomplete, or the enumeration method is unreliable.
  Example: a card's `Edits:` file was converted but a sibling file with the identical helper was left unconverted.
- `decision` — a named artifact with no stated disposition.
  Example: a config key the plan introduced is added but never wired into the loader that reads it.
- `consistency` — the artefact contradicts itself, carries a superseded statement, or violates an established repo convention.
  Example: two batches' implementations of the same interface handle the error case differently.

**Class governs who decides and when the loop stops, never whether a finding gets fixed.**

Omit `## Findings` if zero findings.
Never invent findings to pad.

## Out of scope for this stage

- Re-litigating a decision already recorded in `discussion.md` is out of scope unless new evidence contradicts it.


---

## Output contract

Write your full report to this file: /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/briefs/review-code-holistic-r1.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/briefs/review-code-holistic-r1.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/hanf/Code/loomyard/wts/fabric-destructive-chokepoint/_mill/briefs/review-code-holistic-r1.out.md`
