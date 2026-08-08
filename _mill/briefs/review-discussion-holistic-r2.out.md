MILL_REVIEW_BEGIN
# Review: Scoutengine: rewrite CONSTRAINTS.md as a seam rule, convert leaf test to banned-list, add LSP guard

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Pre-staged CONSTRAINTS.md says "hermeticity", decision forbids it
**Section:** Decisions → "The `lspclient.go` guard … never called 'stdlib-only'" vs. the committed `CONSTRAINTS.md:77`
**Issue:** The decision states every doc string, comment, and message describes the property as "no lyx dependency except logging", never "stdlib-only" **or "hermetic"** — yet the already-committed section reads "This is a hermeticity rule, not a leaf rule", and the pre-staging decision limits mill-go's `CONSTRAINTS.md` obligation to verifying test *names*, so no card would ever reconcile it.
**Fix:** Either drop the "hermetic" prohibition (and say the word is acceptable at section level, given the same bullet immediately disclaims stdlib-only), or widen the plan's `CONSTRAINTS.md` obligation to include amending that one clause.

### [NOTE] `internal/clihelp` slips through the banned list
**Section:** Decisions → "Banned-list test: rename the file, reuse the three existing predicates"
**Issue:** `internal/clihelp` imports `spf13/cobra` (`internal/clihelp/exec.go`, `jsonhelp.go`) but does not end in `cli`, so after conversion a direct `scoutengine` → `clihelp` import passes the guard where the old allowlist rejected it — a real, unacknowledged widening (peers share the same hole).
**Fix:** Add one sentence recording that `clihelp` is deliberately not policed, direct-import-only being the stated scope.

### [NOTE] Cited pre-staging commit hash not visible on the branch
**Section:** Decisions → "CONSTRAINTS.md pre-staged during mill-start"
**Issue:** The discussion pins the rewrite to commit `5748a22f`; this session's branch snapshot shows the five most recent commits as `27ee6aee`, `406624e7`, `99fccc55`, `1bd4eb14`, `9b97a58c`, none of them that hash (content is present and the tree is clean, so the section itself is verified — only the hash citation is not).
**Fix:** Re-confirm the hash before the plan relies on "no plan card owns re-applying it", or drop the hash and cite the file state instead.

## Verdict

GAPS_FOUND
One naming contradiction between the discussion and the already-committed CONSTRAINTS.md section.
MILL_REVIEW_END
