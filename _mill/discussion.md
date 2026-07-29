# Discussion: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
task: 'fabric: Fabric.Commit classify+dispatch + unified diff/status'
slug: fabric-commit-api
status: discussing
parent: main
```

## Problem

`fabric` operates two git repos — the host repo (warp) and its paired weft repo — but its whole job is to make them look, from the outside, like one flat repo. Today a caller that writes files has to know which of the two repos each file physically lives in: host source goes through the warp repo (ordinary `git`), while `_lyx`/`_pattern` overlay content goes through `fabricengine.CommitWeft`. There is no single "commit these files" call that classifies each path and dispatches it to the right repo. This slice (slice 2 of `manifest/designs/fabric-unified-view.md`'s build order) adds that call — `Fabric.Commit([files], msg, snapshotTags)` — plus a unified `Fabric.Diff`/`Status` that merges a per-repo diff across both repos via the correspondence index, so a caller (Go orchestration today, "everything in LYX that writes files" over time) never has to know which repo a file belongs to.

**Why now:** the illusion-first framing of `fabric-unified-view.md` gives `Fabric.Commit` its caller (API uniformity *is* the value — not correspondence, which is recorded weft-side regardless). Slice 1 (config-driven junction list) has landed; `board: move storage to weft:main` and `native clients` (go-git `gitrepo`) are done, so this builds on the final `gitrepo`. This slice is also where the design's flagged open question — the atomicity / partial-failure story for a two-sided commit — must be resolved.

## Scope

**In:**

- `Fabric.Commit(files []string, msg string, snapshotTags []string, opts SyncOptions) (CommitResult, error)` — a new Go method on `*fabricengine.Fabric` (`internal/fabricengine`). Classifies each path as warp-side or weft-side, commits the warp side then the weft side (warp-first), and fires a detached, async push of both repos before returning. Records correspondence for the weft commit exactly as `CommitWeft` does today.
- A pure, unit-testable path classifier: given the layout's `RelPath` and the fabric-config junction name-set (`cfg.Dirs()`), route each worktree-root-relative path to warp or weft.
- `snapshotTags []string` parameter, wired through to write a `Snapshot: <tag>` trailer per tag on the weft commit (alongside the existing `Warp-SHA` trailer). This is the write side only — the consumer/baseline-derivation and `refs/loomyard/snapshot/` retirement are slice 3.
- `CommitResult` return struct + a typed partial-failure error.
- Unified `Fabric.Diff(sinceWarpSHA) ` (committed changes since a point, merged across both repos) and `Fabric.Status()` (uncommitted working-tree changes, merged across both repos), Go-internal API only.
- A detached async-push mechanism for Fabric.Commit that pushes **both** the host and weft repos, mirroring board's engine-spawned detached `lyx <module> sync` pattern (`internal/boardengine/spawn.go` + `sync.go`).
- Docs in the same commit: `internal/fabricengine/doc.go`, the `fabric-unified-view.md` slice-2 line (mark DONE), and the forward-reference in `CONSTRAINTS.md`'s Weft Git Invariant ("who may time a weft commit … once `Fabric.Commit` lands").

**Out:**

- Migrating existing weft-commit callers (`internal/buildercli`, `internal/webstercli`, `internal/perchcli`) onto `Fabric.Commit`. This is pure API addition; caller migration is a later, separate concern.
- Any new CLI verb for the classify+dispatch commit or the unified diff/status (`Fabric.Commit`/`Diff`/`Status` are Go-internal only). The existing `lyx fabric commit`/`status`/`sync` weft-only verbs (`internal/fabriccli/weft_verbs.go`) stay exactly as they are.
- Snapshot **consumption**: deriving a snapshot baseline from tagged weft history and retiring the `refs/loomyard/snapshot/` ref mechanism — slice 3.
- Warp-rebase / remote-reconcile — slice 5. This slice's async push is deliberately best-effort and does not attempt any conflict resolution.
- Multi-subpath support, clone-does-everything, `init` dissolution — slices 4+.
- Any cross-repo transaction / two-phase-commit machinery. Two-sided `Fabric.Commit` is two git commits; the honest partial-failure story below is the whole answer.

## Decisions

### deliverable-is-go-api-only

- Decision: Slice 2 adds only the Go API (`Fabric.Commit`, `Fabric.Diff`, `Fabric.Status`, the classifier, `CommitResult`). No caller migration, no CLI verb.
- Rationale: The build-order line calls this "pure API additions over the existing `Warp`/`Weft` handles, `CommitWeft`, and `ChangedFilesSince`." Migrating live callers and exposing CLI are independently-scoped follow-ups that would balloon the slice.
- Rejected: (a) also migrate builder/webster/perch `weftCommit` callers; (b) add a `lyx fabric commit <files> -m` CLI verb. Both deferred.

### warp-first-ordering

- Decision: A two-sided `Fabric.Commit` commits the **warp** side first, then the **weft** side.
- Rationale: The weft commit is a piggyback whose `Warp-SHA` trailer records the warp commit it corresponds to. For the trailer to name the warp commit that includes *this* `Fabric.Commit`'s warp-side files, warp must commit first. This falls out with near-zero new code: `CommitWeft` already reads warp's current HEAD (`warpHeadSHA` → `f.Warp.CurrentSHA`) for its trailer, so committing warp first and then calling the existing weft-commit path makes the weft trailer point at the new warp SHA automatically.
- Rejected: weft-first (trailer would point at pre-change warp HEAD — correct for correspondence in isolation, but wrong for "these two sides are one logical change"); any rollback-on-failure scheme (see partial-failure decision).

### partial-failure-report-not-rollback

- Decision: No cross-repo transaction. Warp commits first; if the warp commit fails, nothing is attempted on weft and the error propagates (nothing landed). If the warp commit succeeds but the weft commit then fails, the warp commit stays (it is ordinary host git), and `Fabric.Commit` returns a `CommitResult` with `WarpSHA` populated plus a typed partial-failure error naming the landed warp SHA and wrapping the weft error. No auto-rollback / `reset --soft`.
- Rationale: A landed warp commit is a legitimate plain host commit regardless of what weft does; unwinding it would be more surprising than leaving it. The weft side can be re-driven later — the next `Fabric.Commit`/`CommitWeft` reads warp's current HEAD and re-anchors the trailer; the correspondence index self-heals. Matches raddle's "advance only on confirmed success" and the design's "honest cost of the illusion."
- Rejected: best-effort rollback of the first commit on second-commit failure — fake transactionality, and destructive to a valid host commit.

### warp-only-commit-is-plain-git

- Decision: The warp side of `Fabric.Commit` is byte-for-byte a plain `git add` + `git commit` in the host repo: bare commit message (no trailer), no correspondence entry, no fabric write-lock, no exclude-seeding, no lyx marker of any kind. It goes straight through `f.Warp.StageAndCommit(msg, files)`. A `Fabric.Commit` whose files all classify to the warp side degenerates to a single warp-only commit; one whose files all classify to weft degenerates to a single weft-only commit. Both are legitimate.
- Rationale: The host repo is "unrestricted, an ordinary project repo" (Weft Git Invariant). The user works with collaborators who do **not** use lyx-initiated repos, and a warp-only commit must be indistinguishable from a hand-run `git commit` so those repos and workflows stay first-class. Correspondence is one-directional (weft records warp), so warp never needs to route through fabric for the illusion to hold.
- Rejected: wrapping the host commit with fabric's write-lock/trailer/exclude machinery for symmetry — it would pollute an ordinary repo and break the "warp stays ordinary git" property.

### classification-input-contract

- Decision: `Fabric.Commit` accepts paths **relative to the worktree root**. A path is weft-side iff it falls under one of the `RelPath`-scoped junction directories — i.e. it has a path-prefix under one of `ScopedPathspec(l.RelPath, cfg.Dirs())` (`<RelPath>/_lyx`, `<RelPath>/_pattern`, and any future fabric-config-listed name). Everything else is warp-side. The classifier is a pure function (layout `RelPath` + name-set + the path list → two path lists), with no git or filesystem I/O.
- Rationale: Reuses the exact pathspec the fabric config already defines (`cfg.Dirs()`, filtered against `hubgeometry.HubReservedNames()` as `junctionNames`/`WiredNames` do). Prefix-match is the "trivial path-prefix check" the design names. The caller passes the same relative paths it used to write the files.
- Rejected: absolute-path classification by comparing against the two worktree roots (needs I/O, more failure modes); cwd-relative paths (ambiguous, needs resolution).

### commit-result-and-message

- Decision: Return a lean `CommitResult` struct — `{WarpSHA string; WarpCommitted bool; WeftSHA string; WeftCommitted bool}` — and put partial-failure detail in a typed error (e.g. `*PartialCommitError` naming `WarpSHA` and wrapping the weft error). On full success the caller can ignore the struct; it carries enough to report exactly which side(s) landed and to hand the weft SHA to a later push/lookup.
- Rationale: The user's stated need — "ok is enough if everything goes to plan; I only need detail on failure." The struct gives a clean success return and degrades cleanly to single-sided commits; the failure detail lives in the error where a caller looks for it.
- Message handling: the single `msg` argument applies to both commits. The **warp** commit uses the bare `msg`. The **weft** commit uses `msg` + appended `Warp-SHA:` trailer (existing `appendWarpSHATrailer`) + one `Snapshot: <tag>` trailer per `snapshotTags` entry.
- Rejected: positional `(warpSHA, weftSHA, err)`; a single "primary" SHA (loses which side committed).

### snapshot-trailer-written-now

- Decision: Slice 2 includes the `snapshotTags []string` parameter **and** writes a `Snapshot: <tag>` trailer per tag on the weft commit. The consumer side (deriving a snapshot baseline from the latest weft commit carrying a tag, retiring `refs/loomyard/snapshot/`) stays slice 3.
- Rationale: A parameter nothing writes is dead weight. Writing the trailer now is mechanically trivial (more trailer lines on the weft commit), inert until a reader exists (like `_pattern` content nothing pushes yet), and is exactly the head-start that makes slice 3 easier. The user asked to "take it with here, so slice 3 is easier."
- Rejected: accept-but-ignore the parameter (dead param); pull the full slice-3 consumer work forward (scope creep).

### async-push-both-sides-detached

- Decision: `Fabric.Commit` commits synchronously (fast, local), then fires a **detached, fire-and-forget** push of **both** the host and weft repos, and returns immediately. Only the push is async; the commit + correspondence recording are synchronous so the SHAs and `CommitResult` are real on return. The async push honors `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` (no child forked when set), mirroring `spawnPush`/`spawnSync`.
- Rationale: The user's model — "board did local things fast and pushed async when it had time." Board's foreground returns immediately and a detached `lyx board sync` child does the network work; here the commit must stay foreground (its whole contract), so only the slow network push is deferred. Both repos are `*gitrepo.Repo` with `PushCoalesced`, so pushing both is uniform.
- Best-effort semantics: the async push is fire-and-forget. A warp non-fast-forward failure (a non-lyx collaborator advanced the warp remote, or a rebase) is silent and non-fatal — warp reconciliation is the human's job / slice 5, and fabric "stays correct under arbitrary external warp activity." A weft push is lyx-owned and conflicts are rare; `PushCoalesced` serializes concurrent pushes.
- Cross-machine consequence (accepted): weft may be pushed while warp is not (or vice-versa), so another machine pulling weft can see a `Warp-SHA` trailer for a warp commit it does not have yet. This is the drift the existing post-checkout hook already **detects** (not silently mishandles); the correspondence index's `SHAExists`/rebuild path tolerates it.
- Mechanism: mirror board — an engine-level detached spawn in `internal/fabricengine` that launches a detached `lyx fabric` push child covering both paths, via `proc.Detach` + `os.Executable`. The existing weft-only push bypass (`internal/fabriccli/weft_verbs.go`'s hidden `--weft-path` + `PushWeftAt`) is the model to extend (e.g. a hidden `--warp-path` companion so the child pushes both); reuse/consolidate with the existing `fabriccli.spawnPush` rather than duplicating if practical. See open item below.
- Rejected: weft-only async push (user chose both); synchronous push (blocks the caller — the exact cost the user is avoiding); a background goroutine (dies when a short-lived process exits; the detached child is what survives).

### commit-only-not-separate-sync

- Decision: `Fabric.Commit` itself performs the async push (option 1), rather than splitting a commit-only `Commit` from a `Sync` that adds the push.
- Rationale: The "everything in LYX that writes files" caller wants one call whose contract is "the write is durable locally and will reach the remote." One entry point is simpler for that caller.
- Rejected: separate `Fabric.Commit` (commit-only) + `Fabric.Sync` (commit + async push).

### skip-git-weft-scoped

- Decision: `Fabric.Commit` takes `opts SyncOptions`. The **weft** side honors `opts` exactly as `CommitWeft` does today (`WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` bypass). The **warp** side always commits — it is ordinary host git and the `WEFT_*` bypass is weft-scoped with no warp analogue. The async push honors the skip envs for both sides (no child forked).
- Rationale: The skip gate is the weft/CI test bypass; the warp commit is real host git that tests either exercise or simply do not pass warp files to.
- Rejected: gating both sides with `opts` (would make a warp-only `Fabric.Commit` silently skip under the weft CI env); no `opts` at all (breaks the existing test/CI bypass contract).

### unified-diff-status-warp-anchor

- Decision: `Fabric.Diff(sinceWarpSHA)` reports committed changes since a warp SHA, merged across both repos: warp changes = `f.Warp.ChangedFilesSince(sinceWarpSHA)`; weft changes = `f.Weft.ChangedFilesSince(f.WeftSHAForWarpSHA(sinceWarpSHA))`. The result is one merged list, each entry labelled with its side (warp/weft). `Fabric.Status()` is the uncommitted/working-tree variant of the same merge (each repo's current dirty state, merged). Both are Go-internal only.
- Rationale: A warp SHA is the natural user-facing anchor; the correspondence index (`WeftSHAForWarpSHA`) already bridges to the weft SHA. Reuses `ChangedFilesSince` (no new git primitive), matching the design's "ordinary per-repo diff merged via the correspondence index, not a new primitive."
- No-correspondence degradation: when `sinceWarpSHA` has no recorded weft correspondence (index miss — e.g. a warp SHA older than the first weft commit, or pre-lyx history), `Fabric.Diff` returns the warp-side diff plus an empty weft side and a flag/field noting "no weft correspondence for this anchor" — **not** an error. It is a valid state; the caller decides what to do.
- Rejected: a caller-supplied `(warpSHA, weftSHA)` pair (bypasses the index the design says to use); returning `ErrNoCorrespondence` on miss (a common, valid state for pre-lyx history should not be an error); dropping `Status` (the user kept it in Q8).

## Technical context

Everything below is in `internal/fabricengine` unless noted. Key files and reuse points, all read during exploration:

- `fabric.go` — the `Fabric` handle. `Fabric` exposes `Warp *gitrepo.Repo` and `Weft *gitrepo.Repo` as **public fields** (no per-operation forwarding methods; only genuinely cross-repo ops get a method). `New(warpPath, weftPath) (*Fabric, error)` stat-checks both dirs. `SyncOptions{SkipGit, SkipPush}` + `EnvSyncOptions()` read `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`. `ScopedPathspec(relPath, dirs) []string` joins `relPath` with each dir — this is the raw material for the classifier. `DefaultCommitMessage = "weft sync"`. `Fabric` also has unexported `warpPath`/`weftPath` fields (needed for the async-push spawn).
- `weftgit.go` — `CommitWeft(pathspec []string, message string, opts SyncOptions) (sha string, committed bool, err error)`: acquires the fabric write-lock (`ensureWeftLockDir` → `.weft/weft.write.lock`), reads warp HEAD via `warpHeadSHA` (tolerates unborn warp HEAD — no trailer/record then), filters the pathspec (`weftPathspecFilter` drops non-matching positive entries; returns `("", false, nil)` if no positive entry survives), appends the `Warp-SHA` trailer (`appendWarpSHATrailer`), commits via `f.Weft.StageAndCommit`, then calls `RecordCorrespondence`. `Fabric.Commit`'s weft side should reuse this path (extended to also append `Snapshot:` trailers). `ensureWeftLockDir` also seeds `crossModuleMachineLocalExcludes` into the weft repo's `.git/info/exclude` — the single choke point for machine-local artifact exclusion; the weft side of `Fabric.Commit` inherits this by going through `CommitWeft`. Also here: `PushWeft(opts)` (= `f.Weft.PushCoalesced()`), `PullWeft`, `StatusWeft(pathspec)` (dirty/ahead/behind), package-level `PushWeftAt(weftPath, opts)` and `CommitWeftAt(weftPath, msg, opts)` (board's warp-untethered wildcard-stage commit — no trailer, no correspondence).
- `index.go` — `RecordCorrespondence(warpSHA, weftSHA) error`, `WeftSHAForWarpSHA(warpSHA) (string, error)` (returns `ErrNoCorrespondence` on miss, rebuilds once and retries on a stale cached weft SHA, returns `ErrStaleSHA` if still unresolved), `RebuildIndex() error`. `WeftSHAForWarpSHA` is the bridge for `Fabric.Diff`.
- `corrindex.go` / `trailer.go` — the git-free sorted correspondence cache (`corrEntry{WarpSHA, WeftSHA, WarpSeq}`) and the `Warp-SHA:` trailer read/write helpers. `Snapshot:` trailer writing should live alongside `appendWarpSHATrailer` in `trailer.go`.
- `status.go` — `Topology.Status(l *hubgeometry.Layout) (StatusResult, error)`: the host↔weft **pairs** status (branch pairing, junction health, host-pollution). This is a *different* thing from `StatusWeft` (content-sync dirty/ahead/behind) and from the new unified `Fabric.Status` ("what changed in my worktree"). Do not conflate the three; the new one is a fourth, thin merge over `ChangedFilesSince` / working-tree status.
- `junctionnames.go` — `junctionNames`/`WiredNames`/`cfg.Dirs()`: the fabric-config junction name-set, filtered against `hubgeometry.HubReservedNames()`. There is **no** existing per-file "classify weft-vs-host" helper — this slice adds it.
- `internal/gitrepo/gitrepo.go` — `StageAndCommit(msg string, files []string) (sha string, committed bool, err error)`, `StageAllAndCommit(msg string)`, `CurrentSHA()`, `ChangedFilesSince(sha string) ([]string, error)` (returns `ErrInvalidSHA` on a bad SHA; reports the changed-path set, non-contractual order, verbatim non-ASCII, both paths on a rename). `push.go` — `Push()`, `PushCoalesced()` (serialized via `.gitrepo-push.lock`). The warp side uses `StageAndCommit` directly.
- Board's async-push precedent to mirror: `internal/boardengine/spawn.go` (`spawnSync` — detached `lyx board sync` via `proc.Detach` + `os.Executable`) and `internal/boardengine/sync.go` (`CommitWeftAt` + `PushWeftAt`, drains until nothing is left). `internal/fabriccli/spawn.go`'s `spawnPush` launches a detached `lyx fabric --weft-path <abs> push`; `internal/fabriccli/weft_verbs.go` wires the hidden `--weft-path` bypass and the `sync` verb (`CommitWeft` + `spawnPush`).
- `internal/proc` — `proc.Detach(cmd)` (own process group, survives parent exit, windowless). Used by both existing spawn sites.

Layering note: board's detached-spawn helper lives in the **engine** (`boardengine`), which establishes that a `*engine` package may spawn a detached `lyx <module> …` child via `os.Executable`. So a fabric async-push spawn helper may live in `fabricengine`. The child it spawns must reuse an existing `lyx fabric` push path extended to cover both repos (a hidden `--warp-path` alongside the existing hidden `--weft-path`), not a new user-facing verb — consistent with "no new CLI verb."

## Constraints

From `CONSTRAINTS.md` (read this session):

- **Weft Git Invariant** — every git op on the weft repo goes through `internal/fabricengine` in Go, orchestration-driven, never raw git and never an LLM agent. `Fabric.Commit`'s weft side satisfies this by going through `CommitWeft`/`StageAndCommit`. The **timing-control** half ("an LLM agent never drives weft git; weft-commit timing is orchestration's call") is preserved as deliberate *policy*: `Fabric.Commit` is a Go API called by orchestration; agent prompt templates must never instruct invoking it. No code guard is added for this — it is policy, not mechanism (the old accidental `git add`-fails guardrail is intentionally not reintroduced). This slice is where the invariant's own forward-reference — "who may time a weft commit gets a fuller treatment once `Fabric.Commit` lands" — is discharged; update that bullet in the same commit. The anchored-exclusion / cross-module-exclusion rules are inherited unchanged via `CommitWeft`'s `ensureWeftLockDir` choke point.
- **Hub Geometry Invariant** — `internal/hubgeometry` owns all path construction and the geometry tokens (`_lyx`, `_pattern`, `_board`, …). The classifier must derive the weft junction name-set from **fabric config** (`cfg.Dirs()` / `ScopedPathspec`), never enumerate `_lyx`/`_pattern` literals itself. Comparisons and git-pathspec slice literals are exempt from the token rule, but prefer routing through the config name-set anyway (as `status.go`'s pollution scan documents). No new geometry-token literal may appear outside `hubgeometry`; `enforcement_test.go` (`TestEnforcement_GeometryLiterals`) checks this.
- **CLI / Cobra Invariant** — not directly engaged (no new verb). If the async-push child needs a hidden `--warp-path` flag on the existing `lyx fabric` command, keep every fail-able step on the `output` JSON envelope and only exempt the terminal push tail per the existing bypass precedent.
- **Test Tier Purity Invariant** — the pure classifier tests must be untagged Tier-1 (no git spawn). Anything that spawns git (the two-sided commit, diff/status, async push) must be `//go:build integration` (or `smoke`).
- **Hermetic Git Test Environment Invariant** — any git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `internal/fabricengine`'s test packages already have this; new integration test files land in the same package.
- **Sandbox Suite Coverage** — `Fabric.Commit`/`Diff`/`Status` are Go-internal (no CLI verb), so they add no new registered module and trigger no sandbox-coverage obligation. If a hidden `--warp-path` flag is added to `fabric`, it is a flag on an already-covered module, not a new module.

