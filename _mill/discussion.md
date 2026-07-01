# Discussion: Add lyx init --undo / deinit command

```yaml
task: Add lyx init --undo / deinit command
slug: lyx-deinit
status: discussing
parent: main
```

## Problem

`lyx init` (`internal/initcli/initcli.go`) scaffolds a worktree's `_lyx/` topology: it
wires a host↔weft directory junction via `warpengine.WireJunctions`, appends the
junction name to `.git/info/exclude`, creates `_lyx/` and `_lyx/config/`, maintains a
managed block in `.gitignore`, and reconciles all module config files into the
weft-backed `_lyx/config/` directory. There is no lyx-owned way to reverse any of this.
Today, undoing an init means manually deleting the host-side junction, manually
clearing the weft-side directory it points to, and manually editing `.gitignore` — all
outside lyx's ownership.

This surfaced from `sandbox-suite-expand`: its new S6 scenario runs `lyx init` from a
subdirectory to prove lyx works from any subfolder once initialized, and cleaning up
after that scenario currently falls back to ad-hoc filesystem/git housekeeping (the
same category of "plain git is fine for housekeeping lyx doesn't own" already accepted
for S2). Once `lyx init --undo` exists, S6 can call it directly instead of manual
cleanup, and no longer needs the fallback instructions — though that follow-up edit to
`SANDBOX-SUITE.md` is a separate task, out of scope here.

## Scope

**In:**
- A `--undo` bool flag on the existing `lyx init` command (`internal/initcli/initcli.go`).
  Confirmed with the user: **not** a standalone `lyx deinit` command — this feature is
  rarely used in normal workflows (mainly handy for test/sandbox cleanup), so it doesn't
  warrant its own top-level command surface. It stays a flag on `init`.
- Removing the host-side `_lyx` junction — and *only* if it validates as a correct
  junction (see Decisions below); never a blind `rm -rf`.
