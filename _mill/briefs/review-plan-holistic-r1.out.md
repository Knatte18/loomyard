MILL_REVIEW_BEGIN
# Review: gitrepo: generic, repo-agnostic git primitives — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-07-24
```

## Findings

### [NIT] PushCoalesced loop can spin vs board's bounded loop
**Location:** Batch 2 (push) / Card 6
**Issue:** The loop terminates on `hasUnpushed()==false`, unlike board's `sync.go`, whose loop is bounded by commit activity; if `git push` succeeds without setting an upstream (e.g. `push.default=current`, no `autoSetupRemote`), `hasUnpushed` stays true forever and the loop never exits.
**Fix:** Bound the loop (break once a push reports nothing to push / cap iterations), or note the upstream-configured precondition explicitly the way the rebase-retry precondition already is.

### [NIT] Roadmap Done entry should not link, per Maintenance convention
**Location:** Batch 4 (docs-lifecycle) / Card 11
**Issue:** Card 11 says the new Done entry should link "to the shipped package `internal/gitrepo`", but roadmap.md's Maintenance section states Done entries "don't link anywhere" (the design doc is deleted); existing Done entries like "shared infra" use plain code-spans, not links.
**Fix:** Reference `internal/gitrepo` as an inline code-span in the "shared infra"/Done style, not a markdown link.

### [NIT] HermeticGitEnv defined in hermetic.go, not the listed lyxtest.go
**Location:** Batch 1 / Card 4 (also Card 7, Card 9 via TestMain)
**Issue:** Card 4 Requirements call `lyxtest.HermeticGitEnv()` but Context lists `internal/lyxtest/lyxtest.go`; the function actually lives in `internal/lyxtest/hermetic.go`. (Mitigated: the exact nullary call is visible in the `internal/gitexec/testmain_test.go` template the card mirrors.)
**Fix:** Add `internal/lyxtest/hermetic.go` to Context, or note the call is copied verbatim from the provided testmain template.

## Verdict

APPROVE
Plan is complete, DAG-sound, and constraint-faithful; only minor non-blocking nits.
MILL_REVIEW_END
