MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-28
```

## Findings

None. Every `### Decision:` in `## Shared Decisions` is faithfully and precisely implemented by a
card (checked against `internal/reedengine/{server,lock,geometry,watchdog,watchloop}.go`,
`internal/hubgeom/hubgeom.go`, `internal/standalonegeom/reedgeom.go`,
`internal/standalonestate/standalonestate.go`).
Card 2's "exactly five files" `Geometry{` inventory was re-verified by grep and matches exactly:
`lock_test.go`, `server_test.go`, `header_test.go`, `contract_integration_test.go`,
`mouse_boot_integration_test.go`; the four stated facts about the latter three (pre-created
worktree dirs, `t.TempDir()`-backed literals, no-literal reliance on `newIntegrationEngine`/
`setupAttachGeometryFixture`) all check out against the actual file contents.
The Batch Index DAG is acyclic and file-accurate; `## All Files Touched` is exactly the union of
`Edits:`/`Creates:` paths across both batches; step numbering is sequential 1–4 with no gaps; every
card has all five required fields; every `Moves:` is bare `none`, so no rename mechanic is required
and none is missing.
Requirements name stable identifiers throughout (`errWorktreeRootGone`,
`validateToldWorktreeRootLive`, `watchdogDormantCycle`, `watchModeDormant`, `tickerPeriodFor`,
`handleWatchOutcome`, `dormantFrom`, etc.), and every identifier a card's `Requirements:` names
traces to a file in that card's own `Context:` or `Edits:`.
Card 4's `handleWatchOutcome` sentinel/recovery ordering (checked line-by-line against the live
`watchloop.go`) composes correctly with the existing deferred/succeeded/promotion logic and the
existing ticker-swap mechanism, with no new tmux round trip added to the dormant tick.
No new integration-tagged test is added anywhere (batch 1's sweep only touches already-conformant
fixtures; batches 1 and 2's new coverage lands in untagged `server_test.go`/`watchloop_test.go`), so
the integration-tests-added-but-not-run criterion does not trigger, and the plan is transparent
about the pre-existing `TestWatchdogSelfHeal_HookProbeMatchesLiveTmux` failure it deliberately does
not touch or hide.

## Verdict

APPROVE
Both batches are internally consistent, fully grounded in the actual source, and faithfully implement every Shared Decision.
MILL_REVIEW_END