## Testing

TDD candidate (write tests first): the **pure path classifier**. It is a total function with no I/O and a rich input space — the ideal red-green-refactor target.

- **Classifier — Tier-1 unit (untagged, no git):** table-driven. Cases: a path under `_lyx` → weft; under `_pattern` → weft; host source path → warp; `RelPath == "."` vs `RelPath == "sub"` (scoped junction prefixes); a path whose name merely *starts with* a junction name but isn't under it (e.g. `_lyxfoo` must be warp, not weft — test the prefix boundary is a path-segment boundary, not a raw string prefix); empty file list → two empty lists; all-warp and all-weft inputs (the single-sided degenerate cases); a junction name-set of more than two entries. Assert the two output lists partition the input with no path lost or duplicated.
- **Two-sided commit — integration (`//go:build integration`, `HermeticGitEnv`, `weft_integration_test.go` style with a real host+weft pair):** warp-first ordering (the weft commit's `Warp-SHA` trailer names the warp commit that includes this call's warp files, not the prior HEAD); correspondence recorded for the weft SHA; `CommitResult` fields populated correctly for two-sided, warp-only, and weft-only inputs; the warp commit carries **no** trailer and no correspondence entry (plain-git property); `Snapshot: <tag>` trailer present on the weft commit for each `snapshotTags` entry (and absent when `snapshotTags` is empty); message handling (warp bare, weft with trailers).
- **Partial failure — integration:** simulate a weft-commit failure after a successful warp commit (e.g. an induced weft-side error); assert the warp commit stays, `CommitResult.WarpSHA` is set, and the typed partial-failure error names the warp SHA and wraps the weft error. Assert a warp-commit failure leaves weft untouched and returns the error with nothing landed.
- **Skip-git gating — integration/unit:** with `WEFT_SKIP_GIT=1`, the weft side no-ops but a warp-only commit still lands; the async push forks no child. With `WEFT_SKIP_PUSH=1`, commits land, no push child forks.
- **Async push — integration:** assert the push is non-blocking (the call returns before the push completes) and that both repos are pushed when a real upstream is configured; assert best-effort semantics — a warp non-fast-forward push failure does not fail `Fabric.Commit` and does not corrupt local state. Cover the "weft pushed, warp not" cross-machine drift being *detected* (existing post-checkout hook / `SHAExists`), not silently mishandled. Keep push-network tests gated so Tier-1 stays offline.
- **Unified diff/status — integration:** `Fabric.Diff(sinceWarpSHA)` merges warp `ChangedFilesSince` and weft `ChangedFilesSince(WeftSHAForWarpSHA(...))` with correct side labels; the no-correspondence anchor returns the warp side + empty weft + the "no weft correspondence" flag (not an error); `Fabric.Status()` reports the merged uncommitted working-tree state across both repos.

