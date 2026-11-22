// Package fabricengine is lym's sole warp↔weft git-coordination module.
// It is built on two `internal/gitrepo.Repo` instances,
// and it covers warp↔weft topology and commit, push and pull into the paired weft repo.
// fabric is the only module that knows both repos exist.
// The `Fabric` handle holds the unexported `code` and `records` `*gitrepo.Repo` fields, which only this package reaches.
// It adds the cross-repo operations `Commit`, `Pull`, `Diff` and `Status`, which a single `gitrepo.Repo` cannot express.
//
// fabric enforces one branch-naming scheme with no exceptions:
// a warp branch `<branch>` always pairs with weft branch `<branch>-weft`.
// That includes the primary worktree (`main` ↔ `main-weft`).
// The suffix is fixed and non-configurable.
// `main-weft` does not advance on landing, since weft content is per-branch and never a merge participant.
// A landed pair's records live in its archive tag.
//
// Every weft commit fabric makes carries a `Warp-SHA: <sha>` trailer naming the warp SHA it corresponds to.
// That trailer history is the sole source of truth;
// the correspondence index on top of it is a rebuildable performance cache, never authoritative on its own.
// The one exception is a commit made while warp's HEAD is unborn, which carries no trailer and no index entry.
//
// # Pull
//
// `Fabric.Pull` is the unified read path.
// It brings weft level with origin first, then fetches warp and inspects it against its upstream tracking ref.
// Rewrite and anchor determination is ancestry-based (`IsAncestor`), never `SHAExists`:
// `git fetch` never prunes objects, so a rebased-away commit still exists and detection would never fire.
// A warp history rewrite is auto-reconciled when safe:
// the correspondence index is rebuilt from the weft trailer history, weft is re-anchored to the nearest surviving
// `Warp-SHA`, and warp is reset to the new tip.
// `Pull` refuses, with no change to either repo, in these cases:
//   - `ErrWarpDivergedUnpushed`: local warp has unpushed commits and the remote diverged.
//   - `ErrNoSurvivingAnchor`: the rewrite left no recorded correspondence.
//   - `ErrWarpRewound`: origin rewound a task pair's branch, dropping commits the local tip holds.
//     The prime's code branch is the operator's, so a rewind of it resets as a rewrite does.
//   - `ErrWarpDirty`: warp would have to move while its worktree holds uncommitted tracked changes,
//     because every warp advance is a `reset --hard` that would destroy them.
//
// The weft arm never stalls the warp side.
// A replay that stops on a conflict or an uncommitted change returns a `*WeftStopError` beside the warp result.
// `PullResult` reports what each side did, and `*PartialPullError` carries the warp-side-failure contract.
//
// A weft never crosses a change of its anchor marker on the pull and push paths (anchorstop.go).
// `Pull` returns a `*WeftStopError` wrapping `ErrAnchorChange` before the warp arm runs, so neither side moves.
// A push that must replay returns an `*AnchorStopError` before changing anything,
// and the failed-push record has class `anchor_change`.
// `Checkout` and `DropLocal` refuse with an error wrapping `ErrAnchorChange` before they move anything.
// Each refusal names its way forward, which is `lym fabric anchor <slug> --set <anchor>` unless the marker is empty.
//
// `Fabric.RemoteOnlyCommits` reads the warp origin's branch of the checkout's own name.
// It returns that remote tip and the commits on it that HEAD lacks, newest first.
// It mutates no ref except the remote-tracking refs its fetch refreshes.
// `BeamLineCommits` and `BeamCommitsPast` (beamline.go) read a beam pair's own first-parent line.
// Records-only commits, lym's own bookkeeping, are never a landing of work.
//
// # Origin reconcile
//
// `ReconcileOrigin` judges this machine's local branches against origin.
// It runs before anything acts on a run or the hub after the machine may have been away.
// It takes the hub location, a `ReconcileScope` and the `OriginSeams` a call site injects.
// A nil seam reads as no live pair, a free parent lock and no teardown.
// The `OriginReport` embeds `MutationRecord`.
// It holds one `BranchOutcome` per judged branch and one `EntryOutcome` per pair with a batten run record.
// An empty `ReconcileScope.Run` judges the hub; a slug judges that run.
// A branch with no origin tracking ref was never pushed and is not judged.
//
// A judge-only scope moves nothing and writes no `FETCH_HEAD`.
// The reconcile holds both repositories' remote-refs locks (`gitrepo.Repo.HoldRemoteRefs`);
// it holds them from the `ls-remote` pair through the last verdict,
// so a concurrent push is judged against the origin read before it, never as a rewind.
// A transport failure is `OriginReport.Unreachable` with a nil error, nothing judged and nothing moved.
// Any other failure is the returned error.
//
// A moving scope fast-forwards only.
// A code branch is never rebased, so one both sides moved stays diverged.
// A branch it cannot move is reported as dirty, busy, live or anchor_stopped with a `WayForward` line.
// That line is the repair step alone.
// An anchor_stopped branch stops its entry, and one on the prime's records branch stops every entry.
// An entry owned by another machine (`OriginSeams.OwnerOf`) is owned_elsewhere.
// It moves fast-forward only and is never handed to the teardown seam;
// its way forward names `lym adopt <subject>`.
// An ended entry goes to `OriginSeams.Teardown` when it has a local part.
// A teardown that reports busy leaves the pair, and its way forward names the wait and `lym cleanup --apply`.
// A run scope refuses when its task worktree is not registered, unless `ReconcileScope.PairMayBeAbsent` is set.
// Only a batten resume sets it.
//
// # Commit and push
//
// `Fabric.Commit` classifies a caller's mixed file list into warp-side and weft-side paths and commits each, warp first.
// The warp commit is bare, plain git, with no trailer and no correspondence entry, so warp stays ordinary git.
// There is no cross-repo transaction and no rollback:
// a landed warp commit is never unwound if the weft side then fails, and `*PartialCommitError` reports it.
// A weft commit that landed but whose correspondence entry failed to persist is recoverable only by `RebuildIndex`.
// `Commit` refuses, before any lock, a new secret file on either side, as `internal/secretpath` matches it.
// It takes the combined write lock (`.weft/weft.write.lock`) for any committing call and never holds it across the push.
// Whenever anything landed it spawns a detached, fire-and-forget push through `SpawnDetachedPush`.
// `SyncOptions.SkipGit` is weft-scoped there, and `SkipPush` does not stop the push;
// only the `FABRIC_SKIP_GIT` and `FABRIC_SKIP_PUSH` environment variables do.
// `SpawnDetachedPush` returns `ErrSpawnUnderTest` in a test binary.
//
// `Commit(nil, msg, tags, opts)`, snapshot tags with zero files, is a supported call shape:
// it records a baseline (a warp SHA under a tag) with no weft content of its own.
// When `snapshotTags` is non-empty, `commitWeftLocked` lands an empty weft commit even when nothing else changed.
// That commit carries the `Warp-SHA` and `Snapshot:` trailers,
// so a regeneration confirmed against an unchanged warp SHA still advances the correspondence baseline.
// An unborn warp HEAD drops the tags.
// `gitrepo.Repo.CommitEmpty` refuses with `gitrepo.ErrIndexNotEmpty` rather than fold someone's staged work into it.
// The intended pattern is one tagged commit per regeneration at a phase boundary, not a poll.
// The `Snapshot:` trailer tag grammar is restricted so a tag cannot inject a trailer line.
// Reads of it match byte-exact; a tag never recorded reads as `("", nil)`.
//
// fabric never calls gitrepo's `StageAllAndCommit`; all staging is an explicit-list `StageAndCommit`.
// The one exception is `commitWeftAt`, which seeds the weft artifact excludes before every stage.
//
// `Fabric.Diff` answers what changed since a warp SHA on both sides, bridged through the correspondence index.
// A SHA older than the first correspondence gives an empty weft side with `DiffResult.NoWeftCorrespondence`.
// `Fabric.Status` answers what is uncommitted on both sides, with no correspondence anchor involved.
// `Topology.Status` is the separate pairing and junction-health report.
//
// The detached child runs `CoalescePushRequest`, a loop-until-clean coalescing push.
// It runs under the absorbing `fabric.push.lock`, which is distinct from the commit lock.
// Pushes are rebase-free (`gitrepo.PushRebaseFree`), so a rejected push never mutates the calling working tree.
// `PushPairAnchored` is the synchronous form.
// It pushes the records side first and sends no code side unless the records side landed.
// `PairPush.LockWait` bounds the wait for the push lock, and an expiry wraps `ErrPushLockBusy`.
// `IsPushRejected` and `IsPushOwed` classify a failed push for callers outside this package.
//
// **Push classification.** Every push per side goes through `pushClassified`.
// A branch origin no longer has is not pushed, so a parked push never recreates a branch landed or deleted elsewhere.
// A task branch origin rewound is not pushed either, because a plain push would restore the dropped commits.
// A rejection runs the side's retry, a bounded replay over backoff, before anything is recorded.
// A failure that remains is recorded as a `FailedPush`.
// Its class is `not_fast_forward`, `auth`, `transport`, `gone`, `rewound`, `deferred` or `other`.
// The push then returns a `*PushFailedError` carrying the record and a way forward.
// A held lock or an uncommitted change on a replayed path is a deferral that mails nothing.
// Every other record mails the hub's orch once per episode.
// A push that lands removes the record of its side and branch.
//
// A push under a pause defers instead of waiting: it pushes nothing more and leaves its pending flags.
// It writes no failed-push record.
// A holder of `fabric.push.lock` writes `fabric.push.holder` so an error wrapping `ErrPushLockBusy` can name it.
// A pusher writes a `pending.records` or `pending.code` flag before it tries the lock,
// so the last holder to release always sees a flag set after its last push and no committed transition stays unpushed.
//
// A rejected records push first reads origin's record of its run.
// An entry naming another machine stops the push with `ErrHandedOver` or a `*TakeoverError`, neither retryable.
// `HandOverTips`, `TakeOverRun` and `ArchiveParentMerge` are the git steps of a hand-over.
// Each runs under the slug's teardown lock and tags unpushed local tips `archive-owner/<slug>/<tip>`.
//
// A pair's teardown lock is `TeardownLockPath(hub, slug)`, outside both of the pair's worktrees.
// A detached push child takes it shared and exits quietly when a removal holds it.
// `Remove`, `Add` and `DiscardTaskBranch` take it exclusive and fail with `ErrPairPushRunning` when the wait runs out.
//
// # Pairs, junctions and reconcile
//
// `Open(l)` is the only constructor other packages call.
// It derives both paths from l, stat-validates them and performs no wiring; wiring is `Topology`'s job.
// The wired junction set is repo-wide.
// It is the pair's `pathspec` from `fabric.yaml` plus the structural `_lym` and `.lym` directories injected in code.
// `HubReservedNames()` is subtracted from it.
// `configsync` never rewrites a `pathspec:` key already present, so a template change governs new clones only.
// `Reconcile` converges every pair to that set: add what is missing, re-point what is broken, remove what is stale.
// It is fail-closed: an unreadable repo-wide `fabric.yaml` aborts stale removal for that pair.
// A pair that lacks a junction is reported unhealthy on purpose (`CauseJunctionMissing`) until a reconcile.
// Downgrade is unsupported: an older binary's reconcile unwires `.lym` and strands scratch in the weft worktree.
// `Unwire` removes every fabric junction on disk and its exclude entries, never weft content or the repo-wide records.
// `UnwireVerbResult.WeftContent` is `"preserved"` or `"not_present"`, never `"cleared"`.
//
// Every junction is excluded through the warp's `.git/info/exclude`, never a committed `.gitignore`.
// The entry is the junction's anchored path, because a slash-free pattern would untrack same-named directories.
// Every mutation goes through `mutateGitExclude`, which holds a repo-wide flock and replaces the file by rename.
// `fabric.yaml`'s `warp_exclude` key lists paths lym's rendered files occupy; wiring seeds them and never removes one.
//
// The lym-anchor subpath is recorded per worktree as the plain `.lym-anchor` marker at the root of its weft branch.
// `lymcwd.Resolve` is the reader and treats a present record as truth.
// A marker that is present but blank is refused by `Reconcile` (`refuseEmptyOwnMarker`) for that pair alone.
// `lymcwd`'s fallback to `"."` would otherwise wire a second junction set.
// A weft with no committed marker gets one from `Reconcile`, by `treeAnchor`'s rule.
// `lym fabric unwire` leaves every marker.
//
// The warp binding `.lym-warp` and the hub shortname `.lym-shortname` are repo-wide records at the board root.
// `CloneHub` resolves the warp URL from the record and refuses a supplied URL that disagrees.
// This package alone reads and writes both.
// `Reconcile` reports a diverged or missing binding in `ReconcileResult.WarpBinding` and writes nothing.
// The anchor marker, the repo-wide config and these records let `lym fabric reconcile` re-wire a hub with no re-clone.
//
// # The scratch link
//
// Wiring ends by making the `.scratch` at the warp worktree's anchor a directory link to `.lym/scratch` in the weft.
// A real directory would change its modification time with every file an agent creates,
// and every source-scan test hashes that time into its `go test` cache key.
// A build under `.scratch` therefore resolves relative project references from the weft worktree.
// The step never fails wiring and removes nothing; a case it leaves as found is reported with a way forward.
// `lym fabric unwire` keeps the link, and `lym fabric remove` unlinks it through the destructive gate.
//
// # Add, remove and teardown
//
// `Add` handles a fresh pair, a re-add after a plain `Remove`, and a pair moved from another machine.
// A pair is live when its weft branch exists locally, or on origin with no `archive/<slug>/*` tag covering its tip.
// A live pair is adopted and never forked, and nothing on origin is ever force-pushed or deleted for it.
// A true divergence is refused with an `*ErrRemoteLeftover`.
// A removed pair's replaceable leftover is deleted through the destructive gate under a lease.
// `Add` pushes neither branch; its callers spawn the pair's detached push.
// A rollback deletes only the local branches this `Add` created and never an adopted warp branch or an origin branch.
// `AddOptions` carries `Parent`, `AdoptLocal`, `AdoptOnly` and `Beam`.
// `ErrSourceDirty`, `ErrBranchExists` and `*ErrNotLive` are the refusals their callers act on.
// The weft branch is forked with the parent's shed run records dropped from the new worktree.
// Its first weft commit records the pair's origin (`_lym/fabric/origin.json`).
// The record has write-once `parent_branch` and `parent_worktree` fields and an optional `beam` mark.
// `resolveWeftBranch` is the one place a verb decides where a pair's weft branch comes from:
// local first, then origin as a tracking branch, and otherwise the caller forks.
// `ValidatePairSlug` is the one slug rule, matching `ErrInvalidSlug`.
//
// `Remove` is the paired teardown.
// It never deletes a directory git declined to remove unless it is a registered linked worktree of the warp repo.
// Deleting the directory after git refuses would lose a whole git clone, gitdir included, and report success.
// The prime worktree is refused by name.
// `Remove` tags the local weft tip `archive/<slug>/<tip>` before its first mutation, after its refusals,
// so the run records stay reachable once the branch is deleted; a failed tag leaves everything in place.
// Remote steps go to a detached teardown child, and a failure is logged and mailed to the hub's orch.
// A remote branch delete is leased and a branch origin moved since is kept, never a failure.
// A pair already half removed is finished rather than refused, and `ErrPairNotFound` means nothing is left.
// `RemoveLocal` is the same removal with no origin step and no waiting.
// It try-acquires its locks (`ErrPairBusy`) and tags the weft tip `set-aside/<slug>/<sha>`.
// `SnapshotUncommitted` keeps a force removal's uncommitted paths in one parentless commit under such a tag.
// `Cleanup` sweeps local leftovers and `CleanupRemoteWarp` sweeps origin, each trying a pair's teardown lock once.
// The sweep never takes the primary weft branch.
// `Prune` reports a hub directory no registration covers as `Unowned` and never removes it.
// `PairTaskBranch` reads a pair's warp branch from the pair itself, so `branch_prefix` can change while pairs are live.
//
// The hub's shared stores would otherwise start a detached repack under live steps,
// so `Add` sets `gc.auto=0` and `maintenance.auto=false` in both stores and `Remove` runs a foreground `git gc --auto`.
// The removal's gc is best-effort and skipped while another pair's reed session is live.
//
// # The hub pull
//
// `PullHub` brings a hub up to date from origin: Bolt, then the prime pair, then each local beam pair.
// A side that stops never keeps the next from running, and each side reports `moved`, `up_to_date` or `stopped`.
// It pushes nothing.
//
// # Changing an anchor
//
// A worktree's anchor changes in three calls:
// `Topology.BeginAnchorChange`, `AnchorChange.Apply` and `AnchorChange.Finish`.
// `Close` ends every path.
// A change leaves a move record, `anchor-move.json`, in the weft's lock directory before the weft is touched.
// It is removed when the change finishes or is undone, and `MoveInProgress` reads it for every weft writer.
// A failure before the commit is undone.
// A failure after it leaves the record, and the error names the re-run that finishes the wiring.
// A target the warp checkout lacks costs a code commit on the checked-out branch, shared by every machine.
//
// # The worktree seam
//
// `InstallWorktreeSeam` stores one process-wide `WorktreeSeam` for code that renders files into a worktree.
// "Made" fires once a worktree is fully set up.
// "Took in commits" fires when a warp pull or reconcile fast-forwards a code branch;
// weft and Bolt moves fire nothing.
// A seam failure never fails a verb; it is returned as warning text such as `PullResult.SeamWarning`.
//
// # The one-repo illusion at the public API boundary
//
// fabric exists to sell one illusion to every other package:
// a developer, an agent, and every lym module see one repository, called fabric, never the warp/weft split under it.
// Callers outside the owner set use the vocabulary-neutral entry points:
//
//   - `Open`, `OpenParent`, `Fabric.OriginURL`, `Fabric.PushBranch`, `Healthy`
//   - `PushPairAnchored`, `MergeStateActive`, `MidMerge`
//   - `RequireDrivableWorktree`, `NewRefScanner`, `NewReferenceRule`
//
// Such a caller reads only `CommitResult.Committed()`; the raw side fields stay exported for `internal/fabriccli`.
// `Healthy` returns a typed `HealthReason` so a caller switches on `Cause` instead of parsing prose.
// `IsReadOnlyCommand` rejects an unquoted parenthesis and a dollar expansion in a reader segment.
// A zsh glob qualifier or parameter expansion runs code with no substitution marker.
//
// # The fabric vocabulary rule
//
// In production code, the tokens `weft` and `warp` may appear only in the owner set.
// `PATTERN-fabric-boundary` holds the authoritative owner list and the rule in full.
// `TestEnforcement_FabricVocabulary` (`internal/lymcwd/enforcement_test.go`) machine-checks it.
// This package is an owner, so its files use `warp` and `weft` freely.
//
// # The mutation record
//
// Every mutating verb accumulates a `*Mutations` record (mutation.go) naming, in order, the primitives it performed.
// The record lists what changed, not what the verb attempted and not what its success return implies.
// `ok`, or at this layer a nil error, means only that no error was returned, never that nothing happened;
// `mutations` and `partial` keep a consumer from confusing the two.
//
// `Kind` is the closed, string-backed enum of primitives.
// The accumulate-as-you-mutate rule has no exception:
// append an entry immediately after a primitive observably changed state, never before, and never for a no-op or refusal.
// The gate executors in destroy.go auto-record the kinds they perform; the rest are hand-recorded at their success sites.
//
// Every mutating entry point owns exactly one recorder built by `NewMutations`.
// It threads the recorder as a `*Mutations` parameter into everything the call performs, gate executors included.
// It installs `defer func() { res.Mutations = rec.Snapshot() }()` right after construction,
// so the record reaches the caller through every return, early error returns included.
//
// Every mutating verb's result type embeds `MutationRecord` (mutation.go), which exposes `Mutated() Mutations`.
// Read-only verbs' result types, `StatusResult` and `DiffResult` among them, do not embed it.
//
// At the CLI envelope layer (`internal/fabriccli`), every mutating verb's JSON output carries two fixed keys:
// `"mutations"`, always a JSON array (empty, never `null`), and `"partial"`, always a bool (`false`, never absent).
// `partial` is true exactly when `error != nil` and the record is non-empty.
// A handler that fails before it calls its verb, such as a location or usage error, carries neither key.
// `okWithRecord`, `errWithRecord` and `errWithRecordFields` in `envelope.go` implement this rule.
//
// A verb whose result carries per-item outcomes reports an item-level failure as a failure.
// `reconcile` emits its `pairs` array through `errWithRecordFields` whenever any pair carries an `Error`.
// `prune` does not, because its per-entry `Error` also explains a designed refusal.
// The test is whether the field means "this verb failed at its job" or "this verb is telling you what it did not do".
// `CleanupBranchEntry.Error` and `RemoteError` are genuine failures and drive `cleanup`'s exit code;
// a missing `origin` remote exits 0 and is reported in `RemoteSkippedReason`.
// `ReconcileActionVanishedMidWalk` sets no `Error`, because the pair stopped existing rather than failing.
//
// `TestMutationRecord_FabricengineProductionSource` in `cmd/lym/destructiveguard_test.go` guards this rule.
//
// # The destruction chokepoint
//
// `destroy.go` is the one file in this package permitted to perform a destructive primitive;
// every one of them runs the shared four-check pipeline first.
// The primitives are `os.RemoveAll`/`os.Remove`, `git worktree remove`, `git branch -D` and `fslink.Remove`;
// deleting a branch on a remote (`git push <remote> --delete`);
// moving a task branch on a remote (a leased force push);
// a warp checkout's `ResetHard`; and the board checkout's seed-commit drop.
// `removeWorktreeLeftover`, `Topology.RemoveStrayTaskPath` and `Add`'s `removeIgnoredTaskPath` reach a contained delete;
// they use their own ownership proof instead of a git removal, and `force` and dirtiness do not apply to them.
// `PATTERN-fabric-boundary` holds the rules; this section is the rationale the invariant omits.
//
// Data-loss defects in destructive code share one shape:
// an operation acts on a path it does not own, or destroys a path without checking whether work there would be lost.
// When each call site writes its own containment, ownership and dirtiness checks, the checks come out inconsistent.
// A chokepoint does not add a check nobody thought of; it removes the freedom to skip one.
//
// **Why the gate executes rather than approves.**
// A gate a caller consults and then acts on independently is advice, not enforcement.
// The executors in `destroy.go` run the pipeline and then perform the primitive themselves,
// so a raw call to any primitive is mechanically bannable everywhere else in this package.
//
// **Why ownership is a closed enum with no caller-supplied predicate.**
// A caller-supplied predicate would let a call site declare "trust me, this is mine".
// That is the freedom that caused the defects.
// The gate resolves every ownership kind against a fixed set of predicates it owns;
// a call site picks which kind applies, never what the kind checks.
//
// **Why dirtiness scope is a caller declaration.**
//
// "Is there work here to lose" has no single right scope:
// `dirtyScopeTracked`, `dirtyScopeAll` or `dirtinessNA` is declared per call site, never derived from the primitive.
// Deriving it would widen the tracked-only sites.
// It would route git's own untracked-file refusal into a directory-removal fallback that has no equivalent protection.
// The probe lives in `dirtiness.go` so the bypass guard's allowlist can mean "the only file that destroys".
//
// **Why containment resolves symlinks, and why it stops short of the final component.**
//
// A lexical check compares spellings, while every destructive primitive acts on the inode they resolve to,
// so a symlink planted at an intermediate segment would pass the gate and `removePath` would delete outside the hub.
// Both sides therefore go through `filepath.EvalSymlinks`.
// The target's own final component is left unresolved.
// Every junction the gate removes is a link.
// Its resolved leaf would relocate the target into the weft and refuse every legitimate unwire.
//
// **The gate's dirtiness check is not atomic with its act; that is a stated limit.**
//
// No lock spans `git status --porcelain` and the executor's primitive, so a write landing in that window is destroyed.
// The exposure is narrow: `removeGitWorktree` and `resetHardTo` re-check through git;
// only the two recursive-removal fallbacks, which fire after git declined, rely on the probe alone.
//
// **The gate's containment check, by contrast, is bound to its act.**
//
// A symlink planted at an intermediate segment can be dangling when the check runs and live when the executor acts.
// The arbitrary-path executors (`removePath`, `removeLink`) therefore remove through an `os.Root`.
// The `os.Root` is rooted at the declared container,
// so the kernel's `openat` escape refusal closes the window with no lock.
//
// **The create-side minters bind creation to a rooted act the same way.**
//
// `createExclusiveDir` creates its leaf through an `os.Root`.
// `createGitWorktree` cannot, because `git worktree add` resolves its destination itself,
// so `containedWorktreeAdd` has git write into an unguessable 0700 staging directory under the container.
// `containedWorktreeAdd` renames that directory into place and verifies containment after each step.
// It cleans up on failure.
// A same-UID or root planter racing the add can still make git transiently write somewhere it controls;
// the operation is then never reported as success.
//
// **The hub-level writer binds its writes to a rooted act the same way.**
//
// `writeHubWorkspace` writes through an `os.Root` at the hub.
// A static symlink planted at the file's path would carry a raw `os.WriteFile` outside the hub.
// The verb would still report `ok:true`.
// The remaining raw writes in the package target one of two things:
// a git-owned `.git/…` path, or a directory a contained minter created in the same call.
// Only a post-creation same-UID race could redirect them.
// Each is an allowlisted entry with a stated reason in the write-side guard, which PATTERN-fabric-boundary describes.
// `TestNoUncontainedWrite_FabricengineProductionSource` in `cmd/lym/uncontainedwrite_test.go` enforces the guard.
//
// **Why the token-carrying ownership kinds exist, and the limit of what backs them.**
//
// `ownedFreshlyCreatedPath`, `ownedFreshlyCreatedWorktree` and `ownedCreatedBranch` are the token-carrying kinds.
// They let a rollback site prove the gate itself created the target in this same call, which nothing weaker could.
// Their tokens (`createdToken`, `createdBranchToken`) are unexported.
// A same-package composite literal still compiles.
// That property rests on the bypass guard's `createdToken{` and `createdBranchToken{` bans, not on Go's type system.
//
// # The hub workspace file
//
// `RegenerateHubWorkspace` writes `<hub>/<RepoName>.code-workspace`, listing the prime, `Bolt` and every pair by slug.
// Each beam pair also gets `<hub>/<RepoName>-<beam>.code-workspace` (`BeamWorkspacePath`).
// The files are lym-owned and marked with `lym.generated: true`, so a hand edit is overwritten;
// settings belong in the prime's `.vscode/settings.json`.
// A regeneration runs under `HubWorkspaceLockPath(hub)` and builds every file's bytes before writing any.
// It writes through an `os.Root` at the hub.
// `InstallWorkspaceContent` and `PairFolderNamer` are the process-wide seams that supply extra members and folder names.
//
// # The correspondence index's write path
//
// The index is a rebuildable cache of the weft commit trailers.
// `corrIndex.record` composes its upsert through `state.UpdateJSON`.
// That re-reads the base under the lock it writes under,
// so it takes no write lock of its own: a nested acquire self-deadlocks.
// `RebuildIndex` and `refreshCorrIndexAfterSwitch` take no weft write lock, since that would bind every future caller.
// `RebuildIndex` is two-phase, so an interleaved `record()` can lose an entry;
// this is accepted as self-healing: one spurious `no_weft_correspondence` from `lym fabric diff` that a re-run clears.
//
// # The records replay
//
// When both this machine and origin moved a records branch, local commits origin lacks are replayed onto origin's tip.
// The replay is never a merge or a rebase.
// `replayWeft` cherry-picks oldest first and returns `ErrReplayConflict` or `ErrReplayDirty` with the branch unmoved.
// `replayBolt` re-applies a commit by entry through registered `BoltReplayer` values and drops seed commits;
// with none registered it returns `ErrReplayerMissing` before the branch moves.
// A commit whose every path belongs to a run origin has ended is set aside as a local `set-aside/<slug>/<sha>` tag.
// It is not replayed, because replaying it would resurrect the run.
//
// # The merge surface
//
// `MergeIn(source)` merges `source` into the current pair's warp checkout, where a conflict is resolved by hand.
// `Merge(source, opts)` merges into a target pair's checkout, is squash-capable and expected conflict-free;
// a conflict self-aborts the warp side and returns `*ErrMergeInRequired`.
// `MergeInAt(source, warpSHA)` merges a told commit instead of the freshness rule's pick.
// The weft is never a merge participant: a merge carries code and the weft carries system files.
// The one weft-reading refusal is foreign in-flight git state, an operator's plain git merge in the weft checkout.
//
// A merge in progress is tracked by a JSON record, `fabric-merge.json`, kept beside the correspondence index.
// The record is never derived from git state:
// a fast-forward defeats `--no-commit`, a squash writes no `MERGE_HEAD`,
// and a human's plain git can leave real merge state fabric never started (`*ErrForeignMergeState`).
// Every mutating merge verb refuses foreign state rather than touch it.
// `MergeContinue` concludes an attempt once every conflict is resolved,
// and `MergeAbort` restores the recorded pre-merge SHAs.
// Every state-changing step re-persists the record first, so a crash leaves a record a resumed verb can act on.
//
// A crash is continuable only if both sides' `MergeStart` returned;
// otherwise `MergeContinue` refuses (`merge attempt did not reach both sides`),
// and `MergeAbort` is the recovery.
// A crash is abortable only until a conclude has landed:
// a conclude that fails on the second side returns `*ErrMergeIncomplete`,
// and `MergeAbort` then refuses (`merge conclude already landed`).
// Restoring would discard the operator's own resolutions.
// `MergeContinue` skips a side whose commit is recorded, so a resumed run is idempotent.
// It adopts a landed commit the record never learned about only on git's own evidence:
// a two-parent commit of the recorded start and the recorded source.
// A squash conclude is never adopted, since a one-parent commit carries no such evidence.
// `MergeContinue` also refuses a checkout that no longer carries the recorded merge (`recordedMergeGoneReason`),
// so it never concludes an unrelated merge as this one.
//
// If the underlying git failure cannot be fixed by retrying, plain git in the two checkouts is the last resort.
// It must produce the same commit fabric would have:
// resolve each unfinished side and commit it while git's own `MERGE_HEAD` is still live.
// Use `git commit --no-edit` and never `git merge --abort` first,
// so the resulting commit is a merge of this merge's source.
// `MergeContinue`'s adoption arm then accounts for the hand-landed commits and clears the record.
// A side whose `MERGE_HEAD` is already gone is put back first:
// run `git reset --hard` to that side's recorded `warp_start`/`weft_start`, then `git merge` its recorded source.
// A squash attempt has no hand-landed route.
// Plain git alone can never finish the job.
// The record lives in the weft gitdir and keeps the sibling verbs refusing.
//
// `Merge` decides the not-synced precondition twice, because the guard stage never fetches:
// the guard refuses a diverged target with `branch not synced to upstream`,
// and the pre-merge sync step re-decides it after its own fetches, fast-forwarding a behind side.
// A merge verb refuses while the warp checkout's HEAD is detached (`checkout is not on a branch`).
// A conclude-commit there would be reachable from no ref.
// The dirty guard reads the code checkout's tracked dirty paths and reports them in `MergeGuardError.DirtyPaths`.
//
// `Commit`, `Pull`, `Topology.Checkout` and `Topology.Remove` return `*ErrMergeInProgress` while a merge record exists.
// `Merge` takes the combined write lock before its sync step;
// `MergeContinue` and `MergeAbort` take it before reading the record and re-verify under it what they checked before.
// The lock never covers the resolution window between `MergeIn`/`Merge` and `MergeContinue`.
//
// `MergeResult.Committed` and `AlreadyUpToDate` are read off the merge-state record.
// A resumed call and a sequential one therefore agree.
// "Already up to date" means git had nothing to do, not that the trees already agreed:
// a merge whose result tree equals HEAD's own is a real merge and concludes with `Committed` true.
// Only a squash with an empty result is a no-op.
// Conflicted paths surface as one flat, sorted, deduplicated list of worktree-relative paths in `MergeResult.Conflicts`.
// When empty, it is never nil.
// A path outside the single visible worktree self-aborts the attempt with `*ErrUnmergeableState`.
// A conflict under a wired junction name lives in the weft and can be staged only through `MergeStageResolved`.
// The CLI form is `lym fabric merge-stage <path>...`, after which `MergeContinue` proceeds.
// At the CLI a conflicted merge and a hard error both exit 1 with `"ok": false`;
// only a conflict result carries a `conflicts` array.
// Conflict markers name a SHA, never a branch, on both sides.
// The anchor's OS separator is converted to `/` before comparing against git's forward-slash conflict paths.
// There is no post-conclude undo: verify before calling `MergeContinue` or `Merge`.
// A squash-merged branch leaves no ancestry link, so "was this branch merged?" needs a source outside git.
//
// # Git geometry and ref reads
//
// Geometry and ref lookups run in-process through `gitrepo`, so reading a committed file starts no git process.
// A `rev-parse` spawn survives only where git derives the answer from config or revision syntax:
//
//   - `InstallPostCheckoutHook` asks `--git-path hooks`, which honours `core.hooksPath`.
//   - websterengine's `--disambiguate` and `<commit>:<path>` probes are git's own disambiguation.
//
// # The one-repository layout
//
// A one-repository hub keeps its records store's branches and tags on the code store's origin, under `refs/lym/`.
// Its only spelling is the constants `RecordsNamespaceRoot`, `RecordsNamespaceHeads` and `RecordsNamespaceTags`.
// The layout is the records store's origin fetch refspec, read per repository by `originNamespaceAt(dir)`.
// A store whose prime key, push refspecs or a branch's merge setting names the namespace is a records store.
// Its layout read fails when no fetch refspec maps the namespace or when it has no origin.
// The error names the refspecs to restore; read as the identity layout, its board would push to the code branches.
// Every push, listing and fetch of the records side names origin refs through that namespace.
// A board push whose board branch has no upstream is refused with no failed-push record.
// Pushing it unleased could replace another hub's board.
//
// A one-URL `CloneHub` first runs one `ls-remote`, before anything is created.
// It refuses an origin left by the earlier layout.
// With `--single-repo` it also refuses one that already holds a board, the warp binding or the anchor marker.
// The one URL is the code repository, cloned as the prime, and the records store is a `git init` of its own.
// An origin holding the board is joined; one holding none is seeded, with no commit written to any code branch.
// `PushCloneBoard` pushes a seeded clone's records branch and board under leases, then gives both their upstreams.
// A failed push removes the hub, as `AbandonSeededClone` does for a failure before it.
// A push of either ref that ended without origin's answer may still land,
// so the ref on origin is read before any rollback.
// A ref at the seed's tip is withdrawn first: the board is deleted,
// and the records branch returns to the tip the clone's check observed.
// A ref origin does not hold at that tip yet keeps the hub, with a way forward naming the refs origin may hold.
//
// **Platform verification: Linux only.**
// Fabric is written cross-platform,
// and `internal/fslink` exists because Windows uses directory junctions where other platforms use symlinks,
// but its code has been executed on Linux only.
// The branches reasoned about and never driven on Windows are:
// `lymcwd.ValidateAnchorRel`'s volume-rooted rejection (`C:\...`, `\\server\share`);
// `excludePatternFor`'s separator handling, where git wants forward slashes and `filepath` on Windows gives backslashes;
// `lymcwd.samePath`'s case-insensitive branch, which is dead code on Linux;
// and `internal/fslink`'s junction path, which every link fabric creates on Windows goes through.
// The anchor and subpath mechanism has likewise been verified on one platform only.
// Closing the gap needs a Windows host rather than a design.
// The minimum is the existing suite there (`go build`, `go vet`, `go test`, `go test -tags integration`).
// Beyond it, drive these by hand:
// a `--subpath` anchored hub, covering junction targets, `_lym` and `.lym` placement,
// and the cwd gate refusing the warp root and its subdirectories;
// `add`/`remove` slug hygiene; and the `prune` and `clone --reset` ownership refusals.
// Deciding that Windows is not a goal is a valid resolution, but it must be written down.
// `internal/fslink` and `PATTERN-lym-files` currently assert the opposite.
//
// **Local bares.** `SeedLocalBares` creates a bare warp with one commit on `main` and an empty bare weft.
// `SeedLocalOrigin` creates the one bare of a one-repository hub, holding the same commit.
// They are the shapes `CloneHub` clones a hub from, so a throwaway hub of either layout needs no remote and no work tree.
//
// **Test layout.**
// A test builds its hub through `internal/hubforge` only, never a package-local fixture type.
// Test git plumbing goes through `internal/gitkit` only, never a private helper.
// A new test file prefers the external `fabricengine_test` package plus `export_test.go` accessors;
// an internal-package test file states its reason in its header comment.
package fabricengine
