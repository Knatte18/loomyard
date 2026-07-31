# Batch: snapshot-reader

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: snapshot-reader
number: 3
cards: 5
verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch builds the read half of the trailer mechanism: it generalizes `scanWarpSHATrailers` so one `git log` pass captures each commit's `Snapshot` trailer values alongside its `Warp-SHA` value, adds `--topo-order` to that invocation, and exposes `Fabric.SnapshotWarpSHA(tag string) (string, error)` on top. The write half already shipped in slice 2 — `SnapshotTrailerKey`, `snapshotTagPattern`, `ErrInvalidSnapshotTag`, `validateSnapshotTag`, and `appendSnapshotTrailers` all exist in `internal/fabricengine/trailer.go`, and `Fabric.Commit`/`Fabric.CommitWeft` already thread tags down to `commitWeftLocked`. **This batch adds no write-side format work.**

It is a root batch alongside batch 1: it touches only `internal/fabricengine`, which batch 1 and batch 2 never open. Batch 4 depends on it because both edit `internal/fabricengine/index.go` and `internal/fabricengine/doc.go`, and because batch 4's tests read baselines back through `SnapshotWarpSHA`.

**Batch-local decision — `parseSnapshotTags` is not promoted.** It stays a test-local helper in `trailer_test.go`. Promoting it into production was the working plan until discussion review caught that it would be **dead code**: the placeholder-based scan never sees a full commit message, so a promoted full-message parser would have no caller — exactly what batch 1 refuses to tolerate for `remoteName`/`isStrictDescendant`. The Tier-1 unit subject in this batch is the pure record parser extracted from the generalized scan, not `parseSnapshotTags`.

**Batch-local decision — `--topo-order` is a behaviour change to a shipped function, not a no-op.** `scanWarpSHATrailers` today passes no ordering flag at all, so it gets git's default reverse-chronological order, and `RebuildIndex` consumes its output in two order-sensitive ways. Card 10 states the intended outcome; card 13 builds the fixture that can actually witness the change.

## Cards

### Card 10: Generalize the trailer scan and pin its record parser

- **Context:**
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/fabric.go`
- **Edits:**
  - `internal/fabricengine/index.go`
- **Creates:**
  - `internal/fabricengine/index_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give `warpSHATrailerCommit` a third field, `snapshotTags []string`. Extend the format string `scanWarpSHATrailers` builds so it carries a third unit-separated field, `%(trailers:key=Snapshot,valueonly)`, alongside the existing `%H` and `%(trailers:key=Warp-SHA,valueonly)`, keeping the existing `warpSHATrailerFormatUnitSep` and `warpSHATrailerFormatRecordSep` control characters. Add `--topo-order` to the `git log` argument list. Extract the per-record parsing into a **pure, git-free** helper — suggested name `parseTrailerScanRecord`, taking one `\x1e`-delimited record string and returning `(weftSHA, warpSHA string, snapshotTags []string, ok bool)` — so it is testable in Tier 1, and have the scan loop call it. Preserve three existing behaviours exactly: the surrounding-newline trim on each record before splitting; the skip of any record whose `Warp-SHA` field is empty (a snapshot record with no baseline is not usable by the reader either, so the rule holds for both consumers); and the unborn-weft-HEAD tolerance that matches `"does not have any commits yet"` in stderr and returns no commits rather than an error. Note the field-count change: `strings.SplitN(record, sep, 2)` must become a three-way split, and a record shorter than three fields must still parse the fields it does have rather than panic. A commit carrying several `Snapshot:` trailers yields a **multi-line** value for that one placeholder, so the parser must split within the field on newline rather than treat it as a single string — this is the single most likely implementation slip in the whole batch. `RebuildIndex` must keep ignoring the new field entirely; snapshot tags never enter `corrEntry`. Update the comment inside `RebuildIndex` that calls the walk order's purpose "newest-first, matching `git log`'s natural order" — it is now topological order, and the two order-sensitivities must be stated rather than left implicit: dedup by warp SHA is last-assignment-wins over the reversed scan, so for a warp SHA recorded by more than one weft commit the winner is whichever commit the scan listed **first**; and `sort.SliceStable` preserves input order among equal `WarpSeq`, which covers both the `seq = 0` dangling sentinel and genuine side-branch commits at equal first-parent depth. For both cases the intended outcome is **the newest commit in topological order wins**, which is the same rule the reader uses and is what makes reader and index agree. Say why the flag was added: commit date is wall-clock from whichever machine made the commit, snapshot commits arrive from other machines through `Pull`, and a skewed clock placing an older baseline first would under-report staleness — the one failure direction that loses data. Create `internal/fabricengine/index_test.go` as an **untagged** Tier-1 file (no build constraint) covering `parseTrailerScanRecord` only, with no git spawn, no `exec.Command`, and no `lyxtest.Copy` — those tokens fail the Test Tier Purity guard as raw substrings even inside a comment. Write the multi-tag case **first** and watch it fail before implementing the split; that is the TDD candidate here. Cases: no snapshot field; exactly one tag; several tags arriving as a multi-line value; a record carrying a `Snapshot` value but no `Warp-SHA` (skipped); an empty record; and surrounding-newline trimming.