- Clearing the corresponding weft-side `_lyx` content (the junction's target directory),
  including committing and pushing that deletion through `weftengine`.
- Reverting the `.git/info/exclude` line(s) added by `seedGitExclude`.
- Reverting the managed `.gitignore` block added by `gitignore.Ensure`.
- Idempotent: safe to run repeatedly, and safe to run on a directory that was never
  initialized (pure no-op success).
- A small `weftengine.Commit` signature change (add a commit-message parameter) so the
  undo path can commit under an accurate message instead of the hardcoded `"weft sync"`.

**Out:**
- Tearing down the weft worktree/branch itself, or the host worktree — that's
  `warpengine.Remove` / `lyx warp remove`'s job, a much bigger operation. `--undo` only
  reverses what `init` wrote; the worktree and its weft pairing remain intact.
- Any `--force` override for the two hard-error guards below (real non-junction
  directory, junction target mismatch). Both are treated as serious, unexpected states
  that must surface as errors, not be silently bypassed. No force flag is introduced.
- The `SANDBOX-SUITE.md` S6 follow-up edit (replacing its manual-cleanup note with a
  call to `--undo`) — tracked as a separate follow-up task per the proposal, not done
  here.
- Any change to `configsync.ReconcileAll` or the module config template system itself —
  clearing the weft-side `_lyx` directory wholesale already removes the config files it
  wrote; no separate per-module "unreconcile" step is needed.

## Decisions

### Flag vs. standalone command

- Decision: `--undo` is a bool flag on `lyx init`, implemented in the same
  `internal/initcli` package (new file, e.g. `internal/initcli/undo.go`, to keep
  `initcli.go` focused on the forward path).
- Rationale: explicit user preference — this is a rarely-used operation, mainly valuable
  for test/sandbox cleanup, so it doesn't need its own CLI surface. `initcli` is already
  marked as a CONSTRAINTS.md-sanctioned "trivial wrapper" skip-engine package, so adding
  the reverse logic here (rather than spinning up a `deinitengine`) fits the existing
  precedent.
- Rejected: standalone `lyx deinit` command — more discoverable and cleanly separated,
  but adds a whole new CLI module (registration, Short/Long, help-tree tests) for a
  rarely-invoked operation.

### Junction removal safety (real-directory guard)

- Decision: before removing the host `_lyx` junction, validate with `fslink.IsLink`.
  If it exists and is **not** a link (a real directory), hard-error and refuse to touch
  it — mirror `seedLyxJunction`'s own "host repo already contains a real directory;
  it predates weft" guard, with an analogous error message.
- Rationale: this is the proposal's explicit ask ("not blindly rm -rf if it is not
  actually a junction — mirror the safety checks seedLyxJunction already does").
- Rejected: silently skipping a real directory — hides a real anomaly (a non-junction
  `_lyx` predating weft) the user needs to know about.

### Junction target mismatch

- Decision: if the host `_lyx` junction **is** a link, but resolves (via
  `fslink.PointsTo` / `filepath.EvalSymlinks`) to something other than the canonical
  `l.WeftLyxDirFor(slug)` — including the case where the target no longer exists at all
  (e.g. the weft sibling was torn down without going through `--undo`) — hard-error
  immediately. Do not guess, do not clear whatever it actually points to, do not remove
  the junction.
- Rationale: explicit user directive — a mismatched/dangling junction means "something
  has gone ORDENTLIG GALT" (something has gone seriously wrong) and must not be
  overlooked or silently worked around. This overrides the initial instinct to be
  "helpful" by clearing whatever the junction actually points to.
- Rejected: resolving and clearing the junction's actual (mismatched) target — could
  silently mask a corrupted/hand-edited junction or a torn-down weft sibling.

### Abort scope when a hard-error guard fires

- Decision: when either guard above fires (real non-junction directory, or a
  mismatched/dangling junction target), `runUndo` aborts immediately and performs
  **none** of the other steps — no weft-content clearing, no `.gitignore` revert, no
  `.git/info/exclude` revert. The junction-validation step must run and fully succeed
  (or be a clean no-op because the link is simply absent) before any other step is
  attempted. This makes the guard failure atomic-in-effect: either the run proceeds
  past validation and all applicable steps happen, or nothing happens beyond reporting
  the error.
- Rationale: explicit user directive — a non-junction `_lyx` directory in the host is a
  serious error and "should NOT be attempted to be fixed" (bør ikke forsøkes fikset) —
  meaning not just "don't delete the real directory" but "don't touch anything else in
  this run either." Running the other independent steps around a blocked/corrupted
  junction would leave a confusing mixed state (gitignore/exclude/weft-content already
  reverted, but the actual anomaly still sitting there) that's harder to diagnose than
  a clean, fully-untouched abort.
- Rejected: running the other independent steps anyway (weft-content clear, gitignore
  revert, exclude revert) around the blocked junction — maximizes "progress made" per
  invocation, but produces a partially-reverted state that obscures the anomaly instead
  of preserving it for investigation.
- Implementation note for mill-plan: this means `runUndo`'s step ordering matters —
  the junction-validation/removal step (via `warpengine.UnwireJunctions`) must run
  first and its error, if any, must short-circuit before the weft-content-clearing,
  gitignore-revert, or exclude-revert steps are attempted.

### No separate "weft pairing" pre-gate

- Decision: `--undo` does **not** have an early "no weft pairing" gate like plain
  `init` does. Instead, each step independently checks whether its own target exists
  (junction link, weft-side directory, gitignore block, exclude lines) and no-ops if
  absent. Combined with the target-mismatch guard above, this naturally satisfies both
  requirements: a truly never-initialized directory is a clean no-op (nothing exists,
  nothing to check), while a *partially* inconsistent state (e.g. a junction present but
  pointing at a missing/wrong target) is still caught and hard-errored by the
  junction-validation step, not silently skipped.
- Rationale: the proposal requires `--undo` to be "safe to run on a directory that was
  never initialized," which an init-style hard pre-gate would prevent. The per-step
  existence checks give idempotency for the legitimate case, while the mismatch/
  real-dir guards still catch genuine corruption — the two prior decisions already do
  the "don't overlook serious errors" work, so no additional blanket gate is needed.
- Rejected: mirroring init's early "no weft pairing" hard gate — would make `--undo`
  unable to clean up a partially-initialized or already-unpaired directory, directly
  contradicting the idempotent/safe-anywhere requirement.
- Edge case worth flagging for the plan: host junction absent but weft-side `_lyx`
  content still present (e.g. a prior `--undo` run crashed between removing the
  junction and clearing weft content). This is treated as a normal partial-completion
  recovery case, not a hard-error anomaly — the weft-content-clearing step just
  proceeds independently since its own existence check doesn't depend on the junction
  step having run in the same invocation.

### Weft-side content: filesystem delete + weftengine-owned git operations

- Decision: `os.RemoveAll` the weft-side `_lyx` directory (`l.WeftLyxDirFor(slug)`)
  directly — that part is plain filesystem I/O, not a git operation. Then commit that
  deletion through `weftengine.Commit(weftPath, pathspec, message, opts)` (scoped
  pathspec `weftengine.ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName})`),
  and push through `weftengine.Push(weftPath, opts)` — mirroring the existing `weftcli`
  `push` subcommand's pattern exactly (`internal/weftcli/cli.go`'s `push` RunE, lines
  ~183-193): `Push` is called **unconditionally** after `Commit`, not gated on
  `Commit`'s `committed` return value. `weftengine.Push` already no-ops via its own
  internal `hasUnpushed` check when there's nothing to push, so calling it
  unconditionally is both correct and idempotent — and critically, it is what recovers
  a prior partial run where the deletion committed locally but the push itself failed
  (see the residual-risk note under Testing). Gating `Push` behind `committed` would be
  a bug: a rerun after a "committed, push failed" partial state would see `committed ==
  false` (nothing new to stage) and never retry the stuck push.
