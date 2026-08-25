MILL_REVIEW_BEGIN
# Review: loom: Webster-Review producer

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [BLOCKING:design] Cluster forks may never run git
**Section:** `the-round-runs-a-cluster-fan` × `diff-derivation-lives-in-the-rubric-not-in-go`
**Issue:** The fork boilerplate `burlerengine` composes into every fork prompt says forks are read-only and "never run any git command" (`internal/burlerengine/prompt.go:226-229`, restated in `doc.go:175-179`, with a git-mutating Bash call a hard audit error), so the five `standard` forks cannot execute the `git diff $(git merge-base …)..HEAD` derivation the discussion makes the round's only route to its subject — only the handler can.
**Fix:** State how each fork obtains the diff under the chosen fan (handler-inherited context from its phase-1 exploration, versus the handler materialising the diff somewhere the forks may read), and say what happens when the diff exceeds what inherited context carries.

### [BLOCKING:consistency] Q&A log carries two superseded answers
**Section:** `## Q&A log`, entries 1 and 4
**Issue:** Entry 1 still answers "deriving `<lowest batch startSha>..HEAD` from `_lyx/webster/state.json`, falling back to the merge-base", which the `diff-derivation` decision rejects outright and replaces with a no-fallback merge-base; entry 4 still says "the failure is loud at construction", which the same discussion's own risk and `internal/shedrecipe/entries_burler.go:163-167,219` contradict (run-time, inside `Profile.validate`).
**Fix:** Rewrite both answers to match the settled decisions, or mark them explicitly superseded by the two r1-gap entries below them.

### [BLOCKING:scope] Recipe header's "Both review segments" note goes stale
**Section:** Scope, row-count knock-on
**Issue:** `contracts/recipes/loom-recipe.yaml:12-16` states "Both review segments follow the same shared-segment mutual-on_stuck shape" and enumerates the Discussion and Plan pairs; the task makes that three segments, but the inventory lists that file's header only for the "sixteen"→"seventeen" count.
**Fix:** Add the header's segment-enumeration paragraph to the recipe-file edit inventory alongside the count.

### [NIT:consistency] smoke_test.go has no row-count assertion
**Section:** Testing, `internal/loomcli/smoke_test.go`
**Issue:** The file carries no row-count assertion at all; its only affected content is the header comment at line 21 ("backs one of its sixteen rows -- Webster-Review -- with a stub producer"), which becomes false in both the count and the stub claim.
**Fix:** Restate the item as a header-comment rewrite rather than "row-count assertion update only".

## Verdict

REQUEST_CHANGES
Fork/git conflict unresolved; Q&A log and one knock-on inventory item contradict the decisions.
MILL_REVIEW_END
