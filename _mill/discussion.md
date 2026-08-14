# Discussion: Move `<hub>/.lyx` into `<hub>/_board`

```yaml
task: Move <hub>/.lyx into <hub>/_board
slug: hub-dotlyx-into-board
status: discussing
parent: main
```

## Problem

`<hub>/.lyx` is a real directory created by `fabricengine.CloneHub`, sitting as a loose sibling of `<hub>/_board`.
But `_board` is the thing that *means* "shared across every worktree in this hub", so hub-wide state belongs inside it.
Two directories express the same "hub-wide" concept at the same level, which is one too many.
The operator dictates the placement change;
`CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant states the current placement explicitly, so the invariant is what is being rewritten.

**Why now:** raised during the `stencils-directory-reorg` discussion, where placing hub-wide stencils at `<hub>/_board/_lyx/stencils/` made the inconsistency visible, and explicitly deferred out of that task rather than resolved inside it.

The discussion then surfaced a second, larger problem that this task also resolves.
Moving `.lyx` inside `_board` would make hub-wide machine-local scratch visible from inside every worktree, because `_board` is junctioned into each one at `<worktree>/<anchorRel>/_board`.
The operator's judgement — grounded in millhouse's `.wiki` junction, where LLM agents read and edit through the link by mistake despite an explicit `CLAUDE.md` prohibition — is that a shared, writable link planted inside a worktree is unsafe in practice, and that neither naming nor prose rules prevent it.
The `_board` junction is therefore removed in this same task, and the general rule that forbids it is recorded as a new invariant.

## Scope

**In:**

- Move the hub-wide never-tracked tree from `<hub>/.lyx` to `<hub>/_board/.lyx`.
- Add `fabricengine.HubScratchDir(hub string) string` as the sole constructor of that path.
- Relocate `CloneHub`'s creation of the directory from step 4 to after step 7 (`ensureBoardWorktree`), and add an explicit `seedWeftArtifactExcludes(boardDir)` call before it.
- Re-point `reedengine.HubLogsDir` at the new location via `HubScratchDir`, and move `clone_test.go`'s reed idempotency test to `package fabricengine_test` so the new import edge does not close a test-binary cycle.
- Update the reed prose that names the old path: `internal/reedcli/up.go:33` (cobra help) and `internal/reedengine/lifecycle.go:29-33` (doc comment).
- Update the sandbox suite scenarios that encode the `_board` junction: `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F8 (`:237`), F13 (`:254`), F15 (`:364`).
- Drop the now-redundant `.lyx` append in `hubSlugReservedNames()`.
- Delete the `_board` convenience junction in full: both the wiring and the unwiring half, the result field, the CLI envelope key, and their tests.
- Rewrite `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant and add the new Hub Containment Invariant. **Already done in the discussion commit** — see "CONSTRAINTS pre-written" below.
- Update `docs/overview.md` and `manifest/designs/fabric-unified-view.md`.

**Out:**

- Any migration code. No hub on disk needs one — see the "No migration" decision.
- Renaming the `_board` directory itself. It stays `_board`.
- Any replacement mechanism for reaching the board from a worktree (no `lyx ide board` verb, no hub launcher script). `code <hub>/_board` is the answer.
- `_portals` and `_launchers` as they exist today. Only the *plan* to junction them into worktrees is cancelled; the existing hub-level links are untouched.
- `manifest/roadmap.md`. This is a placement and safety change, not a planned roadmap item completing.

## Decisions

### hub-scratch-placement

- Decision: the hub-wide never-tracked tree is `<hub>/_board/.lyx`, the mirrored ephemeral sibling of the existing `<hub>/_board/_lyx` (which already holds `config/fabric.yaml` and `stencils/`).
- Rationale: `_board` is the hub-wide concept; `_lyx`/`.lyx` siblinghood is the existing Durable-vs-Ephemeral rule. `<hub>/_board/_lyx/config/` is the precedent `stencils-directory-reorg` already followed.
- Rejected: leaving it at `<hub>/.lyx` (the status quo the operator dictates away from); nesting it under `_lyx` (durable tree, wrong half).

### eager-creation-kept

- Decision: `CloneHub` keeps pre-creating the directory, moved from step 4 to after step 7 (`ensureBoardWorktree`, `clone.go:310`), with `rec.Append(KindDirCreated, ...)` preserved.
- Rationale: the ordering move is forced — `_board` does not exist at step 4. The operator expects the tree to fill up (machine-local overrides of repo-wide config at `_board/.lyx/config/` are the anticipated next tenant), so it will not sit empty.
- Rejected: dropping eager creation and relying on `reedengine`'s own `os.MkdirAll(HubLogsDir(...))` boot-path call (`lifecycle.go:255`). Viable — `clone_test.go:155` exists precisely to pin that the two calls are idempotent against each other — but the operator prefers the directory to exist from clone.

### seed-excludes-at-clone

- Decision: `CloneHub` calls `seedWeftArtifactExcludes(boardDir)` immediately after `ensureBoardWorktree` and **before** creating `_board/.lyx`.
- Rationale: `_board` is a weft worktree. `seedWeftArtifactExcludes` writes `.lyx/` into the weft repo's `.git/info/exclude` (common gitdir, so one seeding covers every linked weft worktree), but `CloneHub` does not currently call it for `boardDir` — only `reconcile.go:311` does, best-effort. Without it there is a window where `_board/.lyx` reads as untracked dirt to the board dirty-gate.
- Rejected: relying on reconcile's self-healing re-seed (leaves the fresh-clone window open); making the call fatal (the existing best-effort posture at `reconcile.go:311` is deliberate and should not diverge).

### hub-scratch-constructor

- Decision: add `fabricengine.HubScratchDir(hub string) string` returning `<hub>/_board/.lyx`. `reedengine.HubLogsDir` becomes `filepath.Join(fabricengine.HubScratchDir(l.HubPath), "logs")`.
- Rationale: `reedengine` may not name `_board` — `TestEnforcement_GeometryLiterals` restricts that token to `internal/lyxcwd` and `internal/fabricengine`, so `reedengine` must obtain the segment from `fabricengine` regardless. A named constructor follows the told-never-derives pattern `StencilsDir` already established, and gives future tenants one opening to hang off. `reedengine → fabricengine` is a safe new edge: `fabricengine` does not import `reedengine`, and nothing in `fabricengine`'s dependency set does.
- Rejected: `reedengine` calling `BoardDir` and joining `.lyx` itself (same import, repeats the composition at every future tenant); moving `HubLogsDir` into `fabricengine` (gives fabric ownership of a reed-specific path); injecting the scratch path into `reedengine` from `reedcli` (architecturally cleanest, but changes `HubLogsDir`'s signature and every caller for no gain here).

### import-cycle-disposition

- Decision: move `TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` out of `internal/fabricengine/clone_test.go` into a file declaring `package fabricengine_test` in the **same directory**, and drop `clone_test.go`'s `reedengine` import.
- Rationale: `clone_test.go` is `package fabricengine` (in-package), so its imports are compiled into the `fabricengine` test binary and count as `fabricengine`'s own. Adding a production `reedengine → fabricengine` import would therefore close a cycle — `fabricengine`[test] → `reedengine` → `fabricengine` — and Go rejects it at test-binary compile time. The production binary is unaffected: `go list -deps ./internal/fabricengine` reports zero `reedengine` hits, and `clone_test.go` is the **only** file in the package with that import (verified). An external test package is a leaf nothing imports, so it may import both sides without closing any cycle. The test uses only exported API (`lyxdirs.DotLyxDirName`, `lyxcwd.Location`, `reedengine.HubLogsDir`), so it compiles unchanged after the move, and `package fabricengine_test` is already the directory's dominant convention: 71 external test files against 42 in-package.
- Rejected: moving `HubLogsDir` into `fabricengine` so `reedengine` never gains the import; injecting the path from `reedcli`. Both avoid the cycle but change the design settled in `hub-scratch-constructor` to work around a one-file test-package detail.

### slug-reservation-simplified

- Decision: drop the `.lyx` append from `hubSlugReservedNames()`, leaving it identical to `HubReservedNames()`. The reservation is carried by `structuralNeverCommittedDirs` alone.
- Rationale: the task body assumed the reservation depended on `<hub>/.lyx` existing. It does not. `IsReservedHubName` checks four sets in sequence and `.lyx` matches two of them: `hubSlugReservedNames()` (justified by hub geometry — the reason that disappears) and `structuralNeverCommittedDirs`, which is `[]string{lyxdirs.DotLyxDirName}` and is justified by `.lyx` being a structural junction name in every worktree. Removing the append changes no observable behaviour. `slugReservedNames(cfg)` carries the same double coverage and has no production caller at all — its only call site is `structuraldirs_test.go:58`.
- Rejected: keeping the append as belt-and-braces with a rewritten comment (leaves dead code justified by a directory that no longer sits there); keeping it plus a test pinning the double coverage (elevates an accident to a designed property).

### board-junction-deleted

- Decision: delete the `_board` convenience junction entirely — both halves, not just the wiring.
- Rationale: it is pure redundancy (the board is already reachable at `<hub>/_board`, and `docs/overview.md:170` records that no lyx code path reads through the link), and it is the one thing that breaks the fabric illusion from the inside: neither warp nor weft, shared across every worktree, physically writable but bypassing `board.lock` (`fabricengine.BoardWriteLockPath`) if written to by hand. Millhouse's `.wiki` is the empirical case: same shape, dot-prefixed and distinctly named, guarded by an explicit `CLAUDE.md` prohibition — and still edited by mistake. Naming does not fix it, prose does not fix it; removing the path does. Deleting both halves rather than keeping a sweeper is justified by there being **zero `_board` junctions on disk anywhere** (verified: neither `lyx-test-HUB` nor `lyx-fabric-test-HUB` has one), so a sweeper would be dead code from birth.
- Rejected: keeping `unwireBoardLink` as a permanent absence-enforcing sweep in `reconcile` (nothing to sweep); keeping it only on the `unwire`/`remove` call sites (same); renaming the junction to disambiguate it from `<hub>/_board` (millhouse's `.wiki` shows a distinct name does not prevent the mistake); making it read-only (directory junctions have no cross-platform read-only mode, and `internal/fslink`'s contract is `CreateDirLink` only).

### hub-containment-invariant

- Decision: record a new short `## Hub Containment Invariant` in `CONSTRAINTS.md`: no hub-level container is ever junctioned into a worktree. It cancels, explicitly, the unbuilt plan to give every worktree its own `_portals` and `_launchers` junctions.
- Rationale: stating the rule rather than the one-off deletion gives future readers the reason and forecloses the same mistake in two other places. The `_portals` case would be worse than `_board`'s: `<worktree-A>/_portals/<worktree-B>` is a writable path from one worktree into another's `_lyx`, which turns `CLAUDE.md`'s worktree-isolation rule from a geometric guarantee into prose an agent must remember. Cancelling costs nothing — no such junction exists in code, and `manifest/designs/fabric-unified-view.md:25` already states hub-structural entries are "composed at the hub level, not per-worktree weft junctions".
- Rejected: a narrow `_board`-only CONSTRAINTS edit with the general rule deferred; stating the general rule without naming the cancelled `_portals`/`_launchers` plan.