- Rationale: explicit user directive — "all git operations to weft go through the weft
  module" (`weftengine` already owns all git-into-weft operations per its own package
  doc: "Package weftengine owns all git operations into the paired weft worktree").
  `initcli`/`warpengine` must not run raw `gitexec`/`git -C <weft>` commands themselves.
  Auto-push (rather than leaving the commit local) was the user's explicit choice, to
  match the existing pattern where other weft-mutating commands push automatically.
- Rejected: leaving the deletion as an uncommitted/unpushed local change in the weft
  worktree — simpler, but diverges from how every other weft-mutating command in this
  codebase behaves, and from the user's explicit choice.

### weftengine.Commit needs a message parameter

- Decision: extend `weftengine.Commit`'s signature to accept a commit message
  parameter (rather than hardcoding the `"weft sync"` constant). Export the existing
  constant (e.g. `weftengine.DefaultCommitMessage`) so the three existing call sites in
  `internal/weftcli/cli.go` (lines ~153, ~185, ~226) keep passing `"weft sync"`
  explicitly, while `--undo` passes its own message (e.g. `"lyx init --undo: clear
  _lyx"`).
- Rationale: explicit user choice. Reusing the hardcoded `"weft sync"` message for an
  `--undo`-triggered deletion would make the weft branch's history misleading/
  untraceable.
- Rejected: reusing `"weft sync"` as-is — no `weftengine` API change needed, but loses
  traceability of *why* the weft-side `_lyx` content was deleted.

### New warpengine helper: `UnwireJunctions`