- **Commit:** `fabric: generalize the weft trailer scan to capture Snapshot tags`

### Card 11: Add the SnapshotWarpSHA reader

- **Context:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/fabric.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/fabricengine/checkout.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/snapshot.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file holding exactly one exported method: `func (f *Fabric) SnapshotWarpSHA(tag string) (string, error)`. It calls the generalized `scanWarpSHATrailers`, walks the returned commits in order, and returns the `warpSHA` of the **first** record whose `snapshotTags` contains `tag`. Because card 10 added `--topo-order`, "first" is now "newest in topological order", and no commit is ever listed before one of its descendants. Four contract points, each of which is a deliberate decision rather than an implementation detail, and each of which must be stated in the method's godoc. **A tag never recorded in the current branch's history returns `("", nil)`, not an error** — absent is a normal state, it matches the retired `gitrepo.SnapshotSHA` contract exactly, and a first-ever consumer run must be able to read "no baseline, generate everything" without special-casing an error. This is deliberately unlike `index.go`'s `ErrNoCorrespondence`, where a miss signals broken bookkeeping. **The `tag` argument is matched byte-exactly and is not validated** against `snapshotTagPattern`: validation belongs on the write path and already lives there, a tag that could not have been written cannot match anything, and fuzzy matching would let a lookup for `Raddle` silently resolve a baseline recorded as `raddle`, hiding a caller bug behind a convenience. **A dangling `Warp-SHA` is returned raw with a nil error** — when the newest commit carrying the tag names a warp SHA that no longer exists because warp history was rewritten, do not validate, do not skip to an older tagged commit, and do not collapse to `("", nil)`. This is the same validate-at-use posture `RebuildIndex` already takes. Skipping to the next-newest tagged commit would answer with an *older* baseline and under-report staleness; collapsing to empty would conflate "never recorded" with "recorded, then rewritten" for no gain, since both drive the same consumer action. State the three-step consumer idiom in the godoc — read, then `f.Warp.SHAExists(sha)`, then `f.Warp.ChangedFilesSince(sha)`, treating a missing SHA as total staleness — and note that this is not a burden invented here: `ChangedFilesSince`'s own doc already asks callers to check `SHAExists` first. The naive two-step composition hard-errors on exactly the rebase case the mechanism exists to survive. **The reader is per-branch**, because it scans the current weft branch's history. A snapshot recorded on another branch reads as absent after a coordinated `Checkout`, and the consumer regenerates from scratch — the intended behaviour and the safe failure direction. Contrast it with `refreshCorrIndexAfterSwitch`: the correspondence index is a per-worktree *file* that survives a branch switch and can therefore answer cross-branch, so it must be discarded; the reader holds no state and simply stops seeing the other branch's commits.
- **Commit:** `fabric: add Fabric.SnapshotWarpSHA reading the newest tagged weft commit`