### board-stays-hub-reserved

- Decision: `_board` remains in `HubReservedNames()` and in `scanOnDiskJunctionNames`'s skip set.
- Rationale: its membership was never about the junction. It carries the slug reservation (no worktree may be named `_board`) and the `filterHubReserved` wiring guard (a `fabric.yaml` `pathspec` naming `_board` must not wire a colliding per-worktree junction), and the skip keeps the sweep from claiming an operator's own checked-in `_board` directory or symlink. After the deletion, `_board`, `_portals` and `_launchers` are uniform: hub-level containers, never present inside a worktree. What is removed is `_board`'s exception status, not its membership.
- Rejected: removing `_board` from the skip set so the generic sweep sees it (would also start claiming `_portals` and `_launchers`, which are real hub directories).

### no-migration

- Decision: no migration code of any kind. The old `<hub>/.lyx` is left for the operator to delete by hand, and no code tears down `_board` junctions.
- Rationale: verified on disk — the only hub carrying `<hub>/.lyx` is `/home/knatte/Code/lyx-test-HUB`, whose entire content is four tmux log files that reed recreates; `/home/knatte/Code/lyx-fabric-test-HUB` has no `.lyx` at all. No `_board` junction exists in either. There are no lyx-initialised repos beyond the sandbox.
- Rejected: auto-migrating `<hub>/.lyx` → `<hub>/_board/.lyx` on reconcile; auto-removing the old directory on reconcile; a reconcile-time `_board` junction sweep.

