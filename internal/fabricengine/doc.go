// Package fabricengine is lyx's sole warp↔weft git-coordination module, built on two
// `internal/gitrepo.Repo` instances covering warp↔weft topology and commit/push/pull into the
// paired weft repo.
// fabric is the only module that knows both repos exist: the `Fabric` handle holds unexported `warp
// *gitrepo.Repo` and `weft *gitrepo.Repo` fields for anything repo-specific and uncoordinated,
// reachable only from inside this package, and adds a small set of genuinely cross-repo operations
// (`Commit`, `Pull`, `Diff`, `Status`) on top of what gitrepo deliberately doesn't know about.
//
// `Fabric.Pull` (pull.go) is the unified read path: weft is fast-forwarded first via a plain
// `PullWeft`, then warp is fetched and inspected against its upstream tracking ref.
// A clean fast-forward (local warp HEAD still an ancestor of the fetched upstream tip) simply
// advances warp.
// A detected warp history rewrite (rebase or force-push upstream — local warp HEAD is no longer an
// ancestor of the fetched tip) is auto-reconciled whenever it is safe to do so: when local warp
// carries no unpushed commits of its own, weft's correspondence is re-anchored to the nearest
// surviving `Warp-SHA` — via the same empty-commit-with-trailer mechanism `commitWeftLocked`'s
// snapshot rule already uses (see below) — warp is reset to the new tip, and the fresh
// correspondence is recorded.
// Two cases abort loudly and make no change to either repo: local warp already has unpushed commits
// AND the remote diverged (the double-conflict case `Pull` refuses to resolve unattended,
// `ErrWarpDivergedUnpushed`), or the rewrite is so thorough that no recorded correspondence
// survives it at all (`ErrNoSurvivingAnchor`).
// Every rewrite/anchor determination is ancestry-based — `f.warp.IsAncestor`, via `git merge-base
// --is-ancestor` — never `f.warp.SHAExists`: `git fetch` never prunes objects, so a rebased-away
// commit's object survives fetch and `SHAExists` would report true post-fetch, meaning detection
// would never fire (see the reachability-never-object-existence Shared Decision).
// The call's result is `PullResult`, a PATTERN-residue report naming which post-anchor weft commits
// touch the `_lyx/PATTERN.md`/`_lyx/pattern/` paths and therefore need review, since they were
// written against a warp baseline that no longer exists upstream — see pull.go's own doc comment for
// the full flow and the `*PartialPullError` weft-succeeded/warp-failed contract.
// Those paths are scoped through the pair's recorded anchor, so a subpath-anchored hub's residue is
// found at `<anchor>/_lyx/PATTERN.md` rather than silently reported as empty.
//
// fabric enforces one uniform branch-naming scheme, with no exceptions: a warp branch `<branch>` is
// always paired with weft branch `<branch>-weft`, including the primary worktree (warp `main` ↔
// weft `main-weft`).
//
// Every weft commit fabric makes carries a `Warp-SHA: <sha>` trailer recording the warp SHA it
// corresponds to (see WarpSHATrailerKey),
// and a rebuildable correspondence index sits on top as a pure performance layer over that trailer
// history, never authoritative on its own.
// The one exception is a commit made while the warp repo itself has no commits yet (an unborn HEAD,
// e.g.
// a fresh `git init` before the operator's first warp commit): that commit carries no trailer and
// no correspondence entry, since there is no warp SHA yet to name — see commitWeft's warpHeadSHA.
// Normal trailer/record behavior resumes on the first weft commit made after warp's own first
// commit.
//
// fabric never calls gitrepo's `StageAllAndCommit` (board's opt-in wildcard-stage exception, per
// gitrepo's doc.go) — all staging is explicit-list `StageAndCommit`, scoped to a configured
// pathspec.
// The one exception is the package-level `commitWeftAt` function (not a `Fabric` method), which
// wraps board's wildcard-stage commit on its behalf — see `commitWeftAt`'s own doc comment.
//
// The default weft-staging pathspec (template.yaml's `pathspec:` key) is empty, so no optional
// directory is staged by default; `_lyx` itself arrives from `structuralCommittedDirs`, not from
// `pathspec`, so PATTERN content (`_lyx/PATTERN.md`, `_lyx/pattern/`) is committed as ordinary `_lyx`
// content rather than needing its own pathspec entry.
//
// The narrow-pathspec asymmetry below inverts, rather than disappears, once `pathspec` went empty:
// `configsync.ReconcileAll` -> `yamlengine.Reconcile` never rewrites a `pathspec:` key already
// present in a worktree's `fabric.yaml`, only adds one when the key is absent entirely.
// A worktree deployed before this task's template change therefore stays on its own recorded
// `pathspec: _pattern` forever — *wider* than the now-empty template default, not narrower — keeping
// a junction the template no longer wires, until that worktree's repo is re-cloned.
// `applyStaleRemoval` tears down a junction absent from `RepoWiredNames`, but only once that repo's
// recorded `pathspec` is actually empty, which a pre-existing `pathspec: _pattern` worktree's is not
// — so changing `template.yaml` governs newly cloned repos only.
// This is accepted rather than a defect: the sole deployed repo, SANDBOX, is throwaway and re-cloned
// rather than migrated, so no upgrade path is documented here because none exists.
//
// This is a deliberate asymmetry with the junction side (see fabricengine's own junction wiring): a
// junction self-heals on the next `lyx fabric clone`/`add`/reconcile and reports loudly until it
// does, because `WireJunctions` owns junction state outright, whereas `pathspec` is an
// operator-editable config value that `configsync` must never silently overwrite.
//
// The wired junction set is NOT sourced from `pathspec` alone: `WireJunctions`/`UnwireJunctions`
// operate over whatever name-set their caller passes them (see junctionnames.go's
// `junctionNames`/`WiredNames`), and every one of those callers builds that name-set as
// `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ the pair's `pathspec` filtered against
// `HubReservedNames()` — the three hub-structural tokens (`_board`, `_portals`, `_launchers`) that
// can never be a per-worktree junction.
// The two structural sets (`_lyx`, `.lyx`) are injected in code and never read from `pathspec` at
// all; only the third piece — empty today — comes from config.
// A future weft-backed *optional* module is therefore wired with no `fabric` code change at all:
// append its directory name to `template.yaml`'s `pathspec:` default,
// and any worktree whose `fabric.yaml` picks up that wider default wires the new junction the next
// time `WireJunctions` runs against it — subject to the same narrow-pathspec asymmetry above (an
// already-initialised worktree's existing `pathspec:` key is never widened for it automatically).
// This mechanism has no live instance today — the worked example above is purely hypothetical.
// A structural directory has no such asymmetry to worry about: it is never sourced from `pathspec`,
// so it cannot be left behind by a worktree's stale config.
//
// This batch (`.lyx` joining the wired name-set) establishes three operator-facing facts worth
// recording plainly.
// First, downgrade is unsupported — a pre-fix binary's `applyStaleRemoval` removes on-disk junctions
// absent from *its own* `RepoWiredNames`, so running an older `lyx fabric reconcile` after this
// change unwires `.lyx` and strands scratch inside the weft worktree; this is a one-way upgrade,
// never made downgrade-safe.
// Second, upgrade is signalled through health, not a dedicated migration step: an existing worktree
// reports the `.lyx` junction missing (`Healthy` false, `CauseJunctionMissing`) until `lyx fabric
// reconcile` runs — which both wires the junction and, via `seedLyxJunction`'s `.lyx`-only adoption
// branch, moves any pre-existing real `.lyx` content into the weft target rather than hard-erroring.
// That is the documented remedy, not a bug to route around.
// Third, the unwire output envelope changed: `UnwireVerbResult.WeftContent`'s value set is now
// `"preserved"` | `"not_present"` (never `"cleared"`), and the `gitignore` key is gone from the CLI
// envelope entirely — no code path removes a leftover committed `.gitignore` `.lyx/` block left by a
// pre-fix binary; the manual remedy is deleting the `.lyx/` line from the repo's lyx-managed
// `.gitignore` block by hand.
//
// The junction side of that asymmetry has a concrete blast radius worth naming rather than leaving
// an operator to meet as a surprise: every worktree wired before a new name is added to
// `WarpJunctions` lacks that junction, full stop, until `lyx fabric reconcile` re-runs against it —
// `_pattern`'s own now-superseded rollout was one such case, and any future optional-module addition
// would hit the same gap.
// Until re-run, `lyx fabric status` reports that pair not in sync, with `JunctionReason` naming the
// missing junction;
// `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than
// `ReconcileActionAlreadyHealthy` for it — and repairs it, so reconcile *is* the remedy, not merely
// a report;
// and `loom`'s preflight fails `CheckJunction`, sets its `check3BlocksSeed` flag, and blocks the
// run.
// The remedy is one `lyx fabric reconcile` (idempotent: wires the missing junction and materialises
// the weft-side directory) — never `lyx init`, which is gone;
// reconcile clears every one of those three symptoms in a single call.
// This is not suppressed because it should not be: the junction genuinely is missing,
// and a health check that lied about a missing junction is the exact fault the junction-health
// generalisation (`checkJunctionHealth`, `Healthy`, `Status`'s inline verdict) exists to remove.
//
// `Fabric.Commit` classifies a caller's mixed file list into warp-side and weft-side paths and
// dispatches each side to its own commit, warp always first (see the
// warp-first-then-weft-under-one-lock Shared Decision): the warp commit is bare, plain git — no
// trailer, no correspondence entry, preserving the "warp stays ordinary git" property — and only
// once it lands does the weft side commit under the fabric-layer write lock, acquired before the
// warp commit and held across both.
// There is no cross-repo transaction and no rollback: a landed warp commit is never unwound if the
// weft side then fails, so a two-sided call reports, not undoes, partial failure — see commit.go's
// `CommitResult` (independent `WarpSHA`/`WarpCommitted`/`WeftSHA`/`WeftCommitted`) and
// `*PartialCommitError`, which distinguishes the three possible weft outcomes a call can leave
// behind: the weft commit never landed at all;
// it landed and its `Warp-SHA` trailer was recorded in the correspondence index as usual (the
// non-error path);
// or it landed but `RecordCorrespondence` itself failed to persist the index entry
// (`WeftCommitted=true` on the error) — recoverable only via an explicit `RebuildIndex` rescan of
// the trailer history, since the landed commit's own trailer remains the index's sole source of
// truth and `WeftSHAForWarpSHA`'s own one-shot self-correction fires only on a stale *hit*, never
// on the index *miss* a never-written entry produces.
//
// Once `Commit` has attempted the weft side (if any),
// and the combined write lock (see below) has already been released, it fires an unconditional,
// detached, fire-and-forget push of both repos via `SpawnDetachedPush` whenever anything landed on
// either side (`WarpCommitted || WeftCommitted`) — the async-push-both-sides-via-detached-child
// Shared Decision — and `opts.SkipGit`/`opts.SkipPush` interact with this two-step call in ways
// that narrow their general contract, worth stating plainly rather than leaving a caller to infer
// them from behavior. `opts.SkipGit` is **weft-scoped** for `Fabric.Commit` specifically: it gates
// only whether the weft-side commit is attempted at all;
// the warp commit and the async push both proceed regardless of `opts.SkipGit`, a deliberate
// narrowing of `SyncOptions.SkipGit`'s general "skip all git operations if true" contract for this
// one entry point. `opts.SkipPush` is likewise **not consulted** by the async push at all: the
// detached both-sides push gates only on the `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH` environment
// variables, checked helper-internally inside `SpawnDetachedPush` (per the
// async-push-both-sides-via-detached-child Shared Decision) — so a caller passing
// `SyncOptions{SkipPush: true}` to `Fabric.Commit` still triggers the fire-and-forget push unless
// the environment variable is also set.
//
// The detached child that spawn launches no longer runs one single push per side: its bypass
// handler (`internal/fabriccli`'s `push` verb, re-entered with `--warp-path`/`--weft-path`) now
// runs `CoalescePushBothAt`, a loop-until-clean coalescing push that holds a separate absorbing
// push lock — `fabric.push.lock`, under the weft worktree's `.weft/` directory, distinct from the
// commit lock below — for the whole loop, repeatedly rebase-free-pushing whichever side has commits
// and exiting once an iteration advances neither side's HEAD.
// This push is deliberately rebase-free: `gitrepo.PushRebaseFree` is a plain `git push`, never `git
// pull --rebase`, so a remote that has diverged simply leaves that side's commits unpushed and logs
// a warning (`gitrepo.ErrPushRejected`), rather than mutating the calling process's working tree
// out from under it.
// Reconciling a diverged remote is out of scope here (slice 6).
//
// The combined write lock `Fabric.Commit` takes — `.weft/weft.write.lock`, the same lock
// `commitWeft` already used weft-side — is now acquired for ANY committing call, warp-only included
// (see the combined-commit-lock Shared Decision), not only when a weft-side commit is involved,
// closing the race two concurrent warp-only `Fabric.Commit` calls previously ran unlocked.
// The lock is released before the async push above is spawned, never held across it (the
// commit-lock-scoped-to-commit-only Shared Decision): the network push is a separate concern
// running under its own absorbing push lock in the detached child, not this commit-side lock.
//
// `Fabric.Diff` (now wired onto the `lyx fabric diff <since-warp-sha>` CLI verb) answers "what
// changed since this warp SHA, on both sides" by bridging `sinceWarpSHA` to the
// nearest-at-or-before weft SHA the correspondence index resolves it to, via the same
// `resolveRevertTarget` resolver diff.go's `weftAnchorForWarpSHA` calls;
// a `sinceWarpSHA` older than the first recorded correspondence degrades to an empty weft side with
// `DiffResult.NoWeftCorrespondence` set, rather than an error, since a diff from before fabric
// started tracking this pair has no weft baseline to compare against. `Fabric.Status` answers a
// different question — "what is currently uncommitted, on both sides" — via a live worktree read
// (`gitrepo.Repo.WorktreeChangedFiles`, backed by go-git's `Worktree.Status()`) with no
// correspondence anchor involved at all.
//
// fabric now exposes two distinct status-shaped surfaces, deliberately not variations of one
// another: `Topology.Status` reports paired warp↔weft topology (branch pairing, junction health),
// reachable via `lyx fabric pairs`;
// and the unified `Fabric.Status` merges each side's currently-uncommitted worktree changes into
// one side-labelled view, reachable via `lyx fabric status`.
//
// A known asymmetry in `Fabric.Status` is worth recording rather than leaving as a surprise: it may
// surface gitrepo's `.gitrepo-push.lock` operational artifact (`PushCoalesced`'s single-pusher lock
// file, left behind in a repo's worktree root because `lock.FileLock.Release` unlocks without
// deleting it) as an uncommitted change on the warp/warp side but never on the weft side, because
// fabric seeds a `.git/info/exclude` entry for it only on the weft worktree it owns
// (`seedWeftArtifactExcludes`) and deliberately does not manage the warp repo's own git
// configuration.
// This asymmetry is now narrower than it was: the warp-via-fabric async push described above uses
// the lock-free, rebase-free `PushRebaseFree` for both sides (per the
// no-warp-root-gitrepo-push-lock Shared Decision), so that path never creates a
// `.gitrepo-push.lock` at the pristine warp worktree root at all anymore — `Fabric.Status`'s own
// behavior of not suppressing a warp-side artifact is unchanged, but there is normally nothing
// there left for it to surface.
// A warp-side `.gitrepo-push.lock` can still appear only from a non-fabric caller of
// `gitrepo.PushCoalesced` running directly against the warp repo (e.g.
// a manual warp-side push outside fabric) — a lingering lock from that path is still reported by
// `Fabric.Status` like any other untracked warp file, not specially suppressed.
//
// The read half of the same `Snapshot:` trailer mechanism exposes `Fabric.snapshotWarpSHA(tag
// string) (string, error)`, resolving a caller-supplied snapshot tag to the warp SHA recorded under
// it by the newest weft commit (in topological order) carrying a `Snapshot: <tag>` git trailer (see
// `SnapshotTrailerKey`, `snapshotTagPattern`, in trailer.go).
// A tag is restricted to letters, digits, dot, underscore, and hyphen, excluding newline, carriage
// return, and colon — the trailer-injection vector those excluded characters would otherwise open,
// letting a bug- or attacker-supplied tag inject a fake or unrelated trailer line into the commit
// message;
// that restriction is enforced entirely on the write path (`validateSnapshotTag`, called from
// `appendSnapshotTrailers`), never on the read side.
//
// `snapshotWarpSHA` scans the same generalized trailer history `scanWarpSHATrailers` (index.go)
// already builds for `RebuildIndex` — one `git log --topo-order` pass, run fresh on demand every
// call — rather than consulting or maintaining any index of its own: "newest" means newest in
// topological order, where no commit is ever listed before one of its own descendants, the
// identical rule `RebuildIndex`'s own dedup applies, which is what keeps the two answers in
// agreement over the same history.
// No snapshot index was added on top, because a snapshot read is rare — a staleness check at a
// phase boundary, not a hot path — so an index would only buy a second cache-invalidation surface
// for a consumer that does not exist yet, per the trailer-is-truth-no-new-cache decision: the
// `Snapshot` trailer itself is the sole source of truth, and anything built on top of it is a
// rebuildable cache, the same layering the correspondence index already rests on.
//
// A tag never recorded in the current branch's history reads as `("", nil)`, not an error — absent
// is a normal, expected first-run state, matching the retired `gitrepo.SnapshotSHA` contract
// exactly — deliberately unlike index.go's `ErrNoCorrespondence`, where a miss signals broken
// bookkeeping rather than a legitimate absence.
// Matching is byte-exact and unvalidated on the read side: a tag that could not have been written
// can never match anything, so read-side validation would buy nothing, while fuzzy matching would
// let a lookup for `Raddle` silently resolve a baseline recorded as `raddle`, hiding a caller bug
// behind a false-positive convenience.
//
// A dangling `Warp-SHA` — the newest tagged commit names a warp SHA that no longer exists because
// warp history was rewritten — is returned raw, with a nil error: the same validate-at-use posture
// `RebuildIndex` already takes for its own trailer values.
// Skipping back to an older tagged commit would silently answer with an older baseline and
// under-report staleness,
// and collapsing the answer to absent would conflate "never recorded" with "recorded, then
// rewritten" for no benefit, since both drive the same consumer action.
// The intended three-step consumer idiom is: read the SHA via `snapshotWarpSHA`, check
// `f.warp.SHAExists(sha)`, then call `f.warp.ChangedFilesSince(sha)` only if it exists, treating a
// missing SHA as total staleness — not a burden invented here, since `ChangedFilesSince`'s own doc
// comment already asks every caller to check `SHAExists` first.
//
// The reader is per-branch, because it scans only the current weft branch's history: a snapshot
// recorded on another branch reads as absent the moment a coordinated `Checkout` switches the pair
// onto that branch, and the consumer regenerates from scratch — the safe failure direction.
// Contrast this with `refreshCorrIndexAfterSwitch`: the correspondence index is a per-worktree file
// that survives a branch switch and can therefore keep answering cross-branch, which is exactly why
// `Checkout` must discard and rebuild it, whereas `snapshotWarpSHA` holds no state of its own to
// discard and simply stops seeing the other branch's commits once the weft worktree switches away.
//
// The write half closes the one gap the read half's design leaves open: when `snapshotTags` is
// non-empty and no weft commit would otherwise land, `commitWeftLocked` (weftgit.go) lands an
// **empty** weft commit carrying the already-composed `Warp-SHA` and `Snapshot:` trailers, via
// `commitEmptySnapshot` wrapping `gitrepo.Repo.CommitEmpty`.
// There are four triggering cases, all sharing the identical rationale: a caller's regeneration
// confirmed itself current against a warp SHA, and that confirmation needs a home even though
// nothing else changed.
// A warp-only or tags-only `Fabric.Commit` call reaches `commitWeftLocked` with an empty weft
// pathspec, which `weftPathspecFilter` reduces to no positive entry at all.
// A pathspec that survives filtering can still resolve to nothing by the time `git add` runs, which
// `StageAndCommit`'s "did not match any files" tolerance absorbs — reachable only as
// defense-in-depth, since the filter's own pre-check normally keeps this path from firing.
// And — the genuine correctness hole this whole mechanism exists to close — a pathspec that matches
// real, tracked content whose staged bytes are identical to HEAD's makes `StageAndCommit` report
// `committed=false`: without the empty-commit rule, a caller re-running an identical regeneration
// against an unchanged warp SHA would see no weft commit land, the correspondence baseline would
// never advance, and a staleness check would report drift forever despite the regeneration having
// just confirmed itself current.
//
// `Commit(nil, msg, tags, opts)` — snapshot tags with zero files at all — is consequently a
// **supported call shape**, not an accident of a widened predicate: it is how a caller records a
// baseline (a warp SHA under a tag) with no weft content of its own to commit, the
// standalone-snapshot use this design serves without a dedicated method. `Fabric.Commit`'s
// `weftSide` predicate reflects this directly: it is true whenever there are weft files OR
// `snapshotTags` is non-empty, so a tags-only or warp-only-but-tagged call now takes the combined
// write lock and runs `ensureWeftLockDir` where an earlier version of `Commit` did neither —
// correct, since the call is about to write to weft, but a real widening from a predicate that once
// looked only at weft files.
//
// `RecordCorrespondence` runs for an empty commit exactly as it does for a content commit: the
// empty commit carries a real `Warp-SHA` trailer like any other, so `RebuildIndex` — which reads
// trailers, not this call site's own choices — would record it anyway on a full rescan, and
// skipping the call here would only make the incremental and rebuilt indexes diverge.
// One consequence of always recording is now routine rather than rare: a warp SHA does not move
// between a content commit and a later tags-only or unchanged-content call at the same warp HEAD,
// so the later empty commit's `RecordCorrespondence` call **upserts over** the entry the earlier
// content commit wrote for that same warp SHA — `WeftSHAForWarpSHA` and `resolveRevertTarget` then
// resolve to the empty commit, "last recorded wins" being `corrIndex.record`'s own pre-existing
// upsert rule meeting a newly-common input.
// This is accepted as benign, not merely tolerated, because an empty commit's tree is identical to
// its parent's by construction: resolving a diff anchor to it bridges to exactly the same weft tree
// the superseded content commit produced, and `resolveRevertTarget` uses the resolved weft SHA only
// as a bridge target, never for anything that would make the overwrite observable beyond that.
//
// One propagation is a genuine behaviour change worth naming plainly: when
// `gitrepo.Repo.CommitEmpty` refuses via `gitrepo.ErrIndexNotEmpty` — the weft index is carrying
// staged content the combined write lock did not exclude, most likely left behind by an aborted
// earlier run — that refusal now surfaces as a `*PartialCommitError` with `weftCommitted: false`,
// through the exact mapping arm `Fabric.Commit` already uses for an unlanded weft commit.
// The equivalent call before this mechanism existed was a documented silent no-op; failing loudly
// here is deliberate, since silently folding somebody else's staged work into a snapshot commit is
// strictly worse than refusing.
//
// The rule has exactly two exceptions, both narrower than "skip everything."
// An **unborn warp HEAD** still drops the tags exactly as it always has, regardless of
// `snapshotTags`: a snapshot's entire content is a warp SHA to record, and with none yet to name,
// an empty commit carrying only a bare `Snapshot:` trailer with no `Warp-SHA` sibling would force
// `snapshotWarpSHA`'s reader to grow a third "found the tag, but it has no baseline" state — normal
// trailer/tag/record behaviour resumes on the first `commitWeftLocked` call after warp's own first
// commit.
// An **unborn weft HEAD**, by contrast, is deliberately *not* an exception: `commitEmptySnapshot`'s
// `CommitEmpty` call lands the empty commit as weft's own root commit, carrying its trailers like
// any other, because weft having no history yet is not a reason to withhold a baseline warp already
// has. `opts.SkipGit` is unaffected by any of this and is recorded here so a reader of the rule
// does not conclude it overrides the opt-out: `SkipGit` still skips the weft side entirely, tags or
// not, exactly as `Fabric.Commit`'s own doc comment already describes.
//
// The accepted accumulation cost is worth stating honestly rather than leaving implicit: every
// no-op tagged run now appends a weft commit carrying no content, so a consumer checking staleness
// in a tight loop would grow weft history without bound, and the cache-free reader
// (`snapshotWarpSHA`, `RebuildIndex`) scans that whole history on every call.
// This is accepted because the actual call pattern this mechanism serves is one tagged commit per
// regeneration at a phase boundary, not a poll, and because the scan is a single `git log` over a
// history that stays small by construction under that pattern;
// if a consumer ever does poll, the fix belongs on the consumer's side — check `ChangedFilesSince`
// first, and tag only when actually regenerating — not on this mechanism, which has no way to
// distinguish a genuine regeneration from a poll from the trailer history alone.
//
// Cross-clone sharing comes for free from where the trailers live: because `Snapshot:` and
// `Warp-SHA` trailers are part of weft's own versioned commit history,
// and the empty commits carrying them are ordinary weft commits like any other, the existing
// detached both-sides push (see `Fabric.Commit`'s own doc comment) already sends them to the remote
// — no equivalent of the retired `refs/loomyard/snapshot` mechanism's own separate ref push is
// needed for a snapshot baseline to reach another clone.
//
// Slice 5 (the `fabric-clone-subpath` task) landed clone-does-everything, a subpath-in-weft anchor,
// and the dissolution of `lyx init`, replacing the earlier two-step "clone, then `lyx init`" flow
// with a single command that leaves a worktree fully wired. `CloneHub` (clone.go) drives warp+weft
// clone, `_board` worktree materialization, and lyx-anchor resolution in one call;
// the CLI layer (`internal/fabriccli`) then records the anchor and the repo-wide `fabric.yaml` onto
// `weft:main`, creates every warp junction (`_lyx`, `.lyx`), and runs
// `configsync.ReconcileAll` — so a fresh clone or `worktree add` leaves every junction wired and
// every config materialized without a second command. Every junction is excluded through the warp's
// own `.git/info/exclude`, never a committed `.gitignore` in the user's repo.
//
// The lyx-anchor subpath (e.g. `backend` or `.`) is recorded once, on `weft:main`, as the plain
// `.lyx-anchor` marker at `BoardDir(Hub)` (see `internal/lyxcwd/anchor.go`);
// `lyxcwd.Resolve` is the reader. `Resolve` treats the record as truth once present — it sets
// `AnchorRel` from the marker, then hard-errors if cwd does not equal the anchored directory
// exactly — and falls back to `AnchorRel` `"."` only when no marker is recorded yet (mid-clone, a
// lyxtest synthetic hub, or a non-fabric git repo).
// A hub still carrying the pre-rename marker spelling (`lyxcwd.StaleAnchorFileName`) with no
// renamed marker beside it is NOT such a fallback case: it recorded a real subpath under the old
// name, so every resolver returns `lyxcwd.ErrStaleAnchorMarker` rather than answering `"."` — which
// would re-anchor the repo at its root and let `lyx fabric reconcile` wire a second junction set
// there. `CloneHub` refuses the same state at clone time, naming the same literal from the same
// single declarer.
//
// The warp binding is a fourth repo-wide record beside the anchor and the repo-wide `fabric.yaml`
// config, held as a plain single-line file, `.lyx-warp`, at the board root (`<BoardDir>/.lyx-warp`,
// see warpbinding.go), containing the warp URL only.
// `CloneHub` resolves the effective warp URL from that record when the caller supplies no warp URL,
// and writes the record when none exists yet and a warp URL is supplied;
// a supplied URL that disagrees with the recorded one is a hard error, never a silent re-point.
// That resolution runs through a throwaway pre-hub probe clone of the weft remote (warpprobe.go),
// because the hub is named after the warp repo and therefore has no path to resolve into until the
// warp URL is known.
// `Reconcile` backfills the record once per hub from the warp side's own `origin` remote
// (reconcile.go's reconcileWarpBinding), with the CLI layer driving the commit and push, exactly as
// clone's own binding write is committed CLI-side.
//
// The anchor, the repo-wide config, and the warp binding are the three repo-wide records that let a
// later `lyx fabric reconcile` re-wire a hub with no re-clone at all;
// `Unwire` leaves all three untouched.
//
// The wired junction set is likewise a **repo-wide** fact, not a per-worktree one: the `pathspec`
// key lives in the repo-wide `fabric.yaml` at `<BoardDir>/_lyx/config/fabric.yaml`, so every
// worktree in the repo reconciles against the one recorded name-set rather than each carrying its
// own.
// Reconcile (reconcile.go) is declarative convergence to that pathspec — add what's missing,
// re-point what's broken, remove what's stale — and is fail-closed: a missing or unparseable
// repo-wide `fabric.yaml` aborts stale-removal for that pair and touches nothing, rather than
// risking a blanket sweep on a broken read.
//
// `Unwire` (unwire.go) is the per-worktree full deactivation that replaced the deleted `lyx init
// --undo`: it removes every fabric junction actually present on disk (not just the ones the current
// wired name-set names) and their warp `.git/info/exclude` entries, but never deletes weft-side
// content — `_lyx` and `.lyx` are both preserved — and never touches the repo-wide
// `weft:main` records, which survive so a later `lyx fabric reconcile` re-wires the worktree from the
// same anchor and pathspec.
//
// # The one-repo illusion at the public API boundary
//
// fabric exists to sell one illusion to every other package: a developer, an agent, and every lyx
// module see one repository, called fabric,
// never the warp/weft split underneath it.
// `Open(l *lyxcwd.Location) (*Fabric, error)` is the only constructor any other package calls — it
// derives both paths from l and stat-validates them, performing no wiring of its own (wiring is
// Topology's job: Add/Checkout/Reconcile/Remove/Prune/Cleanup).
// `Fabric.Commit`'s `CommitResult.Committed() bool` is the one commit result a consumer outside the
// owner set should read;
// the four raw `WarpSHA`/`WarpCommitted`/`WeftSHA`/`WeftCommitted` fields stay exported only because
// `internal/fabriccli`'s own `lyx fabric weft …` verb prints them by design.
// `RefScanner` (refscanner.go), constructed via `NewRefScanner(l)`, is how a consumer like
// `websterengine`'s audit asks "does this command reference fabric's two-checkout mechanism" without
// ever holding the weft path or the command-spelling pattern itself — fabric owns every word in the
// answer.
// `Healthy(l)` returns a typed `HealthReason` (drift.go) rather than a string a caller would have to
// substring-match, so a caller like `loomengine.Preflight` switches on `HealthReason.Cause` instead
// of parsing prose.
//
// # The fabric vocabulary rule
//
// In production code, the tokens `weft`, `warp`, and the fabric-sense phrase form of `warp` (`warp
// repo`, `warp worktree`, `warpBranch`, …
// — never the bare word, which is ordinary English elsewhere in the repo) may appear only in the
// owner set: `internal/fabricengine` (this package, which implements the illusion),
// `internal/fabriccli` (fabric's own CLI, which exposes the weft to an operator deliberately),
// `internal/weftname` (the `-weft` suffix leaf), `internal/lyxtest` (the test-fixture leaf that
// builds real paired worktrees), `internal/boardengine` (the pre-existing board carve-out, since
// board lives at `weft:main`), `internal/configsync` (string literals and comments, never
// identifiers, for the on-disk legacy config filenames `warp.yaml`/`weft.yaml`), and
// `tools/`/`sandbox/` (the black-box harness naming
// the real `lyx-test-weft`/`lyx-fabric-test-weft` GitHub repos).
// `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) machine-checks
// identifiers, string literals, and comments in every production `.go` file plus the embedded agent
// prompt templates;
// `CONSTRAINTS.md`'s Fabric Vocabulary Invariant records the rule in full, including the phrase-based
// `warp` predicate and the review-only prose-doc split between a doc explaining fabric's own
// mechanism (which keeps the vocabulary) and a doc describing a consumer module's behaviour (which
// rewords).
// As an owner file, this doc comment — and every other file in this package — keeps `warp`/`weft`
// vocabulary freely when explaining the mechanism the illusion sits on.
package fabricengine