- Decision: add an exported `warpengine.UnwireJunctions(l *hubgeometry.Layout, slug
  string) (result, error)` next to `WireJunctions` in `internal/warpengine/junction.go`,
  structured as the mirror image: an unexported `unseedLyxJunction` (the
  validate-then-remove logic from the two Decisions above) followed by an unexported
  `unseedGitExclude` (removes the junction `Name` line(s) `seedGitExclude` added from
  `.git/info/exclude`, reusing the same git-path resolution as the seeder). Return a
  small result capturing what changed (junction removed? exclude changed?) so
  `initcli` can build its JSON status output.
- Rationale: mirrors the existing `WireJunctions`/`seedLyxJunction`/`seedGitExclude`
  structure directly — same file, same package, symmetric naming, no new package
  needed. `warpengine` already owns junction lifecycle (it has `removeHostJunction` for
  the different `Remove`-worktree-teardown path); this is a parallel, narrower entry
  point for the "undo an init, keep the worktree" case.
- Rejected: doing junction validation/removal directly in `initcli` — would duplicate
  `seedLyxJunction`'s target-resolution logic (`fslink.IsLink`, `fslink.PointsTo`,
  `filepath.EvalSymlinks`) instead of reusing it, and would leak `warpengine`'s
  junction/exclude internals across the package boundary.

### New gitignore helper: `gitignore.Remove`

- Decision: add `gitignore.Remove(repoRoot string, entries ...string) (changed bool,
  err error)` to `internal/gitignore/gitignore.go`, structured as `Ensure`'s mirror: it
  parses the same managed block, removes the given entries from the tracked set instead
  of adding them, and removes the entire block (both markers) only when the resulting
  set is empty — restoring `.gitignore` to its pre-init shape in the common case, but
  leaving the block (with the other module's entry intact) when another module still
  contributes to it. Premise correction: `.lyx/` is **not** the only entry ever written
  into this shared block — `internal/vscode/config.go`'s `WriteConfig` also calls
  `gitignore.Ensure(dir, ".vscode/")` into the same managed block. So
  `gitignore.Remove(cwd, ".lyx/")` must only remove the `.lyx/` entry (and the block
  markers, if the set becomes fully empty as a result); if `.vscode/` (or any other
  contributed entry) is still present, the block must survive with that entry intact.
  Idempotent: returns `changed=false` if the block or entries are already absent.
- Rationale: symmetric with `Ensure`; reuses the exact same parsing structure
  (before-block/block-interior/after-block) so behavior stays consistent between the
  two functions.
- Rejected: leaving an empty `# === lyx-managed ===` / `# === end lyx-managed ===`
  block shell in place after removing the only entry — messier revert, doesn't fully
  restore pre-init state.

### Reuse env-based git bypass across CLI packages

- Decision (implementation note, not asked of the user — flagging for mill-plan's
  judgment): `internal/weftcli/cli.go` already has a package-private `envSyncOptions()`
  reading `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` env vars into a `weftengine.SyncOptions`.
  `initcli`'s undo path needs the same bypass for its own integration tests. Recommend
  promoting this helper to an exported `weftengine.EnvSyncOptions()` (moving it from
  `weftcli` into `weftengine`, updating the one call site in `weftcli`) rather than
  duplicating the env-var-reading logic in `initcli` — avoids two packages each owning
  string literals for the same two env vars.
- Rationale: DRY; the bypass is conceptually a `weftengine` testing seam, not specific
  to the `weft` CLI subcommands.
- Flag for mill-plan: this is a small, mechanical, low-risk refactor — confirm during
  planning/review rather than re-litigating with the user.

## Technical context

Key files (all under `internal/`):

- `initcli/initcli.go` — the `lyx init` command and `runInit`. `Command()` builds the
  cobra command; the CLI/Cobra invariant's established flag pattern (see
  `configcli/configcli.go`'s `--print`/`--apply` flags) is: register the flag on the
  `*cobra.Command`, then have the `RunE` closure (via `clihelp.WrapRun`) read it via
  `cmd.Flags().GetBool(...)` and dispatch to a private handler — `runInit` doesn't
  need its signature changed; add a sibling `runUndo(out io.Writer, args []string) int`
  and branch in the `RunE` closure.
- `warpengine/junction.go` — `WireJunctions`/`seedLyxJunction`/`seedGitExclude`: the
  forward-path logic `UnwireJunctions` mirrors. `seedLyxJunction`'s existing
  target-validation pattern (`os.Lstat`, `filepath.EvalSymlinks(target)`,
  `fslink.IsLink`, `fslink.PointsTo`, compare resolved paths) is exactly the logic to
  reuse/mirror for the target-mismatch hard-error guard.
- `warpengine/weftwiring.go` — has an existing (unexported) `removeHostJunction`, but
  that's used by the *full worktree teardown* path (`remove.go`'s `Remove`, which also
  deletes the weft worktree/branch). Do not reuse or extend `removeHostJunction` for
  `--undo` — it belongs to a different, much larger operation; `UnwireJunctions` is a
  new, narrower entry point.
