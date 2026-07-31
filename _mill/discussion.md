# Discussion: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
slug: fabric-snapshot-trailer
status: discussing
parent: main
```

## Problem

LoomYard carries **two** independent SHA-bookkeeping mechanisms today. `internal/gitrepo` stores per-consumer snapshot SHAs as git refs under `refs/loomyard/snapshot/<key>` (`SnapshotSHA`/`SetSnapshotSHA`, plus a fast-forward-only push with an adopt-on-conflict retry loop, `remoteName`, and `isStrictDescendant` — 365 lines in `internal/gitrepo/snapshot.go`). Separately, `internal/fabricengine` records warp↔weft correspondence as a `Warp-SHA:` git trailer on every weft commit, with a rebuildable index cached on top. Both answer the same question — "which warp SHA does this piece of derived state describe?" — through two unrelated storage models, two failure modes, and two remote-sharing stories.

This is slice 4 of the `fabric: unified-repo view` campaign (`manifest/designs/fabric-unified-view.md`, Build order). Slices 1–3 have landed. The design's resolution is to keep exactly one mechanism: a snapshot becomes an optional `Snapshot: <tag>` trailer written alongside the `Warp-SHA` trailer on a weft commit, and a snapshot's baseline is derived by reading the `Warp-SHA` trailer of the newest weft commit carrying that tag. Same layering the correspondence index already uses — the trailer is truth, anything on top is a rebuildable cache. That retires `refs/loomyard/snapshot/` entirely.

**Why now:** slice 3 (`fabric: warp-side commit lock + push coalescing`, commit `d18291c1`) just landed and touched the same `commit.go`/`weftgit.go` surface this slice touches. The design sequences slice 4 immediately after slice 3 precisely so that surface is not reworked twice. Additionally, the ref-based mechanism has **zero production consumers** — it was ported to go-git during `native clients` and has been dead weight ever since — so retiring it now costs no migration.

## Scope

**In:**

- A reader on `Fabric`: `SnapshotWarpSHA(tag string) (string, error)` — scans the current weft branch's history for the newest commit carrying `Snapshot: <tag>` and returns that commit's `Warp-SHA` trailer value.
- Promoting `parseSnapshotTags` from `trailer_test.go` into `trailer.go` as production code.
- Extending the weft-commit path so snapshot tags always produce a weft commit: when `snapshotTags` is non-empty and no weft commit would otherwise land, land an **empty** weft commit carrying the `Warp-SHA` + `Snapshot:` trailers.
- A new CLI-bound `gitrepo` primitive `CommitEmpty(msg string) (sha string, err error)`, plus its pinned-list and `CONSTRAINTS.md` registration.
- Deleting `internal/gitrepo/snapshot.go` and all its test surface, doc sections, and invariant registrations.
- Documentation: `internal/fabricengine/doc.go`, `internal/gitrepo/doc.go`, `CONSTRAINTS.md`, `manifest/designs/raddle.md`, `manifest/designs/fabric-unified-view.md`.

**Out:**

- **No staleness helper.** No `SnapshotStale(tag)`, no `IsStale()`. Callers compose `f.Warp.ChangedFilesSince(f.SnapshotWarpSHA(tag))` themselves. There is no live consumer to shape such a helper's signature against.
- **No snapshot index/cache.** The reader scans git log on demand. No new state file, no `RebuildIndex` extension.
- **No standalone `Fabric.Snapshot(tag)` no-commit call.** The design explicitly defers this until a consumer needs a baseline without producing weft content; the empty-commit rule below covers every case that would have needed it.
- **No CLI verb.** `SnapshotWarpSHA` is Go-internal, like `Fabric.Diff`/`Fabric.Status` (slice 2's resolved open question).
- **No raddle implementation.** raddle remains unbuilt; only its design doc's now-wrong API references get corrected.
- **No changes to the correspondence index** (`corrindex.go`, `index.go`'s `RecordCorrespondence`/`WeftSHAForWarpSHA`/`RebuildIndex`) beyond what the new reader needs. Snapshot tags do not enter `corrEntry`.
- **No `pathspec`/`fabric.yaml` changes.** `_raddle` graduating into `pathspec` is a separate concern (see the misclassification note under Technical context).
- **Slices 5 and 6 are untouched.** `lyx init` dissolution, clone-does-everything, subpath-in-weft, and warp-rebase/remote-reconcile are later slices.

## Decisions

### reader-api-snapshot-warp-sha

- Decision: expose exactly one reader, `func (f *Fabric) SnapshotWarpSHA(tag string) (string, error)`, returning the `Warp-SHA` trailer value of the newest weft commit on the current branch carrying a `Snapshot: <tag>` trailer.
- Rationale: this is precisely what the named consumer (raddle) needs — the host/warp code SHA the snapshot describes, as its staleness baseline. It mirrors the retired `gitrepo.SnapshotSHA(key)` signature 1:1, so the design doc's own framing ("this retires the separate mechanism") reads as a straight substitution.
- Rejected: `SnapshotSHAs(tag) (warpSHA, weftSHA, err)` — no consumer needs the recording weft commit's identity. A struct-returning `Snapshot(tag) (SnapshotRecord, bool, error)` — room to grow that nothing is growing into.

### scan-on-demand-no-index

- Decision: the reader scans weft history on demand via one `git log --format=...` invocation using git's own `%(trailers:key=...,valueonly)` placeholders for both `Warp-SHA` and `Snapshot`, taking the first (newest) record whose snapshot-tag list contains `tag`. No index file, no cache.
- Rationale: snapshot reads are rare — a staleness check at a phase boundary, not a hot path. An index would add a second cache-invalidation surface (branch switches already force `refreshCorrIndexAfterSwitch` for the correspondence index) for no measured benefit. The trailer remains the sole source of truth, which is the design's stated layering; a cache is explicitly optional in that layering, not required. YAGNI.
- Rejected: extending `corrIndex` with a per-tag latest-snapshot map rebuilt by `RebuildIndex` — literal pattern-match on the correspondence index, but buys cache-invalidation complexity for a caller that does not exist. A separate snapshot index file alongside `fabric-corrindex.json` — same cost plus a second file to keep coherent across branch switches.

### miss-reads-as-absent

- Decision: a tag never recorded in the current branch's history reads as `("", nil)`. Not an error.
- Rationale: absent is a normal state, not a failure — matches the retired `gitrepo.SnapshotSHA`'s contract exactly. A first-ever raddle run must be able to read "no baseline, generate everything" without special-casing an error. This is deliberately *unlike* `index.go`'s `ErrNoCorrespondence`, where a miss genuinely signals broken bookkeeping.
- Rejected: a typed `ErrNoSnapshot` wrapped with the tag — symmetric with `ErrNoCorrespondence` and louder, but it forces every caller to `errors.Is` on the ordinary first-run path.

### delete-ref-mechanism-outright

- Decision: delete `internal/gitrepo/snapshot.go` outright in this slice, along with its whole test and documentation surface. No deprecation window.
- Rationale: `SnapshotSHA`/`SetSnapshotSHA` have **zero production callers** outside `internal/gitrepo` itself (verified by a repo-wide grep of non-test Go source; the only non-gitrepo hits are comments in `cmd/lyx/gitrepoboundary_test.go`). There is nothing to migrate, so a deprecation window would serve nobody while leaving two live mechanisms — exactly what the design says to end. `remoteName` and `isStrictDescendant` are used only by this file's own code paths and die with it; the implementer must confirm that with a grep before deleting rather than assuming it.
- Rejected: keeping the API deprecated — perpetuates the duplication the slice exists to remove. Keeping `remoteName`/`isStrictDescendant` speculatively — dead code with no caller.

### snapshot-tags-always-force-a-weft-commit

- Decision: when `snapshotTags` is non-empty and no weft commit would otherwise land, land an **empty** weft commit carrying the `Warp-SHA` and `Snapshot:` trailers. One rule, applied uniformly to all three cases that reach it: zero weft-side files (a warp-only `Commit`), a pathspec that filters to nothing, and weft files whose content is unchanged.
- Rationale: this is the single decision that dissolves what looked like three separate problems. The unchanged-content case is a genuine correctness hole with no other fix — raddle regenerates against warp SHA X, output is byte-identical, no weft commit lands, so the baseline never advances and the staleness check reports drift forever despite raddle having just confirmed itself current. That is precisely the "record a baseline without producing weft content" case `fabric-unified-view.md` anticipated. Once an empty commit solves that, the same mechanism absorbs the misuse cases too, with **no validation, no typed error, and no misuse-handling code** — the module simply does what the caller asked. A snapshot's whole meaning is "this warp SHA, recorded under this tag", and an empty weft commit records exactly that, faithfully.
- Rejected: a typed `*ErrSnapshotNotRecorded` for the misuse cases (this was the working proposal until the empty commit subsumed it) — adding a handling mechanism for incorrect use of the module, when correct use is the caller's obligation. Silent drop with documentation — leaves the unchanged-content hole unfixed. Deferring to raddle's own task — ships a reader whose only named consumer cannot use it correctly.

### unborn-warp-keeps-todays-behaviour

- Decision: when warp has no HEAD (`warpHeadSHA` reports `unborn`), snapshot tags are dropped exactly as today — no trailers, no empty commit, no error.
- Rationale: a snapshot's entire content is a warp SHA, and there is none. Committing an empty weft commit carrying `Snapshot:` with no `Warp-SHA` would record a tagged commit with no baseline and force the reader to grow a third "found the tag, has no SHA" state — reintroducing the complexity the empty-commit rule removes. This is a genuine bootstrap-only state (`git init` → `lyx init` → `lyx config`, before the operator's first host commit — see `weftgit_unborn_warp_test.go`'s own header), and no snapshot consumer runs there. Critically, this costs **no new code and no special-casing**: `commitWeftLocked` already branches on `unborn`, and the new empty-commit rule simply lives inside the existing `if !unborn` arm.
- Rejected: returning an error — makes a legitimate bootstrap state a failure the caller must handle. Committing with `Snapshot:` and no `Warp-SHA` — a baseline-less snapshot record.

### skipgit-still-drops-tags

- Decision: `opts.SkipGit == true` continues to produce no commit and therefore no snapshot trailer, regardless of tags. Unchanged from today.
- Rationale: `SkipGit` is an explicit "touch no git at all" opt-out. Honouring it is correct, not a silent failure.
- Rejected: nothing — this was never in question, and is recorded only so a reader of the empty-commit rule does not conclude it overrides `SkipGit`.

### commit-empty-as-a-new-gitrepo-primitive

- Decision: add `func (r *Repo) CommitEmpty(msg string) (sha string, err error)` to `internal/gitrepo`, running `git commit --allow-empty -m <msg>` and returning the resulting SHA. Always commits (no `committed bool` — an empty commit cannot no-op).
- Rationale: neither existing primitive can be coaxed into an empty commit. `StageAndCommit` returns `("", false, nil)` on an empty file list by deliberate design (an unscoped `git commit` would otherwise commit whatever happened to be staged), and its `diff --cached --quiet` gate returns the same no-op on unchanged content. A single-purpose method reads clearly at its one call site and cannot be misread as a variant of `StageAndCommit`.
- Rejected: `StageAndCommit(msg, files, allowEmpty bool)` — changes every existing call site and puts a boolean flag on the package's most-used method.

### commit-result-reports-the-empty-commit-plainly

- Decision: a warp-only `Commit` carrying snapshot tags returns `WeftCommitted: true` with `WeftSHA` set, where today it returns `false`. No new field.
- Rationale: it is literally true — a weft commit landed. `Commit`'s own async-push gate (`result.WarpCommitted || result.WeftCommitted`) then correctly pushes both sides, which is what should happen: the snapshot trailer must reach the remote for cross-clone sharing, the property the retired ref mechanism achieved by pushing its ref.
- Rejected: a distinguishing `WeftEmpty bool` — no caller needs the distinction. YAGNI.

## Technical context

**The write half already exists.** Slice 2 landed it. `internal/fabricengine/trailer.go` already has `SnapshotTrailerKey = "Snapshot"`, `snapshotTagPattern` (`^[A-Za-z0-9._-]+$`, deliberately excluding newline/CR/colon to close the trailer-injection vector), `ErrInvalidSnapshotTag`, `validateSnapshotTag`, and `appendSnapshotTrailers` (validate-all-before-appending-any, so a single bad tag fails the call with nothing written). `Fabric.Commit(files, msg, snapshotTags []string, opts)` and `Fabric.CommitWeft(pathspec, message, opts, snapshotTags ...string)` already thread tags to `commitWeftLocked`, which appends them after the `Warp-SHA` trailer. **This slice adds no new write-side format work** — only the reader, the empty-commit rule, and the retirement.

**Key files:**

- `internal/fabricengine/trailer.go` — trailer format/parse. `parseWarpSHATrailer` is the shape to mirror. `parseSnapshotTags` currently lives in `trailer_test.go:131` and must move here as production code.
- `internal/fabricengine/index.go` — `scanWarpSHATrailers` (line 188) is the direct model for the new scan: `git log --format=` with `%H` + `%(trailers:key=Warp-SHA,valueonly)`, using `\x1f` (unit) and `\x1e` (record) control-character separators, and tolerating an unborn weft HEAD by matching `"does not have any commits yet"` in stderr and returning no commits. The new scan needs a third field for `%(trailers:key=Snapshot,valueonly)`. Note that a commit with multiple `Snapshot:` trailers yields a **multi-line** value for that placeholder — the parser must split on newline within the field, not assume one value.
- `internal/fabricengine/commit.go:99-102` — `weftSide := len(weftFiles) > 0 && !opts.SkipGit` becomes `(len(weftFiles) > 0 || len(snapshotTags) > 0) && !opts.SkipGit`. Nothing else in `commitBothSides` needs to change: its three-outcome `*PartialCommitError` mapping already handles whatever `commitWeftLocked` returns.
- `internal/fabricengine/weftgit.go:349-400` — `commitWeftLocked`. Two early returns must fall through to the empty commit when tags are present and `!unborn`: `if !positive` (line 368, after `weftPathspecFilter`) and `if !committed` (line 383, after `StageAndCommit`). The existing `RecordCorrespondence(warpSHA, sha)` call at line 395 must also run for the empty commit — it carries a `Warp-SHA` trailer like any other, so the correspondence index must record it or `RebuildIndex` and the incremental path would diverge.
- `internal/gitrepo/gitrepo.go` — `StageAndCommit` (the primitive `CommitEmpty` sits beside), `CurrentSHA` (line 114), `ChangedFilesSince` (line 449).
- `internal/gitrepo/snapshot.go` — the whole file is deleted.

**Deletion footprint (verified by grep):** `internal/gitrepo/snapshot.go`, `internal/gitrepo/snapshot_test.go`; snapshot fixtures and cases in `internal/gitrepo/parity_test.go` (`newSnapshotRefFixture` ~line 70, `writeSnapshotRef` ~line 100, `TestSnapshotSHA_Parity_SetRef`/`_AbsentRef`/`_InvalidKey` ~lines 400-450), `internal/gitrepo/oracle_test.go` (`oracleSnapshotSHA` ~line 132), `internal/gitrepo/keyvalidation_test.go`, `internal/gitrepo/gogit_test.go`; the "# Snapshot remote model" section in `internal/gitrepo/doc.go` (~lines 172-206) plus scattered mentions at lines 17-18, 23, 39, 59, 65, 72-74, 141, 174-202, 253-255; `internal/gitrepo/gitrepo.go:28` (`ErrInvalidSHA`'s doc naming `SetSnapshotSHA`); `internal/gitrepo/gogit.go:89, 108-109, 184`; `internal/gitrepo/push.go:59`.

**Guard tests that will fail unless updated in the same commit:**

- `cmd/lyx/gitrepoboundary_test.go` — `gitrepoPinnedRunBoundMethods` is asserted by **set equality**, so removing `"SnapshotSHA"`, `"advanceAndPushSnapshotRef"`, and `"adoptSnapshotRef"` is mandatory, and adding `"CommitEmpty"` is mandatory. The explanatory comment above the map (lines 38-49) names `SnapshotSHA`, `Push`, `PushCoalesced`, and `SetSnapshotSHA` as its worked examples and must be rewritten. `gitrepoBoundaryMinScannedFiles` documents "7 non-test .go files today (…, snapshot.go)" — that enumeration becomes 6 files; the floor of 5 still holds, but the comment must be corrected.
- `CONSTRAINTS.md`, **gitrepo Client Boundary Invariant** — its CLI-bound set is described as named "exhaustively here, not just in the package doc, because this entry is what a reviewer checks a new call against", and the entry states that any new `gitexec` call inside `internal/gitrepo` must come with an updated entry in the same commit. So: remove `SetSnapshotSHA`'s push and `SnapshotSHA`'s fetch, add `CommitEmpty`, and rewrite the **Known blind spot** bullet, whose only worked example is `SnapshotSHA`'s legitimate mixing of a migrated read with a CLI-bound fetch. A replacement example must be chosen from a surviving pinned method.

**How a caller can end up with snapshot tags and zero weft files** (the analysis behind `snapshot-tags-always-force-a-weft-commit`): `Commit` classifies paths against `WiredNames(f.weftPath)`, which reads `fabric.yaml`'s `pathspec` — today `_lyx` and `_pattern`. Slice 1's record notes `_raddle` is **reserved-only and not yet in `pathspec`**. So raddle calling `Commit(["_raddle/Overview.md"], msg, ["raddle"], opts)` before `_raddle` graduates classifies its output as **warp-side**, yielding zero weft files. The call site looks correct; the config is what is wrong. This route is why silent-drop was unacceptable — it fails at a distance and its only symptom is a snapshot that never appears. The empty-commit rule makes the snapshot land regardless; the misplaced *content* is a separate config bug this slice does not attempt to catch (a warp-side file is a legitimate thing to commit).

**Cross-clone sharing comes for free.** The retired ref mechanism pushed `refs/loomyard/snapshot/*` to the remote so state was shared across clones. Trailers live in weft's own commit history, which is already pushed by the existing detached both-sides push — no equivalent machinery is needed.

**Docs to update** (Documentation Lifecycle, same commit):

- `internal/fabricengine/doc.go` — new section on the snapshot trailer: the tag format and its injection-vector rationale, the read path (newest-tagged-commit-wins, scan-not-cache, miss-is-absent), the empty-commit rule and its three triggering cases, the unborn-warp exception, and `SkipGit`.
- `internal/gitrepo/doc.go` — delete the "# Snapshot remote model" section and every scattered mention listed above.
- `CONSTRAINTS.md` — as detailed above. No *new* invariant is introduced by this slice.
- `manifest/designs/raddle.md` — lines 27, 35, 36, and 50 reference the `SetSnapshotSHA`/`SnapshotSHA` ref API and go actively wrong on this commit. Rewrite to the trailer API: raddle's regeneration records its baseline by passing `snapshotTags=["raddle"]` to `Fabric.Commit`, and the staleness check becomes `f.Warp.ChangedFilesSince(f.SnapshotWarpSHA("raddle"))`. Preserve the existing correct point that the recorded SHA is the **host/warp** SHA raddle describes, not raddle's own weft commit SHA — that is exactly what the `Warp-SHA` trailer holds, so the trailer form satisfies it naturally. Also worth recording there: raddle's "regenerated but unchanged" case is what motivated the empty-commit rule.
- `manifest/designs/fabric-unified-view.md` — mark slice 4 **DONE** in the Build order with the shipped scope, in the same style as slices 1-3. The prose at line 63-67 ("Snapshot-tracking folds into the `Warp-SHA` trailer mechanism") stays as the durable rationale; add the empty-commit rule and the unborn-warp exception to it, since neither was anticipated there. Note that this file's own header says the durable parts fold into `internal/fabricengine`'s package doc when the *whole* item lands and the file is deleted — that is slice 6, not now, so the file stays.
- `docs/overview.md` — checked: its two `gitrepo` references (lines 144, 190) are one-line module descriptions that do not enumerate `SnapshotSHA`, so **no change needed**. `docs/shared-libs/README.md` — checked, no `SnapshotSHA` mention. `manifest/roadmap.md` — **no change**: this is a planned campaign slice, and per CLAUDE.md the roadmap moves only on completing or adding a planned item; the implementer should confirm whether the campaign's roadmap entry tracks per-slice status and update only if it does.

## Constraints

From `CONSTRAINTS.md`:

- **gitrepo Client Boundary Invariant** — go-git owns local object/ref reads; `gitexec` is the only path to the git CLI, and its CLI-bound set is enumerated exhaustively in `CONSTRAINTS.md`. `CommitEmpty` mutates the working tree's history and is therefore CLI-bound: it must be added to both the invariant's list and `gitrepoPinnedRunBoundMethods`, in the same commit. The guard also asserts the bare token `gitexec.` appears exactly once in the package's non-test source (inside `run`), so `CommitEmpty` must go through `r.run`, never `gitexec.RunGit` directly.
- **Test Tier Purity Invariant** — a test file whose first non-empty line is not a `//go:build` constraint mentioning `integration` or `smoke` must not spawn: no `gitexec.RunGit`, no `exec.Command`, no `lyxtest.Copy*`. The reader's tests need real git history and are therefore integration-tagged; the trailer-parsing tests must stay untagged and git-free.
- **Hermetic git env** — every git-spawning test file is checked for the `HermeticGitEnv` token; `internal/fabricengine`'s existing `testmain_test.go` already provides this for the package.
- **Hub Geometry Invariant** — untouched by this slice; no cwd/geometry or `_lyx`/config path logic changes.
- **Documentation Lifecycle** — module docs, `docs/overview.md`, and `CONSTRAINTS.md` land in the same commit as the code. Enumerated under Technical context.

Discovered during discussion:

- **`internal/gitrepo` is geometry-blind** and must stay so. `CommitEmpty` takes only a message; it knows nothing about warp/weft or trailers. All trailer composition stays in `fabricengine`.
- **`appendSnapshotTrailers`'s validate-all-before-append property must survive** the empty-commit path: a bad tag must fail the call before any commit is made, empty or otherwise.
- **`StageAndCommit`'s empty-list guard exists for a reason** (an unscoped `git commit` would commit whatever is already staged). `CommitEmpty` must not reintroduce that hazard — `--allow-empty` with an explicit message and no pathspec commits an empty tree delta only when nothing is staged. The implementer must confirm behaviour when something *is* incidentally staged in the weft worktree: under the combined write lock this should not occur, but the doc comment must state the assumption.

## Testing

**`internal/fabricengine` — untagged Tier-1 unit tests** (`trailer_test.go`, existing file, no git):

- `parseSnapshotTags` after promotion to production code: single tag; multiple tags on one message; no tags; tags interleaved with `Warp-SHA` and an unrelated `Co-authored-by:` trailer; whitespace tolerance around key and value, mirroring `parseWarpSHATrailer`'s existing tolerance. The existing tests that already call the test-local `parseSnapshotTags` must be repointed at the production function rather than duplicated.
- TDD candidate: the multi-value split. `%(trailers:key=Snapshot,valueonly)` returns one value per line for a commit with several tags, and the record parser must split within the field. Write the failing table case first.

**`internal/fabricengine` — integration tests** (new `snapshot_integration_test.go`, `//go:build integration`, reusing `index_integration_test.go`'s `newFabric`/`currentSHA`/`commitWarp` helpers, which share the package):

- Newest-tagged-commit wins: three weft commits tagged `raddle` at three different warp SHAs; the reader returns the newest one's `Warp-SHA`.
- Tag isolation: commits tagged `raddle` and `trace` interleaved; each tag resolves to its own newest commit, not the other's.
- Multiple tags on one commit: a commit tagged both `raddle` and `trace` resolves correctly for each.
- Miss: a tag never recorded returns `("", nil)` — not an error. TDD candidate; this pins `miss-reads-as-absent`.
- Unborn weft HEAD (zero weft commits) returns `("", nil)`, exercising the `"does not have any commits yet"` tolerance `scanWarpSHATrailers` already has.
- Untagged weft commits (history predating the tag, or plain `CommitWeft` calls) are skipped without error.

**`internal/fabricengine` — empty-commit behaviour** (integration; extends `commit_integration_test.go`, which already carries the seam `spawnDetachedPushFn` swap pattern — a recorder/no-op with a deferred restore and no `t.Parallel()`):

- **`TestCommit_WarpOnly_SnapshotTagsDropped` is inverted**, not deleted. Same setup; the assertion flips from "no `Snapshot:` trailer anywhere" to "an empty weft commit landed carrying `Warp-SHA` + `Snapshot: <tag>`". Its doc comment must be rewritten to state the new rule; leaving the old name in place would be actively misleading. TDD candidate, and the clearest single expression of `snapshot-tags-always-force-a-weft-commit`.
- Unchanged weft content + tags: commit the same weft content twice with the same tag; the second call lands an empty commit and `SnapshotWarpSHA(tag)` advances to the newer warp SHA. TDD candidate — this is the correctness hole the rule exists to close, so it should fail before the change and pass after.
- Pathspec filtering to nothing + tags: an empty commit still lands.
- No tags + nothing to commit: still a clean no-op — no empty commit. Guards against the rule over-firing.
- `opts.SkipGit` + tags: no commit, no trailer, no error.
- `CommitResult` shape on a warp-only tagged commit: `WeftCommitted == true`, `WeftSHA` non-empty, and the push seam observed exactly one spawn.
- `RecordCorrespondence` ran for the empty commit: `WeftSHAForWarpSHA(warpSHA)` resolves to the empty commit's SHA, and a subsequent `RebuildIndex` produces an equivalent index (the existing `entries()` comparison helper covers this).
- Invalid tag + otherwise-empty commit: `*ErrInvalidSnapshotTag` returned and **nothing** committed on either side.

**`internal/gitrepo` — `CommitEmpty`** (integration-tagged, alongside the package's existing git-spawning tests):

- Commits on a repo with an existing HEAD; returns a SHA that `SHAExists` confirms and whose tree equals its parent's.
- Two successive calls produce two distinct SHAs.
- Behaviour on an unborn HEAD — decide and pin whichever `git commit --allow-empty` actually does as the root commit; the `fabricengine` caller never reaches it (see `unborn-warp-keeps-todays-behaviour`), but the primitive's contract should be stated rather than left undefined.

**Deletion coverage:** removing `snapshot_test.go` and the parity/oracle/keyvalidation fixtures is not a coverage regression — the deleted code has no callers. The real regression guard is that `cmd/lyx/gitrepoboundary_test.go` and `CONSTRAINTS.md` are updated in the same commit; the boundary test's set-equality assertion fails loudly otherwise, which is the intended forcing function.

**Full-suite requirement:** run both `go test ./...` and `go test -tags integration ./...`. The boundary guard lives in `cmd/lyx` and the trailer tests in `internal/fabricengine`, so a package-scoped run will not catch the guard failures this slice deliberately provokes.

## Q&A log

- **Q:** Reader API shape? **A:** `SnapshotWarpSHA(tag string) (string, error)` — mirrors the retired `SnapshotSHA(key)` 1:1; rejected the weft-SHA-returning and struct-returning variants as unearned.
- **Q:** Scan on demand or build an index cache? **A:** Scan on demand, no index. Reads are rare; an index adds a second cache-invalidation surface for a caller that does not exist yet.
- **Q:** What does a never-recorded tag read as? **A:** `("", nil)` — absent is a normal state, matching the retired contract, so a first-ever run needs no error special-casing.
- **Q:** Delete `refs/loomyard/snapshot/` now or deprecate? **A:** Delete outright — zero production consumers, so nothing to migrate and no deprecation window to serve.
- **Q:** Snapshot tags on a commit that produces no weft commit — silent drop, error, or report in the result? **A:** None of those. The user identified that passing tags with no weft files is caller misuse, and rejected adding a handling mechanism for incorrect use of a module on principle: *"Jeg er veldig liten fan av å legge inn håndteringsmekanismer som skal håndtere at en bruker bruker modulen FEIL. Det er brukeren som skal bruke modulen riktig."* The user then observed that the empty commit chosen for the unchanged-content case solves this one too. It does, and it removes the error path entirely — hence `snapshot-tags-always-force-a-weft-commit`.
- **Q:** How can a caller actually end up with tags and zero weft files? **A:** Via `pathspec` misclassification — `_raddle` is not yet in `fabric.yaml`'s `pathspec`, so raddle's own output would classify warp-side. The call site looks correct and the config is wrong, which is why silent drop was unacceptable: it fails at a distance with no symptom but a missing snapshot.
- **Q:** Unborn warp under the empty-commit rule? **A:** Keep today's behaviour — drop the tags. A snapshot with no warp SHA has no content, and the case is bootstrap-only (`git init` before the first host commit). The user flagged it as a marginal special case; it needs no new code, since `commitWeftLocked` already branches on `unborn` and the new rule lives inside the existing `if !unborn` arm.
- **Q:** New `gitrepo` primitive for the empty commit? **A:** `CommitEmpty(msg) (sha, err)` — single-purpose. Rejected an `allowEmpty bool` parameter on `StageAndCommit`, which would touch every existing call site.
- **Q:** How does `CommitResult` report an empty snapshot commit? **A:** Plainly — `WeftCommitted: true`, `WeftSHA` set. It is true, and it makes the existing async-push gate push the trailer to the remote, which is what cross-clone sharing needs.
- **Q:** Ship a staleness helper alongside the reader? **A:** No. Callers compose `ChangedFilesSince(SnapshotWarpSHA(tag))`; no live consumer exists to shape the signature.
- **Q:** Documentation scope? **A:** `fabricengine/doc.go`, `gitrepo/doc.go`, `CONSTRAINTS.md`, `raddle.md`, and `fabric-unified-view.md`. `raddle.md` is the one that goes actively wrong if skipped. `docs/overview.md` and `docs/shared-libs/README.md` were checked and need no change.
- **Q:** Test tiering? **A:** Untagged Tier-1 units for the promoted `parseSnapshotTags`; integration-tagged tests for the reader, the empty-commit paths, and `CommitEmpty`.
- **Q:** Is `lyx init` being phased out in favour of fabric V2? **A:** Yes, but in **slice 5**, not this one. `initengine`/`initcli` shrink toward deletion as topology folds into clone/worktree-add and the cwd anchor is replaced by the weft-recorded subpath. The only contact point with this slice is that `internal/initengine/undo.go` is a `CommitWeft` call site, and it uses the 3-argument form with no tags, so the empty-commit rule does not reach it.
