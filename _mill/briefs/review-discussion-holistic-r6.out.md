MILL_REVIEW_BEGIN
# Review: Make producer engines runnable without a lyx worktree

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-fable-5 (Fable 5)
reviewed_file: _mill/discussion.md + manifest/designs/producers-standalone.md + manifest/roadmap.md
date: 2026-08-17
```

## Findings

### [BLOCKING:scope] Root pre-run stencil seeding keeps the tier-1-only trigger
**Section:** design doc, T6 "The trigger is tier 1 AND tier 2" **Issue:** `cmd/lyx/main.go:97` sets `cobra.EnableTraverseRunHooks`, so root's `seedStencils` (`main.go:87`) runs before every module pre-run; it triggers on bare `lyxcwd.Resolve` success (`stencilseed.go:51-56`), so in the doc's own downloaded-repo scenario it calls `seedStencilsAt(l.HubPath,…)` with the fictional parent-dir hub and `stencilstore.Reconcile` writes `<repo-parent>/_board/_lyx/stencils/**` — the exact fictional-hub write T6's trigger analysis exists to prevent, and no task states this site's disposition (T6 touches `stencilseed.go` only to swap `buildChannel` for `buildinfo`). **Fix:** assign the root pre-run's trigger a disposition in T6 or T8 — gate `seedStencils` on the same tier-1-AND-tier-2 check, or state why the fictional-hub seed write is accepted.

### [NIT:design] Wired-but-broken hub silently degrades to standalone
**Section:** design doc, T6 mode selection **Issue:** "Everything else is standalone" decides the `(resolved, hub-damaged)` row implicitly — a worktree whose junctions broke selects standalone, relocating config reads and `.lyx` state to `<state>` with no signal that hub breakage is being masked. **Fix:** state the consequence explicitly (silent-degrade accepted, or a distinguishable warning/refusal when tier 1 passes with hub-shaped geometry but tier 2 fails).

### [NIT:consistency] "--stencils-dir is only ever read" vs bootstrap-on-first-use
**Section:** design doc, T6 `--target-dir` refusal rationale; discussion Q&A r5 **Issue:** the in-hub-honour rationale calls `--stencils-dir` "a directory that is only ever *read*", but T6 also bootstraps the flag's directory via `Reconcile` (a write), and whether an explicit `--stencils-dir` is bootstrapped in hub mode is unstated. **Fix:** pin bootstrap scope (standalone default only, or any told dir) and reword the read-only rationale to match.

### [NIT:consistency] Stencil Ownership reword scope misses the seed-pass bullet
**Section:** design doc, T6 "Invariant rewords land in this task's own commit" **Issue:** T6 names only the read-location reword, but the invariant's "seed/refresh pass runs once per process at `cmd/lyx`'s root pre-run" bullet is also falsified by standalone's bootstrap-on-first-use inside a module pre-run. **Fix:** include that bullet in T6's `CONSTRAINTS.md` edit.

## Verdict

REQUEST_CHANGES
Verified evidence is precise throughout; one unassigned fictional-hub write site in the root pre-run blocks.
MILL_REVIEW_END