- `gitignore/gitignore.go` — `Ensure`'s block-parsing structure (start/end markers,
  before/interior/after) to mirror in the new `Remove`.
- `weftengine/weft.go` + `weftengine/sync.go` — `ScopedPathspec`, `Commit`, `Push`,
  `SyncOptions`. `Commit`'s message is currently the hardcoded `commitMessage = "weft
  sync"` constant — needs a message parameter per the Decisions above.
- `weftcli/cli.go` — the only existing caller of `weftengine.Commit`/`Push`
  (subcommands `commit`, `push`/`sync`, ~lines 153/185/226), and owner of the
  `envSyncOptions()` helper to potentially promote into `weftengine`.
- `hubgeometry/hubgeometry.go` — `HostLyxLink(slug)`, `WeftLyxDirFor(slug)`,
  `HostJunctions(slug)`, `LyxDirName`, `ConfigDir`. All geometry token usage in the new
  code must go through these per the Hub Geometry Invariant — no raw `"_lyx"` literals.
- `fslink` package — `IsLink`, `PointsTo`, `Remove` (idempotent: no-ops on
  already-absent paths). Windows junctions vs. other-platform symlinks are already
  abstracted here; no OS-specific code needed in the new logic.

## Constraints

From `CONSTRAINTS.md` (all apply to this task):

- **Hub Geometry Invariant** — no raw `"_lyx"` (or other geometry token) string
  literals in path construction anywhere in the new code (`initcli`, `warpengine`,
  `gitignore`); always go through `hubgeometry.LyxDirName` / the `Layout` methods
  (`HostLyxLink`, `WeftLyxDirFor`, `ConfigDir`, etc.), including in new test fixtures.
- **lyxtest Leaf Invariant** — not directly implicated (no changes to
  `internal/lyxtest` are anticipated), but new integration tests should use
  `lyxtest.CopyPairedLocal` per the existing `initcli_test.go` pattern, not hand-rolled
  fixtures that would pull in `configreg`/feature-package imports.
- **CLI / Cobra Invariant** — `--undo` is a flag, not a new subcommand, so no new
  registration/help-tree entries are needed; but the `Short`/`Long` on the existing
  `lyx init` command must be updated to document `--undo` (concrete example), since
  "help accuracy is a review obligation" whenever observable behaviour changes. No
  `Command()` seam changes beyond registering the flag. Also explicitly re-verify
  `internal/weftcli/cli.go`'s `commit` subcommand `Long` text (lines ~140-141: "The
  commit message is always the fixed string \"weft sync\" ... cannot be customized
  with a flag") — this stays literally true after the `weftengine.Commit` signature
  change (the three `weftcli` call sites keep passing `DefaultCommitMessage`
  explicitly, and no new `weftcli` flag is introduced), but since the change touches
  `Commit`'s message handling directly, this help text must be re-confirmed rather than
  assumed unaffected.
