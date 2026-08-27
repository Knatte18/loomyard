// Package fabricengine is lyx's sole warp↔weft git-coordination module, built on two
// `internal/gitrepo.Repo` instances covering warp↔weft topology and commit/push/pull into the
// paired weft repo.
// fabric is the only module that knows both repos exist: the `Fabric` handle holds unexported `warp
// *gitrepo.Repo` and `weft *gitrepo.Repo` fields for anything repo-specific and uncoordinated,
// reachable only from inside this package, and adds a small set of genuinely cross-repo operations
// (`Commit`, `Pull`, `Diff`, `Status`) on top of what gitrepo deliberately doesn't know about.
//
// `Fabric.Pull` (pull.go) is the unified read path: weft is fast-forwarded first via a plain
// `PullWeft` — skipped as a vacuous success when the weft branch has no upstream yet, the freshly
// bootstrapped hub's ordinary state until its first push lands — then warp is fetched and inspected
// against its upstream tracking ref.
// A clean fast-forward (local warp HEAD still an ancestor of the fetched upstream tip) simply
// advances warp.
// A detected warp history rewrite (rebase or force-push upstream — local warp HEAD is no longer an
// ancestor of the fetched tip) is auto-reconciled whenever it is safe to do so: when local warp
// carries no unpushed commits of its own, the correspondence index is first rebuilt from the weft
// trailer history (the sole source of truth — a re-cloned hub's per-pair index cache starts empty
// while its adopted weft history carries every recorded anchor, and a verdict as final as
// `ErrNoSurvivingAnchor` must never rest on a stale cache), then weft's correspondence is
// re-anchored to the nearest surviving `Warp-SHA` — via the same empty-commit-with-trailer
// mechanism `commitWeftLocked`'s snapshot rule already uses (see below) — warp is reset to the new
// tip, and the fresh correspondence is recorded.
// Two cases abort loudly and make no change to either repo: local warp already has unpushed commits
// AND the remote diverged (the double-conflict case `Pull` refuses to resolve unattended,
// `ErrWarpDivergedUnpushed`), or the rewrite is so thorough that no recorded correspondence
// survives it at all (`ErrNoSurvivingAnchor`).
// A third refusal guards the working tree rather than history: whenever warp would have to move at
// all — fast-forward or reconcile — a warp worktree carrying uncommitted tracked changes aborts
// with `ErrWarpDirty` before anything mutates warp, because every warp advance goes through a
// `reset --hard` that would silently destroy those changes (weft has already been fast-forwarded
// when this fires; warp is untouched).
// Every rewrite/anchor determination is ancestry-based — `f.warp.IsAncestor`, via `git merge-base
// --is-ancestor` — never `f.warp.SHAExists`: `git fetch` never prunes objects, so a rebased-away
// commit's object survives fetch and `SHAExists` would report true post-fetch, meaning detection
// would never fire (see the reachability-never-object-existence Shared Decision).
// The weft ff-pull is non-fatal: a failed upstream probe or a failed weft pull is warned and leaves
// `PullResult.WeftPulled` false, but the warp fetch/reconcile below runs regardless — reconciling a
// weft that failed to pull is a named manual operator step, never something `Pull` resolves for the
// caller.
// The call's result is `PullResult`, a PATTERN-residue report naming which post-anchor weft commits
// touch the `_lyx/PATTERN.md`/`_lyx/pattern/` paths and therefore need review, since they were
// written against a warp baseline that no longer exists upstream — see pull.go's own doc comment for
// the full flow and the `*PartialPullError` warp-side-failure contract, whose `WeftPulled` field now
// faithfully reports whether the weft arm completed rather than asserting it always did.
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
// Adoption MERGES a directory present on both sides, recursively, rather than refusing it, and that
// is load-bearing rather than lenient: `lyx fabric unwire` removes the `.lyx` junction, and the very
// next `lyx` invocation in that worktree that logs at Info-or-above or exits non-zero opens
// `internal/logger`'s durable sink — which is ungated outside `go test`, and whose
// `os.MkdirAll(<anchor>/.lyx/logs)` therefore recreates a REAL warp-side `.lyx/logs` on top of the
// `logs` an earlier adoption already moved into weft.
// Refusing that collision left `reconcile`, the documented remedy, permanently unable to heal the
// pair, and — because `seedLyxJunction` aborts before `seedGitExclude` re-runs — left the warp
// worktree untracked-dirty, which then fed false input to the destruction gate's own dirtiness
// checks. A collision that is not a directory on BOTH sides is still refused, because resolving it
// would mean choosing a winner between two files, which fabric never does on the operator's behalf.
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
// and `internal/preflight`'s `CheckResolved` — wired into `internal/preflightshed` — fails
// `CheckJunction` and blocks the run.
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
// `commitWeftLocked`'s own "did not match any files" tolerance absorbs — reachable only as
// defense-in-depth, since the filter's own pre-check normally keeps this path from firing.
// The tolerance lives in `commitWeftLocked` (weftgit.go), not in `gitrepo.StageAndCommit`, which has
// none: `StageAndCommit` wraps and returns `git add`'s failure like any other, and
// `commitWeftLocked` recognises this one case by matching git's own message text on the way past.
// That match is the single place in this package whose correctness depends on
// `*gitexec.GitError.Error()` continuing to render git's trimmed stderr into the message, so a
// change to that rendering would silently turn this tolerance back into a hard failure.
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
// own `.git/info/exclude`, never a committed `.gitignore` in the user's repo, and the entry written
// there is the junction's own anchored path (`/backend/_lyx`, or `/_lyx` at a root anchor) rather
// than a bare name, since a slash-free gitignore pattern matches at every depth and would untrack
// same-named directories fabric never wired.
// git resolves `info/exclude` to the repo's COMMON gitdir, so the file is shared by every worktree
// in the hub and two verbs running side by side edit the same bytes;
// every mutation of it therefore goes through `mutateGitExclude` (gitexclude.go), which holds a
// repo-wide flock across read, rewrite and write and replaces the file by same-directory rename, so
// a concurrent reader sees either the whole old file or the whole new one.
//
// The lyx-anchor subpath (e.g. `backend` or `.`) is recorded once, on `weft:main`, as the plain
// `.lyx-anchor` marker at `BoardDir(Hub)` (see `internal/lyxcwd/anchor.go`);
// `lyxcwd.Resolve` is the reader. `Resolve` treats the record as truth once present — it sets
// `AnchorRel` from the marker, then hard-errors if cwd does not equal the anchored directory
// exactly — and falls back to `AnchorRel` `"."` only when no marker is recorded yet (mid-clone, a
// gitkit synthetic hub, or a non-fabric git repo).
// A hub still carrying the pre-rename marker spelling (`lyxcwd.StaleAnchorFileName`) with no
// renamed marker beside it is NOT such a fallback case: it recorded a real subpath under the old
// name, so every resolver returns `lyxcwd.ErrStaleAnchorMarker` rather than answering `"."` — which
// would re-anchor the repo at its root and let `lyx fabric reconcile` wire a second junction set
// there. `CloneHub` refuses the same state at clone time, naming the same literal from the same
// single declarer.
//
// A marker that is PRESENT but empty after trimming is a third state, and it is fabric's to refuse
// rather than lyxcwd's: `lyxcwd` deliberately treats it as absent and falls back to `"."`, which is
// exactly right for a hub that never recorded an anchor and exactly wrong for one whose marker was
// truncated.
// Since `Reconcile` is the only verb that wires junctions, it is the only verb that can turn that
// fallback into the second-junction-set damage above, so it reads the marker directly (see
// `refuseEmptyAnchorMarker`) and aborts the pass, leaving `lyxcwd`'s documented fallback untouched.
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
// `Remove` (remove.go) is the paired teardown, and it never deletes a directory git itself declined
// to remove unless that directory is a registered LINKED worktree of the warp repo.
// The rule is load-bearing rather than defensive: `git worktree remove` refuses a main working
// tree, a path belonging to another repo (`<Hub>/_board` is a worktree of the WEFT repo), and a
// worktree carrying state it will not discard — and treating every one of those refusals as licence
// to delete the directory turned an ordinary typo into the loss of a whole git clone, gitdir
// included, reported as success.
// The hub's prime worktree is refused by name before any teardown begins, since it is the warp
// repository rather than a pair.
//
// # The one-repo illusion at the public API boundary
//
// fabric exists to sell one illusion to every other package: a developer, an agent, and every lyx
// module see one repository, called fabric,
// never the warp/weft split underneath it.
// `Open(l *lyxcwd.Location) (*Fabric, error)` is the only constructor any other package calls — it
// derives both paths from l and stat-validates them, performing no wiring of its own (wiring is
// Topology's job: Add/Checkout/Reconcile/Remove/Prune/Cleanup).
// `OpenParent` is the parent-fabric resolution chain built on `List` — it matches the hub's
// worktrees against a caller-supplied branch, skipping any `Prunable` entry (git's own signal that a
// worktree's directory is gone but its administrative entry survives), resolves the match via
// `lyxcwd.ResolveWorktree`, and opens it through `Open`, returning a plain error a caller turns into
// `Stuck` when no live pair matches; `Fabric.OriginURL()` and `Fabric.PushBranch(opts)` are the two
// single-sided methods a caller like `internal/loomcli` reaches for under the same carve-out `Open`'s
// own bullet already states, `OriginURL` wrapping the warp side's `RemoteURL("origin")` and
// `PushBranch` wrapping `PushWarpRebaseFreeAt` under a vocabulary-neutral name `internal/loomcli` is
// not permitted to say itself.
// `Fabric.Commit`'s `CommitResult.Committed() bool` is the one commit result a consumer outside the
// owner set should read;
// the four raw `WarpSHA`/`WarpCommitted`/`WeftSHA`/`WeftCommitted` fields stay exported only because
// `internal/fabriccli`'s own `lyx fabric weft …` verb prints them by design.
// `RefScanner` (refscanner.go), constructed via `NewRefScanner(l)`, is how a consumer like
// `websterengine`'s audit asks "does this command reference fabric's two-checkout mechanism" without
// ever holding the weft path or the command-spelling pattern itself — fabric owns every word in the
// answer.
// `Healthy(l)` returns a typed `HealthReason` (drift.go) rather than a string a caller would have to
// substring-match, so a caller like `preflight.CheckResolved` switches on `HealthReason.Cause`
// instead of parsing prose.
//
// # The mutation record
//
// Every mutating verb accumulates a `*Mutations` record (mutation.go) naming, in order, the
// primitives it actually performed — not the primitives it attempted, and not what its own success
// return value implies happened. `ok` (or, at this layer, a nil error) means only "no error was
// returned"; it has never meant "nothing happened", and mixing the two up is what `mutations` and
// `partial` exist to stop a consumer from doing by accident.
//
// The vocabulary is `Kind` (mutation.go's closed, string-backed enum — `path_removed`,
// `worktree_removed`, `link_removed`, `branch_deleted`, `worktree_reset`, `dir_created`,
// `worktree_created`, `branch_created`, `branch_pushed`, `commit_created`, `link_created`,
// `file_written`, `push_spawned`, `worktree_switched`, `repo_advanced`), a flat `Mutation` entry
// (kind, target, optional detail), and `Mutations`, the ordered accumulator a verb call threads
// through everything it performs.
//
// The accumulate-as-you-mutate rule is simple and has no exception: append an entry immediately
// after a primitive observably changed state, never before, and never for a no-op or a refusal.
// destroy.go's eight gate executors auto-record seven of the sixteen kinds this way, since every
// one of them already funnels through the one chokepoint the Fabric Destruction Chokepoint
// Invariant names; the remaining kinds have no such chokepoint and are hand-recorded at their own
// success sites instead.
//
// Every mutating entry point owns exactly one recorder: it constructs one via `NewMutations`,
// threads it as a `*Mutations` parameter into everything the call performs (gate executors
// included), and — per the record-survives-the-error-return decision — installs
// `defer func() { res.Mutations = rec.Snapshot() }()` immediately after construction, so the
// record reaches the caller through every return statement, including an existing
// `return XResult{}, err` early-return site that predates this mechanism and was never
// individually rewritten.
//
// Every mutating verb's result type embeds `MutationRecord` (mutation.go), exposing the
// accumulated record through one accessor, `Mutated() Mutations`; the read-only verbs' result types
// do not, since nothing was mutated and there is no record to carry.
// There are exactly TWO such types, and which verb each serves is worth stating precisely rather
// than approximately, because the natural guess is wrong in both directions: `StatusResult`
// (status.go, returned by `Topology.Status`) is the **pairs** verb, and `DiffResult` (diff.go) is
// `diff`. The other two read-only verbs have no result type at all — `Fabric.Status`, the `status`
// verb, returns a bare `[]ChangeEntry`, and `List` returns a bare `[]WorktreeEntry` — so there is
// nothing for `destructiveguard_test.go`'s companion table to pin for them, and a reader must not
// conclude from its two rows that the table is under-populated.
//
// At the CLI envelope layer (`internal/fabriccli`), every mutating verb's JSON output therefore
// always carries two fixed keys on top of its own fields, present on both the success and the
// failure path alike: `"mutations"`, always a JSON array (empty, never `null`), and `"partial"`,
// always a bool (`false`, never absent). `partial` derives from exactly one rule — `error != nil
// AND record non-empty` — the shape a caller reads to answer the question `ok` cannot: not "did an
// error come back", but "did this call leave the hub in a state some but not all of the intended
// change landed in". A handler that fails before ever calling its verb (cwd/location resolution,
// `LoadConfig`, an argument usage error) carries neither key, since nothing was mutated and there
// is no result to read a record from — see `internal/fabriccli`'s `envelope.go` for the
// helpers, `okWithRecord`/`errWithRecord`/`errWithRecordFields`, that are this rule's one
// implementation.
//
// A verb whose result carries PER-ITEM outcomes has one further obligation, and `reconcile` is where
// it was learned: an item-level failure must reach the caller as a failure, not only as a field
// inside a success envelope. `runReconcile` used to exit unconditionally through `okWithRecord`, so
// a junction it could not re-point produced `"ok":true`, `"partial":false`, and exit 0 with the real
// reason buried in `pairs[].error` — an unqualified success, to every scripted caller, for a repair
// that did not happen.
// It now emits the same `pairs` array through `errWithRecordFields` whenever any pair carries an
// `Error`, so the per-pair report survives and the verdict is honest.
// `prune` and `cleanup` deliberately do NOT follow: their per-entry `Error` doubles as the
// explanation for a DESIGNED refusal (`Protected`'s "commit them or re-run with --force",
// `Unowned`'s "fabric will not remove it"), so treating it as a failure would report a documented
// outcome as one. The distinction is whether the field means "this verb failed at its job" or "this
// verb is telling you what it deliberately did not do".
//
// One consequence of that rule is a new `ReconcileAction`. `Reconcile` reads `git worktree list`
// once, before its per-pair loop, so a concurrent `remove`/`prune` can delete a pair's directory
// between the enumeration and the iteration that reaches it. That used to fail `readBranch` and be
// reported as `ReconcileActionUnmanagedReported` with `os/exec`'s raw `chdir …: no such file or
// directory` as its `Error` — a verdict meaning something else entirely, and, once a per-pair
// `Error` drives the exit code, an ordinary concurrent teardown that would fail every enclosing
// reconcile. `ReconcileActionVanishedMidWalk` names the race instead and sets no `Error`, because
// nothing failed to reconcile: the pair simply stopped existing.
//
// See CONSTRAINTS.md's Mutation Record Invariant for the machine-enforced half of this rule, and
// `cmd/lyx/destructiveguard_test.go`'s `TestMutationRecord_FabricengineProductionSource` for the
// guard itself.
//
// # The fabric vocabulary rule
//
// In production code, the tokens `weft`, `warp`, and the fabric-sense phrase form of `warp` (`warp
// repo`, `warp worktree`, `warpBranch`, …
// — never the bare word, which is ordinary English elsewhere in the repo) may appear only in the
// owner set: `internal/fabricengine` (this package, which implements the illusion),
// `internal/fabriccli` (fabric's own CLI, which exposes the weft to an operator deliberately),
// `internal/weftname` (the `-weft` suffix leaf), `internal/gitkit` (the test-fixture leaf that
// builds real paired worktrees), `internal/hubforge` (the repo-wide hub-fixture factory, which
// builds every hub fixture in the repo through `fabriccli.CloneAndWire` rather than assembling a
// stand-in by hand, and therefore names both sides), `internal/boardengine` (the pre-existing board
// carve-out, since board lives at `weft:main`), `internal/configsync` (string literals and
// comments, never identifiers, for the on-disk legacy config filenames `warp.yaml`/`weft.yaml`).
// `internal/hubforge` also sits in the narrower `weftname`-import subset alongside
// `internal/fabricengine`, `internal/fabriccli`, and `internal/gitkit`.
// `tools/` and `sandbox/` are deliberately NOT in that owner set: the enforcement walk covers
// `internal/` and `cmd/` only, so an owner entry for them would be a rule that never matches —
// their vocabulary (naming the real `lyx-test-weft`/`lyx-fabric-test-weft` GitHub repos) is a
// review obligation instead. See CONSTRAINTS.md's Fabric Vocabulary Invariant for the authoritative
// list.
// `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) machine-checks
// identifiers, string literals, and comments in every production `.go` file plus the embedded agent
// prompt templates;
// `CONSTRAINTS.md`'s Fabric Vocabulary Invariant records the rule in full, including the phrase-based
// `warp` predicate and the review-only prose-doc split between a doc explaining fabric's own
// mechanism (which keeps the vocabulary) and a doc describing a consumer module's behaviour (which
// rewords).
// As an owner file, this doc comment — and every other file in this package — keeps `warp`/`weft`
// vocabulary freely when explaining the mechanism the illusion sits on.
//
// # The destruction chokepoint
//
// `destroy.go` is the one file in this package permitted to perform a destructive primitive —
// `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D`, `fslink.Remove`, and a warp
// checkout's `ResetHard` — and every one of them runs its shared four-check pipeline first.
// See `CONSTRAINTS.md`'s Fabric Destruction Chokepoint Invariant for the rules;
// this section is the rationale the invariant deliberately omits.
//
// **Why a chokepoint at all.**
// Eight data-loss defects across five review rounds were one shape, not eight mistakes: a
// destructive operation acting on a path it does not own, or destroying it without checking
// whether there is work there to lose.
// Every one of the roughly 28 destructive call sites this slice consolidates already had the
// freedom to write its own containment check, its own ownership check, its own dirtiness probe —
// and that freedom, exercised inconsistently 28 separate times, is what produced the class.
// A chokepoint does not add a check nobody thought of;
// it removes the freedom to skip one.
//
// **Why the gate executes rather than approves.**
// A gate a caller consults and then acts on independently is advice, not enforcement — the
// caller can still reach `os.RemoveAll` directly, and nothing distinguishes "checked, then
// destroyed" from "destroyed". `destroy.go`'s executors (`removePath`, `removeGitWorktree`,
// `removeLink`, `repointLink`, `deleteBranch`, `resetHardTo`) run the pipeline and then perform
// the primitive themselves, so the two can never come apart. This is also what makes the bypass
// guard meaningful: a raw call to any of the five primitives is mechanically bannable everywhere
// else in this package precisely because there is no legitimate reason for one to exist there —
// the gate is not one way to destroy something, it is the only way.
//
// **Why ownership is a closed enum with no caller-supplied predicate.**
// A `func() (bool, string)` ownership parameter would let a call site declare "trust me, this is
// mine" — exactly the freedom that produced the class in the first place, relocated one layer
// down rather than removed. Every ownership kind is instead resolved by the gate itself, against
// a fixed, finite set of predicates it owns (`resolvePathOwnership`, `resolveBranchOwnership`):
// a call site picks which kind applies, never what the kind checks.
// A caller-supplied predicate, or an open interface with per-kind implementations, would both
// re-admit the escape hatch — an interface doubly so, since Go interfaces are open sets unless
// sealed with an unexported method, so closure would have to be re-established rather than
// assumed.
//
// **Why dirtiness scope is a caller declaration, not a primitive-derived default.**
// "Is there work here to lose" does not have one right scope: `ResetHard` and a linked-worktree
// removal both discard tracked changes, so both probe tracked-only (`dirtyScopeTracked`) and
// leave untracked debris alone;
// a stale-pair removal takes the whole directory with it, so it probes tracked and untracked
// alike (`dirtyScopeAll`).
// `prune.go`'s two call sites are the worked example: `removeStalePair`'s worktree teardown
// declares `dirtyScopeTracked`, matching `pull.go`'s `ResetHard` scope exactly, while
// `weftwiring.go`'s `removeWeftWorktree` declares `dirtyScopeAll`, matching
// `refuseDirtyWeftWorktree`'s pre-existing scope.
// Deriving the scope from the primitive itself — "a removal is always untracked-inclusive" —
// would have silently widened the tracked-only sites and opened a new data-loss path: git's own
// untracked-file refusal on `git worktree remove` would then route into a directory-removal
// fallback with no equivalent protection.
// Scope is therefore a caller-declared member of a closed sum type
// (`dirtyScopeTracked`/`dirtyScopeAll`/`dirtinessNA`), with every site keeping the scope it
// already had, rather than a property the primitive silently picks.
//
// **Why the dirtiness probe lives beside the gate, in `dirtiness.go`, rather than inside
// `destroy.go`.**
// The gate calls it;
// read-only callers (`warpclean.go`'s exported `Clean`, `reconcile.go`'s board-status check,
// `checkout.go`'s pre-switch check) call it directly, bypassing the gate entirely, because
// running `git status --porcelain` destroys nothing and gating it would be nonsense.
// Keeping the probe out of `destroy.go` is what lets the bypass guard's allowlist mean "the only
// file that destroys," not "the only file that also happens to run `git status`" — folding the
// probe into the gate file would muddy the one property that file's contents exist to keep
// precise.
//
// **Why containment resolves symlinks, and why it stops short of the final component.**
// The check was purely lexical — `filepath.Rel` over the nominal strings — until fabric's R2
// crucible round planted a symlink at `<Hub>/_launchers/<slug>` and watched `removeLaunchers`'
// gated `removePath` delete two files outside the hub, report `ok:true`, and record the removal
// against hub-relative paths that were never the inodes removed.
// A lexical comparison answers a question nobody is asking: it proves the SPELLING of the target
// sits under the SPELLING of the container, while every destructive primitive acts on the inode
// those spellings resolve to.
// So both sides now go through `filepath.EvalSymlinks` (`ancestors.go`'s `resolveAncestorSymlinks`),
// with an ancestor-walk fallback for the ordinary case of a target that does not exist yet.
// The target's own final component is deliberately left unresolved (`containmentPath`): every
// junction the gate removes is a link living inside the warp worktree and pointing into the weft
// one, so resolving the leaf would relocate the target into weft and make the warp-worktree
// container refuse every legitimate unwire — the fix would have broken the verb it was meant to
// protect.
// `ownedUnderGeometryRoot` resolves the same way for a reason worth naming: it is the ONLY
// ownership kind with no independent authority to cross-check a target against. The two
// worktree-shaped kinds compare against git's own worktree registration and the two link-shaped
// kinds against `fslink.RawTarget`, both of which already carry resolved paths — which is exactly
// why the geometry-root site was the one that fell and the others did not.
//
// **The gate's DIRTINESS check is not atomic with its act, and that is a stated limit rather than an
// oversight.**
// `checkPathDirtiness` runs `git status --porcelain` and returns; the executor then performs the
// primitive. No lock spans the two, so a write landing in that window is destroyed. The exposure is
// narrow by construction — `removeGitWorktree` re-checks through git itself, `resetHardTo`
// delegates to git, and the only recursive-removal sites carrying a real dirtiness scope are the two
// fallbacks that fire *after* git has already declined the removal — but a reader must not take
// "the gate executes rather than approves" to mean the probe and the act are one transaction.
// Closing the window would need a lock held across probe and act at every executor, which is a
// larger claim about every future call path than the residual risk warrants today.
//
// **The gate's CONTAINMENT check, by contrast, IS bound to its act, because R3's review proved it had
// to be.** The containment check resolves symlinks at one instant (`refuseUncontainedPath`,
// `containmentPath`), and a symlink planted at an intermediate segment of a gate target — dangling
// when the check ran, so the check short-circuited on an absent target, then flipped
// live-and-escaping before the executor's own `os.Lstat`+unlink — carried a gated `remove --force`
// outside the hub anyway, a real out-of-hub deletion reported as (partial) success. The two
// arbitrary-path executors (`removePath`, `removeLink`) therefore no longer act on the nominal path:
// `removeContainedPath` removes through an `os.Root` rooted at the gate's declared container, so each
// path component is resolved and unlinked as one `openat` chain that atomically refuses any component
// escaping the container at removal time, while still removing a final-component junction link as a
// link. The containment check stays as defense-in-depth; the rooted act is the actual window-closer,
// and unlike the dirtiness window it needs no lock — the atomicity comes from the kernel's `openat`
// escape refusal, not from a span held across two calls. `removeGitWorktree` and `resetHardTo`
// delegate their act to git, which re-validates at its own instant, so the containment binding lives
// where the arbitrary-path removals are.
//
// **The two CREATE-side minters bind creation to a rooted act the same way, because R5's review proved
// the delete-side asymmetry was live on the create side too.** `createExclusiveDir` and
// `createGitWorktree` are the gate's only path/worktree minters, and both once resolved their target's
// nominal path and let the create follow a symlink planted there. `createExclusiveDir` now creates its
// leaf through an `os.Root` rooted at the parent, so an intermediate-symlink escape is refused at
// `mkdir` time exactly as `removeContainedPath` refuses one at `unlink` time. `createGitWorktree` can
// not be rooted the same way — `git worktree add` is a subprocess that resolves its destination
// argument itself and FOLLOWS a symlink there, writing a whole worktree wherever it points — so
// `containedWorktreeAdd` closes it with a two-level staging structure plus two fail-closed containment
// checks. git's WRITE targets a leaf named after the slug inside an unguessable 0700 random PARENT
// directory created through an `os.Root` rooted at container; the parent's unguessability and mode deny
// a different-UID planter, and its being a real intermediate directory makes `os.Root.Rename` refuse a
// parent-swap (it refuses a symlink at an intermediate SOURCE component). R5 stopped at that rename,
// but `os.Root.Rename` renames a symlink standing at the SOURCE's own final component as a link rather
// than refusing it, so a staging-LEAF symlink planted during git's write was still renamed onto the
// target — an out-of-hub worktree reported as success (R6's review reproduced this 12/12 against a
// same-UID observing planter). R6 binds containment to the act: after git writes, `stagedWorktreeContained`
// confirms the staging leaf is a real directory reached without traversing a symlink, and after the
// rename it confirms the placed target is too; either failure cleans up the escaped worktree and staging
// debris and returns an error, then `git worktree repair` fixes git's registration on the success path.
// A same-UID (or root) planter actively racing the add can still make git transiently write a checkout
// into a directory it already controls — unpreventable by any staging location, since such a planter can
// substitute any path fabric writes to — but the operation is never REPORTED as success and never leaves
// the target a dangling out-of-hub symlink, which is the create-side twin of the delete-side guarantee.
//
// **The two hub-level container WRITERS bind their writes to a rooted act the same way, because R7's
// review proved five rounds of create-side pressure had never looked at them.** `writeLaunchers`
// (launchers.go) and `createPortal` (portals.go) write into `<hub>/_launchers/<AnchorRel>/<slug>` and
// `<hub>/_portals/<AnchorRel>/<slug>` — hub-level structural containers, not the freshly-created worktree —
// and both once wrote through a raw primitive that resolved and followed the container path itself:
// `writeLaunchers` via `os.MkdirAll`+`os.WriteFile`, `createPortal` via `fslink.CreateDirLink`, whose own
// parent-mkdir follows a planted symlink. A STATIC symlink planted at the `<slug>` leaf OR the
// `_launchers`/`_portals` container (no race, no observation) carried the write OUTSIDE the hub — executable
// launcher content to an attacker-chosen path, a portal junction into an out-of-hub directory — while `add`
// reported `ok:true` with a mutation record naming the hub-relative path the bytes never reached, the exact
// delete-side M3 false-success shape and strictly easier to exploit (no timing). This is the same asymmetry
// the delete side already closed for `removeLaunchers`/`removePortal`, live on the create side of the same
// two verbs. Both now write through an `os.Root` rooted at `l.HubPath` (the true containment boundary;
// `_launchers`/`_portals` are never legitimately symlinks): `writeLaunchers` roots every mkdir/write there,
// and `createPortal` materialises the link's parent chain through `ensureContainedLinkParent` before handing
// the leaf to `fslink`, so any component escaping the hub is refused at write time. The remaining raw writes
// in the package are NOT this class — they target a git-owned `.git/…` path (hook.go, gitexclude.go) or a
// worktree/board directory a contained minter (`createExclusiveDir`/`containedWorktreeAdd`) brought into
// being in the same call (clone.go, warpbinding.go, weftgit.go, junction.go's weft-target materialisation),
// where only a post-creation same-UID race, never a static pre-plant, could redirect them — the same accepted
// residual class as the gate's dirtiness window — and each is an allowlisted, reasoned entry in the write-side
// guard rather than a routed write. See CONSTRAINTS.md's Fabric Write-Side Containment Invariant and
// `cmd/lyx/uncontainedwrite_test.go`'s `TestNoUncontainedWrite_FabricengineProductionSource` for the guard.
//
// **The launcher/portal teardown path was the last corner still holding the old shape, because every prior
// round fixed it from the outside and none audited it on its own terms.** R3 rerouted the gate's two
// arbitrary-path executors and R7 rerouted the two hub-level container writers, but three things in that one
// teardown path were left standing, and R8's review found all three. First, `removeLaunchers`' launcher-DIRECTORY
// removal ran the gate's `checkPathRequest` and then acted with a raw, unrooted single-entry removal of the
// nominal path — a THIRD arbitrary-path removal, carrying exactly the check-then-act window R3 closed for the
// other two, with the same false-success shape (the record naming a hub-relative path the removed inode never
// was). It could not use `removePath`, whose directory branch is `RemoveAll` and would destroy operator content
// beside the launchers, so it now calls `removeContainedPath` directly with `recursive` false: the non-recursive
// branch is `os.Root.Remove`, which the OS refuses on a non-empty directory exactly as before, so the
// preservation property is untouched while the unlink becomes one rooted `openat` chain. Second,
// `pruneEmptyAncestors` — which runs immediately after, on both teardown paths — related its walk to the
// container with a purely lexical `filepath.Rel` and removed the nominal path, so with a multi-segment
// `AnchorRel` a symlink planted at an intermediate segment destroyed an out-of-hub directory outright, with no
// race needed at all; its removal is now rooted at the sweep's stop directory, and the lexical `Rel` survives
// only as the loop's termination condition, where it can stop the walk early but never widen it. Both files are
// consequently OFF the destructive guard's allowlist, so a raw removal reintroduced in either now fails the
// guard rather than inheriting a reason written for a call site that no longer exists.
//
// Third, and separate from containment: `refuseUncontainedPath`, the pre-gate guard both teardown helpers open
// with, returned a bare `fmt.Errorf`. Every one of its four best-effort call sites (`Remove` twice, `Prune`
// twice) wraps the call in `surfaceRefusal`, which by design discards anything that is not a
// `*destructiveRefusal` — so the one refusal class that must never be dropped was dropped. A STATIC symlink at
// the `_launchers` container (no race) made the whole teardown refuse correctly, nothing escaping, while
// `lyx fabric remove` reported `ok:true`, `partial:false` and exit 0 with the pair's launcher scripts still on
// disk and no reason for the operator to reach for `reconcile` — R2's M2 dishonest-success shape, relocated onto
// the teardown path. The guard now returns the gate's own refusal type, so all four sites propagate it with no
// call-site change and `RefusalOf` answers for it like any other. The lesson worth keeping is that a refusal's
// TYPE is part of its contract here: a containment check that refuses correctly but is not the type the
// best-effort wrapper propagates is, from the operator's side, indistinguishable from no check at all.
//
// **Why the two token-carrying ownership kinds exist, and the honest limit of what backs them.**
// `ownedFreshlyCreatedPath`/`ownedFreshlyCreatedWorktree` let a rollback site prove "the gate
// itself created this, moments ago, in this same call" — the fabric-hub bootstrap teardown and
// `Add`'s worktree rollback both need exactly that, and nothing weaker would do: a rollback site
// that could destroy anything matching a *shape* rather than a specific gate-minted grant would
// reopen the same freedom every other ownership kind exists to close.
// `createdToken`'s two producers, `createExclusiveDir` and `createGitWorktree`, are declared only
// in `destroy.go`, but being unexported does not by itself stop a same-package composite literal
// — `createdToken{}` compiles anywhere in `package fabricengine`.
// The property that a site cannot declare this kind for a path the gate did not create therefore
// rests on the bypass guard's `createdToken{` ban, not on Go's type system;
// a reader who believes the type system alone enforces it will eventually write one.
//
// # The correspondence index's write path
//
// This section, together with "The destruction chokepoint" above, closes out the fabric crucible
// campaign's four follow-up slices (12-15), all now landed.
//
// **Why `record()` is single-phase.**
// `corrIndex.record` used to compose its upsert from an in-memory snapshot loaded earlier under a
// read lock already released by the time the write happened — a two-phase load-then-write window
// a concurrent writer could land inside, clobbering it. `state.UpdateJSON` closes that window: it
// re-reads the on-disk base under the same exclusive lock it writes under, so `record()`'s upsert
// composes from a base no other writer can have superseded mid-call.
//
// **Why not "re-read under the write lock `record()` already takes."**
// That was the preferred shape on paper, but `record()` takes no write lock in its own frame:
// `state.WriteJSON` acquires and releases the lock internally, and `lock.AcquireWriteLock` opens a
// fresh `flock` handle on every call, so a nested acquire from `corrindex.go` self-deadlocks rather
// than serializing.
//
// **Why `RebuildIndex` and `refreshCorrIndexAfterSwitch` were left alone.**
// Giving them the weft write lock is a claim about every call path that every future caller must
// preserve, not a local fact `corrindex.go` can establish on its own, and it would still leave
// `record()` two-phase against any writer that does not take the weft lock.
//
// **Why `refreshCorrIndexAfterSwitch`'s unlocked `os.Remove`-then-rebuild window is designed, not
// defective.**
// The discard is intended to drop cross-branch entries that would otherwise keep passing
// `SHAExists`, so a concurrent `record()` losing its entry there is the intended behaviour, not an
// oversight.
//
// **The residual window, by name.**
// `RebuildIndex` is itself two-phase — `scanWarpSHATrailers` reads git, then `state.WriteJSON`
// writes, with the scan outside the file's lock — so the interleaving scan → `record()` writes →
// rebuild writes still loses the recorded entry.
// This is accepted, not overlooked: LOW severity and self-healing, because the weft commit
// trailers are the sole source of truth and the index is an explicitly rebuildable cache, so the
// worst observable effect is one spurious `no_weft_correspondence` from `lyx fabric diff` that a
// re-run clears.
// `record()`'s side of the race is closed; the reverse direction, against `RebuildIndex`'s own
// scan-to-write span, is not.
//
// # The merge surface
//
// **The two verbs, and why there are two.** `MergeIn(source)` (merge.go) merges `source` into the
// current pair's warp checkout, in the task worktree where a conflict is meant to be resolved by
// hand. `Merge(source, opts)` merges into a target pair's warp checkout — the pair the caller opened
// a separate handle on — squash-capable via `opts.Squash`, and expected conflict-free: any conflict
// there self-aborts the warp side and returns `*ErrMergeInRequired`, since resolving a conflict
// against an already-checked-out target worktree is not something git permits from another worktree
// of the same repo. The two are not one verb with a flag, because their guards, failure modes, and
// worktrees genuinely differ (see `_mill/discussion-meta.md`'s `two-verbs-mergein-then-merge`
// rejection).
//
// **The weft is never a merge participant, in either verb or either direction.** Everything routed
// to the weft belongs to exactly one worktree and one branch, so there is nothing there for a merge
// to reconcile — a merge carries code, and the weft carries system files, not code. Two consequences
// follow directly: `unifyConflictPaths`' weft conflict list is permanently empty, never populated,
// and `fabriccli`'s junction-staging conflict path is unreachable rather than wrong — both are
// retained plumbing (see the `conclude-and-conflict-plumbing-is-retained` decision), not dead code
// waiting to be deleted.
//
// **The recorded merge.** A merge in progress is tracked by a JSON record, `fabric-merge.json`, kept
// beside the correspondence index — never derived from git state, because derivation fails in ways a
// caller cannot recover from: a fast-forward defeats `--no-commit`'s own conflict window, a squash
// merge records no `MERGE_HEAD` at all, and a human running plain git against either checkout can
// leave real merge state fabric never started. That last case is `*ErrForeignMergeState`: every
// mutating merge verb refuses rather than touch it, while `MergeInProgress` — a read-only probe —
// reports `false` for it, since fabric genuinely has no merge of its own in progress.
//
// **The lifecycle quartet and crash recovery.** `MergeIn`/`Merge` start an attempt; `MergeContinue`
// concludes one once every conflict is resolved in the worktree; `MergeAbort` discards one,
// restoring the warp side to its pre-merge SHA — including a side that only fast-forwarded or never
// moved, but never one whose conclude already landed (see below). Every state-changing step
// re-persists the record before it acts, so
// a crash mid-attempt leaves a record a resumed `MergeContinue`/`MergeAbort` can still read and act
// on — there is no window where the record and the checkouts can drift silently out of reach of the
// quartet.
//
// **Not every crash is continuable, and the record says which.** A side's outcome is persisted only
// once that side's `MergeStart` has returned, so a crash before the first `MergeStart` or between
// the two leaves an empty outcome for a side the attempt never reached. `MergeContinue` refuses such
// a record outright with the guard reason `merge attempt did not reach both sides`, *before* landing
// anything: concluding what it can would commit one side of a merge whose other side was never
// started, leaving the pair non-corresponding with no way to finish it. `MergeAbort` is the one
// correct recovery there, and it always works, because it restores from the recorded pre-merge SHAs
// rather than from how far the attempt got.
//
// **And not every crash is abortable, for the mirror-image reason.** The conclude phase lands warp
// first, then weft, so a conclude that fails on the second side — a policy hook, `commit.gpgsign`
// with no key, a full disk — returns `*ErrMergeIncomplete` and deliberately retains the record with
// the first side's commit already in it. Restoring from the recorded pre-merge SHAs there would
// discard a commit that really landed, and in the `MergeIn`-with-conflicts flow that commit carries
// the operator's own hand-written resolutions, reset away under `force: true`. So `MergeAbort`
// refuses such a record with the guard reason `merge conclude already landed`, and `MergeContinue`
// is the one correct recovery — it skips a side whose committed SHA is recorded, so a resumed run is
// idempotent. Idempotency extends to a conclude the record never learned about: a crash between a
// side's `git commit` and the record re-save leaves the commit landed with the recorded SHA still
// empty, and re-running `git commit` there would fail forever on a clean tree. A resumed
// `MergeContinue` detects that shape and adopts the landed commit into the record instead of
// re-committing, so it finishes the states `MergeAbort` refuses to destroy.
//
// **Adoption is a claim, so it demands evidence a refusal does not.** `MergeAbort`'s guard and
// `MergeContinue`'s adoption arm both look at the same ambiguous signal — HEAD has moved off the
// recorded pre-merge SHA and no `MERGE_HEAD` is live — and they must resolve that ambiguity in
// OPPOSITE directions. For the guard, the ambiguity is safe: over-refusing an abort leaves a merge
// stuck in a state an operator can still inspect. For adoption it is not: the signal is satisfied by
// ANY commit landed on that checkout while the record was live, so keying adoption on it alone made
// an operator's plain `git merge --abort` followed by one unrelated commit come back `committed:
// true` naming that unrelated commit, with the record deleted and the merge source still un-merged.
// A silent false success, and the reason the two are deliberately NOT exact mirrors.
// Adoption therefore rests on git's own parentage rather than on HEAD movement. fabric starts a
// non-squash merge with `git merge --ff --no-commit <sourceSHA>` and records that resolved
// per-side SHA in the merge record, so a genuine conclude-commit is a merge commit with EXACTLY two
// parents: the recorded pre-merge SHA first, that recorded source SHA second. Nothing short of all
// of that is adopted.
// The arity is part of the evidence, not a detail of it. Accepting "at least two parents, with the
// source somewhere among them" admitted a commit fabric can never build: an operator who discards
// the staged merge and then merges the recorded source TOGETHER with an unrelated branch —
// `git merge <source> <other>` — lands a genuine octopus whose first parent is the recorded start
// and whose second is the recorded source. Adopting it reported `committed: true` naming that
// commit, recorded correspondence, and deleted the record, while the checkout carried `<other>`'s
// content that no side of this merge brought in and that no `merge_staged` entry accounts for.
// **A squash conclude is never adopted at all.** `git merge --squash` writes no `MERGE_HEAD` and its
// conclude is an ordinary one-parent commit, so it carries no evidence distinguishing it from any
// other commit — there is nothing there to be sure about, and the non-squash predicate is not
// silently inherited. A crashed squash conclude stays honestly stuck: `*ErrMergeIncomplete`, record
// retained. So does a record written by a binary predating the recorded source SHAs; no evidence is
// not satisfied evidence.
// **The commit arm demands evidence too, and for a while it did not.** Adoption is only half of what
// `concludeMergeSides` does. When the adoption arm reports "not landed", the other half runs
// `git commit` — which concludes whatever `MERGE_HEAD` names, whether or not that is the merge the
// record describes. Nothing checked. So an operator who discarded fabric's staged merge with plain
// `git merge --abort` (permitted verbatim: plain git in the warp checkout is theirs) and started an
// unrelated merge instead had fabric commit THAT merge, write its SHA into the record as this merge's
// conclude, report it as a `merge_committed` mutation with `ok`/`committed: true`, pair it into the
// correspondence index, and then delete the record. The merge source stayed un-merged on that side
// while the other side's half landed for good: a permanently non-corresponding pair, a silent false
// success, and no evidence left that it was one. The adoption arm never saw the shape precisely
// because the operator had not committed — HEAD was still on the recorded start with a live
// `MERGE_HEAD`, so "not landed" was the right answer, handed straight to the defect.
// `MergeContinue` therefore carries the precondition `checkout no longer carries the recorded merge`
// (`recordedMergeGoneReason`), aggregated with its other two. A side pending a conclude passes it in
// three ways, and each is a different question:
//   - `MERGE_HEAD` is exactly the recorded per-side source SHA and nothing else — the ordinary case.
//     The whole head SET must be that one SHA, not merely contain it: `git merge --no-commit <source>
//     <decoy>` leaves a MERGE_HEAD whose FIRST entry is the source, and every rev-parsing spelling of
//     the query reports only that first entry, so `gitrepo.MergeHeads` reads the file itself. This is
//     the uncommitted twin of the octopus the adoption arm refuses.
//   - the recorded source is already an ancestor of that side's HEAD — the merge landed, whatever else
//     has happened since, so there is no false claim left to prevent. This is what keeps the
//     precondition from wedging the crash the adoption arm exists to finish, a conclude with a second
//     merge started on top of it, and a source merged onto a wrong base, all three of which `MergeAbort`
//     also refuses.
//   - no merge is live AND the checkout is clean — `git commit` fails on its own there, so the honest
//     `*ErrMergeIncomplete` already comes back and refusing early would only blind the adoption-evidence
//     tests, every one of which builds exactly that state. The same state with tracked dirt is NOT
//     exempt, because `git commit` succeeds there and lands an ordinary one-parent commit of whatever
//     the operator staged.
//
// A squash record is exempt from the whole precondition, for the reason adoption gives: `git merge
// --squash` writes no `MERGE_HEAD`, so there is no evidence to demand, and refusing every squash
// `--continue` would break the ordinary squash flow. That residual is real and stated rather than
// papered over — a squash record whose side was discarded and restaged by hand still concludes whatever
// is staged. So is an empty recorded source SHA, from a record written by a binary predating them.
// The refusal never wedges a pair on its own: in the state it fires on, the recorded source is nowhere
// in that side's history, so `MergeAbort`'s own `concludeLandedReason` still passes for a side sitting
// on its recorded start and the abort route stays open.
//
// Within those bounds the two refusals still cover every half-finished attempt between them, and
// neither destroys anything.
// The precondition is deliberately wider than the recorded conclude SHA. A side counts as possibly
// concluded when its recorded SHA is set **or** its recorded outcome is `staged`/`conflicted` and its
// HEAD has moved off its recorded pre-merge SHA — because the record learns a conclude SHA only after
// the commit, the `CurrentSHA` read, and the record re-save have all succeeded, so a failure at
// either of the last two leaves a landed commit the record never mentions. Reading HEAD closes that:
// an `up_to_date` side is never concluded and cannot move, a `fast_forwarded` side moved legitimately
// and is still reset, an empty-outcome side never started, and only a `staged`/`conflicted` side can
// have had a commit put on it by anything but the conclude.
// If the underlying git failure cannot be fixed by retrying, plain git in the two checkouts is the
// last resort — but it has to produce the same commit fabric would have: resolve each unfinished
// side and commit it while git's own `MERGE_HEAD` is still live (`git commit --no-edit`, never
// `git merge --abort` first), so the resulting commit really is a merge of this merge's source.
// `MergeContinue`'s adoption arm then accounts for the hand-landed commits and clears the record.
// A side whose `MERGE_HEAD` is already gone is put back first — `git reset --hard` to that side's
// recorded `warp_start`/`weft_start`, then `git merge` its recorded `warp_source`/`weft_source` —
// because adoption checks the FIRST parent too, so a conclude landed on top of some other commit is
// a merge of a different base and is not adopted. A squash attempt has no hand-landed route at all,
// since adoption never accepts a one-parent conclude. Plain git alone can
// never finish the job: the record lives in the weft gitdir where no git command touches it, and
// while it exists every guarded sibling verb keeps refusing.
//
// **The not-synced precondition is decided twice, because once is too early.** `Merge` refuses a
// target genuinely diverged from its own upstream with the guard reason `branch not synced to
// upstream`. The guard stage's own half of that (`syncedToUpstreamReason`) resolves `@{u}` from
// whatever remote-tracking state the checkout already carries — nothing in the guard stage fetches,
// by design, since the guard stage mutates nothing — so a divergence created by someone else's push
// that this checkout has not fetched yet is simply not visible to it: `@{u}` still names a commit
// that IS an ancestor of HEAD, the side classifies as merely ahead, and the guard passes. `Merge`
// then fetches twice on its way in (`resolveMergeSources`, then the pre-merge sync step), so the
// divergence becomes knowable inside the same call that just decided it was absent. Acting on the
// stale answer merged straight over a diverged target and returned `ok` with `committed: true`; the
// operator found out at push time. So the pre-merge sync step re-decides the predicate on post-fetch
// knowledge: per side it classifies equal / behind / ahead / diverged EXHAUSTIVELY, fast-forwards
// `behind`, no-ops `equal` and `ahead`, and refuses `diverged` with the same reason string the guard
// stage would have used. The guard stage stays as the cheap pre-lock fast path that refuses without
// taking the write lock or touching either checkout — which is a real difference, not a duplication:
// a refusal there mutates nothing, while a refusal from the sync step can already have
// fast-forwarded the other side.
//
// **The warp checkout must be on a branch.** A merge verb refuses with the guard reason `checkout is
// not on a branch` while the warp checkout has HEAD pointing straight at a commit. A conclude-commit
// landed on a detached HEAD is reachable from no ref and disappears at the next checkout, and the
// verb has already deleted its own record on the way out by the time that is discovered, so
// `MergeAbort` cannot put it back — refusing before the attempt starts is the only point at which
// that is still recoverable. This matters in practice because `Fabric.CheckoutDetached`/
// `RestoreBranch` exist and webster's integration bisect drives them
// (`internal/websterengine/integration.go`). The weft's own detachment no longer refuses anything: it
// is not a merge participant, so a detached weft HEAD cannot produce the unreachable-commit shape
// this precondition exists to prevent, and the guard set that used to evaluate both sides
// unconditionally — pairDirtyReason, detachedHeadReason, syncedToUpstreamReason, and
// resolveMergeSources' own refusal arm — now evaluates the warp side alone throughout; the weft has
// lost its power to block a merge, on top of having already lost its participation in one.
//
// **What the result flags mean.** `MergeResult.Committed` reports whether the pair now carries this
// merge's conclude-commit, and `AlreadyUpToDate` whether the attempt found the warp side already
// carrying the resolved source. Both are read off the merge-state record's own fields rather than
// hardcoded per return site, which is what makes them answer honestly in the two cases that used to
// lie: a merge that fast-forwarded both sides fabricates no commit at all and reports `Committed`
// false, and a call that finds the work already done *after* taking the write lock — the loser of a
// race the unlocked pre-lock probe deliberately does not close — reports `AlreadyUpToDate` true,
// which is what a strictly sequential run of the same two calls reports.
// "Every return site" includes `MergeContinue`'s, which is the one that resumes such a record after
// a crash: it derives both flags from the same fields rather than answering `AlreadyUpToDate` false
// by construction, so a resumed call and a sequential one agree.
//
// **"Already up to date" means git had nothing to do, not that the trees already agreed.** A merge
// whose source is not an ancestor of HEAD but whose result tree equals HEAD's own tree — the shape
// a cherry-pick, backport, or duplicated hand-edit produces — is a real merge: git writes
// `MERGE_HEAD` and `git commit` would land a proper two-parent commit for it. `gitrepo.MergeStart`
// classifies it `MergeStaged` on the `MERGE_HEAD` probe, so fabric concludes it and reports
// `Committed` true. Treating it as up-to-date was a defect, not a shortcut: fabric reported a clean
// no-op, deleted its own record, and left a live `MERGE_HEAD` in both checkouts that no fabric verb
// would then clear — `MergeAbort` included, since with the record gone the state reads as foreign.
// The squash form is the one case where an empty result really is nothing to do: `git merge
// --squash` writes no `MERGE_HEAD` and has no commit to make, so both sides report `up_to_date` and
// `AlreadyUpToDate` comes back true from the derived field, with the pre-lock probe having said
// otherwise. That is a single-process, race-free route to the derived flag, and it is what
// `TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord` exercises.
//
// **Conflict reporting.** Conflicted paths surface as one flat, lexically sorted, deduplicated list
// of unified, worktree-relative paths — never raw per-repo paths, which would leak the warp/weft
// split, and never absolute paths, which is not what `git merge` hands an operator. A path either
// side's conflict resolves to something outside the single visible worktree tree is unmappable, and
// that self-aborts the whole attempt with `*ErrUnmergeableState` rather than reporting a path that
// would mislead the operator about where to look. `MergeResult.Conflicts` is empty, never nil, on
// every path that carries no conflict, so a consumer's JSON never has to distinguish `[]` from
// `null`.
//
// **Resolving a conflict takes three steps, and the middle one is not skippable.**
// `MergeContinue`'s conflict guard is an INDEX probe (`ConflictedFiles()`), so editing a conflicted
// file to remove its markers does not on its own let the merge conclude — the path has to be marked
// resolved. `MergeStageResolved` is that step: it takes the same unified paths `Conflicts` reported
// and stages each on whichever side's index actually lists it unmerged.
// It is not merely a convenience for callers that would otherwise shell out to `git add`, because for
// a weft-side path there is no `git add` that works. A conflict under a wired junction name — every
// `_lyx/…` conflict — lives in the weft checkout, reachable from the single visible worktree only
// through the junction, and git refuses to stage through it (`pathspec … is beyond a symbolic link`).
// So a merge whose conflicts land on the weft side is completable ONLY through this verb.
// A refusal for that reason names the paths, and it did not always. `MergeContinue` reads both sides'
// `ConflictedFiles()` to decide the guard and then threw the answer away, returning
// `unresolved conflicts remain` and nothing else. That is un-actionable in the one flow that reaches
// it: an operator who staged some of the reported paths and not all of them cannot re-run `merge-in`
// to reprint the list (a merge is already in progress), `lyx fabric status` reports a remaining
// weft-side conflict as an ordinary weft change indistinguishable from any other, and plain
// `git status` in the visible worktree does not see it at all, because it lives across the junction.
// The only route back to the list was raw git inside the weft checkout — the one place the Fabric
// illusion says an operator never has to look, which is the same argument that made `merge-stage` a
// shipped gap. The refusal now carries the still-unresolved paths, mapped through the same
// `unifyConflictPaths` the conflict result uses, and the CLI reports them under `unresolved` rather
// than `conflicts`: `conflicts` is the documented discriminator between a conflict result and a hard
// failure, so it stays exclusive to the former.
// That listing is best-effort and never replaces the refusal: a geometry read that fails, or a
// remaining path that maps nowhere in the visible tree, yields no list rather than a partial one that
// would mislead the operator about what is left.
// That made it a shipped gap rather than an internal detail while the verb had no CLI surface at all
// and existed solely for `internal/mergeresolve`: an operator following `lyx fabric merge-in`'s own
// help reached a `merge --continue` that refused forever, with the only escape being raw git inside
// the weft checkout — the one place the Fabric illusion says they never have to look. `lyx fabric
// merge-stage <path>...` is the surface, and the CLI's help now names the full resolve → stage →
// continue sequence rather than the two-step one that cannot finish.
//
// **The one path-separator reconciliation, and why its guard is shaped oddly.** The visible-tree
// membership test compares two values that do NOT arrive in the same separator convention: the
// conflicted path is git's own output and is always forward-slash, while `AnchorRel` comes back from
// `lyxcwd.ValidateAnchorRel` as an OS-separator path. On Windows a multi-segment anchor recorded as
// `apps/backend` therefore arrives as `apps\backend`, and the joined prefix never matches what git
// reports — every `_lyx` conflict under such an anchor is then classed unmappable and the whole
// merge self-aborts with `*ErrUnmergeableState`. Converting the anchor's separator to `/` is what
// closes that, and it must convert the OS separator specifically, not every backslash: on a POSIX
// host a backslash is an ordinary filename character and a directory really can be named
// `weird\name`.
// The conversion is written against an explicit separator argument rather than calling
// `filepath.ToSlash`, purely so it can be tested. `filepath.ToSlash` is the identity function
// wherever the OS separator already is `/`, so on every host this project builds on, deleting it
// changes no observable behaviour and no test can fail — which is exactly how a Windows-only fix
// rots unnoticed. The separator-explicit form lets a POSIX test drive the Windows spelling directly.
// One atom stays beyond runtime reach here — the `os.PathSeparator` argument at the entry point,
// indistinguishable from a hardcoded `/` on this host — and is pinned by source inspection instead,
// with that limitation stated rather than papered over. No Windows host has existed at any point in
// this module's history, so this is the strongest available proof, not a substitute for one that was
// skipped.
//
// **A conflict result is not a failure, and a script tells them apart by the envelope.** At the CLI
// both a conflicted merge and a hard error exit 1 with `"ok": false` — the shared envelope has no
// third outcome and no distinct exit code. The discriminator is the payload: a conflict result, and
// only a conflict result, carries a `conflicts` array (`errConflictsWithRecord`), and it reports
// `partial: false` even with a non-empty mutation record, since the engine returned no error.
//
// **SHA-labelled conflict markers.** Every merge names a SHA, never a branch, in its conflict
// markers on both sides — resolving the weft side's own SHA independently, rather than reusing the
// warp source's marker text, so a marker never leaks the fact that a second repo exists underneath.
//
// **Sibling refusals and the write lock's scope.** Exactly four sibling verbs carry an explicit
// refusal, and they are the four whose write would corrupt or be corrupted by a live merge:
// `Commit`, `Pull`, `Topology.Checkout` and `Topology.Remove` all return `*ErrMergeInProgress` while
// a merge record exists (`Commit` additionally refuses foreign git-level merge state with no record,
// as `*ErrForeignMergeState` — fabric has no merge of its own in progress there, and the foreign
// error's plain-git advice is the one that actually clears the state),
// so a pair mid-merge cannot be pulled, committed, or torn down out from under the resolution in
// progress. `Remove` guards two distinct subjects: the pair being removed being mid-merge itself,
// and — via `mergeSourceInFlight` — some *other* pair in the hub being mid-merge on this pair's
// branches, which would otherwise delete the weft branch that merge is resolving against.
// That record-based refusal has one window it cannot cover, and the write lock is what covers it
// instead: `Merge`'s pre-merge sync step mutates both checkouts BEFORE the record is written, so
// while it runs the four sibling verbs see no record and do not refuse. `Merge` therefore takes the
// write lock ahead of the sync rather than after it. `MergeIn` has no sync step and mutates nothing
// before its record, so it is the one verb that can still defer acquisition — though what it (and
// `Merge`) checked before acquiring is re-verified under the lock: the record's absence is
// re-checked (a record written by another process mid-wait refuses with the in-progress guard
// reason instead of being silently overwritten), foreign git-level merge state and pair dirtiness
// are re-probed (a human's mid-wait plain-git merge would otherwise read as this merge's own
// conflict — and be force-reset by `Merge`'s conflict path — while mid-wait tracked dirt would be
// reset away by the genuine-`MergeStart`-error self-abort; the guard stage decided both before the
// verb's fetches and the lock wait, a window of real seconds), and the recorded pre-merge SHAs are
// read under the lock, never before it, so a commit landed by the lock's previous holder becomes
// the recorded start rather than something `MergeAbort` would reset through. The residual window
// between those re-checks and `MergeStart` itself stays open — no re-check closes a TOCTOU against
// an external actor — but the seconds-wide parts are covered. `MergeContinue` and `MergeAbort` go
// further and acquire the lock before reading the record or evaluating any guard at all — the
// conclude-landed guard in particular must be able to see a conclude that lands while the abort
// waits, or the abort would destroy it from a stale answer.
// The rest of the mutating surface is deliberately unguarded and safe for stated reasons rather than
// by omission: the push family (`PushWeft`, `PushWarpAt`, `CoalescePushBothAt`, `SpawnDetachedPush`)
// pushes a committed branch tip an uncommitted merge has not moved; `Cleanup` cannot touch a
// checked-out weft branch and a mid-merge pair is materialized by definition; `Prune` only acts on a
// pair whose warp worktree is already gone; `Reconcile`, the junction verbs and the hook installer
// touch filesystem links, never an index or a ref; `Add` builds a new pair off a branch tip;
// `RebuildIndex` rewrites an explicitly rebuildable cache; `RecordCorrespondence` must stay
// unguarded, since the merge verbs call it themselves while their own record is still live;
// `MergeStageResolved` carries the foreign-state refusal every mutating merge verb carries, but no
// record precondition and no lock, because with foreign state refused it can only ever write DURING
// a merge (no fabric merge and no foreign merge means no path is conflicted, so the call errors
// before staging anything, and while a fabric merge IS in progress the guarded siblings already keep
// every other fabric writer out — see its own godoc in mergestage.go for the full argument, and note
// that "no merge in progress" and "nothing conflicted" are not the same condition, which is exactly
// what the foreign arm covers); and
// `ResetHard`'s `force: false` plus tracked-dirtiness gate already refuses against a merge worktree,
// which is dirty by definition.
// `CheckoutDetached`/`RestoreBranch` are the one knowing exception — raw primitives driven only by
// webster's integration bisect, and left unguarded because a merge-record probe does not belong in a
// primitive this doc classes as raw. The attached-HEAD precondition above is *not* what closes them:
// that precondition stops a merge from starting while a checkout is detached, and says nothing about
// detaching a checkout that is already mid-merge, which is the order these two primitives can
// produce. What actually closes the dangerous part is git: `checkout --detach` refuses outright
// while unmerged index entries exist, so the long window — an operator sitting on conflict markers —
// is unreachable. The narrow window that stays open is the resolved-but-not-concluded one (index
// clean, `MERGE_HEAD` live, record live), where the detach succeeds and drops `MERGE_HEAD`, stranding
// the record. That is a known, accepted hazard belonging to the caller that drives the bisect, not to
// the merge primitive. The combined `.weft/weft.write.lock`
// covers only the mutating steps of a merge call — `Merge`'s pre-merge sync step, starting the
// attempt, and concluding it — never
// the resolution window itself: an operator may take arbitrarily long editing conflict markers
// between `MergeIn`/`Merge` and `MergeContinue` with the lock released, exactly as plain git leaves a
// conflicted worktree unlocked between `git merge` and `git commit`.
//
// **No post-conclude undo, by design.** Once `MergeContinue`/`Merge` lands a conclude-commit,
// nothing in this layer can undo it: the verify-before-conclude discipline plus `MergeAbort` covers
// the whole uncommitted attempt window, but a landed merge is final at the Fabric layer until a
// separate two-sided reset-to-SHA verb exists (see the `fabric: merge-conflict primitive` item's
// Someday follow-up in `manifest/roadmap.md`). A consumer that needs an undo after concluding must
// verify before calling `MergeContinue`/`Merge`, or accept that layer's own finality.
//
// **Squash leaves no ancestry link.** A squash-merged branch's history carries no merge commit
// linking it to its target, so "was this branch merged?" cannot be answered from git alone after a
// squash; a consumer needing that answer (branch cleanup, archive tagging) needs a source outside
// git — this is a direct consequence of shipping squash as an option, not a defect in it.
package fabricengine
