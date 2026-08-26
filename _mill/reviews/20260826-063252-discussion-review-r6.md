MILL_REVIEW_BEGIN
# Review: Fix Bouncer anchor-path and run-dir clearing

```yaml
duration_s: 200.0
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class); exact model ID reported to me as claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-08-26
```

## Findings

### [NIT:decision] loom.md "The gate" is In with no disposition
**Section:** Scope → In (Docs) **Issue:** Every other named site carries a reword/delete/leave disposition, but `manifest/designs/loom.md` ("The gate") gets none, and reading it shows no falsified claim there — it delegates to the package docs and never names a root or the four-mode branch — so a plan writer cannot tell whether an edit is owed at all. **Fix:** State the disposition explicitly (e.g. "add the re-entry/clear sentence" or "no change owed; listed only because it is the design home").

### [NIT:consistency] roadmap item text carries the corrected route claim
**Section:** Scope (roadmap Planned → Done) / Problem **Issue:** `manifest/roadmap.md:16` asserts defect 2 is "confirmed present in the shipped `Discussion-Validate` → `Discussion-Write` → `Discussion-Bouncer` path", which the Problem section correctly refutes (`Discussion-Bouncer`'s `on_done` is `Plan-Write`, nothing routes back), and the Scope entry says only "Planned → Done". **Fix:** Say the Done-list summary must not carry the in-run Discussion claim forward.

## Verdict

APPROVE
Decisions complete, inventories reproducible, and every cited site verified against source.
MILL_REVIEW_END