### Card 12: Integration coverage for the reader

- **Context:**
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/trailer.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/testmain_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file, first non-empty line `//go:build integration`, reusing `index_integration_test.go`'s existing `newPlainWarpRepo`, `currentSHA`, `commitWarp`, `commitWeftWithTrailer`, and `newFabric` helpers rather than building a parallel harness — they share the package. Where a test needs a commit carrying `Snapshot:` trailers, drive it through `f.CommitWeft(pathspec, msg, opts, tags...)`, which already threads tags today. Cover nine cases. **Newest-tagged-commit wins:** three weft commits tagged `raddle` at three different warp SHAs; the reader returns the newest one's `Warp-SHA`. **Tag isolation:** commits tagged `raddle` and `trace` interleaved; each tag resolves to its own newest commit, never the other's. **Multiple tags on one commit:** a commit tagged both `raddle` and `trace` resolves correctly for each — this is the integration-level witness for card 10's multi-line-value split. **Miss:** a tag never recorded returns `("", nil)` and no error. Write this one first; it is the TDD candidate here and it pins the absent-is-not-an-error decision. **Unborn weft HEAD:** a weft repo with zero commits returns `("", nil)`, exercising the `"does not have any commits yet"` tolerance the scan already carries. **Untagged weft commits are skipped** without error — history predating the tag, or plain `CommitWeft` calls with no tags. **A commit carrying a `Snapshot:` trailer but no `Warp-SHA` trailer is skipped**, not returned as an empty baseline; construct it by committing while warp has an unborn HEAD, or by writing the message directly. **Byte-exact matching:** a tag recorded as `raddle` is resolved by neither `Raddle` nor `raddle ` with a trailing space; both read as absent and neither errors. **Per-branch scoping:** record a tag on the weft worktree's current branch, switch that worktree to another branch with a plain `git checkout -b <other>` through `lyxtest.MustRun` inside the same lightweight fixture, and assert `SnapshotWarpSHA(tag)` returns `("", nil)` rather than answering cross-branch. Do **not** reach for `Topology.Checkout` here, for two reasons. It needs a full `*hubgeometry.Layout`, and the only fixture in this package that builds one lives in the **external** test package `fabricengine_test`, whereas `index_integration_test.go`'s helpers and therefore this whole file are in `package fabricengine` — so that fixture is not reachable from here at all, not merely heavier. And it would test the wrong thing: `SnapshotWarpSHA` scans the weft worktree's current branch and nothing else, so a weft-side branch switch is the whole mechanism under test, while the coordinated checkout is only how that state arises in production. That last case matters more than its size suggests — per-branch scoping is an explicitly chosen and documented contract that would otherwise ship with no test behind it.
- **Commit:** `fabric: integration coverage for SnapshotWarpSHA`

### Card 13: Pin the topological-order change with a discriminating fixture

