MILL_REVIEW_BEGIN
# Review: Add cross-repo code search to prowler

```yaml
duration_s: 90.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] `repo:`-in-query exit code stated as both 2 and 1
**Demoted-from:** BLOCKING
**Section:** "Reject a caller query containing `repo:`" vs "Exit codes: 2 for usage shape, 1 for everything else" vs Q&A log
**Issue:** The `repo:` guard decision says "exit 2 with a usage error" and the Q&A log repeats "Reject with exit 2", while the Exit-codes decision and the Testing scenario list both pin it at 1 (`a caller query containing repo: → 1`) — the plan writer and the harness assertion cannot both be right.
**Fix:** State one value in all four places; by the section's own shape-vs-semantics rule (11 refs still satisfy the synopsis, so does a query with `repo:`) that value is 1, and the two stale exit-2 mentions should be corrected.

### [BLOCKING:design] SKILL.md's second-argument rule left undecided under plural repos
**Section:** "Skill dispatch surface is updated too" / Scope (SKILL.md body)
**Issue:** SKILL.md today carries an explicit disambiguation paragraph — a second token is forwarded to `github-tree.sh` as `<path>` only when it matches `^[A-Za-z0-9._/-]+$` with no whitespace — and the new hint `"<owner/repo>... [path | search term] [question]"` breaks it two ways the discussion never resolves: a bare search term (`tree-sitter`, `wgpu`) satisfies that same accepted set and would route to the tree script, and a plural `<owner/repo>...` list has no stated terminator telling the model where repos end and the second slot begins.
**Fix:** Decide and record how that paragraph reads after the change — the disambiguation predicate for path-vs-search-term and how the repo list is delimited — since the routing rule alone does not determine it.

### [NIT:consistency] "only edit either file receives" contradicts Scope
**Section:** "Skill dispatch surface is updated too" (the line "This is the only edit either file receives")
**Issue:** Scope and Constraints both have SKILL.md's *body* gaining the new-script documentation and the routing rule, so the sentence is true of `INDEX.md` only.
**Fix:** Narrow the sentence to `INDEX.md`.

### [NIT:design] Empty/whitespace query has no stated disposition
**Section:** "Reject a caller query containing `repo:`" / Testing argument-rejection scenarios
**Issue:** `<query>` is positional and mandatory by arity, but an empty-string query is accepted by the API (verified: a qualifier-only `q` returns 1744 hits), so `github-code-search.sh "" owner/repo` silently degrades to "first 100 files in the repo".
**Fix:** Say whether an empty/whitespace-only query is rejected (and at which exit code) or deliberately allowed.

### [NIT:design] `repo:` guard false-positives on a literal content search
**Section:** "Reject a caller query containing `repo:`"
**Issue:** The guard is a substring test, so a legitimate search for the literal token `repo:` in code/config (e.g. a YAML key) is rejected with no way to express it.
**Fix:** Note this as an accepted, documented limitation, or state the narrower condition (qualifier position) the guard tests.

## Verdict

REQUEST_CHANGES
One exit code stated two ways; SKILL.md's argument-disambiguation rule left unresolved.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