### stale-exclude-line-accepted

- Decision: no code removes a leftover `_board` line from a warp repo's `.git/info/exclude`. Accepted and documented; the remedy, if one is ever found, is deleting the line by hand.
- Rationale: `wireBoardLink` seeds that line (`junction.go:441`) and `unwireBoardLink` is its only unseeder (`unwire.go:168`), so deleting both halves would strand the line in any hub a current binary had already wired — silently git-ignoring an operator's own future `_board` path. Verified on disk that no such hub exists: both `lyx-test-HUB` and `lyx-fabric-test-HUB` carry only `_lyx` in their warp exclude files, no `_board` line, so there is nothing to clean up. Writing an unseeder purely for a state that exists nowhere is the same dead-code trap the `board-junction-deleted` decision rejected for the junction sweeper.
- Rejected: keeping `unseedGitExclude(..., BoardDirName)` alive as a reconcile-time cleanup; leaving the disposition unstated (the reviewer's point — the "zero junctions on disk" evidence does not by itself cover the exclude line, which is why it is verified separately here).

### no-board-access-replacement

- Decision: nothing replaces the junction as a way to reach the board from a worktree.
- Rationale: `code <hub>/_board` in a separate editor window is sufficient, and building a `lyx ide board` verb or a hub launcher script to replace a convenience we just concluded is a footgun reintroduces the problem in softer form. YAGNI.
- Rejected: a hub-level launcher script that opens the board; an `lyx ide board` verb. Either is a trivially small follow-up if the operator later wants one.

### design-doc-records-reversal

- Decision: `manifest/designs/fabric-unified-view.md:97-105` keeps its account of the shipped `_board` junction and gains a note that the decision was reversed, with the reason.
- Rationale: a design doc that silently deletes a reversed decision loses the reasoning that makes the reversal legible; a future reader would otherwise re-propose the junction.
- Rejected: deleting the paragraphs; moving them to a separate "Reversed decisions" section.

### board-scratch-visibility-moot

- Decision: no note is needed about `<hub>/_board/.lyx` being visible from inside worktrees.
- Rationale: it would have been, had the junction survived. With the junction deleted in the same task, there is no path from any worktree to `<hub>/_board`, so the concern never materialises.
- Rejected: shipping the move and the junction deletion as two tasks, which would have left a window where hub scratch was worktree-visible.

## Technical context

### The hub-level `.lyx` surface is tiny

Exactly two production sites touch it:

- `internal/fabricengine/clone.go:247` — `os.MkdirAll(filepath.Join(hubPath, lyxdirs.DotLyxDirName))`, at clone step 4, followed by `rec.Append(KindDirCreated, dotLyxPath, "")`. The surrounding comment block justifies the current placement and reservation and must be rewritten, not merely re-indented.
- `internal/reedengine/lifecycle.go:34` — `HubLogsDir(l)` returns `filepath.Join(l.HubPath, lyxdirs.DotLyxDirName, "logs")`. `lifecycle.go:255` does `os.MkdirAll` on it during boot. `stateDir()` immediately below is `AnchorPath()`-anchored and must **not** change.

Every other `lyxdirs.DotLyxDirName` use in the repo is `AnchorPath()`-anchored (logger, shuttle, scout, burler, perch, webster, loom, reed's own `stateDir`) and is out of scope.

### Ordering inside `CloneHub`

`clone.go` step numbering: step 4 creates the hub directory via `createExclusiveDir` and currently creates `.lyx`; step 5 clones warp; step 6 clones weft; step 7 (`clone.go:310`) is `boardDir := BoardDir(hubPath)` + `ensureBoardWorktree`. The new order is `ensureBoardWorktree` → `seedWeftArtifactExcludes(boardDir)` → `MkdirAll(HubScratchDir(hubPath))` → `rec.Append`.
Failures in steps 5-7 go through `teardownHub(rec, cwd, hubPath, hubTok, err)`; the relocated creation sits after that machinery is established, so its own failure should follow the surrounding step-7 posture rather than step 4's direct return.

### The `_board` junction surface, complete

Wiring — `wireBoardLink` (`internal/fabricengine/junction.go:378-441`, unexported), called from three sites:

- `internal/fabricengine/clone.go:407`
- `internal/fabricengine/add.go:205` (preceded by a comment at `add.go:203` explaining unconditional wiring)
- `internal/fabricengine/reconcile.go:397`

Unwiring — `unwireBoardLink` (`internal/fabricengine/unwire.go:125-178`), called from two sites:

- `internal/fabricengine/unwire.go:86`
- `internal/fabricengine/remove.go:104`, where it is followed by a manual `linksRemoved++` because the generic sweep's count cannot include it

Result and CLI surface:

- `UnwireResult.BoardJunctionRemoved` (`internal/fabricengine/unwire.go:46`) with its explanatory comment at lines 40-45
- `internal/fabriccli/unwire.go:34` — the `"board_junction_removed"` envelope key

Docs and comments referencing it: `internal/fabricengine/doc.go:406`, `reconcile.go:293`, `reconcile.go:773`, `docs/overview.md:168-171`, `manifest/designs/fabric-unified-view.md:97-105`.

Tests: `internal/fabricengine/boardjunction_integration_test.go` (delete outright — the file's entire subject is this junction), `internal/fabriccli/cli_test.go:768-791` (`TestRunCLI_Unwire_ReportsBoardJunctionRemoval`, a standalone test asserting the envelope key is present and true), `internal/fabriccli/cli_test.go:887-896` (two cases keyed on `unwireBoardLink`'s ownership refusal), and `internal/fabriccli/envelopecontract_integration_test.go` (the envelope key).

### Prose and scenario surfaces naming the old paths

Beyond code, three user-visible or operator-facing texts assert the current geometry and go stale silently:

- `internal/reedcli/up.go:33` — cobra long help: "enables server verbose logging to `<hub>/.lyx/logs/`".
- `internal/reedengine/lifecycle.go:29-33` — `HubLogsDir`'s doc comment, which describes the directory as hub-anchored.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — F8 (`:237`) requires `_lyx`, `.lyx` **and `_board`** to "land as links inside `<warp>/<dir>/`"; F13 (`:254`) describes fabric-owned links as "those pointing into the paired weft worktree **or the hub's `_board`**"; F15 (`:364`) requires `/_board`, `/_lyx` and `/.lyx` to be present "exactly once each" in the warp `.git/info/exclude` after every round. All three become false once the junction is gone.

### Why the junction needed its own machinery

`scanOnDiskJunctionNames` skips every `HubReservedNames()` entry, `_board` included, so the junction could never appear in the generic sweep or in `JunctionsRemoved`. That single blind spot is why there is a separate wiring function, a separate unwiring function, a separate result field, a separate CLI key, a standalone `seedGitExclude(..., BoardDirName)` call and a matching `unseedGitExclude` — four parallel special cases for one convenience link. Deleting the junction removes all of them and leaves `Remove` in its natural shape: sweep every junction, then delete the worktrees.

### Enforcement that will bite

- `TestEnforcement_GeometryLiterals` (`internal/lyxcwd/enforcement_test.go`, `geometryTokenOwners` at line 267) bans `"_board"` and `".lyx"` as string literals in path-construction context outside their owner directories. `"_board"` is owned by `internal/lyxcwd` + `internal/fabricengine`; `".lyx"` by `internal/lyxdirs`. `HubScratchDir` must therefore live in `fabricengine` and compose from `BoardDir` + `lyxdirs.DotLyxDirName`.
- `cmd/lyx/constructoranchoring_test.go:96` and `:144` both assert `reedengine.HubLogsDir(l) == filepath.Join(hub, ".lyx", "logs")`, once on an anchored fixture and once unanchored, with the file's header comment (line 17) naming `HubLogsDir` as the sole `HubPath`-anchored constructor. Both assertions and the header need updating to the board-anchored path.
- `internal/fabricengine/clone_test.go:140` asserts `filepath.Join(res.HubPath, lyxdirs.DotLyxDirName)` exists after clone; `clone_test.go:155` (`TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx`) pre-creates the old path at line 157.
- `internal/fabricengine/structuraldirs_test.go:58` exercises `slugReservedNames`, whose composition changes.
- `internal/fabricengine/junctionnames_test.go:244-246` comments on `hubSlugReservedNames()`'s membership.

### Unaffected, despite looking related

- The per-worktree `.lyx` junction (`<worktree>/<anchorRel>/.lyx` → the paired weft's `.lyx`) and its adoption branch at `junction.go:201`. `dotlyxjunction_integration_test.go` covers that and stays.
- `structuralNeverCommittedDirs`, `WiredNames`/`RepoWiredNames`, `PathspecNames`, `seedWeftArtifactExcludes`'s content.
- `reedengine`'s `stateDir()`, and every `AnchorPath()`-anchored `.lyx` accessor in every other module.

## Constraints

From `CONSTRAINTS.md`:

- **Durable-vs-Ephemeral State Invariant** — rewritten by this task. See below.
- **Hub Containment Invariant** — added by this task. See below.
- **Lyxdirs Single-Declarer Invariant** — `_lyx`/`.lyx` literals only in `internal/lyxdirs`; every caller uses the constants.
- **Cwd Resolution Invariant** — geometry is structural, never config-overridable; weft-sibling paths and junction construction belong to `fabricengine`, never `lyxcwd`. `HubScratchDir` belongs in `fabricengine` for this reason as much as for the literal ban.
- **Fabric Vocabulary Invariant** — naming discipline for the new constructor and any renamed identifiers.
- **Mutation Record Invariant** — the relocated `MkdirAll` keeps its `rec.Append(KindDirCreated, ...)`; every deleted wiring path removes its `KindLinkCreated` record with it.
- **Fabric Destruction Chokepoint Invariant** — relevant to deleting `unwireBoardLink`, which routes through `removeLink`/`pathRequest`. Removing a call site must not weaken the chokepoint for the remaining callers.
- **Fabric Write-Side Containment Invariant** — `<hub>/_launchers/…` and `<hub>/_portals/…` writes must route through an `os.Root` rooted at the hub. Untouched here, but named because the new Hub Containment Invariant sits next to it conceptually.
- **CLI / Cobra Invariant** — the `board_junction_removed` envelope key is CLI-observable output; its removal is a contract change.
- **Documentation Lifecycle** — `docs/overview.md` and the fabric design doc update in the same commit as the code.
- **Sandbox Suite Coverage** — `sandbox/fabric-suite.cmd` and `sandbox/reed-suite.cmd` both exercise paths this task moves.

### CONSTRAINTS pre-written

Unusually, `CONSTRAINTS.md` is amended in the **discussion** commit rather than the implementation commit, at the operator's explicit instruction.
The reason: reviewers read `CONSTRAINTS.md`, and a reviewer reading the old Durable-vs-Ephemeral bullet would treat this entire design as an invariant violation.
The operator judged that risk larger than the cost of the alternative.
`CLAUDE.md` states CONSTRAINTS changes land in the same commit as the code, so between this commit and the implementation, `CONSTRAINTS.md` describes a state the code does not yet have.
This is known, deliberate, and short-lived — mill-plan and mill-go should treat the amended bullets as the specification to implement, not as a discrepancy to report.

The two changes already committed:

1. Durable-vs-Ephemeral State Invariant — the sibling-exception bullet now reads "sole exception the hub-wide pair under `BoardDir(hub)`", and the former `<hub>/.lyx` bullet now describes `<hub>/_board/.lyx`, names `fabricengine.HubScratchDir` as sole constructor, names the post-board-worktree creation ordering, and attributes the slug reservation to `structuralNeverCommittedDirs`.
2. A new `## Hub Containment Invariant` section between Durable-vs-Ephemeral and gitkit Leaf.

## Testing

**`internal/fabricengine`** — the bulk of the work.

- Update `clone_test.go:140` to assert the new path. `clone_test.go:155`'s `TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx` **moves to `package fabricengine_test`** (same directory) and pre-creates the board-anchored path. Its value survives the move intact and it must be kept, not deleted — but it cannot stay in-package, per the `import-cycle-disposition` decision. After the move, `clone_test.go` must import `reedengine` nowhere; that absence is what keeps the production edge legal.
- New coverage for `HubScratchDir`: it returns `<hub>/_board/.lyx` and composes from `BoardDir`, including on a `--subpath`-anchored hub, where it must **not** pick up `AnchorRel` (the board's `_lyx` tree is flat).
- Clone-ordering coverage: after `CloneHub`, `_board/.lyx` exists **and** the weft repo's `.git/info/exclude` already carries `.lyx/`, so a `git status` in `_board` reports clean. TDD candidate — this is the decision most likely to be silently mis-ordered, and the assertion is what makes the ordering load-bearing rather than incidental.
- `structuraldirs_test.go:58` — `slugReservedNames`'s composition changes; the assertion that `.lyx` is still refused as a slug must survive, sourced from `structuralNeverCommittedDirs`. TDD candidate: write the "`.lyx` is still a refused slug" assertion against the simplified `hubSlugReservedNames()` before removing the append, so the removal is proven behaviour-preserving rather than assumed.
- `junctionnames_test.go` — update the comments at 244-246 and any assertion on `hubSlugReservedNames()`'s membership.
- Delete `boardjunction_integration_test.go` outright.
- Regression coverage that no `_board` link is created: after `clone`, `add`, and `reconcile`, `<worktree>/<anchorRel>/_board` does not exist, and the warp `.git/info/exclude` carries no `_board` line. This is the assertion that gives the new Hub Containment Invariant teeth, and it should exist for all three verbs, not just clone.
- `Remove`'s `LinksRemoved` count must stay correct once the manual `linksRemoved++` is deleted.

**`internal/reedengine`** — `HubLogsDir` re-points. Its existing callers (`lifecycle.go:255`, `internal/reedcli/smoke_debuglog_test.go:40,169`) exercise it end-to-end; the smoke tests should keep passing unchanged, which is itself the assertion that the move is transparent to reed. The `up.go:33` help string and the `lifecycle.go:29-33` doc comment are prose, not behaviour — the CLI/Cobra Invariant's help-accuracy obligation is what makes them a review check rather than a test.

**`tools/sandbox`** — F8, F13 and F15 in `SANDBOX-FABRIC-SUITE.md` must be rewritten before either suite is run, not after: their current text instructs the operator to *confirm* a `_board` link exists, so running them unchanged produces a false FAIL. F8 drops `_board` from its link list and should instead confirm the warp anchored directory has **no** `_board` entry; F13 drops "or the hub's `_board`" from its ownership description; F15 drops `/_board` from the exactly-once exclude list. `SANDBOX-REED-SUITE.md` names no `.lyx` path and needs no edit (verified).

**`cmd/lyx`** — `constructoranchoring_test.go` at 96 and 144, plus the header comment at line 17. `notransients_test.go` needs no change (it is `AnchorPath()`-scoped).

**`internal/fabriccli`** — `envelopecontract_integration_test.go` drops the `board_junction_removed` key; `cli_test.go:887-896`'s two `unwireBoardLink` ownership cases are deleted with the function.

**Sandbox** — `sandbox/fabric-suite.cmd` and `sandbox/reed-suite.cmd` must both pass. The reed suite is the real end-to-end proof the log directory moved without breaking the server; the fabric suite proves clone/add/reconcile/remove/unwire still work with the junction gone.

**Cross-platform** — the junction deletion removes `fslink` call sites rather than adding any, so no new Windows exposure. The `_board/.lyx` creation is a plain `MkdirAll`, not a link.

## Q&A log

- **Q:** Should `CloneHub` still pre-create the scratch directory after the move, or leave it to reed's own `MkdirAll`? **A:** Keep it, relocated to after the board worktree exists — the operator expects hub-wide state to accumulate there.
- **Q:** Is `<hub>/_board/.lyx` where all repo-wide setup lands? **A:** `_board` as a whole is; the tracked half (`_board/_lyx/`) takes configuration, the untracked half (`_board/.lyx/`) takes machine-local runtime state and anticipated machine-local config overrides.
- **Q:** What replaces the slug-name reservation? **A:** Nothing — `structuralNeverCommittedDirs` already carries it, independently of hub geometry.
- **Q:** How do existing hubs migrate? **A:** They do not. Verified on disk: only the sandbox has `<hub>/.lyx`, holding four disposable tmux logs.
- **Q:** Should the `_board` junction be removed, given millhouse's `.wiki` gets edited by mistake despite an explicit prohibition? **A:** Yes — and fold it into this task rather than splitting, since the teardown code already exists and the two changes share a doc surface. Operator accepts losing the convenience and will open a separate editor window on `<hub>/_board`.
- **Q:** Keep `unwireBoardLink` as a sweeper for junctions made by older binaries? **A:** No — zero `_board` junctions exist on disk anywhere, so a sweeper is dead code from birth.
- **Q:** Do `_portals` and `_launchers` break the same illusion? **A:** Not as they exist today — their links live at hub level and point inward, so nothing is visible from inside a worktree. But the operator's unbuilt plan to junction them *into* every worktree is cancelled by the new invariant, because `<worktree-A>/_portals/<worktree-B>` would make worktree isolation prose instead of geometry.
- **Q:** Does `_board` stop being reserved? **A:** No — it stays in `HubReservedNames()` for the slug reservation and the wiring guard. What it loses is its exception status.
- **Q:** Write CONSTRAINTS now or in the implementation commit? **A:** Now, before the discussion reviewer runs, so reviewers do not read the superseded invariant and treat the design as a violation.
- **Q:** (Review r1, blocking) A production `reedengine → fabricengine` import closes a cycle through `clone_test.go`, which is `package fabricengine` and imports `reedengine`. How is it resolved? **A:** Move that one test to `package fabricengine_test` in the same directory — not a new directory, and already the directory's dominant convention (71 external files against 42 in-package). The `HubScratchDir` design is unchanged; moving `HubLogsDir` into fabric or injecting the path from `reedcli` were rejected as reworking a settled design around a one-file test-package detail.
- **Q:** (Review r1) Does a leftover `_board` line in a warp `.git/info/exclude` need cleanup code? **A:** No — verified on disk that neither sandbox hub has one. Accepted and documented, with hand-removal as the remedy if one is ever found.
- **Q:** How are the remaining review rounds handled? **A:** Operator handed the rest to auto mode: every finding is fixed by best judgment with no further prompts.
