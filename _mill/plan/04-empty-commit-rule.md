# Batch: empty-commit-rule

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: empty-commit-rule
number: 4
cards: 7
verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/...
depends-on: [2, 3]
```

## Batch Scope

This batch makes snapshot tags always produce a weft commit: when `snapshotTags` is non-empty and no weft commit would otherwise land, `commitWeftLocked` lands an **empty** weft commit carrying the `Warp-SHA` and `Snapshot:` trailers. It depends on batch 2 for `gitrepo.CommitEmpty` and `ErrIndexNotEmpty`, and on batch 3 for `SnapshotWarpSHA` (which every behavioural assertion here reads baselines back through) and for `internal/fabricengine/index.go` and `doc.go`, which both batches edit.

The rule closes a genuine correctness hole with no other fix. raddle regenerates against warp SHA X, the output is byte-identical, so no weft commit lands, so the baseline never advances and the staleness check reports drift forever despite raddle having just confirmed itself current. Once an empty commit fixes that, the same mechanism absorbs the caller-misuse shapes too — with no validation, no typed error, and no misuse-handling branch. That is the whole point: the module does what the caller asked, faithfully.

**Batch-local decision — the rule is uniform across all four reachable cases.** Zero weft-side files (a warp-only `Commit`, or the tags-only shape); a pathspec that `weftPathspecFilter` reduces to no positive entry; weft files whose content is unchanged so `StageAndCommit` reports `committed == false`; and `StageAndCommit` failing with git's `"did not match any files"`, which `commitWeftLocked` tolerates. The fourth is defence-in-depth, reachable only when `weftPathspecFilter`'s own pre-check is bypassed, and it is semantically identical to the second — the pathspec matched nothing — so it must not behave differently. Missing it would leave one uncovered silent-drop path in a rule whose entire value is being exhaustive.

**Batch-local decision — two exceptions, and only two.** `opts.SkipGit` still produces no commit and therefore no trailer, regardless of tags: it is an explicit "touch no git at all" opt-out, and honouring it is correct rather than a silent failure. An **unborn warp** HEAD still drops the tags exactly as today, because a snapshot's entire content is a warp SHA and there is none; committing a `Snapshot:` trailer with no `Warp-SHA` would force the reader to grow a third "found the tag, has no baseline" state. An **unborn weft** HEAD is *not* an exception — the rule fires and lands weft's empty root commit.

**Sizing note the implementer must not get wrong.** The unborn-warp rule does **not** live inside `commitWeftLocked`'s existing `if !unborn` block; that block covers only trailer composition. All three fall-through points sit outside it, so each needs its own explicit guard. What is genuinely cheap is the decision, not the code: `unborn` is already computed and already in scope at every one of those points, so honouring it adds a conjunct rather than a new code path.

## Cards

### Card 15: Widen Commit's weft-side predicate and correct its error wording

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `Fabric.Commit`, widen the `weftSide` predicate from `len(weftFiles) > 0 && !opts.SkipGit` to `(len(weftFiles) > 0 || len(snapshotTags) > 0) && !opts.SkipGit`. Nothing else in `commitBothSides` changes: its three-outcome `*PartialCommitError` mapping already handles whatever `commitWeftLocked` returns, and its `committing` computation (`len(warpFiles) > 0 || weftSide`) picks up the new case for free. Record the knock-on in `Commit`'s godoc rather than leaving it to be discovered: a tags-only or warp-only tagged call now sets `committing == true`, so it takes the combined write lock and runs `ensureWeftLockDir` where it previously did neither. That is correct — the call is about to write to weft — but it falsifies the godoc's current claim that a fully degenerate no-op call takes no lock, runs no `ensureWeftLockDir`, and spawns no push. Reword it so the degenerate case is the genuinely degenerate one: nothing on either side **and no tags**. Also state that `Commit(nil, msg, tags, opts)` — tags with zero files — is a **supported call shape**, not an accident of the predicate: it is how a caller records a baseline without producing weft content, which is the standalone-snapshot use the design deferred rather than a new method. Fix `PartialCommitError`'s wording, both its type godoc and the string its `Error()` method builds on the unlanded-weft branch: both open by asserting a warp commit landed, which is false for the newly first-class tags-only shape. With zero warp files the `err != nil && !weftCommitted` arm interpolates an **empty** `WarpSHA` and reads "warp commit  landed, weft commit failed". This is not a pre-existing wart to accept — the tags-only shape is new in this batch, and it is the shape that exposes the wording. Reword both so the no-warp-commit case reads correctly while the ordinary two-sided case still does.
- **Commit:** `fabric: let snapshot tags alone select the weft side of Commit`

### Card 16: Land an empty weft commit at all three fall-through points

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/index.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `commitWeftLocked`, make all three early returns fall through to an empty commit when `len(snapshotTags) > 0` and `!unborn`. The three are: the `if !positive` return after `weftPathspecFilter`; the `"did not match any files"` tolerance inside the `StageAndCommit` error branch; and the `if !committed` return. The third is the one that is easy to miss — it sits inside a guard clause after an error branch rather than reading as one of the pathspec guards. **Each needs its own guard**; do not try to hoist the rule into the existing `if !unborn` block, which covers only trailer composition and sits above all three. The fall-through tail is the same in all three places, so factor it into one helper on `Fabric` rather than writing it three times: call `f.Weft.CommitEmpty(commitMessage)` — `commitMessage` already carries the `Warp-SHA` trailer and the `Snapshot:` trailers, composed before the pathspec filter runs — then call `f.RecordCorrespondence(warpSHA, sha)` and return `(sha, true, err)` with the same "the commit already exists on disk, so report it alongside a recording failure rather than swallowing it" posture the existing tail uses. `RecordCorrespondence` must run for the empty commit: it carries a real `Warp-SHA` trailer like any other commit, so `RebuildIndex` (which reads trailers, not this decision) would record it anyway, and skipping it would make the incremental and rebuilt indexes diverge. When `CommitEmpty` returns `ErrIndexNotEmpty`, return `("", false, err)` and let `commitBothSides` map it through its existing `err != nil && !weftCommitted` arm to a `*PartialCommitError` with `WeftCommitted: false` — the unlanded-weft outcome it already models. No new error shape and no new branch. Record that propagation as a **real behaviour change** in `commitWeftLocked`'s godoc: the `!committed` path is a documented silent no-op today, and on a tagged call with a dirty weft index it now becomes an error. That is reachable — the combined write lock serialises fabric's own callers but does not exclude staged entries an aborted earlier run left behind — and failing loudly is the point, because silently folding someone else's staged work into a snapshot commit is strictly worse than refusing. `appendSnapshotTrailers`'s validate-all-before-appending-any property must survive this path unchanged: an invalid tag fails the call before anything is committed, empty or otherwise, and the existing composition already runs before the filter, so preserve that ordering. Rewrite `commitWeftLocked`'s godoc, which currently describes all three early returns as unconditional no-ops and states that the unborn-warp arm lands "no Snapshot tags (there is no trailer block to append them to)" — fold in the empty-commit rule, its four triggering cases, the unborn-warp exception, and the `ErrIndexNotEmpty` propagation. Rewrite `CommitWeft`'s godoc too: its description of the trailing `snapshotTags` variadic now understates what tags do, since `CommitWeft` is an exported entry point that inherits the rule.
- **Commit:** `fabric: land an empty weft commit whenever snapshot tags are present`

### Card 17: Record that empty commits make duplicate warp-SHA recordings routine

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/revert.go`
- **Edits:**
  - `internal/fabricengine/index.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update the comment inside `RebuildIndex` that calls a warp SHA recorded by more than one weft commit "a rare but legal history shape". This batch makes it routine: the warp SHA does not move when a tags-only or unchanged-content call fires, so an empty commit calling `RecordCorrespondence(warpSHA, emptyWeftSHA)` **upserts over** the entry a preceding content commit wrote for the same warp SHA, and `WeftSHAForWarpSHA(X)` and `RevertWithWeft(X)` then resolve to the empty commit. Record that this is accepted, not worked around, and why: an empty commit's tree is identical to its parent's by construction, so resolving a revert target to the empty commit restores the same weft tree the content commit produced, and "last recorded wins" is already `corrIndex.record`'s own documented upsert rule — this is existing semantics meeting a newly-common input, not new semantics. Say explicitly why the alternatives were rejected, because both look tempting: skipping `RecordCorrespondence` for empty commits would make the incremental and rebuilt indexes diverge, since `RebuildIndex` reads trailers and would record the commit anyway; and special-casing empty commits inside the index to keep the content commit as the winner would make the index disagree with a trailer scan, breaking the "trailer is truth, index is a rebuildable cache" layering the whole design rests on. **The benign-ness argument is load-bearing, so verify it rather than accepting the reasoning**: read `RevertWithWeft` and `resolveRevertTarget` in `revert.go` and confirm they use the resolved weft SHA only as a reset target. If revert does anything with that commit beyond its tree — walks parents, reports the SHA to an operator, anchors a later diff — the consequence must be re-examined and written down here, not assumed away.
- **Commit:** `fabric: record that empty snapshot commits take over the correspondence entry`

### Card 18: Invert the warp-only drop test and pin the unchanged-content hole

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These are the two TDD cases for this batch — write both before cards 15 and 16 are complete, watch them fail, then make them pass. **`TestCommit_WarpOnly_SnapshotTagsDropped` is inverted, not deleted.** Keep the setup; flip the assertion from "no `Snapshot:` trailer landed anywhere" to "an empty weft commit landed carrying both a `Warp-SHA` and a `Snapshot: <tag>` trailer". Rename it so the name states the new rule, and rewrite its doc comment — leaving the old name in place would be actively misleading to the next reader, and the name is currently the clearest single statement of the behaviour this batch reverses. **Unchanged weft content plus tags:** commit the same weft content twice with the same tag against two different warp SHAs; the second call must land an empty commit and `SnapshotWarpSHA(tag)` must advance to the newer warp SHA. This is the correctness hole the whole rule exists to close, so it must fail before the change and pass after — verify that it does rather than assuming. Reuse the file's existing `newCommitFixture`, `seedFabricConfig`, `writeWarpFile`, and `swapPushRecorder` helpers; the last one is the `spawnDetachedPushFn` seam swap with its deferred restore and no `t.Parallel()`, and every test in this file already follows that pattern.
- **Commit:** `fabric: pin that snapshot tags force a weft commit even with no content`

### Card 19: Cover the remaining empty-commit paths and the two exceptions

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
  - `internal/fabricengine/weftgit_unborn_warp_test.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Cover the remaining paths. **Tags-only call** — `Commit(nil, msg, []string{"raddle"}, opts)`: an empty weft commit lands, `SnapshotWarpSHA` resolves to warp's current HEAD, and the combined write lock was taken. This pins the supported call shape rather than leaving it emergent. **Pathspec filtering to nothing** (the `!positive` path) plus tags: an empty commit still lands. **The `"did not match any files"` tolerance path** plus tags: an empty commit still lands. That path is normally shielded by `weftPathspecFilter`'s own pre-check, so the test must construct a case that reaches `StageAndCommit`'s failure directly — read `weftgit_pathspec_integration_test.go` first to see how the existing pathspec cases are built. If the path cannot be reached without contrivance, say so explicitly in the card's completion note rather than quietly dropping the case. **Unborn weft HEAD plus tags:** the empty commit becomes weft's root commit, carrying its trailers, and `SnapshotWarpSHA` resolves against it. **Unborn warp HEAD plus tags:** no commit, no trailer, no error — today's behaviour, unchanged; `weftgit_unborn_warp_test.go` shows how that fixture is built. **`opts.SkipGit` plus tags:** no commit, no trailer, no error. **No tags and nothing to commit:** still a clean no-op — no empty commit and no lock churn beyond today's. That one guards against the rule over-firing, which is the failure mode a uniform rule invites. **`CommitResult` shape on a warp-only tagged commit:** `WeftCommitted == true`, `WeftSHA` non-empty, and the push seam observed exactly one spawn — the async-push gate is `result.WarpCommitted || result.WeftCommitted`, and the snapshot trailer must reach the remote for cross-clone sharing, which is the property the retired ref mechanism achieved by pushing its ref. **Invalid tag on an otherwise-empty commit:** `*ErrInvalidSnapshotTag` returned and **nothing** committed on either side, pinning that validate-all-before-append survives the new path. **Dirty weft index plus tags on the `!committed` path:** `CommitEmpty` refuses, and `Fabric.Commit` surfaces a `*PartialCommitError` with `WeftCommitted == false` while the warp commit stands — the accepted no-op-becomes-error transition. **Via `CommitWeft` directly**, not only via `Fabric.Commit`: `CommitWeft` with a pathspec matching nothing plus a tag lands the empty commit. The rule lives in `commitWeftLocked` and `CommitWeft` is an exported entry point that inherits it and now claims so in its godoc; an exported contract with no test is exactly the kind that drifts.
