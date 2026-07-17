MILL_REVIEW_BEGIN
# Review: Extend codeintel lookup to non-Go languages via LSP

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [NOTE] Hung-server teardown may itself block
**Section:** Decisions → lsp-client-surface (Deadline / cancellation contract)
**Issue:** On `ErrServerTimeout` the server is by definition unresponsive, yet the recovered client's `close` does a graceful `shutdown`/`exit` handshake that can also hang, so "tears down the subprocess" needs a forced process kill, not the graceful path.
**Fix:** State that timeout teardown hard-kills the subprocess (skips/bounds the graceful `shutdown`/`exit`) so cleanup can't re-block on the same fault.

### [NOTE] TypeScript/Rust built-ins ship unmeasured
**Section:** Scope → In / Decisions → measurement-matrix
**Issue:** Registry built-ins include TypeScript and Rust, but the measurement matrix covers only Go, Python (pyright+pylsp), and C#, so two shipped entries are never validated against a live server.
**Fix:** Note explicitly that the TS/Rust entries ship as unverified registry config (marker→command only), out of the measurement scope, so a plan writer doesn't assume they need benchmark validation.

### [NOTE] Overlay schema for markers + AND/OR match-mode undefined
**Section:** Decisions → language-server-registry / language-detection
**Issue:** Entries carry markers, an all-of/any-of match-mode, command, and install-hint — a richer shape than modelspec's Engine/Model/Defaults — but the overlay YAML field names and the match-mode's closed vocabulary (with loud validation) are left implicit.
**Fix:** Confirm (even briefly) that match-mode is a validated closed vocab riding the same `KnownFields(true)` loud-error path, so mill-plan pins the entry schema rather than inventing it.

## Verdict

APPROVE
Design is complete and grounded; remaining items are non-blocking clarifications for the plan.
MILL_REVIEW_END
