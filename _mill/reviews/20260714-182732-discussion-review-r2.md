MILL_REVIEW_BEGIN
# Review: Investigate the unexplained lyx mux server crash

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [GAP] debug_log field type breaks "clear non-numeric error"
**Section:** Decisions § debug-log-config-key
**Issue:** The decision promises the Go level→argv helper emits "a clear error for non-numeric env input," but if `Config.DebugLog` is an `int` (the key is described as int), `yaml.Unmarshal` fails first with a cryptic `cannot unmarshal !!str into int` before the helper ever runs — the resolved template value is unquoted, so `LYX_MUX_DEBUG=abc` yields `debug_log: abc`.
**Fix:** Specify `DebugLog` as a `string` field on the Config struct so the helper parses/validates it (numeric + 0–2 range) and owns the clear error, or state where numeric parsing happens ahead of unmarshal.

### [NOTE] New required key forces reconcile on existing hubs
**Section:** Scope In / Decisions § debug-log-config-key
**Issue:** `configengine.Load` returns `missing keys: debug_log; run "lyx config reconcile"` when the template gains a key the on-disk mux.yaml lacks — so every already-initialized hub (including the sandbox Hub that needs the logging) fails all `lyx mux` verbs until reconciled; not mentioned in scope or migration.
**Fix:** Note the reconcile requirement for existing hubs in the plan (and any operator-facing help), matching the repo's strict-template contract.

### [NOTE] Stale doc-comments quote the old dead-session wording
**Section:** Decisions § resume-hint-in-requireSessionLocked (Note)
**Issue:** The note flags only *tests* asserting the old string, but code comments quote the exact old wording `no mux session; run "lyx mux up"` in `internal/muxengine/strand.go` (lines ~308, 351, 411) and `internal/muxcli/attach.go` (line ~53); they go stale when the message is enriched.
**Fix:** Include those comments in the update sweep alongside the tests.

## Verdict

GAPS_FOUND
One feasibility gap (debug_log field type vs non-numeric error) needs resolving before planning.
MILL_REVIEW_END
