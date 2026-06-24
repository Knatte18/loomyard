The `wiki/` directory and `wiki/proposal-weft-repo.md` do not exist in this worktree. This is expected per CLAUDE.md (the wiki is a separate daemon-owned repo, not in the worktree), so it's a reference to an external authoritative doc, not a defect — but a plan writer cannot read it from this tree. This is worth a NOTE, not a GAP, since the discussion is self-contained on the decisions.

Let me do a final check on whether the strict-abort teardown has any concurrency/path-safety concern worth flagging, and check the board derivation edge cases for the `git@host:user/repo.git` SCP form rewrite.

The discussion's board-URL rewrite rule is `…<repo>.git → …<repo>.wiki.git`. Let me verify the edge case: GitHub wiki URLs only work for `https`/`git@github.com` forms. Now I have enough to compose the review.

All major technical claims verified. Let me assess the criteria for gaps:

1. **Undecided items** - all decisions have a chosen option. None left open.
2. **Scope** - clearly in/out delineated.
3. **Constraint coverage** - path invariant, doc lifecycle, naming, output contract all covered.
4. **Failure modes** - abort-if-exists, strict-abort on clone failure with teardown, wiki-404 precondition all addressed. One concern: teardown of partial Hub - what if teardown itself fails (e.g. file locks on Windows)?
5. **Testing** - thorough integration test strategy named.
6. **Ambiguity** - board-URL rewrite for non-`.git` weft URL forms is unspecified.
7. **Feasibility** - verified against source.
8. **Decisions** - each has rationale + rejected alternatives.

Let me note the specific ambiguities I found around the URL rewrite and teardown failure.

MILL_REVIEW_BEGIN
# Review: ly-git-clone hub-creator (host, weft, board)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-24
```

## Findings

### [NOTE] Board-URL rewrite undefined for non-`.git` weft URL
**Section:** Decisions → board-url-derivation-default-weft-wiki; Technical context → Name derivation
**Issue:** Name derivation explicitly handles no-`.git` host URL forms, but the weft→wiki rewrite is specified only as `…<repo>.git → …<repo>.wiki.git`; behavior when the weft URL lacks a trailing `.git` is unstated.
**Fix:** State the rule for a `.git`-less weft URL (append `.wiki.git` to the basename, or require `.git` and error otherwise) so the derivation has one defined output.

### [NOTE] Teardown-failure path not addressed
**Section:** Decisions → board-is-essential-strict-abort; Testing → strict-abort
**Issue:** Strict-abort removes the partial Hub, but on Windows a `git clone` child or AV lock can make the `os.RemoveAll` itself fail, leaving the half-Hub the design calls "a half-built trap."
**Fix:** Note the expected behavior when teardown fails (report the residual path and exit non-zero) so the plan handles the nested-failure case.

### [NOTE] `proposal-weft-repo.md` not readable from this worktree
**Section:** Technical context → Reference
**Issue:** The discussion cites `wiki/proposal-weft-repo.md` §6-7 as a reference, but the wiki is daemon-owned and absent from the worktree, so a plan writer cannot open it.
**Fix:** Confirm the discussion's decisions are self-sufficient (they appear to be) and treat the proposal as background only, not required reading.

## Verdict
APPROVE
Decisions are complete and verified against source; only minor clarifications noted, none blocking.
MILL_REVIEW_END
