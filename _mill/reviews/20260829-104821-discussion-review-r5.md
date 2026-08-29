MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
duration_s: 129.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: claude-opus-4 class (Anthropic); exact build not self-verifiable
reviewed_file: _mill/discussion.md
date: 2026-08-29
```

## Findings

### [NIT:consistency] GitHub Auth Invariant never disposed of
**Demoted-from:** BLOCKING
**Section:** ## Constraints
**Issue:** The section enumerates 13 invariants and claims "every listed invariant" governs Go/`internal/`, but `CONSTRAINTS.md` holds ~40, and the one that literally names `gh` — GitHub Auth Invariant, "All GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`" — is absent, while this task's whole deliverable is a committed script that shells out to `gh` and relies on its ambient auth.
**Fix:** Name that invariant explicitly and state the reason it does not bind (e.g. it binds Go packages in the root module; `cmd/lyx/ghguard_test.go` scans only non-test `.go` files under the module root, and `plugins/prowler` is a separate nested module and a plugin, with the existing `github-repo-explorer` SKILL.md already directing `gh api` calls), rather than dismissing the whole file by a partial list.

### [NIT:consistency] Fixture-reuse rationale contradicts the two-level fixture design
**Section:** ### Offline harness with a stub `gh` …
**Issue:** The rationale claims one JSON fixture "serve[s] both the recursive and non-recursive query of the same tree", but the truncated-fallback tests require a node whose recursive response is truncated with strictly fewer subtrees than its non-recursive response — those are necessarily two different fixture bodies with different `truncated` values.
**Fix:** Restate the reuse claim as applying to untruncated flat trees only, or drop it; keying fixtures by full endpoint-plus-query is already what the stub does.

### [NIT:consistency] Superseded "without buffering" clause in the Q&A log
**Section:** ## Q&A log — "Sort the output for cross-branch determinism?"
**Issue:** Its rationale says tests "can assert exact output without buffering the whole listing", which contradicts the adopted all-or-nothing decision that buffers the entire listing in memory; the decision body (line 134) already flags this, the Q&A entry does not.
**Fix:** Update that Q&A "Why" to drop the buffering clause, leaving determinism as the sole argument.

### [NIT:design] SKILL.md rewrite does not say how `[path]` reaches the script
**Section:** ### SKILL.md replaces the walk prose entirely
**Issue:** The rewrite is specified as "resolve the script's absolute path, then call it", but the skill's `argument-hint` advertises `<owner/repo> [path] [question]` and today's body never mentions `path`; the discussion never says the rewritten block must show the optional second argument or how to tell a `path` from a `[question]`.
**Fix:** State that the two-step block documents both invocation forms and how the skill decides whether a second token is a path or the question.

## Verdict

APPROVE
Constraint coverage dismisses CONSTRAINTS.md by a partial list, omitting the one `gh`-specific invariant.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
