MILL_REVIEW_BEGIN
# Review: fabric: live-state integration harness (slice 13)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic, Opus-class; self-assessment)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Checkout's dirtiness rows rest on a nonexistent state
**Section:** `tranche-1-state-matrix` + `three-expectation-kinds-and-the-scope-table`
**Issue:** The `Proceeds` rationale cites "`dirtyWarpUntracked` — and any untracked-only weft state — must make `Checkout` succeed", but the nine states contain no `dirtyWeftUntracked`, and `checkout.go:42` probes *only* `worktreeDirty(scopeTracked, weftWorktree)` — so `dirtyWarpUntracked × Checkout` proceeds vacuously (no warp probe exists at all), and the weft-side scope divergence against `Remove`'s `scopeAll` weft probe (`remove.go:79`) is never exercised by any cell.
**Fix:** Either add a `dirtyWeftUntracked` state (the cell that actually proves the scope parameter is real) or re-cite the divergence to the warp-side pair (`Add` scopeTracked proceeds vs `Remove` scopeAll refuses) and say so explicitly.

### [BLOCKING:design] `dirtyWarpTracked × Checkout` has no derivable expectation
**Section:** `three-expectation-kinds-and-the-scope-table` (scope table + derivation rule)
**Issue:** The derivation rule covers only "scopeTracked vs scopeAll against an untracked-only state"; `Checkout` has no warp-side dirtiness probe anywhere (`checkout.go:38-85` goes straight to `git switch`), so a tracked-dirty prime warp yields a git-decided outcome — success carrying changes across, or a `git switch` failure with rollback — and the table has no `Checkout`/warp row to derive from.
**Fix:** Add the missing row (or an explicit omission-with-reason) stating the expected outcome for tracked-dirty warp under `Checkout`, including whether a `switch` failure must leave no half-switched pair.

### [BLOCKING:design] Manifest walk's link-traversal rule is unstated
**Section:** `survival-assertion-mechanism` (the per-entry record)
**Issue:** The entry record defines kind `link` plus `RawTarget`, but the document never says the walk *stops* at a link; wired junctions carry absolute targets into the weft sibling *inside the same hub*, so a descending walk double-records weft content under both the weft path and each warp junction path, and every legitimate weft `_lyx` change then reports as an unpermitted mutation under a warp key. Traversal of a Windows junction differs from a POSIX symlink, so this is not settled by "it works on Linux".
**Fix:** State explicitly that the walk records a link as a leaf and never descends through it, on both platforms, and add it to the Windows portability rule list.

### [BLOCKING:consistency] 182-cell count contradicts the omit-with-reason rule
**Section:** `tranche-1-verb-table` (resulting cell count) vs `dirty-what-per-cell`
**Issue:** `8 ordinary verbs × 9 states × 2 anchors = 144` assumes a full product, but `dirty-what-per-cell` mandates omitting `Cleanup`'s four structural-state cells (−8 cells) and omits any other verb/state pair with no nameable path; the stated 182 total is therefore wrong by at least 8, and unresolvable until the omission set is enumerated.
**Fix:** Enumerate the omitted (verb, state) pairs with their reasons and restate the total from that enumeration, or drop the exact total in favour of the enumeration alone.

### [NIT:consistency] "Nine sabotage rows" does not map onto nine cells
**Section:** Testing → Sabotage-proving (completion gate)
**Issue:** The completion gate requires nine rows keyed to the "nine evidence-table scenarios", but that bullet list contains one bullet covering six hostile inputs, one covering two distinct states, and one ("every `clean`-state cell") that is not a cell at all.
**Fix:** Name the nine concrete cells (state, verb, anchor) the sabotage table's rows key on.

## Verdict

REQUEST_CHANGES
Three unresolved mechanism/derivation gaps plus a self-contradicting cell count.
MILL_REVIEW_END
