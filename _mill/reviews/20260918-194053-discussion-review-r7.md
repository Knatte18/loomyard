MILL_REVIEW_BEGIN
# Review: fabric: no remote/GitHub branch deletion

```yaml
duration_s: 109.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: claude-opus-4-20250514 (best-effort; presented to me as "Opus 5")
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [NIT:consistency] Testing contradicts absent-ref substring decision
**Demoted-from:** BLOCKING
**Section:** Testing → `internal/gitrepo` vs Decisions → absent-remote-ref-is-success
**Issue:** The decision states "One substring is enough … there is no second spelling to match and no list is needed. The test pins that exact substring", while Testing says "The absent-ref case is the reason to cover both stderr spellings git uses" — the plan writer gets a single `strings.Contains` from one section and a two-spelling matcher plus two-case test from the other.
**Fix:** Delete or rewrite the Testing line to match the decision (one pinned substring, `remote ref does not exist`), or, if a second spelling really exists, name it in the decision and drop the "no list is needed" claim.

### [NIT:design] No-origin pre-check's gating on `remote` is unstated
**Section:** Decisions → no-origin-is-resolved-once-per-verb
**Issue:** The decision says the `RemoteURL("origin")` check runs "once per verb, before the loop" without saying it runs only when `remote` is true; combined with Technical context's "emit these three the same way rather than conditionally", a literal reading makes a plain `cleanup --apply` / `remove` on a remoteless repo emit a non-empty `remote_skipped_reason` — an observable change to a path Scope does not claim.
**Fix:** State explicitly that the pre-check is skipped, and `RemoteSkippedReason` stays empty, when `remote` is false.

### [NIT:design] Mixed/local-only synthesised error string left unformatted
**Section:** Decisions → remote-failure-is-a-cli-failure ("The synthesised error")
**Issue:** The remote-only and `remove` cases get exact `fmt.Errorf` formats, but the local-`Error`-only and mixed cases get only "the CLI builds a single string naming each failing class it actually saw" — and the local-only case is a new non-zero exit this task introduces, so its wording is newly observable CLI output.
**Fix:** Pin the local-failure and combined formats the same way the other two are pinned, since the r4 rationale for pinning them applies identically.

## Verdict

APPROVE
One self-contradiction on stderr matching; everything else verified against source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
