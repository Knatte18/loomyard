MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
duration_s: 196.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] Remove's envelope is hand-built, not struct-driven
**Demoted-from:** BLOCKING
**Section:** Technical context → "Envelope" ("New result fields ride the existing result structs; no envelope change is needed").
**Issue:** `runRemoveWithFlag` (`internal/fabriccli/fabric.go:810-814`) passes an explicit `map[string]any{"slug","path","links_removed"}` to `okWithRecord`; `RemoveResult` is never marshalled, so `RemoteBranchDeleted`/`RemoteBranchError` would be invisible in `lyx fabric remove`'s JSON. (Cleanup is fine — it passes `r.Entries`.)
**Fix:** State that `runRemoveWithFlag`'s fields map gains the two remote keys, and pin the exact JSON key names there.

### [BLOCKING:design] Verdict/exit code for a failed remote deletion unstated
**Section:** Decisions → remote-failure-is-non-fatal; per-entry-remote-reporting.
**Issue:** `doc.go:560-572` fixes the repo rule: a per-item failure must reach the caller as a failure, and `prune`/`cleanup` are carved out only because their `Error` explains a *designed refusal*. An offline/auth/protected-branch push failure is not a designed refusal, yet the decision routes it through `okWithRecord` → `"ok":true`, `"partial":false`, exit 0, with the real failure buried in a field.
**Fix:** Decide and record explicitly whether `--remote` failure keeps exit 0 / `partial:false`, and reconcile that against doc.go's per-item-failure rule (an amendment to the carve-out, or the failure path).

### [BLOCKING:design] "Whatever meta-test pins destroy.go" is not an enumeration
**Section:** Testing → "Enforcement tests to expect".
**Issue:** Deferring to "whatever meta-test pins destroy.go's executor or primitive set; find it before writing" leaves the plan writer to rediscover named, closed ledgers that exist today: `cmd/lyx/destructiveguard_test.go`'s `destructiveGuardBannedTokens` (raw-substring ban list — the sixth primitive gets no chokepoint enforcement unless a token for `push --delete` is added) and `destructiveGuardRecordingExecutors` (per-executor row), plus `manifestObservableKind` (`internal/fabricengine/livestate_mutationoracle_test.go:32`), a closed map that fails loud on any new `Kind` and therefore must classify `KindRemoteBranchDeleted` as a git-state kind.
**Fix:** Name these artefacts and their required dispositions in the Testing/Scope sections rather than delegating the search.

### [NIT:consistency] Primitive enumeration lives in destroy.go, not CONSTRAINTS.md
**Section:** Scope → In, bullet 7.
**Issue:** "the Fabric Destruction Chokepoint Invariant's enumerated primitive list grows from five to six" — CONSTRAINTS.md's invariant enumerates no primitives; the five-primitive list is `destroy.go:1-4`'s header (whose recording contract also says "eight executors").
**Fix:** Point the bullet at destroy.go's header (both the primitive count and the executor count), and state what, if anything, changes in CONSTRAINTS.md.

### [NIT:design] "origin not configured" folded into the generic failure path
**Section:** Decisions → absent-remote-ref-is-success / hardcoded-origin.
**Issue:** Only "remote ref does not exist" maps to `(false, nil)`; a weft repo with no `origin` remote yields one identical git error per orphan branch across a whole `cleanup --apply --remote`.
**Fix:** Say explicitly that this is intended noise, or name it as a second idempotent-success case.

## Verdict

REQUEST_CHANGES
Envelope claim is wrong for Remove; failure-verdict and enforcement-ledger dispositions are unstated.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 2._
MILL_REVIEW_END