- **Documentation Lifecycle** — this adds new observable CLI behaviour (a flag with
  real side effects), so the same commit must update the relevant `docs/modules/` entry
  for `initcli`/`warpengine`/`gitignore` if one exists, and record the new
  `UnwireJunctions`/`gitignore.Remove` cross-cutting pieces in `CONSTRAINTS.md` if they
  rise to the level of a structural invariant (likely not — they're mirror-image
  helpers of existing exported functions, not new invariants). `docs/overview.md` has
  an existing **init** bullet (line ~212: "scaffolds the `_lyx/` directory structure
  and creates all module config files via reconciliation against templates...") that
  describes `init`'s observable behaviour — confirm during planning whether this bullet
  needs a one-line addition mentioning `--undo`, per this repo's CLAUDE.md instruction
  to update `docs/overview.md` when the module table/execution stack changes.
  `docs/roadmap.md` is **not** touched — this is a feature addition, not a
  planned-milestone completion.

## Testing

Per-module test approach (new integration tests, `//go:build integration`, following
the existing `initcli_test.go` / `warpengine/remove_test.go` style with
`lyxtest.CopyPairedLocal`):

- **`initcli` (`internal/initcli/undo_test.go`, new file)** — the primary test surface.
  TDD candidates:
  - `TestRunInit_Undo_HappyPath` — init, then `--undo`; assert the host junction is
    gone, the weft-side `_lyx` directory is gone *and committed* (check weft `git log`/
    `git status --porcelain` is clean after commit+push), the `.gitignore` managed
    block is fully removed (not just emptied), and the `.git/info/exclude` line is gone.
  - `TestRunInit_Undo_NeverInitialized` — run `--undo` on a freshly paired fixture with
    no prior `init`; expect `ok=true`, all status fields report the "not present"/
    "unchanged" no-op values, no error.
  - `TestRunInit_Undo_Idempotent` — run `--undo` twice in a row; second run must be a
    clean no-op matching `TestRunInit_Undo_NeverInitialized`'s expectations.
  - `TestRunInit_Undo_RealDirectoryGuard` — two-phase setup required, not a single
    pre-creation: `seedLyxJunction` (inside `WireJunctions`) itself hard-errors on a
    real non-junction `_lyx` directory, so `init` cannot succeed at all if the real
    directory exists *before* it runs — gitignore/exclude/weft-content would never get
    created in that case. Instead: (1) run `init` successfully first (so the junction,
    gitignore block, exclude line, and weft-content all legitimately exist), (2) then
    remove the junction and replace it with a real directory (simulating external
    corruption after the fact), (3) then run `--undo`. Assert `--undo` hard-errors, and
    *everything* is left untouched — not just the real directory itself, but also the
    weft-side `_lyx` content, the `.gitignore` managed block, and the `.git/info/exclude` line
    (per the "abort scope" decision: the guard firing aborts the whole run, no partial
    revert of the other independent steps).
  - `TestRunInit_Undo_TargetMismatch` — after init, repoint the junction (or hand-craft
    one) at a different target than `WeftLyxDirFor(slug)`; assert `--undo` hard-errors
    without removing the junction, and without touching the weft-side content,
    `.gitignore`, or `.git/info/exclude` either (same full-abort assertion as above).
  - `TestRunInit_Undo_PartialRecovery` — simulate a crash-between-steps state (junction
    already removed, weft-side content still present) and assert `--undo` finishes the
    job cleanly (no error), covering the "no separate pairing gate" decision's edge
    case. Also cover the narrower partial state where the weft-side deletion was
    already committed locally but the push failed (e.g. a transient network error mid-
    `weftengine.Push`): assert a rerun still succeeds and pushes the pending commit —
    this is exactly why `Push` must be called unconditionally after `Commit` rather
    than gated on `Commit`'s `committed` return value (see the Decisions section).
  - Use `WEFT_SKIP_PUSH=1` (and/or `WEFT_SKIP_GIT=1` where appropriate) in tests that
    don't need to exercise the real commit/push path, mirroring existing env-based test
    bypass conventions.

- **`warpengine` (`internal/warpengine/junction_test.go` or a new
  `unjunction_test.go`)** — unit-level coverage for `UnwireJunctions` in isolation:
  happy path (valid junction + exclude entry removed), real-directory guard, target-
  mismatch guard, and the RelPath-subpath case (mirroring
  `TestRemoveSubpathJunction`'s existing subpath scenario, since nested `_lyx` at
  `RelPath != "."` is a real, already-tested geometry case).

- **`gitignore` (`internal/gitignore/gitignore_test.go`, extend existing file)** —
  `Remove`: entry present → removed and block deleted (only entry); entry present
  among others → block survives with entry gone; entry absent → `changed=false`,
  file/content untouched; no `.gitignore` file at all → `changed=false`, no file
  created.

- **`weftengine` (`internal/weftengine/sync_test.go`, extend existing file)** —
  `Commit` with an explicit message parameter: verify the passed message lands in the
  commit (not the old hardcoded constant), and that the three existing `weftcli`
  behaviors are unchanged when passing `DefaultCommitMessage` explicitly.

Avoid prescribing exact assertion shapes for the JSON output contract — mill-plan
should design that, but it must carry enough fields to distinguish "removed/cleared/
reverted" from "was already absent/unchanged" for each of: host junction, weft-side
content, git-exclude, gitignore (symmetric to `runInit`'s existing status-map JSON
shape).

## Q&A log

- **Q:** Standalone `lyx deinit` command, or a flag on `lyx init`? **A:** `lyx init
  --undo`. This function will rarely be used in normal workflows — mainly handy for
  test/sandbox cleanup — so it doesn't need its own command surface.
- **Q:** The weft-side `_lyx` directory is git-tracked in the weft branch. How should
  `--undo` clear it? **A:** Delete it — but remember that all git operations against
  the weft repo must go through the weft module (`weftengine`), never raw `gitexec`
  calls from `initcli`/`warpengine` directly.
- **Q:** If the host junction is a link but points somewhere other than the canonical
  `WeftLyxDirFor(slug)` (e.g. stale after a slug rename), what should `--undo` clear?
  **A:** This is a serious error — something has gone genuinely wrong — and must not be
  overlooked (i.e., hard-error, don't guess-and-clear).
- **Q:** Should `--undo` also strip the `_lyx` line(s) it added to
  `.git/info/exclude`, for full symmetry with the `.gitignore` revert? **A:** Yes,
  remove them.
- **Q:** Should `--undo` have the same "no weft pairing" hard gate that plain `init`
  has? **A:** Again: a state where this gate would matter (host artifacts present but
  weft pairing missing/broken) is a serious error and must not be overlooked — same
  reasoning as the target-mismatch answer above, which is what shaped the "no separate
  gate, but the validation steps still catch real inconsistency" decision.
- **Q:** After committing the weft-side deletion, should `--undo` also push it to the
  remote, or leave it local/unpushed? **A:** Auto-push, matching the existing pattern
  where other weft-mutating commands push automatically.
- **Q:** `weftengine.Commit` hardcodes its commit message to `"weft sync"`. How should
  `--undo`'s commit be labeled? **A:** Extend `weftengine.Commit` to accept a message
  parameter, so the undo path's commit is accurately labeled instead of reusing the
  generic `"weft sync"` message.
- **Q:** (discussion-review round 1 gap) When a hard-error guard fires — real
  non-junction `_lyx` directory, or a junction pointing at the wrong/missing target —
  should `--undo` abort immediately (touching nothing else), or still run the other
  independent steps (weft-content clear, gitignore revert, exclude revert) around the
  blocked junction? **A:** A `_lyx` directory in the host that isn't a proper junction
  is a serious error, and no fix should even be attempted — full abort, nothing else
  runs in that invocation.
