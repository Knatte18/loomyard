MILL_REVIEW_BEGIN
# Review: websterengine + webstercli told-geometry, and Webster standalone entry

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] `<state>` is the operator's real home dir in tests
**Section:** the-two-roots / new-package-internal-standalonegeom / Testing
**Issue:** `standalonestate.Derive` resolves `<state>` from live `XDG_STATE_HOME`/`HOME`/`LOCALAPPDATA` (`standalonestate.go:31-44,85-91`), so the pinned tier-1 wiring-function test, the `RunCLIIn` integration test, and the standalone `stencilstore.Reconcile` seed all write into `~/.local/state/lyx/<hash8>/**` — outside any `t.TempDir()`; the discussion never states an env-redirect or injection seam, and never says whether `standalonegeom`'s builders take `(stateDir, hash8)` as parameters or call `Derive` themselves.
**Fix:** pin the builder signature as told `(target, stateDir, hash8)` (Derive called only at the CLI boundary) and state how both test tiers redirect `<state>`, since Testing already assumes a "stubbed `<state>`/`hash8`".

### [BLOCKING:design] nil-Bisector path rests on a false premise
**Section:** standalone-integration-failure-is-recorded-with-unknown-localization
**Issue:** "takes `BisectAndEscalate`'s existing no-SHA path" only holds for empty `shas`; in standalone `accumulatedCardSHAs` is normally non-empty, and `bisect` returns index 0 for one SHA (recording a real SHA, not `"unknown"`) and calls `repo.CurrentBranch()` for two or more (`integration.go:98-112`) — a nil-pointer panic with a nil bisector.
**Fix:** state explicitly that standalone bypasses `BisectAndEscalate`/`bisect` and calls `RecordIntegrationFailure`/`AppendIntegrationFailure` directly, and pin the non-panic test at ≥2 accumulated SHAs (a 0- or 1-SHA fixture passes under the broken implementation).

### [BLOCKING:consistency] "existing reed suite stays green unchanged" is false
**Section:** Testing → `internal/reedengine`
**Issue:** three test files build `reedengine.Geometry` literals directly rather than through `hubgeom` (import cycle) — `contract_integration_test.go:409,517`, `mouse_boot_integration_test.go:48` — and each drives a real tmux `new-session`/`split-window`; once the spawn sites read `PaneCwd`, those literals spawn with `-c ""`.
**Fix:** name these three literals as in-scope edits (add a `PaneCwd` row) and drop the "unchanged" claim, or state the fallback that makes an unset `PaneCwd` behave as today.

### [NIT:consistency] reedengine geometry doc obligations are incomplete
**Section:** Constraints → Documentation Lifecycle
**Issue:** the list names only `geometry.go:1`'s "seven-field" sentence and the new field's comment, but `AnchorPath`'s own comment (`geometry.go:20-22`, "and the cwd every pane is spawned with") becomes false, and `geometry.go:12-13`/`doc.go:23` state `ServerName(hubPath)` is SocketKey's derivation and hubgeom is *the* teller — both contradicted by standalonegeom's pinned `lyx-<hash8>`.
**Fix:** add those three doc sites to the same-commit list.

### [NIT:consistency] batcher move leaves two stale statements unlisted
**Section:** batcher-moves-to-the-degrading-side / constraints-rewords-land-here-not-in-t8
**Issue:** only the pinned sets are listed, but CONSTRAINTS' Config Strictness "watch item for T7/T10" paragraph still asserts batcher is strict for lack of a standalone entry, and `internal/batcher/config.go:26-33`'s `Active` doc comment documents the strict absent-`_lyx` error verbatim.
**Fix:** name both as same-commit edits alongside the pinned-set move.

## Verdict

REQUEST_CHANGES
Three blocking gaps: state-dir hermeticity, the nil-bisector premise, and reed's direct Geometry literals.
MILL_REVIEW_END