- **Commit:** `fabric: cover every empty-commit path and both of its exceptions`

### Card 20: Pin the correspondence overwrite

- **Context:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/revert.go`
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/syncweft_integration_test.go`
- **Edits:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the case card 17 documents but does not pin: a warp SHA recorded by **two** weft commits — a content commit, then a tags-only commit at the same unchanged warp HEAD. Assert three things. `WeftSHAForWarpSHA(X)` resolves to the **empty** commit, because last recorded wins. `RebuildIndex` produces the same answer as the incremental path on that history — reuse the `corrIndex.entries()` comparison `syncweft_integration_test.go`'s existing equivalence test already uses. And the weft tree reachable through that entry is **identical** to the content commit's tree, which is the assertion that makes the overwrite benign rather than merely tolerated. Add one more case in the same file: **dangling `Warp-SHA`** — record a snapshot, rewrite warp history so the recorded SHA no longer resolves, then read. `SnapshotWarpSHA` returns the dangling SHA raw with a nil error, and `f.Warp.SHAExists` on it reports false. That pins the reader's validate-at-use posture and demonstrates the three-step consumer idiom in executable form, which is worth more than the doc paragraph describing it.
- **Commit:** `fabric: pin the correspondence overwrite and the dangling-baseline read`

### Card 21: Document the empty-commit rule in fabricengine's package doc

