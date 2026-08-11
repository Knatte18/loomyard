MILL_REVIEW_BEGIN
# Review: batcher: split out of webster into a standalone configreg module with its own batcher.yaml

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4 class (Anthropic), exact build unverifiable from inside
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:scope] Doc-site list misses in-code file-header claims
**Demoted-from:** BLOCKING
**Section:** Decisions → doc-amendments (the nine-site list, which explicitly "claims completeness for sites whose claim this task falsifies")
**Issue:** The enumeration only reached package `doc.go` files and `.md` docs, so it misses three production file-header/struct-doc comments that name webster as `Select`'s caller: `internal/batcher/registry.go:1–3` ("webster resolves the config-chosen active batcher back out by name via the exported Select" — false once `webstercli` calls `Active` and no webster code calls `Select`), and `internal/websterengine/recordbatch.go:34` / `beginbatch.go:52` ("the batchifier-derived execution batches (see internal/batcher.Select) `run` computed once at entry" — `run` will compute them from the injected `RunDeps.Batcher`).
**Fix:** Either widen the enumeration method to all production comments naming `batcher.Select`/webster-owned batcher config (grep `batcher.Select` across `internal/**/*.go`, not just `doc.go`) and add the resulting sites as numbered steps, or state explicitly that these pointer-style comments are out of scope and why the completeness claim survives.

## Verdict

APPROVE
Doc-site enumeration claims completeness but misses three falsified in-code comments.
MILL_REVIEW_END
