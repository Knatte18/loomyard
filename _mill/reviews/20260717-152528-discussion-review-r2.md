MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [GAP] Missing-required-field detection unspecified
**Section:** status-json-typed-and-strict / no-half-finished-prior-run / Testing (Tier 1)
**Issue:** `DisallowUnknownFields` rejects *extra* fields but silently zero-fills *absent* ones, so a plain-typed struct cannot detect a missing required field (e.g. absent `start_sha`/`pause_requested`/`next_action`/`bounced_to`) — yet the test plan asserts "Missing required field → seed-incoherent" and the schema checklist requires all nine fields "present".
**Fix:** Pin the presence-detection mechanism and its scope — pointer/`json.RawMessage` fields (true presence) vs a pragmatic non-empty check on the string fields only — and reconcile which of the nullable/bool fields must be structurally present vs may zero-fill.

### [NOTE] ReadJSONStrict conflates read errors with parse errors
**Section:** strict-read-mechanism (Error classification) / result-error-contract
**Issue:** `ReadJSONStrict` wraps both `os.ReadFile` failures ("read state") and decode failures ("unmarshal state") as one `err` (state.go:62-73); routing every non-nil `err` to `seed-incoherent` misclassifies a genuine I/O read error as a determined precondition failure instead of the escalated infra `error` result-error-contract reserves for "something broke while checking."
**Fix:** Clarify that a read/IO error surfacing through `ReadJSONStrict` (vs a decode error) maps to `seed-unreadable` or the escalate path, not `seed-incoherent`.

## Verdict

GAPS_FOUND
One gap on missing-field detection must be resolved before planning; one classification note.
MILL_REVIEW_END
