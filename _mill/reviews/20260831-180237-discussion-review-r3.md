MILL_REVIEW_BEGIN
# Review: Surface merge-in-progress in fabric status

```yaml
duration_s: 144.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Foreign-state exclusion rests on a false premise
**Section:** Scope → Out, "Foreign (plain-git) merge state" **Issue:** The rationale is that surfacing foreign/git-level merge state "is new semantics, not exposing the existing API", but `MergeStateActive(l *lyxcwd.Location) (bool, error)` is already exported (`internal/fabricengine/mergestateactive.go:36`) and answers exactly that question from the `l` that `statusCmd`'s closure already holds (`weft_verbs.go:89`) — no new engine API required, so the exclusion is an unstated design choice, not a mechanical constraint. **Fix:** Name `MergeStateActive` explicitly and give it a disposition with a real rationale (e.g. one-field scope discipline / the roadmap item's wording), rather than a "needs new API" argument the source contradicts.

### [NIT:consistency] Verification command cannot run the new test
**Demoted-from:** BLOCKING
**Section:** Testing, final line **Issue:** The new test uses `hubforge.NewHub`, which the Test Tier Purity Invariant bars from untagged files (every existing peer, e.g. `internal/fabriccli/merge_cli_integration_test.go:1`, carries `//go:build integration`), yet the discussion prescribes verification "over `./internal/fabriccli/...`" with no `-tags integration`, so the stated TDD red step and both scenarios would silently not execute. **Fix:** State that the new test file carries the `integration` build tag and that verification runs it with `-tags integration` (plus the untagged tier for the regression pins).

### [NIT:scope] No disposition for the error-path decision's coverage
**Section:** Testing, scenarios 1–3 vs Decisions → `error-handling` **Issue:** The decision that a non-nil `MergeInProgress()` error fails the verb has no named test scenario and no statement that it is deliberately left uncovered. **Fix:** Add one line either naming a scenario or recording it as intentionally untested (hard to induce without corrupting the record).

### [NIT:consistency] Predicate attribution and line refs slightly off
**Section:** Problem, the two-predicate bullet list **Issue:** `commit.go:123` and `pull.go:237` call `f.mergeRecordExists()` directly, not `mergeBlocksMutation` (whose only call sites are `checkout.go:48` and `remove.go:65`), and `mergeSourceInFlight` is called at `remove.go:76` with the refusal at `:81`. **Fix:** Reword to "the this-pair record predicate (`mergeRecordExists`, reached directly or via `mergeBlocksMutation`)" and correct the two line references.

## Verdict

REQUEST_CHANGES
Foreign-state exclusion rationale is factually wrong; test verification command omits the required integration tag.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