- **Context:**
  - `internal/fabricengine/commit.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the snapshot section batch 3 added, keeping the file's existing register and the repo's one-line-per-paragraph markdown rule. Cover: the empty-commit rule and its four triggering cases; the supported tags-only call shape and what it is for; the `ErrIndexNotEmpty` propagation as an unlanded-weft `*PartialCommitError`, named as a real behaviour change from a previously-silent no-op; the unborn-warp exception and the contrasting unborn-weft root-commit behaviour, with the reason the two differ (a snapshot's entire content is a warp SHA, so there is nothing to record when warp is unborn, whereas an unborn weft simply gets its root commit); `SkipGit`, recorded so a reader of the rule does not conclude it overrides the opt-out; and the accepted accumulation cost. State that cost honestly: every no-op tagged run now appends a weft commit carrying no content, so a consumer checking staleness in a tight loop would grow weft history without bound and the cache-free reader scans that history on every call. It is accepted because the actual call pattern is one tagged commit per regeneration at a phase boundary rather than a poll, and because the scan is a single `git log` over a history that is small by construction; if a consumer ever does poll, the fix belongs on the consumer — check `ChangedFilesSince` first, tag only when regenerating — not on this mechanism. Also note that cross-clone sharing comes for free: trailers live in weft's own commit history, which the existing detached both-sides push already sends, so no equivalent of the retired mechanism's ref push is needed.
- **Commit:** `fabric: document the snapshot empty-commit rule`

## Batch Tests

`verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/...` — the same scope and the same two pre-existing Windows failures as batch 3; see the overview's `known-pre-existing-windows-test-failures` Shared Decision.

Coverage lands in two existing files: `commit_integration_test.go` (cards 18 and 19 — the inverted drop test, the unchanged-content hole, every remaining empty-commit path, and both exceptions) and `snapshot_integration_test.go` (card 20 — the correspondence overwrite and the dangling baseline). Nothing new is written in the untagged tier: every assertion in this batch needs real git history, and `commitWeftLocked`'s behaviour is not reachable without spawning git.

Two cases carry disproportionate weight. The unchanged-content case in card 18 is the only one that fails before the change and passes after, so it is the executable statement of why this batch exists — if it passes before the implementation, the fixture is wrong. The dirty-weft-index case in card 19 is the only witness for the accepted no-op-becomes-error transition, which is the single behaviour regression this batch knowingly ships.
