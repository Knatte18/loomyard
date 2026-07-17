MILL_REVIEW_BEGIN
# Review: loom: Preflight phase (precondition validation)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [NOTE] Schema doc "Parse discipline" still says KnownFields(true)
**Section:** status-json-typed-and-strict / Reconciliation with the pinned schema doc
**Issue:** The discussion mandates updating only checklist item 1, but `status-schema.md`'s "Parse discipline" paragraph (line 118) still describes the JSON status.json strict parse as "the same `KnownFields(true)` discipline" — a `yaml.Decoder` API, which the discussion itself flags as wrong for a JSON seed.
**Fix:** Since the doc is edited in the same commit anyway, note that the Parse-discipline wording should also be aligned to `json.Decoder.DisallowUnknownFields()`.

### [NOTE] Lock-acquire error is a third mode outside the two sentinels
**Section:** strict-read-mechanism → Error classification / seed-read-path
**Issue:** `ReadJSONStrict` can fail on `lock.AcquireReadLock` (neither `ErrRead` nor `ErrDecode`), but the error classification only splits `ErrDecode` → `seed-incoherent` vs `ErrRead` → escalate; a lock-acquire failure falls into an unspecified branch.
**Fix:** State that any non-`ErrDecode` error (including lock-acquire) escalates as `error` per result-error-contract, so only `ErrDecode` maps to a Report failure.

## Verdict
APPROVE
Thorough round-5 discussion; claims verified against source, only two minor doc/edge NOTEs remain.
MILL_REVIEW_END
