I have verified the key claims against source. The technical details in the discussion check out (weft commit message fixed to "weft sync", warp reconcile has no `--apply`, config reconcile is dry-run by default, init requires weft pairing and calls `gitignore.Ensure(cwd, ".lyx/")`). I found one substantive gap around S6's cleanup scope and one accuracy note about board resolution.

MILL_REVIEW_BEGIN
# Review: Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-01
```

## Findings

### [GAP] S6 cleanup misses weft-side junction target
**Section:** Decisions → "Subfolder init scenario (S6)" durability note
**Issue:** `init` runs `WireJunctions` first, so the nested `_lyx/` is a *junction* into `<hub>/<slug>-weft/<subdir>/_lyx` (`warpengine/junction.go`), and `ReconcileAll(cwd, true)` writes the module YAMLs *through* that junction into the weft worktree — the cleanup note only names "remove the nested `_lyx/` and revert `.gitignore`", leaving the weft-side `<subdir>/_lyx/config/*.yaml` (and any weft commit) behind, so a later run still sees "exists" not "not yet initialized".
**Fix:** State that S6 cleanup must also remove the weft-side `<slug>-weft/<subdir>/_lyx` target (or clarify that deleting through the junction achieves this), else the durability guarantee the note exists to provide does not hold.

### [NOTE] "board resolves against subdir's own _lyx" is only half true
**Section:** Decisions → "Subfolder init scenario (S6)"; Scope/In
**Issue:** `board`'s data dir is `hubgeometry.BoardDir(layout.Hub)` = `<hub>/_board` (hub-level, cwd-depth-invariant — `boardcli/cli.go` L103); only its *config* read is subfolder-scoped. Running `board list` from the subdir returns the same hub board as from root, so it does not demonstrate subfolder-scoped resolution the way `config` does.
**Fix:** Frame `config` as the subfolder-scoping demonstrator; treat `board`-from-subdir as a "still works from subdir" smoke check, not proof that resolution is subdir-scoped.

## Verdict

GAPS_FOUND
S6's durability cleanup underspecifies the weft-side junction target it must remove.
MILL_REVIEW_END