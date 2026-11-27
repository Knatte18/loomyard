// verbresults.go holds the plain option and result types of the hub-changing verbs, their constants and pure methods.

package fabriclayout

// AddResult describes a newly created worktree pair by its slug, branch and path.
// It embeds MutationRecord, which records the mutations accumulated over the call.
type AddResult struct {
	MutationRecord
	Slug   string `json:"slug"`
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// BoltSkip names why a pull-first write did not run: Bolt could not be brought up to date.
// The empty value means Bolt is up to date and the writes ran.
type BoltSkip string

const (
	// BoltSkipFetchFailed means the fetch from the remote failed.
	BoltSkipFetchFailed BoltSkip = "fetch_failed"
	// BoltSkipDiverged means Bolt holds commits the moved upstream lacks and they could not be replayed onto it.
	BoltSkipDiverged BoltSkip = "diverged"
	// BoltSkipDirty means an uncommitted change in Bolt blocked bringing it up to date.
	BoltSkipDirty BoltSkip = "dirty"
)

// BoltWriteResult reports the outcome of a pull-first write sequence.
// SHAs lists the landed commits in order, and Committed is true when any landed.
// A non-empty Skipped means no write ran, and SkipDetail names the way forward.
type BoltWriteResult struct {
	MutationRecord
	SHAs       []string
	Committed  bool
	Skipped    BoltSkip
	SkipDetail string
}

// CheckoutResult describes a completed switch of the warp and weft worktrees.
// It embeds MutationRecord, which carries the mutations recorded over the call.
type CheckoutResult struct {
	MutationRecord
	// Branch is the warp branch the warp worktree now points to,
	// and the weft worktree points to RecordsBranchName(Branch).
	Branch string `json:"branch"`
	// RecordsWorktree is the filesystem path to the weft sibling worktree.
	RecordsWorktree string `json:"records_worktree"`
}

// CleanupBranchEntry records what a cleanup sweep did with one orphaned weft branch.
type CleanupBranchEntry struct {
	// Branch is the weft branch name.
	Branch string `json:"branch"`
	// Deleted reports whether the branch was actually deleted from the weft repo.
	// It is false on a dry run.
	// It is also false for a protected or skipped entry, and when deletion itself failed.
	Deleted bool `json:"deleted"`
	// Protected reports whether the branch was kept rather than deleted.
	// A branch without the "-weft" suffix is protected, because it is not fabric-managed.
	// A branch checked out at a worktree is protected too, since git branch -D cannot delete it.
	// Such an entry sets no Error.
	Protected bool `json:"protected,omitempty"`
	// Error is non-empty when apply is true and branch deletion failed.
	// It makes `lym fabric cleanup --apply` exit non-zero; a designed refusal sets no Error.
	Error string `json:"error,omitempty"`
	// RemoteDeleted reports whether this branch's copy on the remote was observably removed.
	// It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteDeleted bool `json:"remote_deleted,omitempty"`
	// RemoteError is non-empty when the remote deletion was attempted and did not succeed.
	// Its text names the layer that refused: the gate's own refusal, or the remote deletion itself.
	RemoteError string `json:"remote_error,omitempty"`
	// RemoteKeptReason is non-empty when the remote copy was left because it moved after the archive read its tip.
	// Such a branch is another process's live branch, not a failure, so it sets no RemoteError.
	RemoteKeptReason string `json:"remote_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local tip before the branch was deleted.
	// It is empty on a dry run or with no origin; a failed archive sets Error and keeps the branch, local and remote.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the archive tag at origin's tip, made when that tip is neither the local tip nor its ancestor.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// Skipped is true when the sweep left the branch whole because its slug's teardown lock was held.
	// A skipped entry sets no Error, and a later sweep takes the slug.
	Skipped bool `json:"skipped,omitempty"`
	// SkippedReason names the held teardown lock and the re-run that takes the slug.
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// CloneOptions carries the parameters of CloneHub as named fields.
// Named fields keep two adjacent string parameters, one of them optional, from being passed in the wrong order.
type CloneOptions struct {
	// WeftURL is the weft repository URL. Required.
	WeftURL string
	// WarpURL is the warp repository URL.
	// Optional: when empty, it is resolved from the warp binding recorded on the weft candidate.
	WarpURL string
	// Subpath is the lym-anchor subpath to resolve.
	Subpath string
	// Reset tears down an existing hub before cloning, for an idempotent re-clone.
	Reset bool
	// ForceBootstrap bypasses the old-order guard that refuses to bootstrap a warp-shaped repository as a weft.
	// The guard is reached only when a fresh binding is written, so the field has no effect on other paths.
	// Every other path ignores it.
	ForceBootstrap bool
	// Shortname is the repository's short name.
	// It is required when the weft is a fresh bind;
	// on a bound weft it must equal the recorded shortname, or record one if the weft has none.
	Shortname string
	// SingleRepo seeds a one-repository hub from WeftURL, which is then the code repository and the only URL.
	// The records and the board live under origin's records namespace, and the hub is pushed there by PushCloneBoard.
	// A plain one-URL clone of an origin that already holds that board joins it with no flag.
	SingleRepo bool
}

// CommitResult reports what Fabric.Commit did on each side: the SHA that landed and whether a commit was made.
// Unchanged content is a legitimate no-op, so a side can report that no commit was made.
// It embeds MutationRecord, which holds the mutations the call recorded.
type CommitResult struct {
	MutationRecord
	WarpSHA       string
	WarpCommitted bool
	WeftSHA       string
	WeftCommitted bool
}

// Committed reports whether Fabric.Commit landed a commit on either side.
// Read this rather than the four raw fields, which stay exported because fabriccli prints them.
func (r CommitResult) Committed() bool {
	return r.WarpCommitted || r.WeftCommitted
}

// DiscardResult reports what a discard of a task branch did.
// It embeds MutationRecord, which holds the mutations accumulated over the call.
// ArchiveTags lists the archive tags that now hold the discarded tips, none when no branch existed.
type DiscardResult struct {
	MutationRecord
	ArchiveTags   []string `json:"archive_tags,omitempty"`
	LocalDeleted  bool     `json:"local_deleted,omitempty"`
	OriginDeleted bool     `json:"origin_deleted,omitempty"`
}

// HubPullSideKind names which part of the hub a side of a hub pull covers.
type HubPullSideKind string

// The side kinds of a hub pull, in the order the pull visits them.
const (
	HubPullBoard HubPullSideKind = "board"
	HubPullPrime HubPullSideKind = "prime"
	HubPullBeam  HubPullSideKind = "beam"
)

// HubPullOutcome is what a hub pull did to one side.
type HubPullOutcome string

// The outcomes of one side of a hub pull.
const (
	// HubPullMoved means the side's local branches moved to take in origin's commits.
	HubPullMoved HubPullOutcome = "moved"
	// HubPullUpToDate means the side already held everything origin had, or has no upstream to pull from.
	HubPullUpToDate HubPullOutcome = "up_to_date"
	// HubPullStopped means the side could not be brought up to date; the detail names the way forward.
	HubPullStopped HubPullOutcome = "stopped"
)

// HubPullSide is one side of a hub pull: Bolt, the prime pair or one local beam pair.
// Worktree is the board directory's name for Bolt and the pair's worktree name otherwise.
type HubPullSide struct {
	Kind     HubPullSideKind
	Worktree string
	Outcome  HubPullOutcome
	Detail   string
}

// HubPullResult reports every side of a hub pull in pull order, with the mutation record accumulated over all of them.
type HubPullResult struct {
	MutationRecord
	Sides []HubPullSide
}

// Stopped reports whether any side of the hub pull stopped.
func (r HubPullResult) Stopped() bool {
	for _, side := range r.Sides {
		if side.Outcome == HubPullStopped {
			return true
		}
	}
	return false
}

// MergeOptions controls Merge.
// Squash selects a squash merge, and Message overrides the conclude commit's message.
type MergeOptions struct {
	Squash  bool
	Message string
}

// MergeResult reports what a merge verb did to the pair.
// AlreadyUpToDate reports a no-op: both sides' resolved source SHA was already an ancestor of that side's HEAD.
// Conflicts lists the unified, worktree-relative paths a merge attempt left conflicted.
// It is empty, never nil, on every return, error returns included.
// Committed reports whether the pair now carries this merge's conclude commit.
// A merge that fast-forwarded both sides creates no commit and reports Committed false.
// A call that finds the work already done after taking the lock reports AlreadyUpToDate true.
// It reports the same answer a strictly sequential run of the same calls gives.
// MutationRecord is embedded and carries the mutations accumulated over the call.
type MergeResult struct {
	MutationRecord
	AlreadyUpToDate bool     `json:"already_up_to_date"`
	Conflicts       []string `json:"conflicts"`
	Committed       bool     `json:"committed"`
}

// StageResult is the result of the merge staging verbs; it embeds MutationRecord to report what was staged.
type StageResult struct {
	MutationRecord
}

// MidMergeKind classifies the merge state MidMerge finds on a pair.
type MidMergeKind int

const (
	// MidMergeNone means no merge state exists anywhere on the pair.
	MidMergeNone MidMergeKind = iota
	// MidMergeParked means the fabric merge-state record exists: a merge verb stopped part-way and left it for a resolver.
	MidMergeParked
	// MidMergeForeign means no fabric record exists, but foreign git-level merge state sits on either side of the pair.
	MidMergeForeign
)

// MidMergeState is MidMerge's answer.
// Conflicts lists the still-conflicted paths in the pair's one-repo form, which is the form fabric merge verbs use,
// so every listed path is accepted by `lym fabric merge-stage`.
// It is an empty slice, never nil.
// Verb, Source, SourceSHA and StartSHA are filled only when Kind is MidMergeParked, and are empty otherwise:
// the parked record's verb (`merge-in` or `merge`), its caller-supplied source branch,
// and the code side's resolved source SHA and pre-merge HEAD SHA.
// The probe is read-only and carries no mutation record.
type MidMergeState struct {
	Kind      MidMergeKind
	Conflicts []string
	Verb      string
	Source    string
	SourceSHA string
	StartSHA  string
}

// OwnerArchive is one owner archive tag on origin whose commit the slug's branch on that origin does not contain.
type OwnerArchive struct {
	// Slug is the pair the tag archives.
	Slug string `json:"slug"`
	// Side is the repository the tag sits in, OwnerArchiveSideCode or OwnerArchiveSideRecords.
	Side string `json:"side"`
	Tag  string `json:"tag"`
	// Commit is the full hex name of the commit the tag names.
	Commit string `json:"commit"`
	// Branch is the slug's branch on that side's origin the commit was judged against.
	Branch string `json:"branch"`
	// Dir is the local repository the tag was fetched into, where `git log <tag>` shows it.
	Dir string `json:"dir"`
}

// PruneEntry describes one stale or orphaned pair that Prune has identified.
type PruneEntry struct {
	// CodeWorktree is the absolute path to the (missing or absent) warp worktree.
	CodeWorktree string `json:"code_worktree"`
	// RecordsWorktree is the absolute path to the weft worktree sibling.
	RecordsWorktree string `json:"records_worktree"`
	// Reason describes why this pair was flagged for pruning.
	Reason string `json:"reason"`
	// Removed reports whether the weft worktree was actually deleted.
	// It is false on a dry run and true only when apply is true and removal succeeded.
	Removed bool `json:"removed"`
	// Protected reports whether this entry's weft worktree has uncommitted tracked changes and force was not given.
	// It is computed in both modes, so a dry run reports what --apply would act on.
	Protected bool `json:"protected,omitempty"`
	// Unowned reports that the enumerated path is not a linked worktree of this hub's weft repo.
	// Fabric never removes it, force included.
	// The orphan pass matches on directory name alone.
	// So an ordinary user directory or an unrelated git clone with a weft-suffixed name is reported here, not deleted.
	// A directory at a pair's task worktree path that no registration covers is reported the same way.
	// Like Protected, it is computed in both modes.
	Unowned bool `json:"unowned,omitempty"`
	// Error is non-empty when an entry was not removed and the operator needs to know why.
	// It covers a failed removal, a protected entry, and an unowned path fabric refuses to touch.
	Error string `json:"error,omitempty"`
}

// PullResult reports what Fabric.Pull did on each side, independently.
// When a warp history rewrite forced a reconcile, it also carries the re-anchor baseline.
// It embeds MutationRecord, which carries the mutation record accumulated over the call.
type PullResult struct {
	MutationRecord
	// WeftPulled reports whether the weft was brought level with origin.
	// That is by a fast-forward or by a replay of its local commits onto origin's.
	// It is also true when the weft branch has no upstream yet, as on a freshly bootstrapped hub.
	// Its suffixed primary branch exists only locally until the first push lands.
	// There is nothing to pull from, so skipping is success, not failure.
	// A weft that origin moved fails the pull when a conflict or an uncommitted change blocks the move.
	// The field is false only on that failure and when the weft's origin could not be reached.
	// The warp fetch and reconcile run regardless of its value.
	WeftPulled bool
	// WarpFetched reports whether the warp fetch ran and succeeded.
	WarpFetched bool
	// WarpAdvanced reports whether the warp branch pointer actually moved.
	// It moves either by a clean fast-forward reset or by a reconcile reset.
	WarpAdvanced bool
	// NewWarpHEAD is the fetched upstream tip SHA warp advanced to.
	// It is empty when warp did not advance, either because it was already up to date or on an abort path.
	NewWarpHEAD string
	// RewriteDetected reports whether the warp pull was a non-fast-forward.
	// Local warp HEAD is then not an ancestor of the fetched upstream tip.
	// That means upstream's history was rewritten (rebase or force-push), not merely advanced.
	RewriteDetected bool
	// Reconciled reports whether a re-anchor weft commit was written after a rewrite.
	// The commit re-binds weft's correspondence to the new warp HEAD.
	Reconciled bool
	// AnchorWarpSHA is the warp SHA of the surviving correspondence entry, the confirmed re-anchor baseline.
	// It is populated only when Reconciled is true.
	AnchorWarpSHA string
	// AnchorWeftSHA is the weft SHA of that same entry, the weft-side re-anchor baseline.
	// It is populated only when Reconciled is true.
	AnchorWeftSHA string
	// ReanchorWeftSHA is the SHA of the new empty weft anchor commit.
	// The commit binds to NewWarpHEAD through its Warp-SHA trailer.
	// It is populated only when Reconciled is true.
	ReanchorWeftSHA string
	// SeamWarning is the warning text of a failed worktree event fired for the commits warp took in.
	// It is empty when no event failed or no seam is installed.
	// A seam failure never fails the pull.
	SeamWarning string
}

// ReconcileAction describes the corrective action applied to one warp↔weft pair.
type ReconcileAction string

const (
	// ReconcileActionWeftRecreated means a missing weft worktree was recreated from a local or origin branch.
	ReconcileActionWeftRecreated ReconcileAction = "weft_recreated"

	// ReconcileActionJunctionRepointed means at least one broken or dangling warp junction was re-pointed.
	// The pair's junctions are all repaired in one call.
	//
	// Detail therefore names every junction the call repaired, not only the one that failed the health check.
	ReconcileActionJunctionRepointed ReconcileAction = "junction_repointed"

	// ReconcileActionRawAdopted means a warp worktree created outside lym got a dormant weft counterpart.
	// The weft branch and worktree are both created.
	// No junction is wired yet.
	// Running the reconcile sweep again wires it once the pair exists.
	ReconcileActionRawAdopted ReconcileAction = "raw_adopted"

	// ReconcileActionUnmanagedReported means a warp worktree is on an unmanaged branch with no weft sibling.
	// The pair is reported but left untouched.
	// No weft worktree is created for it.
	ReconcileActionUnmanagedReported ReconcileAction = "unmanaged_reported"

	// ReconcileActionAlreadyHealthy means the pair required no corrective action.
	ReconcileActionAlreadyHealthy ReconcileAction = "already_healthy"

	// ReconcileActionStaleRemoved means the pair needed no junction added or re-pointed.
	// Declarative stale removal still deleted at least one on-disk junction absent from the repo-wide pathspec.
	//
	// It is reported rather than as healthy.
	//
	// A consumer keying off Action, not only Detail, must see that convergence altered the pair.
	ReconcileActionStaleRemoved ReconcileAction = "stale_removed"

	// ReconcileActionVanishedMidWalk means a warp worktree listed by git was gone when the pass reached it.
	//
	// The usual cause is a concurrent `lym fabric remove` or `prune`, not a fault in this hub.
	//
	// It is reported under this action, never as an unmanaged pair.
	//
	// Naming the race lets an operator tell a transient failure from a real defect,
	// and it lets a script reading the per-pair Error do the same.
	// No Go runtime message needs decoding for that.
	// The action name alone gives that verdict.
	ReconcileActionVanishedMidWalk ReconcileAction = "vanished_mid_walk"
)

// WarpBindingOutcome describes how the recorded warp-URL binding compares with the warp side's origin remote.
type WarpBindingOutcome string

const (
	// WarpBindingOutcomePresent means a binding already exists and normalizes to the warp side's origin remote.
	// In that case nothing is written.
	WarpBindingOutcomePresent WarpBindingOutcome = "present"

	// WarpBindingOutcomeDiverged means the existing binding normalizes to a different URL than the warp origin;
	// the record is left untouched and the reconcile pass still succeeds.
	WarpBindingOutcomeDiverged WarpBindingOutcome = "diverged"

	// WarpBindingOutcomeSkipped means there is nothing to compare.
	// The warp side has no origin remote to read, as in a synthetic test hub or a locally-initialised warp,
	// or the board records no binding.
	WarpBindingOutcomeSkipped WarpBindingOutcome = "skipped"
)

// ReconcilePairResult describes the outcome for one warp↔weft pair.
type ReconcilePairResult struct {
	// CodeWorktree is the absolute path to the warp worktree.
	CodeWorktree string `json:"code_worktree"`
	// RecordsWorktree is the absolute path to the expected weft sibling.
	RecordsWorktree string `json:"records_worktree"`
	// Action is the corrective action taken (or reported).
	Action ReconcileAction `json:"action"`
	// Detail provides human-readable context for the action.
	Detail string `json:"detail,omitempty"`
	// Error is non-empty when the reconcile step encountered an error.
	Error string `json:"error,omitempty"`
}

// ReconcileResult is the top-level result of one reconcile pass.
// Its embedded mutation record holds the changes accumulated over the call.
type ReconcileResult struct {
	MutationRecord
	// Pairs is the ordered list of per-worktree reconcile outcomes.
	Pairs []ReconcilePairResult `json:"pairs"`

	// WarpBinding is the outcome of comparing the recorded warp-URL binding with the warp origin.
	// It runs once per reconcile pass, after the pair loop, and is never reported per pair.
	//
	// The binding is a hub-wide fact held in the board directory;
	// reporting it per pair would leave unclear which pair owns it.
	//
	// The CLI handler builds its own envelope, so this tag documents intent only.
	WarpBinding WarpBindingOutcome `json:"warp_binding"`

	// WarpBindingDetail provides human-readable context for the binding outcome.
	// Like that outcome, it is a once-per-pass, repo-wide field, never a per-pair one.
	// Its JSON tag documents intent only, since the CLI handler never serializes this struct.
	WarpBindingDetail string `json:"warp_binding_detail,omitempty"`
}

// RemoteWarpBranchEntry describes the fate of one branch on the warp origin in a sweep.
type RemoteWarpBranchEntry struct {
	// Branch is the branch name on origin.
	Branch string `json:"branch"`
	// Candidate reports whether every deletion condition held, whether or not apply ran.
	Candidate bool `json:"candidate"`
	// Deleted reports whether the branch was observably deleted from origin.
	Deleted bool `json:"deleted"`
	// Reason names the condition that kept a non-candidate; empty for a candidate.
	// Under apply a kept candidate's Reason also names a held teardown lock or a hub checkout.
	Reason string `json:"reason,omitempty"`
	// Error is set when apply's delete of a candidate was refused, its lease was lost, or its teardown lock failed.
	Error string `json:"error,omitempty"`
}

// RemoteWarpCleanupResult reports a sweep of the warp origin: one entry per branch, and the mutations the sweep made.
type RemoteWarpCleanupResult struct {
	MutationRecord
	// Entries lists every branch on origin and its disposition; always an array.
	Entries []RemoteWarpBranchEntry `json:"entries"`
	// SkippedReason is non-empty when the sweep did not run, such as when the warp repo has no origin remote.
	SkippedReason string `json:"skipped_reason,omitempty"`
}

// PlainArchiveTag is an archive tag under a one-repository origin's plain refs/tags/, where an earlier binary pushed it.
// No verb of the one-repository layout reads that prefix, so nothing else names the tag.
type PlainArchiveTag struct {
	// Ref is the tag's full ref on origin.
	Ref string `json:"ref"`
	// SHA is the object the ref points at.
	SHA string `json:"sha"`
	// WayForward is the commands that move the tag under the code store's archive prefix and delete the plain ref.
	WayForward string `json:"way_forward"`
}

// The steps a Remove call reports in RemoveResult.Steps, a closed set; each is appended only after it changed state.
// The two origin steps are the teardown child's, so they are listed only when its steps ran in process.
const (
	// RemoveStepRecordCommit is the commit of the sibling's pending records.
	RemoveStepRecordCommit = "record_commit"
	// RemoveStepArchive is the archive tag made on the local weft tip before the teardown;
	// its push to origin is the teardown child's.
	RemoveStepArchive = "archive"
	// RemoveStepTaskWorktree is the removal of the task worktree.
	RemoveStepTaskWorktree = "task_worktree"
	// RemoveStepSiblingWorktree is the removal of the sibling worktree.
	RemoveStepSiblingWorktree = "sibling_worktree"
	// RemoveStepSiblingBranch is the deletion of the sibling's local branch.
	RemoveStepSiblingBranch = "sibling_branch"
	// RemoveStepTaskBranch is the deletion of the task's local branch.
	RemoveStepTaskBranch = "task_branch"
	// RemoveStepSiblingBranchOnOrigin is the deletion of the sibling's branch on origin.
	RemoveStepSiblingBranchOnOrigin = "sibling_branch_on_origin"
	// RemoveStepTaskBranchOnOrigin is the deletion of the task's branch on origin.
	RemoveStepTaskBranchOnOrigin = "task_branch_on_origin"
)

// RemoveResult reports the outcome of removing a worktree pair.
// It embeds MutationRecord, which holds the mutations the call made.
type RemoveResult struct {
	MutationRecord
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	LinksRemoved int    `json:"links_removed"`
	// Steps lists the steps this call performed, in order, from the RemoveStep constants; always an array.
	// A teardown child's steps are listed only when they ran in process;
	// then RemoveStepArchive stands for the local tag alone.
	Steps []string `json:"steps"`
	// Finished is true when the task worktree was already gone at entry, so the call completed a half-removed pair.
	Finished bool `json:"finished"`
	// StrayPath names a path at the task worktree's location that is not a registered linked worktree.
	// Remove never deletes a path found there at entry.
	// A directory that a late writer refilled after the task worktree's removal is reported here too;
	// when a last removal could not delete it, Remove fails with an error saying the path was kept.
	StrayPath string `json:"stray_path,omitempty"`
	// StrayPathDeleted is true when a caller's separate delete removed a stray directory at the task worktree's path;
	// Remove itself never sets it.
	StrayPathDeleted bool `json:"stray_path_deleted,omitempty"`
	// RemoteBranchDeleted reports whether the pair's weft branch was observably removed from the remote.
	// It is true only when the remote deletion was attempted and actually removed a ref.
	RemoteBranchDeleted bool `json:"remote_branch_deleted,omitempty"`
	// RemoteBranchError is non-empty when the remote deletion was attempted and did not succeed.
	// Its text always names the layer that said no: the gate's own refusal, or the remote deletion itself.
	RemoteBranchError string `json:"remote_branch_error,omitempty"`
	// RemoteBranchKeptReason is non-empty when the pair's weft branch on the remote was left in place;
	// the branch moved after the archive read its tip.
	// A kept branch is another process's live branch, not a failure of Remove.
	RemoteBranchKeptReason string `json:"remote_branch_kept_reason,omitempty"`
	// RemoteSkippedReason is the once-per-verb reason no deletion of the sibling branch's remote copy was attempted:
	// with remote, a weft repo with no origin remote configured;
	// without remote, a sibling branch left only on origin, which is kept.
	RemoteSkippedReason string `json:"remote_skipped_reason,omitempty"`
	// CodeBranchDeleted reports whether the pair's local warp branch was deleted after the teardown.
	CodeBranchDeleted bool `json:"code_branch_deleted"`
	// CodeBranchKeptReason is non-empty when the warp branch was left in place, naming why:
	// the destructive gate's refusal, or a failure to delete it.
	// A kept branch is not a failure of Remove.
	CodeBranchKeptReason string `json:"code_branch_kept_reason,omitempty"`
	// RemoteCodeBranchDeleted reports whether the pair's task branch was observably removed from the warp repo's origin.
	// It is true only with remote, when the branch's work was landed.
	RemoteCodeBranchDeleted bool `json:"remote_code_branch_deleted"`
	// RemoteCodeBranchKeptReason is non-empty when remote was set and the task branch on origin was left in place;
	// it names why: the gate's refusal, or a failure to look it up or delete it.
	// A kept branch is not a failure of Remove.
	RemoteCodeBranchKeptReason string `json:"remote_code_branch_kept_reason,omitempty"`
	// ArchiveTag names the archive/<slug>/<tip> tag made on the local weft tip before the teardown;
	// its push to the weft origin is the teardown child's.
	// It is empty when none was made.
	ArchiveTag string `json:"archive_tag,omitempty"`
	// OriginArchiveTag names the second archive tag, made at origin's weft tip.
	// It is pushed when that tip is neither the local tip nor an ancestor of it, and is empty when none was needed.
	OriginArchiveTag string `json:"origin_archive_tag,omitempty"`
	// ArchiveSkippedReason is non-empty when no archive was attempted;
	// that is the case only for a weft repo with no origin remote configured.
	ArchiveSkippedReason string `json:"archive_skipped_reason,omitempty"`
	// RemoteHandedOff is true when the remote steps went to the detached teardown child;
	// it runs them after Remove returns, and the remote outcome fields then stay empty.
	RemoteHandedOff bool `json:"remote_handed_off,omitempty"`
	// SetAsideTag names the local set-aside/<slug>/<tip> tag put on the weft tip by a local removal;
	// it takes the place of an archive tag and is empty when none was put.
	SetAsideTag string `json:"set_aside_tag,omitempty"`
	// WorkspaceSkipped is true when a local removal found the hub workspace lock held and left the workspace file stale.
	WorkspaceSkipped bool `json:"workspace_skipped,omitempty"`
	// SnapshotTag names the snapshot tag that kept the pair's uncommitted paths before a forced local removal;
	// it is empty when none was taken.
	SnapshotTag string `json:"snapshot_tag,omitempty"`
	// SnapshotPaths lists the paths the snapshot tag holds.
	SnapshotPaths []string `json:"snapshot_paths,omitempty"`
	// SiblingBranch names the pair's sibling branch the removal tore down, read from the pair before its teardown.
	SiblingBranch string `json:"sibling_branch,omitempty"`
}

// TakeOverResult reports what a takeover did.
type TakeOverResult struct {
	// Tags lists the owner archive tags it pushed.
	Tags []string
	// Retries counts the pushes it repeated after origin rejected one.
	Retries int
}

// UnwireVerbResult summarizes what Unwire changed, or on a dry run what it would change.
// It embeds MutationRecord, which carries the mutation record accumulated over the call;
// a dry run records no mutation.
type UnwireVerbResult struct {
	MutationRecord
	// Applied is true only when the junctions and exclude lines were actually removed;
	// false on a dry run.
	Applied bool
	// JunctionsRemoved lists the Name of each warp junction that was actually present and removed.
	// It is empty when no junction was wired.
	// On a dry run it lists the junctions that would be removed, all left in place.
	JunctionsRemoved []string
	// WeftContent is "preserved" when the weft-side _lym exists, and "not_present" otherwise.
	// Unwire never deletes weft-side content, so .lym is never touched either.
	// On Windows an open handle inside it makes the weft worktree's removal fail; stop the daemons and re-run.
	WeftContent string
	// GitExclude is "reverted" or "unchanged".
	// On a dry run "reverted" means an exclude line would be stripped, and it is left in place.
	GitExclude string
}
