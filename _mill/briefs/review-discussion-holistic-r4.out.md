MILL_REVIEW_BEGIN
# Review: Add cross-repo code search to prowler

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Repo-list scan swallows a multi-segment path
**Section:** "SKILL.md's argument-disambiguation paragraph is rewritten", part 1
**Issue:** Part 1 classifies *every leading* token matching `^[^/[:space:]]+/[^/[:space:]]+$` as a repo ref, so `helix-editor/helix src/parser` reads as two repo refs — the second token is a one-slash, whitespace-free path — and the list "ends at the first leading token that does not match" never terminates before it; part 2's single-repo path branch then never fires, and today's documented usage (`SKILL.md` line 15: `bash "$TREE_SH" <owner/repo> <path>` scoped to a repo-relative directory) silently routes to search against a nonexistent repo.
**Fix:** State how the boundary handles a second slash-bearing token (e.g. stop the repo list after the first token when the remainder is phrasing-marked as a path, or require the repo list to be contiguous *and* the remainder decided by phrasing per part 2), so the existing tree+path invocation still routes correctly.

### [BLOCKING:consistency] Q&A log contradicts Scope on INDEX.md
**Section:** Q&A log, final entry vs Scope / "Skill dispatch surface is updated too" / Constraints
**Issue:** The Q&A entry answers "Which docs must land in the same commit?" with "`README.md` and `SKILL.md` only", while Scope, the dispatch-surface decision, and the Constraints section all require the `plugins/prowler/skills/INDEX.md` row edit (verified: line 6 duplicates the SKILL.md description verbatim today) — a plan writer taking the Q&A log as the disposition would ship the two files disagreeing.
**Fix:** Update the Q&A answer to name `INDEX.md` alongside `README.md` and `SKILL.md`, or mark it superseded.

### [NIT:design] `<owner>/<repo>` validation predicate unspecified
**Section:** "Exit codes: 2 for usage shape, 1 for everything else"
**Issue:** "an invalid `<owner>/<repo>` ref → 1" never names the predicate, and two incompatible ones are in play in the same task: `github-tree.sh` line 49 uses `^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`, while the rewritten SKILL.md rule proposes `^[^/[:space:]]+/[^/[:space:]]+$`.
**Fix:** State which predicate the script uses and whether the SKILL.md rule's looser shape is deliberate.

### [NIT:design] Preflight status extraction and stub shape not pinned
**Section:** "Preflight each repo against the core rate-limit bucket" / "The new stub keys fixtures on a request key"
**Issue:** The preflight distinguishes 404/401/403, but the discussion never says how the status is obtained; `github-tree.sh`'s `fetch()` parses it out of the raw error body `gh` writes to stdout only because the call carries `--jq`, and the preflight call as described (`gh api repos/<owner>/<repo>`) carries none — which also leaves the new stub's accepted shape for a bare two-argument `api <endpoint>` call unstated.
**Fix:** Say whether the preflight call carries a `--jq` expression and how its HTTP status is recovered, so the stub's shape check and the preflight-failure scenarios are specifiable.

## Verdict

REQUEST_CHANGES
Disambiguation rule breaks the existing repo+path case; Q&A log contradicts Scope on INDEX.md.
MILL_REVIEW_END