- **Context:**
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/index_integration_test.go`
  - `internal/fabricengine/syncweft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the one test that can actually witness card 10's ordering change, plus a rebuild-equivalence assertion on the same history. This card exists because **none** of the existing or otherwise-listed cases can witness it: `TestRebuildIndex_EqualsIncrementallyBuiltIndex` in `syncweft_integration_test.go` is three linear `SyncWeft` rounds, and every card 12 case is linear too — shapes where date order and topological order coincide, so they pass identically before and after the flag and prove nothing. Build a weft history where the two orders genuinely differ: a side branch merged back into the current branch, carrying a snapshot commit whose **committer date is back-dated** behind a topologically-older commit on the mainline. Set the date through `GIT_COMMITTER_DATE`, and note that there is **no existing convention to copy** — `lyxtest.MustRun(tb, dir, args...)` takes no environment, `gitexec.RunGit` takes none either, and no test in this package sets a git environment variable today. So this one commit must bypass both helpers: build a raw `exec.Command("git", ...)` — the same shape `syncweft_integration_test.go`'s own helpers already use — and set `cmd.Env` scoped to that single call. Do **not** reach for a process-wide `os.Setenv`: several tests in this package run under `t.Parallel()`, so a process-wide override would leak into whichever of them happens to be committing at the same moment, and the resulting failure would be intermittent and blamed on the wrong test. The package's `TestMain` already runs under `lyxtest.HermeticGitEnv()`, so the raw command must inherit `os.Environ()` and append to it rather than replacing it wholesale, or it loses the hermetic settings. Then assert two things on that one history. First, `SnapshotWarpSHA(tag)` returns the **topologically-newest** baseline, not the date-newest one. Second, `RebuildIndex` agrees with the incrementally-built index on the same history — reuse the `corrIndex.entries()` comparison the existing equivalence test already uses. Without this fixture, `--topo-order` ships as an unverified change to a shipped invocation. Also record in the test's own doc comment what the residual ambiguity is and that it is accepted rather than solved: when two snapshot commits for the same tag sit on genuinely **concurrent** branches, neither is an ancestor of the other, and "newest" is a topological choice between incomparable commits. Both are legitimate baselines; whichever is chosen, the consumer's `ChangedFilesSince` against it reports a superset or equal set of the truly-changed files, and over-reporting is the safe direction.
- **Commit:** `fabric: pin topological trailer-scan ordering with a back-dated merge fixture`

### Card 14: Document the snapshot read path in fabricengine's package doc

- **Context:**
  - `internal/fabricengine/snapshot.go`
  - `internal/fabricengine/index.go`
  - `internal/fabricengine/trailer.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the snapshot-trailer read path to the package comment, following the file's existing one-paragraph-per-topic register and the repo's markdown rule of one line per paragraph with no hard wrapping. Cover: the tag format and the injection-vector rationale behind `snapshotTagPattern` excluding newline, carriage return, and colon; the read path — newest-tagged-commit-wins in **topological** order, scan-not-cache, and why no index was added; miss-reads-as-absent; **per-branch scoping** and its contrast with `refreshCorrIndexAfterSwitch`; byte-exact tag matching with no read-side validation; and a dangling `Warp-SHA` returned raw with the three-step consumer idiom spelled out rather than implied. State plainly that the trailer is the sole source of truth and that anything built on top is a rebuildable cache — the same layering the correspondence index already rests on — because that is what justifies having no snapshot index at all. Do not document the empty-commit rule here; it does not exist yet and batch 4 owns that paragraph.
- **Commit:** `fabric: document the snapshot trailer read path`

## Batch Tests

`verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./cmd/lyx/...` — the package tree this batch touches, plus `cmd/lyx` for the same reason batches 1 and 2 include it: `TestTierPurity_UntaggedTestsSpawnNothing` and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` are module-wide walks that live there and fire on a test file added anywhere. That risk is concrete in this batch, not theoretical — card 10 creates an **untagged** `index_test.go`, and the tier-purity guard matches `gitexec.RunGit`, `exec.Command`, and `lyxtest.Copy` as raw substrings, so even naming one of them in a comment trips it. The `-tags integration` build also compiles and runs `internal/fabricengine`'s untagged tests, including that new file, so one command covers both tiers.

The two skipped tests fail on Windows **before any change in this task** — see the overview's `known-pre-existing-windows-test-failures` Shared Decision. They are not this batch's regressions and must not be fixed here.

New coverage is `index_test.go` (Tier 1, the record parser, with the multi-tag split written first as the TDD case) and `snapshot_integration_test.go` (the reader's nine contract cases plus card 13's ordering fixture). Card 13 is the load-bearing one for the riskiest change in this batch: `--topo-order` alters a shipped `git log` invocation that `RebuildIndex` also consumes, and no linear-history test can tell the two orderings apart. Measured runtime for the package's integration tier on the untouched tree is about 135s.