Do not prescribe exact assertion shapes — that is mill-plan's job. Follow `internal/fabricengine`'s existing `*_integration_test.go` fixtures and the golang testing skill.

## Q&A log

- **Q:** Slice-2 deliverable scope — API only, migrate callers, or add a CLI verb? **A:** Go API only; no caller migration, no CLI verb.
- **Q:** Two-sided commit ordering — weft-first or warp-first? **A:** Warp-first. Weft is a piggyback whose trailer records warp's commit SHA, so warp must commit first for the trailer to name the right commit. It also falls out cheaply since `CommitWeft` already reads warp HEAD.
- **Q:** Partial-failure handling when the second commit fails? **A:** Report, do not roll back. Warp-first; a landed warp commit stays (it's plain git); return `CommitResult` + typed partial-failure error naming the warp SHA.
- **Q:** `Fabric.Commit` return shape? **A:** User delegated the choice. Lean `CommitResult{WarpSHA, WarpCommitted, WeftSHA, WeftCommitted}` + partial-failure detail in a typed error; "ok" suffices on the happy path, detail only on failure.
- **Q:** `snapshotTags` — include now or defer to slice 3? **A:** Include now, and write the `Snapshot:` trailer now, to make slice 3 easier (user delegated the how; chose to write the trailer to avoid a dead param).
- **Q:** `Fabric.Diff`/`Status` — Go-internal or CLI verb? **A:** Go-internal only.
- **Q:** Classifier input contract? **A:** Worktree-root-relative paths; weft iff under a `RelPath`-scoped junction dir (`ScopedPathspec(RelPath, cfg.Dirs())`), else warp.
- **Q:** Diff/Status anchor? **A:** A warp SHA; bridge to weft via the correspondence index; degrade (not error) on a no-correspondence anchor.
- **Q:** Host-side commit treatment + single-sided commits? **A:** Warp side is plain `git add`/`git commit` — no lock, no trailer, no exclude, no lyx marker — precisely so non-lyx collaborators' repos and hand-run git stay first-class; warp-only and weft-only commits both legitimate.
- **Q:** Test/CI skip (`SyncOptions`) — one side or both? **A:** Weft side honors `opts`; warp side always commits (the `WEFT_*` bypass is weft-scoped).
- **Q:** Does `Fabric.Commit` push? **A:** Yes, but the push MUST be async (fire-and-forget, detached) so it never blocks — the board model (fast local, deferred network). The commit stays synchronous.
- **Q:** Which side(s) get the async push? **A:** Both — it's the same underlying `gitrepo` module either way. Async push is best-effort; a warp non-fast-forward failure is silent and non-fatal.
- **Q:** Commit+push in one call, or separate `Commit`/`Sync`? **A:** One call — `Fabric.Commit` commits then fires the async push.

## Open items for mill-plan

- **Async-push child wiring.** Decide the concrete mechanism for the detached both-repos push: extend the existing hidden `lyx fabric --weft-path push` bypass with a companion hidden `--warp-path` so one detached child pushes both, and place the spawn helper in `internal/fabricengine` (mirroring `boardengine.spawnSync`) — reusing/consolidating with `fabriccli.spawnPush` rather than duplicating it. Confirm `proc.Detach` + `os.Executable` is the exact call shape, and that the child honors `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`.
- **`Snapshot:` trailer format.** Confirm the trailer key/spelling (`Snapshot: <tag>`, one line per tag) and that it coexists cleanly with the `Warp-SHA` trailer in `trailer.go`'s read/write helpers, so slice 3's reader can parse it back.
- **Partial-failure error type.** Name and shape the typed error (`*PartialCommitError` or similar) and where it lives.
- **`CommitResult` placement.** Which file the struct + typed error land in (likely a new `commit.go`).
