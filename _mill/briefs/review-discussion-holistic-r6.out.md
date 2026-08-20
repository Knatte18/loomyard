MILL_REVIEW_BEGIN
# Review: Bouncer: the generic review-gate producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model per environment metadata; exact version not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-20
```

## Findings

### [BLOCKING:consistency] Seed fallback trigger stated two ways
**Section:** "Seed call — spawns, with a mechanical fallback" (l.259-260) + "Focus-file synthesis" (l.175, l.184) + Testing (l.562)
**Issue:** The seed fallback fires "when the seed spawn fails at any point" (l.175, l.260) but l.184 says "Both synthesis paths fire on absent **or** unparseable" — these disagree for a `Run` error / non-`done` outcome where the agent *did* write a valid `round-1-focus.md`, and the test bullet at l.562 asserts `round: 1` with both lists empty for *every* failure mode, which contradicts the l.184 reading and also contradicts this design's own harvest principle ("never discard a judgment the spawn actually produced"), which is defined for the judge call only and has no stated seed-side analogue.
**Fix:** State one rule — the seed fallback fires only when `round-1-focus.md` is absent or unparseable (the seed-side twin of harvest) — and re-word the l.562 scenario so a spawn failure that nonetheless left a parseable focus file keeps that file.

### [NIT:scope] `manifest/designs/shed.md` edit left unspecified
**Section:** Scope (l.33), against `manifest/designs/shed.md:294-307`
**Issue:** The `doc.go` edit list pins five sections and names the sentence that becomes false, but the shed.md obligation is only "a note in the engine-adapter section"; that section's live claims — "one adapter per distinct *engine* type" (l.302) and the perch/SingleLLM enumeration (l.305-307) — are exactly what a second `shuttleengine`-backed adapter carrying new logic falsifies, the same falsehood the discussion is precise about for `doc.go:4`.
**Fix:** Name the shed.md amendment with the same precision as the `doc.go` list: which sentence changes and to what.

### [NIT:design] `ResolveRound`'s non-ENOENT stat error unspecified
**Section:** "An exported round-resolution helper" (l.389-404)
**Issue:** The contract errors only on an unreadable `runDir`; a report file whose `os.Stat` fails for a reason other than not-exist (permissions, I/O) would then read as "absent", silently truncating the contiguous scan or returning `0` and re-seeding an already-judged segment — the exact "must never look like a fresh segment" failure the runDir clause exists to prevent.
**Fix:** Pin that any stat error other than not-exist is returned as an error, not treated as an absent report.

## Verdict

REQUEST_CHANGES
Seed-fallback trigger is specified two incompatible ways; two minor gaps besides.
MILL_REVIEW_END
