# Batch: design-docs

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: design-docs
number: 5
cards: 2
verify: go test -count=1 ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/... && go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/...
depends-on: [1, 4]
```

## Batch Scope

This batch corrects the two `manifest/designs/` documents that go actively wrong on this task and marks slice 4 of the `fabric: unified-repo view` campaign done. It is deliberately last and depends on both leaf batches: `raddle.md` describes the consumer API, which is not final until batch 4's empty-commit rule lands, and marking a slice DONE can only be truthful once the deletion (batch 1) and the mechanism (batches 2–4) have both landed.

Every module doc, godoc, `CONSTRAINTS.md` clause, and guard comment this task touches has already landed in the same commit as the code that falsified it, per the Documentation Lifecycle. What is left here is campaign-level design bookkeeping, which has no code commit to ride along with — `manifest/designs/` documents the campaign, not a module.

`docs/overview.md` and `docs/shared-libs/README.md` were both checked when this plan was written and need **no** change: `docs/overview.md`'s two `gitrepo` references are one-line module descriptions that do not enumerate `SnapshotSHA`, and `docs/shared-libs/README.md` has no mention at all. `manifest/roadmap.md` is batch 1's, not this batch's — its edit is a stale-API correction inside an existing `## Done` entry, not a status movement, so it belongs with the deletion that made it stale.

## Cards

### Card 22: Rewrite raddle.md against the trailer API

- **Context:**
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/commit.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `manifest/designs/raddle.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This file references the retired ref API in four places and goes actively wrong on this task, which makes it the one design document that must not be skipped. Rewrite the regeneration step so raddle records its baseline by passing `snapshotTags=["raddle"]` to `Fabric.Commit`, replacing the `SetSnapshotSHA("raddle", <sha>)` call. Rewrite the staleness check as the **three-step** idiom — `SnapshotWarpSHA("raddle")`, then `f.Warp.SHAExists`, then `f.Warp.ChangedFilesSince` — and not as the two-step composition, so a post-rebase raddle regenerates instead of hard-erroring on a warp SHA that no longer resolves. Rewrite the merge-lock paragraph's closing sentence, which motivates its "advance only on confirmed success" discipline by pointing at `SnapshotSHA` as prior art elsewhere in fabric; the discipline survives and the reference does not. Rewrite the reading-list entry that points at the fabricengine package for "the `SnapshotSHA`/`SyncWeft` mechanics this design relies on". **Preserve** the existing correct point that the recorded SHA must be the **host/warp** code SHA raddle describes — the last host commit before the regeneration — and not raddle's own resulting weft commit SHA: that is exactly what the `Warp-SHA` trailer holds, so the trailer form satisfies the requirement naturally and the paragraph gets stronger, not weaker. Add one thing the file does not say today: raddle's "regenerated but unchanged" case is what motivated the empty-commit rule, and it is why a no-op regeneration still advances the baseline instead of reporting drift forever. Leave the file's other uses of the word "snapshot" alone — the section describing raddle files as a snapshot of the codebase before a plan started is about raddle's own semantics, not the retired API.
- **Commit:** `fabric: rewrite raddle's design against the snapshot trailer API`

### Card 23: Mark slice 4 done and correct fabric-unified-view.md's premise

- **Context:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/commit.go`
  - `manifest/designs/raddle.md`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Build order, mark slice 4 (`**Snapshot-as-trailer**`) **DONE** with the shipped scope, in the same style slices 1–3 already use — each names the landing task and points at `internal/fabricengine/doc.go`'s package comment for the shipped behaviour. Do the same here. Keep the `## Snapshot-tracking folds into the Warp-SHA trailer mechanism` prose as the durable rationale, and add to it the two things it did not anticipate: the empty-commit rule and the unborn-warp exception. **One clause in that section is a required correction, not an addition.** It currently reads that a standalone no-commit snapshot call is only warranted if a consumer must record a baseline without producing weft content — "which raddle/trace (both commit their output) never do; leave it out until a real caller appears". That is precisely the raddle regenerated-but-unchanged case this task's central decision exists to fix. Left as written, the design document would keep asserting that the case the slice was built for cannot arise. Rewrite it to record that the case **does** arise and that it is served by tags-on-`Commit` — with an empty commit when there is no content — rather than by a separate method. That preserves the paragraph's actual conclusion, which is that no standalone `Fabric.Snapshot` is needed; only its premise was wrong. Do **not** delete this file: its own header says the durable parts fold into the fabricengine package doc when the whole item lands and the file is retired, and that is slice 6, not now. Leave slices 5 and 6 untouched.
- **Commit:** `fabric: mark unified-view slice 4 done and correct its snapshot premise`

## Batch Tests

`verify:` runs both tiers over the three package trees this task touches: `go test -count=1 ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/...` followed by the same three under `-tags integration` with the two known-failing `fabricengine` tests skipped. This is the discussion's full-suite requirement, deliberately scoped rather than repo-wide, and the scoping is a measured decision rather than convenience: a repo-wide `go test -tags integration ./...` on the untouched worktree tip already fails in `internal/traceengine` (four tests — a `.exe`-suffix path assertion, a temp-file locking race, and a `gopls` install needing the network), and a repo-wide untagged `go test ./...` already fails in `internal/logger`, `internal/tracecli`, and `internal/traceengine`, with `internal/proc` flaky. Pointing this batch's gate at those packages would make it red for reasons this task cannot cause and must not fix. See the overview's `known-pre-existing-windows-test-failures` Shared Decision for the full baseline and for what it means for `pipeline.done_gate`.

Running both tiers here matters because the two gates catch different things and neither subsumes the other. `cmd/lyx`'s guards — the boundary set-equality check, the tier-purity walk, and the hermetic-env walk — are what fail loudly if any earlier batch left the pinned method set, a build tag, or a `TestMain` inconsistent, and they are module-wide walks that a package-scoped run in an earlier batch could not have exercised against the final tree. The integration tier is where every behavioural assertion in batches 2–4 actually lives.

This batch writes no new test. Both cards are prose in `manifest/designs/`, which has no runnable surface; the verify command exists to confirm the whole task's tree is green at the point the campaign is marked done, not to test these two files.
