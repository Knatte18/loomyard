MILL_REVIEW_BEGIN
# Review: the standalone CLI path

```yaml
duration_s: 162.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Residual-class claim is false for Resolve failures
**Section:** § Decisions → mode-trigger
**Issue:** The decision claims "the requirement is met for every damage class that leaves `<hub>/_board/_lyx` intact", but `preflight.HubPresent` returns `(nil, false)` whenever `lyxcwd.Resolve` errors (`predicates.go:58-61`), and `Resolve` errors with `ErrCwdOutsideAnchor` for any cwd that is not exactly `AnchorPath()` and with `ErrStaleAnchorMarker` on a stale marker (`lyxcwd.go:67-78`) — both in a fully healthy hub with `<hub>/_board/_lyx` present, so `lyx burler run` from a subdirectory of a wired worktree silently becomes a standalone session writing into the per-OS state directory instead of today's loud cwd-gate error.
**Fix:** State the disposition for the `Resolve`-error class explicitly (accept-and-record as a second residual, or refuse when `Resolve` fails with anything other than `ErrNotAGitRepo`) and correct the "every damage class" sentence, which a plan writer would otherwise quote as settled.

### [NIT:consistency] "Exactly two deviations" omits the not-a-repo case
**Section:** § Decisions → hub mode is byte-identical
**Issue:** The enumeration names the plain-git-repo reclassification and the three envelope fields as the only two deviations, but the Problem section's *first* symptom — a non-git-worktree cwd where the pre-run aborts today (`burlercli/cli.go:68-74`) — now succeeds standalone, and it is not on the list even though the sibling non-hub case is.
**Fix:** Add it as a third named deviation, or restate the enumeration as covering hub-mode resolved paths only, so "any third deviation is a bug in this plan" stays checkable.

### [NIT:scope] `resultEnvelope` signature change hits a pinned test
**Section:** § Technical context → Files this task edits
**Issue:** `internal/burlercli/run.go:182`'s `resultEnvelope(result)` gaining three parameters breaks `TestResultEnvelope_ForkCountNilGuard` (`cli_test.go:285`, `:305`), but the `cli_test.go` table row names only the `TestRunCLI_Run_MissingProfile` state-root redirect.
**Fix:** Extend that row to name the envelope-shape test as a second reason the file is edited.

### [NIT:scope] perch's reporting fields are not enumerated
**Section:** § Decisions → perch's three `layout` uses / operator visibility
**Issue:** burler's decision lists `mode`/`stateDir`/`stencilsDir` as receiver fields the wiring branches set, but perch's decision lists only `stencilsDir`/`anchorRel`/`openFabric`, while its run verb must read `mode`/`stateDir` off the same receiver.
**Fix:** Name the same three reporting fields on `perchCLI` so both CLIs' receiver inventories are complete.

## Verdict

REQUEST_CHANGES
One false residual-class claim hides a silent standalone fallback inside healthy hubs.
MILL_REVIEW_END
