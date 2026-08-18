MILL_REVIEW_BEGIN
# Review: the standalone CLI path

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (Anthropic Opus-class assistant)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:scope] Flipping-test enumeration misses two perch tests
**Section:** Testing — "Two existing untagged tests flip" **Issue:** `internal/perchcli/run_test.go:39` (`TestRunCLI_Run_MissingProfile`) and `:58` (`TestRunCLI_Run_InvalidRunID`) also `t.Chdir(t.TempDir())` and drive the real `run` subcommand, carrying the same "documented double-failure shape" comment; after this change both enter `wireStandalone` and `Reconcile` into the operator's live state directory from an untagged test — the exact hazard the section names, for four tests, not two. **Fix:** state the enumeration method (every untagged test in either package that chdirs and drives a non-group subcommand) and give all four the same state-root redirect and doc-comment disposition.

### [BLOCKING:design] perch's second stencils consumer has no disposition
**Section:** "perch's three `layout` uses" / flags **Issue:** perch resolves `fabricengine.StencilsDir(layout.HubPath)` at two sites — `run.go:301` (`perchengine.Run`) and `cli.go:152` (`burlerengine.New`'s fourth argument for the nested burler engine) — and the discussion reroutes only the first, leaving unstated whether the nested burler engine gets the same told `stencilsDir` (and therefore whether `--stencils-dir` reaches it in hub mode). **Fix:** state that both consumers take the one told `stencilsDir` from whichever wiring branch ran, and pin it in the perch wiring test.

### [BLOCKING:consistency] Design-doc row absent though its condition is met
**Section:** Constraints / Discovered / Files-this-task-edits **Issue:** Constraints says `manifest/designs/producers-standalone.md` is updated "only if T8's entry needs a correction note", and Discovered then establishes two staleness points (`producers-standalone.md:481`–`497`'s `Ready`-class trigger plus `_board` discriminator, and the CONSTRAINTS rewords) with "the plan should record that divergence" — yet the file table carries no row for it. **Fix:** resolve the condition explicitly: either add the design-doc row with the correction note, or state why the divergence record lives in the plan/commit message and the doc waits for T10's deletion.

### [NIT:design] "wired-but-broken hub is refused" claim is broader than the mechanism
**Section:** mode-trigger decision **Issue:** `preflight.HubPresent` stats `<hub>/_board/_lyx` only, so a hub damaged precisely there returns false and silently degrades to standalone — the requirement is met for damage that leaves that directory intact, not for all damage. **Fix:** record the residual class honestly (as the repo's other invariants do) rather than claiming the requirement is satisfied outright.

### [NIT:consistency] Truth-table row `(resolved, hubPresent=false)` never occurs
**Section:** Testing — mode-selection truth table **Issue:** `preflight.HubPresent` returns `(nil, false)` on both failure paths (`internal/preflight/predicates.go:57-66`), so the plain-downloaded-repo row and the unresolvable row are indistinguishable at `wire`'s boundary — as `webstercli/wiring.go`'s own doc comment states. **Fix:** keep both rows as `wire`-contract cases but note that the caller supplies a nil `loc` for each, so `wireStandalone` must never read `loc`.

## Verdict

REQUEST_CHANGES
Three gaps: an incomplete flipping-test enumeration, perch's second stencils consumer, and the design-doc row.
MILL_REVIEW_END
