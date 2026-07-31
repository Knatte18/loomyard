MILL_REVIEW_BEGIN
# Review: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 5 (claude-opus-5)
reviewed_file: C:\Code\loomyard\wts\fabric-snapshot-trailer\_mill\discussion.md
date: 2026-07-31
```

## Findings

### [GAP] "Newest tagged commit" rests on git log date order
**Section:** `scan-on-demand-no-index` / Technical context (`index.go` bullet)
**Issue:** The reader takes "the first (newest) record" from `git log --format=` with no `--topo-order`/`--first-parent` (`index.go:193` passes only `log --format=`), so ordering is commit-date descending — and the discussion's own "Cross-clone sharing comes for free" paragraph makes merged-in snapshot commits from another machine (via `gitrepo.Pull`, which can create a weft merge) reachable, where clock skew or a merge topology can put an older baseline first and thereby under-report staleness — the one direction `reader-returns-a-dangling-warp-sha-raw` says actually loses data.
**Fix:** State the ordering basis explicitly (date order accepted with rationale, or `--topo-order`/`--first-parent` added to the generalized scan) and note whether the correspondence rebuild's ordering is affected by the same choice.

### [GAP] Per-branch scoping decided but never tested
**Section:** `reader-api-snapshot-warp-sha` / Testing
**Issue:** Per-branch scoping is called out as deliberate behaviour that "must be stated in `fabricengine/doc.go`", but the Testing section enumerates a pinning case for every other decision (miss, dangling SHA, tags-only, unborn weft, SkipGit) and none for this one — leaving an explicitly-chosen, documented contract unpinned by the discussion's own "an exported contract with no test is exactly the kind that drifts" standard.
**Fix:** Add an integration case: record a tag on one weft branch, `Checkout` to another, assert `SnapshotWarpSHA(tag)` returns `("", nil)` and does not answer cross-branch.

### [GAP] `crucible/fabric-review-prompt.md:23` missing from the footprint
**Section:** Technical context, "Docs to update" / deletion footprint
**Issue:** The footprint is presented as exhaustive and names `crucible/gitrepo-review-prompt.md:21` as a required edit, but `crucible/fabric-review-prompt.md:23`'s live "What to read" list also describes `internal/gitrepo/**` as "`Repo`, `StageAndCommit`, `Push`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/`SetSnapshotSHA`" — the same class of live instruction that goes wrong on this commit.
**Fix:** Add that line to the required-edit list (drop the two snapshot names), applying the same live-instruction-vs-frozen-findings distinction already used for the gitrepo prompt.

### [GAP] `fabric-unified-view.md:67` is contradicted, but told to stay
**Section:** "Docs to update" → `manifest/designs/fabric-unified-view.md`
**Issue:** The instruction keeps the line 63-67 prose "as the durable rationale" and only adds to it, yet line 67 states a standalone no-commit snapshot is warranted only if a consumer must record a baseline without weft content — "which raddle/trace (both commit their output) never do" — which is precisely the raddle regenerated-but-unchanged case this slice's central decision exists to fix, and which the supported tags-only call shape now serves.
**Fix:** Name line 67's clause as a required correction, not just an addition, so the design doc does not keep asserting the case the slice was built for cannot arise.

### [NOTE] Reader-side tag matching semantics unspecified
**Section:** `reader-api-snapshot-warp-sha` / `miss-reads-as-absent`
**Issue:** The write path validates every tag against `snapshotTagPattern` and fails with `*ErrInvalidSnapshotTag` (`trailer.go:48-72`), but the reader's `tag` argument has no stated validation, matching rule (exact vs trimmed vs case-insensitive), so a malformed tag silently reads as absent rather than erroring.
**Fix:** State one sentence: the reader matches trailer values byte-exactly and does not validate its argument (invalid tags simply never match), or validates symmetrically with the writer.

### [NOTE] Two more stale comment sites in `gogit_test.go`
**Section:** Technical context, "Two further comment sites … listed here because this footprint is presented as exhaustive"
**Issue:** `gogit_test.go`'s `linkedParityFixture` type doc (632-639, "plus remoteName, hasUnpushed, and isStrictDescendant") and its `sharedSHA` field comment (643-646, "the value `refs/loomyard/snapshot/<key>` is set to from the main worktree") go stale with the harness cut; the enumerated sweep names only the constant (630), the `update-ref` seeding (684), and the harness doc (710-716).
**Fix:** Add both to the comment sweep, or note that the existing general "re-read the godoc of every function it touches" obligation is intended to cover test-fixture type docs too.

## Verdict

GAPS_FOUND
Reader ordering basis, an untested per-branch contract, and two missed doc corrections.
MILL_REVIEW_END
