MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus 5 (Anthropic), best-effort self-assessment
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Acceptance grep cannot see deleted-doc link rot
**Section:** Testing → Acceptance commands, item 4 **Issue:** None of the three declared patterns (`builderengine|buildercli`; phase/gate token; module-word list `lyx builder`/`builder.yaml`/`builder-suite`/`builder suite`/`SANDBOX-BUILDER-SUITE`/`_lyx/builder`/`.lyx/builder`) matches a surviving `builder-contract.md` or `plan-format.md` reference — exactly the failure class the discussion itself calls undetectable downstream ("Task B's zero-hit grep … cannot see a dangling `builder-contract.md#…` anchor"). Verified live misses the gate would pass: `manifest/designs/loom.md:29` links `[plan-format.md v2](../../docs/reference/plan-format.md)` (not in any inventory), `docs/reference/status-schema.md:3` links `plan-format.md` alongside the `builder-contract.md` link the discussion does list, `internal/fabricengine/trailer_test.go:42–56` pins the `"builder: <label>"` weft commit-subject form (`builder_style_subject`), and `internal/fabricengine/refscanner_test.go:20,37` uses `master-builder` worktree fixtures. **Fix:** Add deleted-filename patterns (`builder-contract`, `plan-format.md` as a link target) and a `builder:`-prefix/`/builder/` path-fragment pattern to the acceptance grep, or state that the gate is partial and name the residual review obligation.

### [NIT:scope] weftgit_exclude_test disposition rests on a false premise
**Demoted-from:** BLOCKING
**Section:** Technical context → Go sites, `internal/fabricengine/weftgit_exclude_test.go` **Issue:** The instruction to delete `:279`, `:280`, `:285` because "the `webster` fixtures on the adjacent lines already prove it" holds only for the two `.lyx` lines; `:285` writes `_lyx/<rel>/builder/state.json`, the test's only **durable positive control**, and there is no webster durable twin (`:281–282` are `.lyx`-only). It is asserted at `:302–305` (`durable := lyxRel + "/builder/state.json"`), a line the inventory does not list, so the stated edit leaves the test asserting on a file it no longer writes. **Fix:** State the disposition for `:302–305` explicitly — either rename `:285`/`:302` to a `webster` durable fixture (preserving the "does not over-match real state" property documented at `:270–272`) or record that the positive control is dropped and why.

### [NIT:consistency] Wrong line cited for the Fabric Git Invariant start
**Section:** Technical context (`CONSTRAINTS.md:219` note) and Constraints **Issue:** Both places say the Fabric Git Invariant "begins at `:173`"; it begins at `CONSTRAINTS.md:187` (`:219` is correctly inside it). A stale anchor in the very note warning against editing the wrong invariant. **Fix:** Drop the start-line number or correct it to `:187`, consistent with the discussion's own locate-by-content rule.

## Verdict

REQUEST_CHANGES
Acceptance gate misses deleted-doc link rot; one test disposition rests on a false premise.
MILL_REVIEW_END
