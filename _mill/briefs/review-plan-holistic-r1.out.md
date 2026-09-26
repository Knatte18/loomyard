MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-26
```

## Findings

### [BLOCKING:design] New sink-reading tests risk a t.Parallel() data race
**Location:** batch 02-fabric-mutation-trace, card 3
**Issue:** Every existing test in `internal/fabricengine/mutation_test.go` calls `t.Parallel()` as its first line (13 occurrences, no exception). Card 3 adds four new tests there (`TestMutations_AppendLogsOneRecord`, `TestMutations_AppendRefLogsOneRecord`, `TestMutations_ExtendLogsNothing`, `TestMutations_NilReceiverLogsNothing`) that arm `internal/logger`'s package-level durable-sink singleton via `SetDurableSinkDir` and assert on "exactly one line" in the resulting trace file. If written to match the file's own established convention, these would run concurrently with each other (and are unsafe to run concurrently at all, since `sinkDirOverride`/`sinkPath`/`header` are shared mutable package state one test's `SetDurableSinkDir` call resets out from under another). The codebase already documents this exact hazard elsewhere — `internal/burlercli/cli_test.go` states outright "this case is not t.Parallel(): a successful pre-run reaches the real ... trace sink" — but the Shared Decision `trace-tests-use-sink-override` and card 3's requirements never say these four tests must avoid `t.Parallel()`.
**Fix:** Add a line to card 3 (or the Shared Decision) stating the four new sink-reading tests must not call `t.Parallel()`, contradicting the file's otherwise-universal convention.

### [BLOCKING:scope] Two stale-comment fixups named by the discussion are dropped from the plan
**Location:** batch 05-ly-drive-recipe-blind, card 15 (documentation lifecycle)
**Issue:** `_mill/discussion.md`'s own grounding notes (~line 281, "Comments naming ly-drive's retry rule or orphan claim") name four files carrying comments that quote or describe the pre-rewrite skill's retry-rule/orphan-claim/step-cap text: `internal/battencli/step_test.go:27`, `internal/shedadapters/bouncer_seed_test.go:475`, `internal/loomcli/smoke_bootstrapwiring_test.go:150`, `internal/battenshed/doc.go:15-29`. Card 9 rewords the first, card 15 rewords the fourth — but `internal/shedadapters/bouncer_seed_test.go:475` (quotes the old skill verbatim: "there is no orphan: the next step attaches to the agent rather than abandoning it") and `internal/loomcli/smoke_bootstrapwiring_test.go:150` (quotes the old skill's "literal first instruction" as `lyx loom status`, which the rewritten recipe-blind skill never says) are in neither file's `Edits:` anywhere in the plan, nor in `## All Files Touched`. Both comments describe skill text card 13 removes (the loom-specific orphan gloss, and `lyx loom status` as opposed to `lyx shed status`), so both go stale the moment card 13 lands.
**Fix:** Add both files to card 15's (or a new card's) `Edits:`, with a requirement to reword each comment to stop quoting/describing the pre-rewrite skill text.

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps: an unaddressed test-parallelism hazard in card 3, and two discussion-named stale-comment fixups missing from card 15's scope.
MILL_REVIEW_END
