MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x-class model (exact build not self-verifiable)
reviewed_file: _mill/discussion.md
date: 2026-07-31
```

## Findings

### [GAP] CommitEmpty's index-sweep contract is self-contradictory
**Section:** `commit-empty-as-a-new-gitrepo-primitive` + Constraints ("StageAndCommit's empty-list guard") + Testing (`internal/gitrepo — CommitEmpty`)
**Issue:** The decision specifies bare `git commit --allow-empty -m <msg>` (no pathspec), which by construction commits whatever is in the index, yet the Testing section requires a test that "with a staged-but-uncommitted file present, `CommitEmpty` must not sweep it into the commit" — that test cannot pass against that command, and the property it defends is `gitrepo.StageAndCommit`'s own documented one (`gitrepo.go:133-153`: an automated commit "can never sweep up a half-staged edit someone else left in the index").
**Fix:** Decide the mechanism explicitly — either scope the commit (`--only -- ` / a pre-commit index check that refuses) or drop the not-swallowed requirement and state plainly in the decision, the doc comment, and the test list that `CommitEmpty` commits the current index.

### [GAP] roadmap.md carries a live SnapshotSHA API enumeration
**Section:** Technical context → "Docs to update" ("`manifest/roadmap.md` — **no change**")
**Issue:** `manifest/roadmap.md:74` describes gitrepo as providing "generic, repo-agnostic git primitives (`StageAndCommit`, …, `SnapshotSHA`/ `SetSnapshotSHA`)" — a present-tense API list that goes wrong on this commit; the discussion dismisses roadmap edits purely on the CLAUDE.md status-movement rule, which is a different question, and lines 62/64 also name the ref API in historical Done text.
**Fix:** Name line 74 as a required same-commit edit (or record an explicit decision that Done-list module descriptions are frozen history), and state whether 62/64 stay as-written.

### [NOTE] "lives inside the existing `if !unborn` arm" is inaccurate
**Section:** `unborn-warp-keeps-todays-behaviour` (rationale)
**Issue:** In `weftgit.go` the `if !unborn` block is only the trailer-composition step (lines 356-362); all three fall-through points (368, 374-382, 383) sit outside it, so each needs a new explicit `!unborn && len(snapshotTags) > 0` guard plus the CommitEmpty + `RecordCorrespondence` tail — not "no new code and no special-casing".
**Fix:** Reword the rationale to match the (correct) Technical-context bullet, so a plan writer does not size the change from the rationale.

### [NOTE] No test pins the empty-commit rule via `CommitWeft` directly
**Section:** Testing → "empty-commit behaviour"
**Issue:** Every listed case drives `Fabric.Commit`, but the rule lives in `commitWeftLocked`, and exported `CommitWeft(pathspec, msg, opts, tags...)` (weftgit.go:411) inherits it and gets a godoc update saying so — an untested exported contract.
**Fix:** Add one `CommitWeft`-with-tags case (pathspec matching nothing, tags present → empty commit lands).

### [NOTE] Branch scoping of the reader is unstated for consumers
**Section:** `reader-api-snapshot-warp-sha` / `scan-on-demand-no-index`
**Issue:** The reader scans "the current weft branch", so after a coordinated `Checkout` a snapshot recorded on another branch reads as absent and the consumer regenerates everything — the correspondence index needed `refreshCorrIndexAfterSwitch` (index.go:250) for the mirror-image reason, and this consequence is nowhere recorded.
**Fix:** State per-branch scoping as intended behaviour in the decision and in `fabricengine/doc.go`'s new section.

### [NOTE] Empty-commit accumulation is never weighed
**Section:** `snapshot-tags-always-force-a-weft-commit`
**Issue:** Every no-op tagged run now appends a weft commit forever, growing weft history and the full-history `git log` the cache-free reader scans on each call; neither cost is acknowledged.
**Fix:** Record the trade-off (one line) and, if accepted, say so explicitly rather than leaving it implicit.

### [NOTE] Crucible gitrepo prompt still lists snapshot.go as in-scope source
**Section:** Technical context → "Deletion footprint"
**Issue:** `crucible/gitrepo-review-prompt.md:21` and `:101` name `snapshot.go` as code to read/match style against for a re-run of that module review; not in the footprint.
**Fix:** Either add it to the footprint or state that crucible prompts are frozen round records and stay stale by design.

## Verdict

GAPS_FOUND
Two resolvable gaps: CommitEmpty's index contract contradicts its own test, and a stale roadmap API list.
MILL_REVIEW_END
