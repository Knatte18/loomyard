MILL_REVIEW_BEGIN
# Review: self-report Tier 2: per-agent friction notes for unsupervised runs

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-12
```

## Findings

### [BLOCKING:design] mergeresolve exclusion rests on inverted ordering
**Section:** Decisions § "Seven spawn sites get the directive; Bouncer and mergeresolve do not"
**Issue:** The rationale says a `mergeresolve` note "would land after aggregation has already read the directory" — but `mergeresolve.New`/`Resolve` is called synchronously inside `landingshed/finalize.go:86` and `publish.go:77`, i.e. inside `shed.Run`, which returns before `drive.go:132`'s `Reflect` call, so such a note *would* be aggregated.
**Fix:** Either re-decide with a true premise (e.g. exclude it because the resolver is a narrow, strictly-contracted session like the Bouncer's) or include it; do not leave the stated reason contradicting the trigger ordering the discussion itself establishes.

### [BLOCKING:design] Note ids are per-site, not per-invocation
**Section:** Decisions § "Filename uniqueness comes from a caller-supplied id"
**Issue:** The premise "every one of the seven spawn sites already holds a unique identity" does not hold across repeated invocations: `recoverSpawn` is re-runnable for the same batch (it timestamp-archives a stale report each time, `recoverbatch.go:126–154`), and a producer row re-executes on crash-resume while notes are deliberately retained — so `<batch>-recovery`, `Discussion-Write`, `Plan-Write` collide and the later note silently overwrites the earlier, exactly the failure the decision rejects for agent-chosen names.
**Fix:** Decide an invocation-scoped disambiguation rule (e.g. the non-clobbering suffix scheme webster already uses for stale reports, or an attempt/round counter) and state it in the id table.

### [NIT:consistency] `logger.Info` names a report path that no longer exists
**Section:** Decisions § "The reflection step can never change the run's outcome" (l.260) vs § "Consumed notes are archived"
**Issue:** On `"reflected"` the `Info` line names "the report file's path", but a clean return has already renamed the friction directory to `friction-<ts>/`, so the pre-archive path is stale by the time it is logged.
**Fix:** State which path is logged — the post-archive location inside `friction-<ts>/`.

### [NIT:consistency] Several cited line numbers have drifted
**Section:** Decisions (l.78) and § "A stencil that never got the marker warns" (l.248)
**Issue:** `burlerengine/engine.go`'s `MkdirAll` is at `:113` (not `:112`); `stencilstore/reconcile.go`'s dev-build and edited-stencil warns are at `:106`/`:126` (not `:98`/`:124`). The behaviours cited are correct; only the anchors are stale.
**Fix:** Refresh those three citations so the plan writer's greps land.

## Verdict

REQUEST_CHANGES
One false-premise exclusion and one unsound id-uniqueness premise need resolving before planning.
MILL_REVIEW_END
