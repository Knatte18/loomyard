MILL_REVIEW_BEGIN
# Review: Reduce git spawns in warpengine integration tests — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-14
```

## Findings

### [BLOCKING] Status half of spawn-count guard is vacuous
**Location:** batch 1 / card 4 (`TestResolveSpawnsDoNotScale`)
**Issue:** `Status` reaches the per-iteration `Resolve`/`hostLayoutFor` (status.go:129) only *after* the weft-exists check (status.go:110-116); a worktree with no weft sibling hits `continue` first. Card 4 adds bare hub-sibling host worktrees via `git worktree add` with no weft counterpart, so pre-fix `--show-toplevel` for `Status` stays at 1 (prime only) for both N=2 and N=4 — the non-growth assertion passes even with the per-iteration `Resolve` regression present, so it locks nothing. (Reconcile is fine: reconcile.go:113 runs `Resolve` before the weft check, so it scales with N.)
**Fix:** For the `Status` measurement, pair each added host worktree with an existing weft sibling (a full pair, e.g. via `w.Add` as reconcile_test.go does) so `Status` passes the `os.Stat(weftPath)` gate and reaches `hostLayoutFor` for every added worktree.

### [NIT] Card 4 guard comment overstates Status scaling
**Location:** batch 1 / card 4 (guard comment)
**Issue:** The prescribed comment says pre-change `Status`/`Reconcile` "called `Resolve` once per enumerated worktree, so `--show-toplevel` scaled linearly with worktree count" — true for `Reconcile`, but `Status` only spawns `Resolve` for worktrees whose weft sibling exists.
**Fix:** Once the fixture pairs each added host worktree (finding above), the statement becomes accurate; word it as "per enumerated worktree with a present weft sibling" or ensure all added worktrees are paired.

## Verdict

REQUEST_CHANGES
Status spawn-count guard is vacuous as specified; fix the card-4 fixture to pair added worktrees.
MILL_REVIEW_END
