# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/fabric-snapshot-trailer rm <file>`.

### From discussion.md

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
- Generalizing `index.go`'s `scanWarpSHATrailers` so one scan captures each commit's `Snapshot` trailer values alongside its `Warp-SHA` value, serving both the correspondence rebuild and the new reader.
- Extending the weft-commit path so snapshot tags always produce a weft commit: when `snapshotTags` is non-empty and no weft commit would otherwise land, land an **empty** weft commit carrying the `Warp-SHA` + `Snapshot:` trailers.
- A new CLI-bound `gitrepo` primitive `CommitEmpty(msg string) (sha string, err error)`, plus its pinned-list and `CONSTRAINTS.md` registration.
- Deleting `internal/gitrepo/snapshot.go` and all its test surface, doc sections, and invariant registrations.
- Documentation: `internal/fabricengine/doc.go`, `internal/gitrepo/doc.go`, `CONSTRAINTS.md`, `manifest/designs/raddle.md`, `manifest/designs/fabric-unified-view.md`.

**Out:**

- **No staleness helper.** No `SnapshotStale(tag)`, no `IsStale()`. Callers compose the read themselves — via the three-step idiom `SnapshotWarpSHA` → `f.Warp.SHAExists` → `f.Warp.ChangedFilesSince`, not the two-step composition (see `reader-returns-a-dangling-warp-sha-raw`). There is no live consumer to shape such a helper's signature against.
- **No snapshot index/cache.** The reader scans git log on demand. No new state file, no `RebuildIndex` extension.
- **No standalone `Fabric.Snapshot(tag)` method.** The design defers a dedicated no-commit snapshot call until a consumer needs one. No new method is added — but note that under the empty-commit rule, `Commit(nil, msg, []string{"raddle"}, opts)` (tags, zero files) already *is* that call: it lands an empty weft commit recording the baseline. That shape is supported deliberately, not incidentally (see `snapshot-tags-always-force-a-weft-commit`).
- **No CLI verb.** `SnapshotWarpSHA` is Go-internal, like `Fabric.Diff`/`Fabric.Status` (slice 2's resolved open question).
- **No raddle implementation.** raddle remains unbuilt; only its design doc's now-wrong API references get corrected.
- **No changes to the correspondence index's data shape.** Snapshot tags never enter `corrEntry`. But this is *not* a claim that the index is untouched: `RebuildIndex` and `RecordCorrespondence` are both affected in behaviour — by the scan's new `--topo-order` (see `reader-orders-by-topology-not-commit-date`) and by empty commits recording against an already-recorded warp SHA (see `empty-commits-take-over-the-correspondence-entry`). Both effects are in scope and must be tested; only the schema is out of scope.
- **No `pathspec`/`fabric.yaml` changes.** `_raddle` graduating into `pathspec` is a separate concern (see the misclassification note under Technical context).
- **Slices 5 and 6 are untouched.** `lyx init` dissolution, clone-does-everything, subpath-in-weft, and warp-rebase/remote-reconcile are later slices.

## Decisions

### reader-api-snapshot-warp-sha

- Decision: expose exactly one reader, `func (f *Fabric) SnapshotWarpSHA(tag string) (string, error)`, returning the `Warp-SHA` trailer value of the newest weft commit on the current branch carrying a `Snapshot: <tag>` trailer.
- Rationale: this is precisely what the named consumer (raddle) needs — the host/warp code SHA the snapshot describes, as its staleness baseline. It mirrors the retired `gitrepo.SnapshotSHA(key)` signature 1:1, so the design doc's own framing ("this retires the separate mechanism") reads as a straight substitution.
- Scoping, stated deliberately: the reader is **per-branch**, because it scans the current weft branch's history. A snapshot recorded on another branch reads as absent after a coordinated `Checkout`, and the consumer regenerates from scratch. That is the intended behaviour and the safe failure direction — regenerating unnecessarily costs time, whereas answering with another branch's baseline would report "current" against history this branch never had. It is also the mirror image of why the correspondence index needs `refreshCorrIndexAfterSwitch` (`index.go:250`): that index is a per-worktree *file* that survives a branch switch and can therefore answer cross-branch, so it must be discarded; the reader holds no state and simply stops seeing the other branch's commits. This must be stated in `fabricengine/doc.go`, not left for a consumer to discover.
- Rejected: `SnapshotSHAs(tag) (warpSHA, weftSHA, err)` — no consumer needs the recording weft commit's identity. A struct-returning `Snapshot(tag) (SnapshotRecord, bool, error)` — room to grow that nothing is growing into.

### scan-on-demand-no-index

- Decision: the reader scans weft history on demand via one `git log --format=...` invocation using git's own `%(trailers:key=...,valueonly)` placeholders for both `Warp-SHA` and `Snapshot`, taking the first (newest) record whose snapshot-tag list contains `tag`. No index file, no cache. The scan is implemented by **generalizing the existing `scanWarpSHATrailers`** to capture a third field, not by adding a sibling scanner.
- Rationale: snapshot reads are rare — a staleness check at a phase boundary, not a hot path. An index would add a second cache-invalidation surface (branch switches already force `refreshCorrIndexAfterSwitch` for the correspondence index) for no measured benefit. The trailer remains the sole source of truth, which is the design's stated layering; a cache is explicitly optional in that layering, not required. YAGNI. Generalizing the existing scanner keeps one git-log plumbing site, one copy of the `\x1f`/`\x1e` separator convention, and one copy of the unborn-HEAD tolerance.
- Rejected: extending `corrIndex` with a per-tag latest-snapshot map rebuilt by `RebuildIndex` — literal pattern-match on the correspondence index, but buys cache-invalidation complexity for a caller that does not exist. A separate snapshot index file alongside `fabric-corrindex.json` — same cost plus a second file to keep coherent across branch switches. A **sibling** scan function beside `scanWarpSHATrailers` — duplicates the format string, separators, and unborn-HEAD tolerance. Promoting `parseSnapshotTags` from `trailer_test.go` into production and having the reader parse full `%B` commit messages — this was the working plan until review caught that it creates **dead production code**: the placeholder-based scan never sees a full message, so the promoted function would have no caller, which is exactly what `delete-ref-mechanism-outright` refuses to tolerate for `remoteName`/`isStrictDescendant`. `parseSnapshotTags` therefore stays a test helper.

### miss-reads-as-absent

- Decision: a tag never recorded in the current branch's history reads as `("", nil)`. Not an error.
- Rationale: absent is a normal state, not a failure — matches the retired `gitrepo.SnapshotSHA`'s contract exactly. A first-ever raddle run must be able to read "no baseline, generate everything" without special-casing an error. This is deliberately *unlike* `index.go`'s `ErrNoCorrespondence`, where a miss genuinely signals broken bookkeeping.
- Rejected: a typed `ErrNoSnapshot` wrapped with the tag — symmetric with `ErrNoCorrespondence` and louder, but it forces every caller to `errors.Is` on the ordinary first-run path.

### reader-orders-by-topology-not-commit-date

- Decision: the generalized scan passes `--topo-order` to `git log`, and "newest" means the first record in that topological order. `scanWarpSHATrailers` today (`index.go:193`) passes only `log --format=`, which is git's default reverse-**chronological** order.
- Rationale: commit date is attacker-free but not trustworthy — it is wall-clock from whichever machine made the commit. This discussion's own "cross-clone sharing comes for free" property makes that concrete: snapshot commits arrive from other machines through `gitrepo.Pull`, which can produce a weft merge, so a second machine with a skewed clock can place an **older** baseline first in date order. That would under-report staleness, which `reader-returns-a-dangling-warp-sha-raw` identifies as the one failure direction that actually loses data (the consumer believes it is current and skips a regeneration it needed). `--topo-order` removes clock-dependence entirely: no commit is ever listed before one of its descendants.
- Residual ambiguity, stated rather than papered over: when two snapshot commits for the same tag are on **concurrent** branches merged into the current history, neither is an ancestor of the other and "newest" is a topological choice between incomparable commits. Both are legitimate baselines for the tag; whichever is chosen, the consumer's `ChangedFilesSince` against it reports a superset or equal set of the truly-changed files. Over-reporting is the safe direction, so this is accepted, not solved.
- Effect on the correspondence rebuild, called out because this changes a **shipped** function's git invocation. An earlier draft claimed `RebuildIndex` is "insensitive to the input ordering". That is **wrong**, and the correction matters more than the original claim did. `RebuildIndex` (`index.go:283-314`) is order-sensitive in two distinct ways:
  1. **Dedup by warp SHA is last-assignment-wins over the reversed scan**, so for a warp SHA recorded by more than one weft commit, the winner is whichever commit the **scan listed first**. Changing the scan's order can therefore change which weft SHA a warp SHA resolves to. The code comment at `index.go:283-287` calls that duplicate shape "a rare but legal history shape" — see `empty-commits-take-over-the-correspondence-entry`, which makes it routine and must update that comment.
  2. **`sort.SliceStable` preserves input order among equal `WarpSeq`**, and equal `WarpSeq` is not exotic: it covers both the `seq = 0` dangling sentinel (line 305, assigned to every trailer naming a warp SHA that no longer resolves) and genuine side-branch commits at equal first-parent depth.
  So the intended outcome must be stated rather than assumed: for both cases the **newest commit in topological order wins**, which is the same rule the reader uses, and which is what makes reader and index agree. Topological order is a strict improvement here for the same reason it is for the reader — a back-dated commit can no longer displace a topologically-newer one — but it is a behaviour change to a shipped function, not a no-op, and the plan must treat it as one.
- Rejected: leaving date order and documenting the risk — the failure is silent and lands in the direction that loses data. Adding `--first-parent` as well — it would exclude snapshot commits that arrived through a merge, which is over-reporting in the safe direction but discards a baseline another machine legitimately recorded; `--topo-order` alone fixes the ordering hazard without dropping commits. (Note `warpSeq` does use `--first-parent`, for a different purpose: a stable ordering *key*, not a reachability filter.)

### empty-commits-take-over-the-correspondence-entry

- Decision: an empty snapshot commit calls `RecordCorrespondence(warpSHA, emptyWeftSHA)` like any other trailer-bearing commit, and therefore **upserts over** the entry a preceding content commit wrote for the same warp SHA. `WeftSHAForWarpSHA(X)` and `RevertWithWeft(X)` subsequently resolve to the empty commit. This is accepted, not worked around.
- Rationale: the warp SHA does not move when a tags-only or unchanged-content call fires, so a repeated recording against the same warp SHA is unavoidable once empty commits exist — this slice turns `index.go:283-287`'s "rare but legal history shape" into a routine one, and that comment must be updated to say so. The overwrite is benign for content because **an empty commit's tree is identical to its parent's by construction**: resolving a revert target to the empty commit restores the same weft tree the content commit produced. "Last recorded wins" is also already the index's own documented upsert rule (`corrIndex.record`), so this is the existing semantics meeting a newly-common input, not a new semantics.
- Implementer obligation, because the benign-ness argument is load-bearing: verify it against `RevertWithWeft`'s actual behaviour rather than accepting the tree-identity reasoning. If revert does anything with the target commit beyond its tree (walks parents, reports the SHA to an operator, anchors a later diff), the consequence must be re-examined and recorded, not assumed away.
- Rejected: skipping `RecordCorrespondence` for empty commits — the commit carries a real `Warp-SHA` trailer, so `RebuildIndex` (which reads trailers, not this decision) would record it anyway and the incremental and rebuilt indexes would diverge. Keeping the content commit as the winner by special-casing empty commits in the index — makes the index disagree with a trailer scan, breaking the "trailer is truth, index is a rebuildable cache" layering this whole design rests on.

### reader-matches-tags-byte-exactly

- Decision: the reader compares its `tag` argument to trailer values **byte-exactly** — no trimming beyond the whitespace the trailer parser already strips around the value, no case folding, no normalisation — and does **not** validate the argument against `snapshotTagPattern`. An invalid or misspelled tag simply never matches and reads as absent, per `miss-reads-as-absent`.
- Rationale: the write path is where validation belongs and already is (`validateSnapshotTag`, `trailer.go:48-72`, rejecting the trailer-injection charset). A tag that could not have been written cannot match anything, so validating on read would turn a guaranteed-absent lookup into an error for no additional safety. Byte-exact matching also keeps the reader honest about what is in the commit: fuzzy matching would let `Raddle` silently resolve a baseline recorded as `raddle`, hiding a caller bug behind a convenience.
- Rejected: validating symmetrically with the writer and returning `*ErrInvalidSnapshotTag` — an error path for a lookup whose answer is already known. Case-insensitive or trimmed matching — hides caller typos.

### reader-returns-a-dangling-warp-sha-raw

- Decision: when the newest commit carrying `Snapshot: <tag>` names a `Warp-SHA` that no longer exists (a warp history rewrite removed it), `SnapshotWarpSHA` returns that SHA **raw**, with a nil error. It does not validate, does not skip to an older tagged commit, and does not collapse to `("", nil)`. Staleness surfaces at use, never at read.
- Rationale: this is the same posture the correspondence index already takes and states — `RebuildIndex` records a trailer's warp SHA even when it no longer resolves, and validation happens at use via `f.Warp.SHAExists` (`index.go:268-271`). The trailer is the source of truth; the reader reports what the trailer says. Skipping to the next-newest tagged commit would silently answer with an *older* baseline and thereby **under**-report staleness — the one failure direction that actually loses data. Collapsing to `("", nil)` would conflate "never recorded" with "recorded, then rewritten", destroying the diagnostic difference for no gain, since both already drive the same consumer action.
- Consumer idiom, which must be stated in `fabricengine/doc.go` and in `raddle.md` rather than left implicit: check `f.Warp.SHAExists(sha)` **before** passing the result to `ChangedFilesSince`, and treat a missing SHA as total staleness (regenerate everything). This is not an extra burden invented here — `ChangedFilesSince`'s own doc (`gitrepo.go:430-434`) already says callers are expected to check `SHAExists` first and treat a missing SHA as staleness. The naive composition `f.Warp.ChangedFilesSince(f.SnapshotWarpSHA(tag))`, written earlier in this discussion, is therefore wrong as written: it hard-errors on exactly the rebase case the mechanism exists to survive. The corrected three-step idiom (read → `SHAExists` → `ChangedFilesSince`) is what a consumer writes.
- Rejected: validating inside the reader and returning `ErrStaleSHA` — makes every consumer handle an error for a state that has one obvious response, and diverges from the index's validate-at-use posture. Skipping to the next-newest tagged commit — under-reports staleness. `("", nil)` — lossy.

### delete-ref-mechanism-outright

- Decision: delete `internal/gitrepo/snapshot.go` outright in this slice, along with its whole test and documentation surface. No deprecation window.
- Rationale: `SnapshotSHA`/`SetSnapshotSHA` have **zero production callers** outside `internal/gitrepo` itself (verified by a repo-wide grep of non-test Go source; the only non-gitrepo hits are comments in `cmd/lyx/gitrepoboundary_test.go`). There is nothing to migrate, so a deprecation window would serve nobody while leaving two live mechanisms — exactly what the design says to end. `remoteName` and `isStrictDescendant` are used only by this file's own code paths and die with it; the implementer must confirm that with a grep before deleting rather than assuming it.
- Rejected: keeping the API deprecated — perpetuates the duplication the slice exists to remove. Keeping `remoteName`/`isStrictDescendant` speculatively — dead code with no caller.

### snapshot-tags-always-force-a-weft-commit

- Decision: when `snapshotTags` is non-empty and no weft commit would otherwise land, land an **empty** weft commit carrying the `Warp-SHA` and `Snapshot:` trailers. One rule, applied uniformly to **all four** cases that reach it:
  1. Zero weft-side files — a warp-only `Commit`, or the tags-only shape `Commit(nil, msg, tags, opts)`.
  2. A pathspec that `weftPathspecFilter` reduces to no positive entry (`!positive`, `weftgit.go:368`).
  3. Weft files whose content is unchanged, so `StageAndCommit` reports `committed == false` (`weftgit.go:383`).
  4. `StageAndCommit` failing with git's `"did not match any files"`, which `commitWeftLocked` tolerates (`weftgit.go:374-382`). This is the third early return, reachable only when `weftPathspecFilter`'s own pre-check is bypassed — it is defense-in-depth, and it is semantically identical to case 2 (the pathspec matched nothing), so it must not behave differently. Missing it would leave one uncovered silent-drop path in a rule whose entire value is being exhaustive.
- Decision (failure propagation): when `CommitEmpty` refuses with `ErrIndexNotEmpty`, `commitWeftLocked` returns `("", false, err)` and `commitBothSides` maps it through its existing `err != nil && !weftCommitted` arm to a `*PartialCommitError` with `WeftCommitted: false` — the unlanded-weft outcome it already models. No new error shape, no new branch. This is **accepted as a real behaviour change** and must be recorded in `commitWeftLocked`'s godoc: the `!committed` path at `weftgit.go:383` is a documented silent no-op today, and on a tagged call with a dirty weft index it now becomes an error. The dirtiness in question is not something the combined write lock excludes — the lock serialises fabric's own callers, but an aborted earlier run can leave staged entries behind — so this is reachable, and failing loudly is the point: silently folding someone else's staged work into a snapshot commit is strictly worse than refusing.
- Decision (call shape): **tags with zero files is a supported call shape**, not an accident of the `weftSide` predicate. It is how a caller records a baseline without producing weft content — the standalone-snapshot use the design deferred — and it must be pinned by a test rather than left as emergent behaviour.
- Rationale: this is the single decision that dissolves what looked like three separate problems. The unchanged-content case is a genuine correctness hole with no other fix — raddle regenerates against warp SHA X, output is byte-identical, no weft commit lands, so the baseline never advances and the staleness check reports drift forever despite raddle having just confirmed itself current. That is precisely the "record a baseline without producing weft content" case `fabric-unified-view.md` anticipated. Once an empty commit solves that, the same mechanism absorbs the misuse cases too, with **no validation, no typed error, and no misuse-handling code** — the module simply does what the caller asked. A snapshot's whole meaning is "this warp SHA, recorded under this tag", and an empty weft commit records exactly that, faithfully.
- Accepted cost, recorded rather than left implicit: every no-op tagged run now appends a weft commit that carries no content. A consumer that checks staleness on a tight loop grows weft history without bound, and the cache-free reader scans that history on every call. This is accepted because the actual call pattern is one tagged commit per regeneration at a phase boundary — not a poll — so the growth is on the order of one commit per raddle run, and the scan is a single `git log` over a repo whose history is small by construction. If a consumer ever does poll, the fix is on the consumer (check `ChangedFilesSince` first, tag only when regenerating), not a mechanism change here.
- Rejected: a typed `*ErrSnapshotNotRecorded` for the misuse cases (this was the working proposal until the empty commit subsumed it) — adding a handling mechanism for incorrect use of the module, when correct use is the caller's obligation. Silent drop with documentation — leaves the unchanged-content hole unfixed. Deferring to raddle's own task — ships a reader whose only named consumer cannot use it correctly.

### unborn-warp-keeps-todays-behaviour

- Decision: when warp has no HEAD (`warpHeadSHA` reports `unborn`), snapshot tags are dropped exactly as today — no trailers, no empty commit, no error.
- Rationale: a snapshot's entire content is a warp SHA, and there is none. Committing an empty weft commit carrying `Snapshot:` with no `Warp-SHA` would record a tagged commit with no baseline and force the reader to grow a third "found the tag, has no SHA" state — reintroducing the complexity the empty-commit rule removes. This is a genuine bootstrap-only state (`git init` → `lyx init` → `lyx config`, before the operator's first host commit — see `weftgit_unborn_warp_test.go`'s own header), and no snapshot consumer runs there.
- Sizing correction: an earlier draft claimed this costs "no new code" because the rule "lives inside the existing `if !unborn` arm". That is wrong, and a plan writer must not size the change from it. `weftgit.go`'s `if !unborn` block covers only trailer composition (lines 356-362); all three fall-through points (368, 374-382, 383) sit **outside** it, so each needs its own explicit `!unborn && len(snapshotTags) > 0` guard plus the `CommitEmpty` + `RecordCorrespondence` tail. What is genuinely cheap is the *decision* — `unborn` is already computed and already in scope at every one of those points, so honouring it adds a conjunct, not a new code path. Size the work from the Technical context bullet, not from this rationale.
- Rejected: returning an error — makes a legitimate bootstrap state a failure the caller must handle. Committing with `Snapshot:` and no `Warp-SHA` — a baseline-less snapshot record.

### unborn-weft-lands-an-empty-root-commit

- Decision: when **weft** has no HEAD (born warp, zero weft commits) and the empty-commit rule fires, it lands weft's **root** commit — empty, carrying its `Warp-SHA` + `Snapshot:` trailers. No exception, no special case.
- Rationale: this is a distinct state from unborn *warp* and was missed on the first pass. Weft's root commit is already created this way for ordinary content commits — `StageAndCommit` against an unborn weft HEAD works today and produces the root commit — so an empty root commit is the same operation minus a tree delta. Nothing downstream objects: the commit carries a real `Warp-SHA` (warp is born by definition in this case), so `RecordCorrespondence` records it normally and the reader resolves it normally. Adding an exception here would mean two unrelated carve-outs in a rule whose value is uniformity, to avoid an outcome with no demonstrated harm.
- Rejected: skipping the empty commit when weft is unborn — a second exception beside `unborn-warp-keeps-todays-behaviour`, justified only by an aesthetic objection to an empty root commit. Asserting the state is unreachable — it is not: a freshly created weft branch is exactly this state.
- Consequence: `CommitEmpty`'s unborn-HEAD behaviour is **reachable from `fabricengine`** and must be specified, not left as "pin whatever `git commit --allow-empty` happens to do in a primitive nothing calls".

### skipgit-still-drops-tags

- Decision: `opts.SkipGit == true` continues to produce no commit and therefore no snapshot trailer, regardless of tags. Unchanged from today.
- Rationale: `SkipGit` is an explicit "touch no git at all" opt-out. Honouring it is correct, not a silent failure.
- Rejected: nothing — this was never in question, and is recorded only so a reader of the empty-commit rule does not conclude it overrides `SkipGit`.

### commit-empty-as-a-new-gitrepo-primitive

- Decision: add `func (r *Repo) CommitEmpty(msg string) (sha string, err error)` to `internal/gitrepo`. It first verifies the index carries nothing beyond HEAD, returning a typed `ErrIndexNotEmpty` when it does, and only then runs `git commit --allow-empty -m <msg>`, returning the resulting SHA. Always commits when it commits at all (no `committed bool` — an empty commit cannot no-op).
- Decision (the pre-check, specified concretely so it is not left to the implementer to discover git's behaviour): branch on the **born/unborn** state using the package's existing typed detection — `r.CurrentSHA()` returning `ErrNoCommits` — never by matching git's stderr.
  - **Born HEAD:** `git diff --cached --quiet`, with the exit-code mapping `StageAndCommit` already uses verbatim (`gitrepo.go`'s own `switch`): **0** → index matches HEAD, proceed; **1** → staged differences exist, return `ErrIndexNotEmpty`; **anything else** → a genuine git failure, returned as an error including stderr.
  - **Unborn HEAD:** there is no HEAD to diff against, so do **not** use `diff --cached` here at all. Use `git ls-files --cached`: empty output → proceed; any output → `ErrIndexNotEmpty`. This is exact, needs no empty-tree constant (which is hash-algorithm-dependent and would be a latent SHA-256 bug), and avoids betting the contract on `diff --cached`'s unborn-HEAD semantics.
  This matters because the unborn path is **reachable in production**, not hypothetical: `unborn-weft-lands-an-empty-root-commit` routes straight through it.
- Rationale: neither existing primitive can be coaxed into an empty commit. `StageAndCommit` returns `("", false, nil)` on an empty file list by deliberate design, and its `diff --cached --quiet` gate returns the same no-op on unchanged content. A single-purpose method reads clearly at its one call site and cannot be misread as a variant of `StageAndCommit`. The index pre-check exists because a bare `git commit --allow-empty` is **not** empty by construction — it commits whatever is in the index, which would silently sweep up a half-staged edit. `StageAndCommit`'s own doc (`gitrepo.go:133-153`) establishes never-sweeping as a package norm and enforces it by scoping the commit to an explicit pathspec; `CommitEmpty` has no pathspec to scope with, so it enforces the same intent by refusing instead. Refusing loudly beats both silently sweeping and relying on `--only`'s no-pathspec behaviour, which is not something to bet a contract on.
- Stated limitation, so the contract does not over-claim: this is a **weaker** guarantee than `StageAndCommit`'s, not "the same norm". `StageAndCommit` is structurally safe — `git commit … -- <files>` (`gitrepo.go:216`) cannot commit an unlisted path no matter what races it. `CommitEmpty` is check-then-commit across **two** git spawns, so an index write landing in the window between them is still swept. Closing that window is not possible without a pathspec to scope by, and the residual risk is small (fabric's own callers serialise under the combined write lock; the window is milliseconds; the writer would have to be an out-of-band process in the weft worktree). It is accepted — but `CommitEmpty`'s godoc must say "refuses if the index is dirty when checked", not "cannot sweep the index", so a future reader does not build on a guarantee that is not there.
- Rejected: bare `git commit --allow-empty` with the never-sweep property simply dropped — this was the first-pass decision, and it contradicted a test this discussion itself demanded; either the guard or the requirement had to go, and the guard is two lines. `git commit --allow-empty --only -m <msg> --` — depends on `--only`-with-no-paths semantics rather than an explicit check. `StageAndCommit(msg, files, allowEmpty bool)` — changes every existing call site and puts a boolean flag on the package's most-used method.
- Note for the implementer: the pre-check is a second `r.run` call inside `CommitEmpty`, which is already CLI-bound and on the pinned run-bound list, so it adds no new boundary surface.

### commit-result-reports-the-empty-commit-plainly

- Decision: a warp-only `Commit` carrying snapshot tags returns `WeftCommitted: true` with `WeftSHA` set, where today it returns `false`. No new field.
- Rationale: it is literally true — a weft commit landed. `Commit`'s own async-push gate (`result.WarpCommitted || result.WeftCommitted`) then correctly pushes both sides, which is what should happen: the snapshot trailer must reach the remote for cross-clone sharing, the property the retired ref mechanism achieved by pushing its ref.
- Rejected: a distinguishing `WeftEmpty bool` — no caller needs the distinction. YAGNI.

## Technical context

**The write half already exists.** Slice 2 landed it. `internal/fabricengine/trailer.go` already has `SnapshotTrailerKey = "Snapshot"`, `snapshotTagPattern` (`^[A-Za-z0-9._-]+$`, deliberately excluding newline/CR/colon to close the trailer-injection vector), `ErrInvalidSnapshotTag`, `validateSnapshotTag`, and `appendSnapshotTrailers` (validate-all-before-appending-any, so a single bad tag fails the call with nothing written). `Fabric.Commit(files, msg, snapshotTags []string, opts)` and `Fabric.CommitWeft(pathspec, message, opts, snapshotTags ...string)` already thread tags to `commitWeftLocked`, which appends them after the `Warp-SHA` trailer. **This slice adds no new write-side format work** — only the reader, the empty-commit rule, and the retirement.

**Key files:**

- `internal/fabricengine/trailer.go` — trailer format/parse. `parseWarpSHATrailer` is the shape to mirror. `parseSnapshotTags` stays where it is, in `trailer_test.go:131`, as a test helper — it is **not** promoted (see `scan-on-demand-no-index`'s rejected list).
- `internal/fabricengine/index.go` — `scanWarpSHATrailers` (line 188) is **generalized**, not copied: `git log --format=` with `%H` + `%(trailers:key=Warp-SHA,valueonly)` gains a third field `%(trailers:key=Snapshot,valueonly)` **and the `--topo-order` flag** (see `reader-orders-by-topology-not-commit-date`; the call currently passes no ordering flag at all, so this is a deliberate change to a shipped invocation), keeping the existing `\x1f` (unit) and `\x1e` (record) control-character separators and the unborn-weft-HEAD tolerance (matching `"does not have any commits yet"` in stderr and returning no commits). `warpSHATrailerCommit` gains a `snapshotTags []string` field. Two consumer-facing consequences the implementer must preserve: (a) `RebuildIndex` must keep ignoring the new field entirely — snapshot tags never enter `corrEntry`; (b) the existing behaviour of **skipping commits with no `Warp-SHA`** must stay, since a snapshot record without a baseline is not usable by the reader either. A commit with multiple `Snapshot:` trailers yields a **multi-line** value for that placeholder, so the record parser must split on newline within the field rather than assume one value. Extracting that record-splitting into a pure, git-free helper keeps it testable in Tier 1 (see Testing).
- `internal/fabricengine/commit.go:99-102` — `weftSide := len(weftFiles) > 0 && !opts.SkipGit` becomes `(len(weftFiles) > 0 || len(snapshotTags) > 0) && !opts.SkipGit`. Nothing else in `commitBothSides` needs to change: its three-outcome `*PartialCommitError` mapping already handles whatever `commitWeftLocked` returns. Note the knock-on: a tags-only or warp-only tagged call now sets `committing == true`, so it takes the combined write lock and runs `ensureWeftLockDir` where it previously did neither — which is correct (it is about to write to weft), but it makes `Commit`'s own godoc wrong (see the godoc list below).
- `internal/fabricengine/weftgit.go:349-400` — `commitWeftLocked`. **Three** early returns must fall through to the empty commit when tags are present and `!unborn`: `if !positive` (line 368, after `weftPathspecFilter`), the `"did not match any files"` tolerance inside the `StageAndCommit` error branch (lines 374-382), and `if !committed` (line 383). The third one is easy to miss — it sits inside an error branch rather than reading as a guard clause. The existing `RecordCorrespondence(warpSHA, sha)` call at line 395 must also run for the empty commit — it carries a `Warp-SHA` trailer like any other, so the correspondence index must record it or `RebuildIndex` and the incremental path would diverge.
- `internal/gitrepo/gitrepo.go` — `StageAndCommit` (the primitive `CommitEmpty` sits beside), `CurrentSHA` (line 114), `ChangedFilesSince` (line 449).
- `internal/gitrepo/snapshot.go` — the whole file is deleted.

**Deletion footprint.** Production code: `internal/gitrepo/snapshot.go` in full.

**Documentation and comments — swept mechanically, not from a list.** Six review rounds each found comment sites the previous round's "exhaustive" enumeration had missed. That is evidence about the method, not about the reviewers: a case-insensitive `snapshot` grep finds **38 mentions in `internal/gitrepo`'s non-test source alone** (doc.go 28, gogit.go 7, gitrepo.go 2, push.go 1). Enumerating those by line in a design document does not converge, and claiming the list is exhaustive gives false confidence. The instruction to the implementer is therefore the grep, not the list:

> Run `grep -rin snapshot internal/gitrepo/ cmd/lyx/` across production **and** test source, and resolve every hit — delete it, or rewrite it against a surviving method. Nothing may be left naming `SnapshotSHA`, `SetSnapshotSHA`, `refs/loomyard/snapshot/`, or `validSnapshotKey` as live. Then repeat with separate greps for `remoteName` and `isStrictDescendant`, which carry no "snapshot" substring and are therefore invisible to the first pass.

The list below is a **starting point plus worked examples of the cases that are easy to miss** — explicitly not a complete inventory:

- `internal/gitrepo/doc.go` — the whole "# Snapshot remote model" section (~172-206) **and** the "# The self-correcting snapshot pattern" section including its heading at line 70; plus the package summary at line 6 ("snapshot tracking" listed as a capability), line 100 ("snapshot/correspondence tracking"), line 269 (the `PlainOpen` worked example, "reports every existing `refs/loomyard/snapshot/*` key as absent"), and mentions at 17-18, 23, 39, 59, 65, 72-74, 141, 253-255.
- `internal/gitrepo/gitrepo.go:28` (`ErrInvalidSHA`'s doc naming `SetSnapshotSHA`) and **`gitrepo.go:429`** — `ChangedFilesSince`'s own godoc, "matching the snapshot model's SHA-to-SHA determinism". This is precisely why the grep is mandatory: `ChangedFilesSince` is otherwise untouched by this slice, so the "re-read the godoc of every function you touch" obligation never reaches it, yet its comment dangles the moment doc.go's snapshot section is deleted.
- `internal/gitrepo/gogit.go:82` (locking-discipline ref-read bullet, "CurrentBranch, `remoteName`, and both snapshot ref reads" — drop both names), `gogit.go:88` (the object-lookup bullet in the same paragraph, naming **`isStrictDescendant`** and `SetSnapshotSHA`'s `^{commit}` canonicalization — drop both, keeping `SHAExists`, `ChangedFilesSince`, and `hasUnpushed`, which all survive), plus `gogit.go:89, 108-109, 184`.
- `internal/gitrepo/push.go:59`.
- `internal/gitrepo/parity_test.go:591` — the doc comment on the **surviving** `TestStageAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit`, which motivates the test by "a wrong SHA straight into a caller's `SetSnapshotSHA`". The test stays; its rationale needs rephrasing against a surviving consumer.
- `internal/gitrepo/gogit_test.go`'s **surviving** `gogitLinkedFixture` (89-139): `commonSnapshotRef = "refs/loomyard/snapshot/gogittest"` (109), its `update-ref` seeding (127), its read-back (180), and the comments at 94-95, 102-103, 107-108. This fixture **compiles fine** after the cut — it uses raw git and a generic `handle.Reference()` call, never the deleted methods — so nothing forces the change. Rename the constant and rewrite the comments anyway: the ref name is arbitrary to the test's purpose (any ref living in the common dir proves the shared-common-dir read), and leaving it frames a retired namespace as live.

Tests — this is where the first pass was wrong, and the boundary matters because two of these files hold **surviving** coverage that must not be deleted along with the snapshot cases:

- `internal/gitrepo/snapshot_test.go` — deleted in full.
- `internal/gitrepo/keyvalidation_test.go` — **kept**, with `TestValidSnapshotKey` (line 11) removed and `TestValidSHA` (line 41) retained. `validSHA` survives and is used by `ResetHard`, `ChangedFilesSince`, and `CheckoutDetached`; deleting the file wholesale would silently drop that table. Its header comment ("covers validSnapshotKey and validSHA") needs updating too.
- `internal/gitrepo/parity_test.go` — remove `newSnapshotRefFixture` (~70), `writeSnapshotRef` (~100), `TestSnapshotSHA_Parity_SetRef` (402), `_AbsentRef` (420), `_InvalidKey` (450), `TestSnapshotSHA_Parity_UnreadableStore` (471), `TestSnapshotSHA_MixedBackend_ReadsRefCLISideAdvanceWrote` (711), and `TestSetSnapshotSHA_MixedBackend_RepackBetweenCommitAndCanonicalization` (753). Everything else stays — in particular `forcePackIndexFreeze` (569) and its three other call sites.
- `internal/gitrepo/oracle_test.go` — remove `oracleSnapshotSHA` (~132).
- `internal/gitrepo/gogit_test.go` — remove `TestRemoteName_Parity` (415), `TestIsStrictDescendant_Parity` (456), `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead` (904), and the `freezePackIndex` helper (887, used only by that last test). **Also**, and this is the compile-breaking one: the linked-worktree parity harness `runLinkedWorktreeParityChecks` (717, driven by `TestLinkedWorktree_Parity` at 847 and run twice — direct and via junction) calls all three doomed methods inline — `remoteName()` at 758, `SnapshotSHA(gogitParitySnapshotKey)` at 772, and `isStrictDescendant(...)` at 829. Those three subtests must be removed from the harness, along with the helpers and fixture seeding only they need: `oracleRemoteName` (376), `oracleIsStrictDescendant` (400), `oracleSnapshotSHA` (597), the `gogitParitySnapshotKey` constant (630) together with its own doc comment above it (626-629), the `git update-ref refs/loomyard/snapshot/...` seeding inside `newLinkedParityFixture` (684), and the harness doc comment at 710-716 that enumerates the checks. Two fixture comments go stale with the same cut and must be swept: `linkedParityFixture`'s type doc (632-639, "plus `remoteName`, `hasUnpushed`, and `isStrictDescendant`" — `hasUnpushed` survives, the other two do not) and its `sharedSHA` field comment (643-646, "the value `refs/loomyard/snapshot/<key>` is set to from the main worktree" — the field survives, that clause does not). Miss any of these and `internal/gitrepo` does not compile after `snapshot.go` is deleted — the test package references methods that no longer exist.

**Two coverage judgements in that list, made explicitly rather than by omission:**

- `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead` is the hard fingerprint-gated-reindex case (frozen go-git pack index, new commit, `git gc` repack before the read). Losing it is acceptable because `parity_test.go:652`'s `TestSHAExists_MixedBackend_RepackBetweenCommitAndRead` is the **same scenario** anchored on `SHAExists`, a surviving method, with its own independent `forcePackIndexFreeze` helper — verified by reading both. The gate keeps a live witness; what is lost is a second witness for one gate, not unique coverage. Do not re-anchor it onto another method just to keep the count up.
- The **linked-worktree/junction parity harness loses three of its read-side cases** (`remoteName`, `SnapshotSHA`, `isStrictDescendant`), which is a larger proportional cut than the repack case. Accepted, because what that harness exists to prove is that a `Repo` opened on a linked worktree or through a junction resolves the *shared* common-dir state rather than the per-worktree gitdir — a property of `goGit`'s handle resolution, not of any one method. The surviving checks in the same harness still exercise that property across both entry paths. The deletion narrows the method sample, not the property under test. If a later reader wants the sample widened again, the right move is adding a surviving-method case to the harness, not resurrecting the snapshot ref.

**Guard tests that will fail unless updated in the same commit:**

- `cmd/lyx/gitrepoboundary_test.go` — `gitrepoPinnedRunBoundMethods` is asserted by **set equality**, so removing `"SnapshotSHA"`, `"advanceAndPushSnapshotRef"`, and `"adoptSnapshotRef"` is mandatory, and adding `"CommitEmpty"` is mandatory. **Two** comments in this file use `SnapshotSHA` as their worked example, not one: the file header's "# The one blind spot this guard cannot see" (lines 9-19) and the explanatory comment above the map (lines 38-49, which also names `Push`, `PushCoalesced`, and `SetSnapshotSHA`). Both must be rewritten. The replacement example exists and has been verified: **`StageAndCommit`** is a surviving mixed method — three `r.run` calls (add, diff, commit) followed by a migrated go-git `CurrentSHA` read (`gitrepo.go:114`, fully on go-git) — so the blind spot it illustrates is unchanged in kind. `CommitEmpty` will be a second such method. `gitrepoBoundaryMinScannedFiles` documents "7 non-test .go files today (…, snapshot.go)" — that enumeration becomes 6 files; the floor of 5 still holds, but the comment must be corrected.
- `CONSTRAINTS.md`, **gitrepo Client Boundary Invariant** — its CLI-bound set is described as named "exhaustively here, not just in the package doc, because this entry is what a reviewer checks a new call against", and the entry states that any new `gitexec` call inside `internal/gitrepo` must come with an updated entry in the same commit. So: remove `SetSnapshotSHA`'s push and `SnapshotSHA`'s fetch, add `CommitEmpty`, and rewrite the **Known blind spot** bullet, whose only worked example is `SnapshotSHA`'s legitimate mixing of a migrated read with a CLI-bound fetch. A replacement example must be chosen from a surviving pinned method.

**How a caller can end up with snapshot tags and zero weft files** (the analysis behind `snapshot-tags-always-force-a-weft-commit`): `Commit` classifies paths against `WiredNames(f.weftPath)`, which reads `fabric.yaml`'s `pathspec` — today `_lyx` and `_pattern`. Slice 1's record notes `_raddle` is **reserved-only and not yet in `pathspec`**. So raddle calling `Commit(["_raddle/Overview.md"], msg, ["raddle"], opts)` before `_raddle` graduates classifies its output as **warp-side**, yielding zero weft files. The call site looks correct; the config is what is wrong. This route is why silent-drop was unacceptable — it fails at a distance and its only symptom is a snapshot that never appears. The empty-commit rule makes the snapshot land regardless; the misplaced *content* is a separate config bug this slice does not attempt to catch (a warp-side file is a legitimate thing to commit).

**Cross-clone sharing comes for free.** The retired ref mechanism pushed `refs/loomyard/snapshot/*` to the remote so state was shared across clones. Trailers live in weft's own commit history, which is already pushed by the existing detached both-sides push — no equivalent machinery is needed.

**Godoc comments that become factually wrong on this commit** (same-commit obligation, easy to miss because they are not "docs" in the file-list sense):

- `Fabric.Commit` (`commit.go:69-92`) — "A fully degenerate no-op call (nothing on either side) takes no lock, runs no `ensureWeftLockDir`, and spawns no push" is falsified by a tags-only or warp-only tagged call, which now takes the lock and pushes.
- `commitWeftLocked` (`weftgit.go:317-348`) — describes all three early returns as unconditional no-ops, and states the unborn-warp arm lands "no Snapshot tags (there is no trailer block to append them to)". Both need the empty-commit rule folded in.
- `CommitWeft` (`weftgit.go:402-410`) — "The trailing snapshotTags variadic lets a caller (a later batch's two-sided `Fabric.Commit`) attach one or more `Snapshot:` trailers" now understates what tags do; it inherits the empty-commit behaviour and should say so.
- `PartialCommitError` (`commit.go:30-41` godoc and `commit.go:54`'s `Error()` string) — both open with "landed a warp commit", which is false for the newly first-class tags-only shape: with zero warp files the `err != nil && !weftCommitted` arm produces `"warp commit  landed, weft commit failed: …"` with an **empty** `WarpSHA` interpolated. Reword both so the no-warp-commit case reads correctly. This is not a pre-existing wart to accept — the tags-only shape is new in this slice, and it is the shape that exposes the wording.

These three are the ones provably wrong today. The general obligation stands beyond the list: the implementer must re-read the godoc of **every** function it touches for the same class of staleness, since a comment describing an early return or a no-op is exactly what this change invalidates. The obligation explicitly covers **test-fixture type and field docs**, not just production godoc — the `linkedParityFixture` comments above are a worked example of that class, and they are the easiest kind to miss because nothing fails to compile when they go stale.

**Docs to update** (Documentation Lifecycle, same commit):

- `internal/fabricengine/doc.go` — new section on the snapshot trailer: the tag format and its injection-vector rationale, the read path (newest-tagged-commit-wins, scan-not-cache, miss-is-absent, **per-branch scoping**, and a dangling `Warp-SHA` returned raw with the three-step consumer idiom spelled out), the empty-commit rule and its four triggering cases, the supported tags-only call shape, the `ErrIndexNotEmpty` propagation as an unlanded-weft `*PartialCommitError`, the unborn-warp exception and the contrasting unborn-weft root-commit behaviour, the accepted empty-commit accumulation cost, and `SkipGit`.
- `internal/gitrepo/doc.go` — delete the "# Snapshot remote model" section and every scattered mention listed above.
- `CONSTRAINTS.md` — as detailed above. No *new* invariant is introduced by this slice.
- `manifest/designs/raddle.md` — lines 27, 35, 36, and 50 reference the `SetSnapshotSHA`/`SnapshotSHA` ref API and go actively wrong on this commit. Rewrite to the trailer API: raddle's regeneration records its baseline by passing `snapshotTags=["raddle"]` to `Fabric.Commit`, and the staleness check becomes `f.Warp.ChangedFilesSince(f.SnapshotWarpSHA("raddle"))`. The staleness check must be written as the **three-step** idiom (`SnapshotWarpSHA` → `SHAExists` → `ChangedFilesSince`), not the two-step composition, so a post-rebase raddle regenerates instead of hard-erroring — see `reader-returns-a-dangling-warp-sha-raw`. Preserve the existing correct point that the recorded SHA is the **host/warp** SHA raddle describes, not raddle's own weft commit SHA — that is exactly what the `Warp-SHA` trailer holds, so the trailer form satisfies it naturally. Also worth recording there: raddle's "regenerated but unchanged" case is what motivated the empty-commit rule.
- `manifest/designs/fabric-unified-view.md` — mark slice 4 **DONE** in the Build order with the shipped scope, in the same style as slices 1-3. The prose at lines 63-67 ("Snapshot-tracking folds into the `Warp-SHA` trailer mechanism") stays as the durable rationale, with the empty-commit rule and the unborn-warp exception added, since neither was anticipated there. **One clause on line 67 is a required correction, not an addition:** it currently reads that a standalone no-commit snapshot call is warranted only if a consumer must record a baseline without producing weft content — "which raddle/trace (both commit their output) never do; leave it out until a real caller appears". That is exactly the raddle regenerated-but-unchanged case this slice's central decision exists to fix, and the supported tags-only call shape now serves it. Left as-is, the design doc would keep asserting that the case the slice was built for cannot arise. Rewrite it to record that the case *does* arise, and that it is served by tags-on-`Commit` (with an empty commit when there is no content) rather than by a separate method — which preserves the paragraph's actual conclusion, that no standalone `Fabric.Snapshot` is needed. Note that this file's own header says the durable parts fold into `internal/fabricengine`'s package doc when the *whole* item lands and the file is deleted — that is slice 6, not now, so the file stays.
- `manifest/roadmap.md:74` — **required edit.** The `## Done` section's `gitrepo` entry is a present-tense module inventory ("generic, repo-agnostic git primitives (`StageAndCommit`, `Push`, `PushCoalesced`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/`SetSnapshotSHA`)") and goes factually wrong on this commit. Drop the two snapshot names. This is **not** a roadmap status movement — CLAUDE.md's "roadmap moves only on completing or adding a planned item" rule governs status, and is a different question from correcting a stale API enumeration inside an existing entry. Lines 62 and 64 also name the ref API, but as historical descriptions of what the `git-native-library` spike and the `native clients` migration covered *at the time*; those are accurate history and **stay as written**.
- `crucible/gitrepo-review-prompt.md:21` — **required edit**, one word: the "What to read" list names `snapshot.go` among the files a re-run of that module review must read. That is a live instruction, and it breaks on this commit. The findings history further down the same file (F1, F3, R1, F-R3-2, all referencing `SnapshotSHA`/`SetSnapshotSHA`) is a **frozen round record and stays stale by design** — it documents what past rounds found, not what the code is.
- `crucible/fabric-review-prompt.md:23` — **required edit**, same class: its "What to read" list describes `internal/gitrepo/**` as fabric's git operator, enumerating "`Repo`, `StageAndCommit`, `Push`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/`SetSnapshotSHA`". Drop the two snapshot names. Same live-instruction-vs-frozen-findings split applies to the rest of that file.
- `docs/overview.md` — checked: its two `gitrepo` references (lines 144, 190) are one-line module descriptions that do not enumerate `SnapshotSHA`, so **no change needed**. `docs/shared-libs/README.md` — checked, no `SnapshotSHA` mention.

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
- **Never sweeping the index is a package norm, not a local preference.** `StageAndCommit` enforces it *structurally*, by scoping its commit to an explicit pathspec so an automated commit "can never sweep up a half-staged edit someone else left in the index" (`gitrepo.go:133-153`). `CommitEmpty` has no pathspec, so it can only approximate that with a check-then-commit pre-check that refuses — a weaker, race-window-bearing guarantee that must be documented as such. See `commit-empty-as-a-new-gitrepo-primitive`.

## Testing

**`internal/fabricengine` — untagged Tier-1 unit tests** (no git spawn):

- The **record parser** extracted from the generalized `scanWarpSHATrailers` — a pure function from one `\x1e`-delimited record to `(weftSHA, warpSHA, snapshotTags)`. Cases: no snapshot field; one tag; several tags (the multi-line value); a record with a `Snapshot` value but no `Warp-SHA` (must be skipped, per the scan's existing rule); an empty record; surrounding-newline trimming, which the existing scan already has to do.
- TDD candidate: **the multi-value split.** `%(trailers:key=Snapshot,valueonly)` returns one value per line for a commit with several tags, and the parser must split within the field rather than treat it as one string. Write the failing case first — this is the single most likely implementation slip in the scan change.
- `trailer_test.go` is otherwise unchanged. `parseSnapshotTags` stays a test-local helper there; existing tests that use it keep working as-is.

**`internal/fabricengine` — integration tests** (new `snapshot_integration_test.go`, `//go:build integration`, reusing `index_integration_test.go`'s `newFabric`/`currentSHA`/`commitWarp` helpers, which share the package):

- Newest-tagged-commit wins: three weft commits tagged `raddle` at three different warp SHAs; the reader returns the newest one's `Warp-SHA`.
- Tag isolation: commits tagged `raddle` and `trace` interleaved; each tag resolves to its own newest commit, not the other's.
- Multiple tags on one commit: a commit tagged both `raddle` and `trace` resolves correctly for each.
- Miss: a tag never recorded returns `("", nil)` — not an error. TDD candidate; this pins `miss-reads-as-absent`.
- Unborn weft HEAD (zero weft commits) returns `("", nil)`, exercising the `"does not have any commits yet"` tolerance `scanWarpSHATrailers` already has.
- Untagged weft commits (history predating the tag, or plain `CommitWeft` calls) are skipped without error.
- A commit carrying a `Snapshot:` trailer but **no** `Warp-SHA` trailer is skipped, not returned as an empty baseline — pins that the generalized scan keeps its existing skip rule.
- `RebuildIndex` still produces the same correspondence index after the scan is generalized — pins that snapshot tags never leak into `corrEntry`. The existing `entries()` comparison helper covers the assertion.
- **A history where topological and date order genuinely differ** — this is the one test that can witness the ordering change, and none of the existing or otherwise-listed cases can. `TestRebuildIndex_EqualsIncrementallyBuiltIndex` (`syncweft_integration_test.go:117`) is three linear `SyncWeft` rounds, and every reader case listed above ("newest wins", tag isolation) is linear too — shapes where date order and topological order coincide, so they would pass identically before and after the flag and prove nothing. Build the discriminating fixture: a weft side branch merged back, carrying a snapshot commit whose **committer date is back-dated** behind a topologically-older commit on the mainline (settable via `GIT_COMMITTER_DATE` in the fixture). Then assert both (a) `SnapshotWarpSHA(tag)` returns the topologically-newest baseline, not the date-newest one, and (b) `RebuildIndex` agrees with the incremental index on that same history. Without this fixture, `--topo-order` is an unverified change to a shipped invocation.
- **A warp SHA recorded by two weft commits** — a content commit followed by a tags-only commit at the same warp HEAD. Assert `WeftSHAForWarpSHA(X)` resolves to the empty commit (last recorded wins), that `RebuildIndex` produces the same answer as the incremental path, and that the weft tree obtained via that entry is identical to the content commit's. Pins `empty-commits-take-over-the-correspondence-entry`, which turns a shape the code calls "rare but legal" into a routine one.
- **Per-branch scoping**: record a tag on one weft branch, run a coordinated `Checkout` to another, then assert `SnapshotWarpSHA(tag)` returns `("", nil)` and does not answer cross-branch. Pins the scoping decision in `reader-api-snapshot-warp-sha`, which is otherwise the one explicitly-chosen and documented contract with no test behind it.
- **Byte-exact tag matching**: a tag recorded as `raddle` is not resolved by `Raddle` or `raddle ` — both read as absent, and neither errors. Pins `reader-matches-tags-byte-exactly`.

**`internal/fabricengine` — empty-commit behaviour** (integration; extends `commit_integration_test.go`, which already carries the seam `spawnDetachedPushFn` swap pattern — a recorder/no-op with a deferred restore and no `t.Parallel()`):

- **`TestCommit_WarpOnly_SnapshotTagsDropped` is inverted**, not deleted. Same setup; the assertion flips from "no `Snapshot:` trailer anywhere" to "an empty weft commit landed carrying `Warp-SHA` + `Snapshot: <tag>`". Its doc comment must be rewritten to state the new rule; leaving the old name in place would be actively misleading. TDD candidate, and the clearest single expression of `snapshot-tags-always-force-a-weft-commit`.
- Unchanged weft content + tags: commit the same weft content twice with the same tag; the second call lands an empty commit and `SnapshotWarpSHA(tag)` advances to the newer warp SHA. TDD candidate — this is the correctness hole the rule exists to close, so it should fail before the change and pass after.
- Pathspec filtering to nothing (`!positive`) + tags: an empty commit still lands.
- The `"did not match any files"` tolerance path + tags: an empty commit still lands. This path is normally shielded by `weftPathspecFilter`, so the test must construct the case that reaches `StageAndCommit`'s failure directly — if it cannot be reached without contrivance, say so in the plan rather than quietly dropping the case.
- **Tags-only call** — `Commit(nil, msg, []string{"raddle"}, opts)`: an empty weft commit lands, `SnapshotWarpSHA` resolves to warp's current HEAD, and the combined write lock was taken. Pins the supported call shape.
- Unborn **weft** HEAD + tags: the empty commit becomes weft's root commit, carrying its trailers, and `SnapshotWarpSHA` resolves against it. Pins `unborn-weft-lands-an-empty-root-commit`.
- No tags + nothing to commit: still a clean no-op — no empty commit, no lock churn beyond today's. Guards against the rule over-firing.
- `opts.SkipGit` + tags: no commit, no trailer, no error.
- `CommitResult` shape on a warp-only tagged commit: `WeftCommitted == true`, `WeftSHA` non-empty, and the push seam observed exactly one spawn.
- `RecordCorrespondence` ran for the empty commit: `WeftSHAForWarpSHA(warpSHA)` resolves to the empty commit's SHA, and a subsequent `RebuildIndex` produces an equivalent index (the existing `entries()` comparison helper covers this).
- Invalid tag + otherwise-empty commit: `*ErrInvalidSnapshotTag` returned and **nothing** committed on either side.
- **Dirty weft index + tags on the `!committed` path**: `CommitEmpty` refuses, and `Fabric.Commit` surfaces a `*PartialCommitError` with `WeftCommitted == false` while the warp commit stands. Pins the accepted no-op-becomes-error transition.
- **Dangling `Warp-SHA`**: record a snapshot, rewrite warp history so the recorded SHA no longer resolves, then read. `SnapshotWarpSHA` returns the dangling SHA raw with a nil error, and `f.Warp.SHAExists` on it reports false — pins `reader-returns-a-dangling-warp-sha-raw` and demonstrates the three-step consumer idiom.
- **Via `CommitWeft` directly**, not only via `Fabric.Commit`: `CommitWeft(pathspec-matching-nothing, msg, opts, "raddle")` lands the empty commit. The rule lives in `commitWeftLocked`, and `CommitWeft` (`weftgit.go:411`) is an exported entry point that inherits it and gets a godoc update saying so — an exported contract with no test is exactly the kind that drifts.

**`internal/gitrepo` — `CommitEmpty`** (integration-tagged, alongside the package's existing git-spawning tests):

- Commits on a repo with an existing HEAD; returns a SHA that `SHAExists` confirms and whose tree equals its parent's.
- Two successive calls produce two distinct SHAs.
- **Unborn HEAD: creates the root commit, empty.** A specified contract, not incidental behaviour — `fabricengine` reaches it via `unborn-weft-lands-an-empty-root-commit`.
- **Unborn HEAD with a staged file: `ErrIndexNotEmpty`**, exercising the `git ls-files --cached` branch of the pre-check. This case is what proves the pre-check was specified for both states rather than only the born one.
- Nothing incidentally staged is swallowed on a **born** HEAD either: with a staged-but-uncommitted file present, `CommitEmpty` returns `ErrIndexNotEmpty` (checkable via `errors.Is`) and commits nothing.

**Deletion coverage.** Removing `snapshot_test.go` and the snapshot cases from the parity/oracle files is not a coverage regression — the deleted production code has no callers. Two qualifications, both from review:

- `keyvalidation_test.go` is **kept**, not deleted: `TestValidSHA` covers surviving production (`validSHA`, used by `ResetHard`/`ChangedFilesSince`/`CheckoutDetached`).
- The lost `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead` is redundant with `parity_test.go:652`, which pins the same fingerprint-gated-reindex gate on `SHAExists`. Verified by reading both; recorded here so a later reader does not mistake the deletion for an unnoticed coverage hole.

The real regression guard is that `cmd/lyx/gitrepoboundary_test.go` and `CONSTRAINTS.md` are updated in the same commit; the boundary test's set-equality assertion fails loudly otherwise, which is the intended forcing function.

**Full-suite requirement:** run both `go test ./...` and `go test -tags integration ./...`. The boundary guard lives in `cmd/lyx` and the trailer tests in `internal/fabricengine`, so a package-scoped run will not catch the guard failures this slice deliberately provokes.

## Q&A log

- **Q:** Reader API shape? **A:** `SnapshotWarpSHA(tag string) (string, error)` — mirrors the retired `SnapshotSHA(key)` 1:1; rejected the weft-SHA-returning and struct-returning variants as unearned.
- **Q:** Scan on demand or build an index cache? **A:** Scan on demand, no index. Reads are rare; an index adds a second cache-invalidation surface for a caller that does not exist yet.
- **Q:** What does a never-recorded tag read as? **A:** `("", nil)` — absent is a normal state, matching the retired contract, so a first-ever run needs no error special-casing.
- **Q:** Delete `refs/loomyard/snapshot/` now or deprecate? **A:** Delete outright — zero production consumers, so nothing to migrate and no deprecation window to serve.
- **Q:** Snapshot tags on a commit that produces no weft commit — silent drop, error, or report in the result? **A:** None of those. The user identified that passing tags with no weft files is caller misuse, and rejected adding a handling mechanism for incorrect use of a module on principle: *"Jeg er veldig liten fan av å legge inn håndteringsmekanismer som skal håndtere at en bruker bruker modulen FEIL. Det er brukeren som skal bruke modulen riktig."* The user then observed that the empty commit chosen for the unchanged-content case solves this one too. It does, and it removes the error path entirely — hence `snapshot-tags-always-force-a-weft-commit`.
- **Q:** How can a caller actually end up with tags and zero weft files? **A:** Via `pathspec` misclassification — `_raddle` is not yet in `fabric.yaml`'s `pathspec`, so raddle's own output would classify warp-side. The call site looks correct and the config is wrong, which is why silent drop was unacceptable: it fails at a distance with no symptom but a missing snapshot.
- **Q:** Unborn warp under the empty-commit rule? **A:** Keep today's behaviour — drop the tags. A snapshot with no warp SHA has no content, and the case is bootstrap-only (`git init` before the first host commit). The user flagged it as a marginal special case. ⚠️ **Superseded in part (review r2):** the original claim that this "needs no new code, since the rule lives inside the existing `if !unborn` arm" is **wrong** — that arm covers only trailer composition, and each of the three fall-through points needs its own guard. Size the change from `unborn-warp-keeps-todays-behaviour`'s sizing-correction bullet, not from this line.
- **Q:** New `gitrepo` primitive for the empty commit? **A:** `CommitEmpty(msg) (sha, err)` — single-purpose. Rejected an `allowEmpty bool` parameter on `StageAndCommit`, which would touch every existing call site.
- **Q:** How does `CommitResult` report an empty snapshot commit? **A:** Plainly — `WeftCommitted: true`, `WeftSHA` set. It is true, and it makes the existing async-push gate push the trailer to the remote, which is what cross-clone sharing needs.
- **Q:** Ship a staleness helper alongside the reader? **A:** No; no live consumer exists to shape the signature. ⚠️ **Superseded in part (review r3):** the two-step composition `ChangedFilesSince(SnapshotWarpSHA(tag))` originally given here is **wrong** — it hard-errors on a dangling warp SHA. The correct idiom is three steps: `SnapshotWarpSHA` → `SHAExists` → `ChangedFilesSince`. See `reader-returns-a-dangling-warp-sha-raw`.
- **Q:** Documentation scope? **A:** `fabricengine/doc.go`, `gitrepo/doc.go`, `CONSTRAINTS.md`, `raddle.md`, and `fabric-unified-view.md`. `raddle.md` is the one that goes actively wrong if skipped. `docs/overview.md` and `docs/shared-libs/README.md` were checked and need no change.
- **Q:** Test tiering? **A:** Untagged Tier-1 units plus integration-tagged tests for the reader, the empty-commit paths, and `CommitEmpty`. ⚠️ **Superseded in part (review r1):** the Tier-1 subject is **not** a promoted `parseSnapshotTags` — that promotion was dropped as dead code. It is the pure record-parser extracted from the generalized scan. See `scan-on-demand-no-index`.
- **Q:** (review r1) `commitWeftLocked` has a third silent-drop return — the `"did not match any files"` tolerance at `weftgit.go:378-380`. Does it fall through to the empty commit? **A:** Yes. It is semantically identical to `!positive` (the pathspec matched nothing), so treating it differently would leave one uncovered drop path in a rule whose value is exhaustiveness. Verified the code; the finding was accurate.
- **Q:** (review r1) Unborn *weft* HEAD makes `CommitEmpty` create weft's root commit — is that intended? **A:** Yes, allow it with no exception. Weft's root commit is already created this way for content commits, so an empty one is the same operation minus a tree delta, and the commit still carries a real `Warp-SHA`. Rejected adding a second carve-out beside unborn-warp for an outcome with no demonstrated harm.
- **Q:** (review r1) Promoting `parseSnapshotTags` creates dead production code, since the reader uses git's trailer placeholder and never parses a full message. **A:** Correct, and it contradicted this discussion's own delete-dead-code reasoning. Dropped the promotion; instead **generalize `scanWarpSHATrailers`** to capture the `Snapshot` field, keeping one git-log plumbing site. `parseSnapshotTags` stays a test helper.
- **Q:** (review r1) The deletion footprint understates test loss. **A:** Confirmed and corrected. `keyvalidation_test.go` is kept for `TestValidSHA`; `gogit_test.go` loses three tests plus `freezePackIndex`; three further `parity_test.go` cases the review did not name (471, 711, 753) were found and added. The lost mixed-backend reindex case is redundant with `parity_test.go:652` on `SHAExists` — verified, and recorded so the deletion is not later mistaken for an oversight.
- **Q:** (review r1) Three godoc comments go wrong on this commit and are missing from the docs list. **A:** Added `Commit`, `commitWeftLocked`, and `CommitWeft`, plus a general obligation to re-read the godoc of every touched function — those three are only the ones provably wrong today.
- **Q:** (review r1, NOTE) Does the guard file's own header also name `SnapshotSHA`, and does a replacement example exist? **A:** Yes to both. `gitrepoboundary_test.go` uses it in *two* comments (header 9-19 and map 38-49). `StageAndCommit` is a verified surviving mixed method — three `r.run` calls plus a migrated go-git `CurrentSHA` — and `CommitEmpty` will be a second.
- **Q:** (review r1, NOTE) Is `Commit(nil, msg, tags, opts)` — tags with zero files — supported or incidental? **A:** Supported deliberately. It is how a caller records a baseline without producing weft content, i.e. the standalone-snapshot use the design deferred, and it is pinned by its own test.
- **Q:** (review r2) `CommitEmpty`'s contract contradicts its own test — a bare `git commit --allow-empty` commits the index, so "must not sweep a staged file" is unsatisfiable. **A:** Correct. Resolved by adding an index pre-check that **refuses** with `ErrIndexNotEmpty`, rather than dropping the never-sweep property. `StageAndCommit` establishes never-sweeping as a package norm and enforces it via pathspec scoping; `CommitEmpty` has no pathspec, so it enforces the same norm by refusing.
- **Q:** (review r2) `manifest/roadmap.md` was dismissed as needing no change. **A:** Wrong dismissal — line 74's `## Done` entry is a present-tense module inventory naming `SnapshotSHA`/`SetSnapshotSHA` and goes factually wrong. Now a required edit. The CLAUDE.md rule invoked to skip it governs *status movement*, a different question. Lines 62/64 stay: they describe what past work covered at the time.
- **Q:** (review r2, NOTE) Does the empty-commit rule really "live inside the existing `if !unborn` arm"? **A:** No — that arm covers only trailer composition (356-362); all three fall-through points sit outside it and each needs its own guard. The rationale was rewritten so a plan writer does not under-size the change from it.
- **Q:** (review r2, NOTE) Is the rule tested through `CommitWeft` directly? **A:** It was not. Added a case — `CommitWeft` is an exported entry point that inherits the rule and gets a godoc update claiming it.
- **Q:** (review r2, NOTE) What happens to a snapshot after a branch switch? **A:** It reads as absent — the reader is per-branch by construction. Recorded as intended behaviour and the safe failure direction (regenerate unnecessarily, never answer from another branch's history), and contrasted with why the correspondence index needs `refreshCorrIndexAfterSwitch`.
- **Q:** (review r2, NOTE) Does empty-commit accumulation cost anything? **A:** Yes, and it is now stated and accepted: one commit per tagged regeneration, with the scan over a small history. A polling consumer would grow history without bound, but the fix for that belongs on the consumer.
- **Q:** (review r2, NOTE) `crucible/gitrepo-review-prompt.md` still lists `snapshot.go` as source to read. **A:** Line 21 is a live instruction and is now a required edit; the findings history in the same file is a frozen round record and stays stale by design.
- **Q:** (review r3) Does the deletion footprint actually compile? **A:** No — `gogit_test.go`'s linked-worktree parity harness (`runLinkedWorktreeParityChecks`, 717) calls `remoteName`, `SnapshotSHA`, and `isStrictDescendant` inline, and the fixture seeds a snapshot ref. Verified; all three subtests plus their oracle helpers, the key constant, the `update-ref` seeding, and the harness doc comment are now enumerated. Missing them breaks the build.
- **Q:** (review r3) What does the reader return when the newest tagged commit's `Warp-SHA` no longer exists after a warp rewrite? **A:** The dangling SHA, raw, with a nil error — matching the correspondence index's validate-at-use posture. The two-step composition written earlier in this discussion was wrong: it hard-errors on exactly the rebase case the mechanism must survive. Consumers use `SnapshotWarpSHA` → `SHAExists` → `ChangedFilesSince`, which is what `ChangedFilesSince`'s own doc already asks of callers.
- **Q:** (review r3) What does `CommitEmpty`'s index pre-check do on an unborn HEAD, where the root-commit contract requires it to run? **A:** Now specified per state: born → `git diff --cached --quiet` with `StageAndCommit`'s exact 0/1/other exit mapping; unborn → `git ls-files --cached` must be empty. Branch on the typed `CurrentSHA`/`ErrNoCommits` detection, never on stderr text, and avoid the empty-tree constant (hash-algorithm-dependent).
- **Q:** (review r3) What happens when `CommitEmpty` refuses inside the weft path? **A:** It propagates as the existing unlanded-weft `*PartialCommitError` (`WeftCommitted: false`) with the warp commit standing. Accepted, and recorded as a real change: a documented silent no-op becomes an error when the weft index is dirty and tags are present. Refusing beats folding someone else's staged work into a snapshot commit.
- **Q:** (review r3, NOTE) Any stale comments left outside the enumerated footprint? **A:** Two: `gogit.go:82`'s locking-discipline paragraph, and `parity_test.go:591`'s rationale for a **surviving** test that motivates itself via `SetSnapshotSHA`. Both added to the sweep.
- **Q:** (review r4) What does "newest tagged commit" actually order by? **A:** Commit date, until now — `scanWarpSHATrailers` passes no ordering flag. That is clock-dependent, and cross-clone sharing makes another machine's skewed clock able to put an older baseline first, under-reporting staleness. Added `--topo-order`. Concurrent merged branches leave a genuine topological ambiguity between incomparable commits; accepted and stated, since either choice over-reports rather than under-reports. `RebuildIndex` should be unaffected — held by its existing equivalence test, not by the reasoning.
- **Q:** (review r4) Per-branch scoping was decided and documented but never tested. **A:** Correct, and it was the only decision without a pinning case. Added an integration test: record on one branch, `Checkout`, assert `("", nil)`.
- **Q:** (review r4) `crucible/fabric-review-prompt.md:23` also enumerates the ref API. **A:** Added as a required edit, same live-instruction-vs-frozen-findings split as the gitrepo prompt.
- **Q:** (review r4) `fabric-unified-view.md:67` says raddle/trace never need a baseline without weft content. **A:** That clause is contradicted by this slice's central decision — it is now a required *correction*, not just an addition. Its conclusion (no standalone `Fabric.Snapshot` method) survives; its premise does not.
- **Q:** (review r4, NOTE) How does the reader match tags? **A:** Byte-exact, with no validation of the argument — an invalid tag cannot have been written, so it simply never matches. Validating on read would add an error path to a lookup whose answer is already known.
- **Q:** (review r4, NOTE) More stale comments in `gogit_test.go`? **A:** Two fixture comments (`linkedParityFixture`'s type doc and its `sharedSHA` field doc). Added, and the general re-read obligation now says explicitly that it covers test-fixture docs — the class that goes stale without breaking the build.
- **Q:** (review r5) Is `RebuildIndex` really insensitive to scan order, as round 4 claimed? **A:** No — that claim was wrong. It is order-sensitive twice over: dedup by warp SHA is last-assignment-wins over the reversed scan (so the scan's *first* listing wins), and `sort.SliceStable` preserves input order among equal `WarpSeq`, which includes the `seq = 0` dangling sentinel. Intended outcome now stated explicitly: newest-in-topological-order wins, matching the reader, so index and reader agree. `--topo-order` is a behaviour change to a shipped function, not a no-op.
- **Q:** (review r5) Can any listed test actually witness the topo-vs-date change? **A:** No — every existing and listed case is linear history, where the two orders coincide. Added a discriminating fixture: a merged side branch with a back-dated committer date, asserting both the reader's pick and rebuild equivalence.
- **Q:** (review r5) What happens to the correspondence entry when an empty commit records against an already-recorded warp SHA? **A:** It overwrites it — `WeftSHAForWarpSHA` and `RevertWithWeft` then resolve to the empty commit. Accepted: the empty commit's tree is identical to its parent's by construction, so the restored weft tree is unchanged, and "last recorded wins" is already the index's documented rule. This slice makes `index.go:283-287`'s "rare but legal" shape routine, so that comment must be updated and the case pinned by a test. The tree-identity argument is load-bearing, so the implementer must verify it against `RevertWithWeft` rather than accept the reasoning.
- **Q:** (review r5, NOTE) Is `PartialCommitError`'s wording still correct? **A:** No — both its godoc and its `Error()` string assert a warp commit landed, which is false for the tags-only shape this slice makes first-class (it interpolates an empty `WarpSHA`). Added to the same-commit staleness sweep.
- **Q:** (review r5, NOTE) Is `CommitEmpty`'s guard equivalent to `StageAndCommit`'s? **A:** No, and the discussion over-claimed by saying "the same norm". `StageAndCommit` is structurally safe via pathspec scoping; `CommitEmpty` is check-then-commit across two spawns and retains a race window. Accepted with the window documented, and the godoc must say "refuses if the index is dirty when checked", not "cannot sweep the index".
- **Q:** (review r5, NOTE) Any comment site still missing? **A:** `gogit.go:88`'s object-lookup bullet names `isStrictDescendant` as well as `SetSnapshotSHA`; only the latter was listed. Added, with a note that `hasUnpushed` in the same sentence survives.
- **Q:** (review r6, Fable) Is the deletion footprint's comment list actually exhaustive? **A:** No — a sixth round found four more sites, including `gitrepo.go:429` in a function this slice otherwise never touches. Rather than extend the list again, the exhaustiveness claim was **dropped** and replaced with a mandatory `grep -rin snapshot` sweep plus separate greps for `remoteName`/`isStrictDescendant` (which carry no "snapshot" substring). The enumeration is now labelled a starting point with worked examples. Enumeration was the wrong instrument: there are 38 case-insensitive hits in non-test source alone.
- **Q:** (review r6, NOTE) `gogitLinkedFixture` writes to the retired ref namespace but still compiles. **A:** Rename the constant and rewrite its comments anyway — the ref name is arbitrary to what the fixture proves, and leaving it presents a deleted namespace as live. Added to the sweep.
- **Q:** Is `lyx init` being phased out in favour of fabric V2? **A:** Yes, but in **slice 5**, not this one. `initengine`/`initcli` shrink toward deletion as topology folds into clone/worktree-add and the cwd anchor is replaced by the weft-recorded subpath. The only contact point with this slice is that `internal/initengine/undo.go` is a `CommitWeft` call site, and it uses the 3-argument form with no tags, so the empty-commit rule does not reach it.


### From _mill/plan/00-overview.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
slug: fabric-snapshot-trailer
approved: true
started: '20260731T091500Z'
parent: main
root: ""
verify: null
```

### From _mill/plan/01-retire-ref-mechanism.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: retire-ref-mechanism
number: 1
cards: 5
verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
depends-on: []
```



- **Edits:**
  - `internal/gitrepo/gogit_test.go`
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/parity_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/snapshot_test.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
  - `crucible/gitrepo-review-prompt.md`
  - `crucible/fabric-review-prompt.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-commit-empty-primitive.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: commit-empty-primitive
number: 2
cards: 4
verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
depends-on: [1]
```



- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/gitrepo/commitempty_integration_test.go`
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-snapshot-reader.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: snapshot-reader
number: 3
cards: 5
verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./cmd/lyx/...
depends-on: []
```



- **Edits:**
  - `internal/fabricengine/index.go`
- **Creates:**
  - `internal/fabricengine/index_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/snapshot.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-empty-commit-rule.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: empty-commit-rule
number: 4
cards: 7
verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./cmd/lyx/...
depends-on: [2, 3]
```



- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/index.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/commit_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/snapshot_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-design-docs.md


```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: design-docs
number: 5
cards: 2
verify: go test -count=1 ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/... && go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/...
depends-on: [1, 4]
```



- **Edits:**
  - `manifest/designs/raddle.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `crucible/fabric-review-prompt.md`
- `crucible/gitrepo-review-prompt.md`
- `internal/gitrepo/doc.go`
- `manifest/designs/fabric-unified-view.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content. This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file; it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path. Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk; keep only the other side's unrelated edit. Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file. The resolution keeps the item only under `## Done`; it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`. Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item. The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/fabric-snapshot-trailer add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/fabric-snapshot-trailer rm <file>` instead of editing; that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/fabric-snapshot-trailer rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity. The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C /home/knatte/Code/loomyard/wts/fabric-snapshot-trailer` for any git commands; do not `cd`. Worktree cwd is `/home/knatte/Code/loomyard/wts/fabric-snapshot-trailer`.
