// mutation.go declares the mutation entry and its closed kind vocabulary as plain data.
// It also declares the record value and the carrier of a result's record.
// It is the only file that declares the Kind enum.
// A new member lands in the same commit as its recording site and its cmd/lym/destructiveguard_test.go guard entry,
// and no other file in the tree declares a kind string literal.

package fabriclayout

import "encoding/json"

// Kind names the fixed, closed vocabulary of mutation kinds a Mutation entry can carry.
// Its string values are part of fabric's public JSON contract, so an existing constant's spelling must never change.
type Kind string

// The fixed set of mutation kinds fabric records.
// The kinds the destruction gate performs are auto-recorded by its executors;
// the rest are hand-recorded at their success sites, since no chokepoint covers them.
const (
	// KindPathRemoved records a deletion of a single path or a directory tree.
	KindPathRemoved Kind = "path_removed"
	// KindWorktreeRemoved records a `git worktree remove`.
	KindWorktreeRemoved Kind = "worktree_removed"
	// KindLinkRemoved records a link's removal, including the removal half of a repoint.
	KindLinkRemoved Kind = "link_removed"
	// KindBranchDeleted records a `git branch -D`.
	KindBranchDeleted Kind = "branch_deleted"
	// KindRemoteBranchDeleted records a `git push <remote> --delete`.
	// It is recorded only for a delete that observably removed a ref on the remote.
	// It has its own kind because only the local deletion is recoverable,
	// and the remote copy is gone for good once pushed away.
	// A consumer switching on kind must be able to tell the two deletions apart.
	KindRemoteBranchDeleted Kind = "remote_branch_deleted"
	// KindRemoteBranchUpdated records a leased push once it observably moved a task branch on the remote.
	// Its Target is the remote ref (`<remote>/<branch>`) and its Detail is the commit the branch was moved to.
	// The remote ref survives and only its tip moves.
	KindRemoteBranchUpdated Kind = "remote_branch_updated"
	// KindWorktreeReset records a worktree reset of either kind, hard or keep.
	KindWorktreeReset Kind = "worktree_reset"
	// KindDirCreated records a directory creation at a success site, including the exclusive mint of a new directory.
	// It is recorded only for a directory that was absent beforehand.
	KindDirCreated Kind = "dir_created"
	// KindPathRenamed records a rename that moved a directory tree into place.
	// Its Target is the destination and its Detail is the hub-relative source path.
	KindPathRenamed Kind = "path_renamed"
	// KindWorktreeCreated records a `git worktree add`.
	KindWorktreeCreated Kind = "worktree_created"
	// KindBranchCreated records a verb success site that created a branch.
	// Its Detail is "side=<code|records> repo=<abs path>".
	KindBranchCreated Kind = "branch_created"
	// KindBranchPushed records a verb success site that pushed a branch.
	// Its Detail is "side=<code|records> repo=<abs path> remote=<name>", with remote omitted when unknown.
	KindBranchPushed Kind = "branch_pushed"
	// KindCommitCreated records a verb success site that landed a commit.
	KindCommitCreated Kind = "commit_created"
	// KindLinkCreated records a verb success site that created a link.
	// It includes the create half of a repoint.
	KindLinkCreated Kind = "link_created"
	// KindFileWritten records a verb success site that rewrote a file (.git/info/exclude, config).
	KindFileWritten Kind = "file_written"
	// KindPushSpawned records that a push was launched in a detached child process.
	// The current process does not observe that push run to completion.
	KindPushSpawned Kind = "push_spawned"
	// KindWorktreeSwitched records Checkout's `git switch` sites.
	KindWorktreeSwitched Kind = "worktree_switched"
	// KindRepoAdvanced records a repo advance: a weft pull, a warp advance, or a weft branch brought up to its origin.
	//
	// Target is the advanced worktree root.
	// For a behind weft branch, Detail is the branch and its new tip.
	// The word repo is side-agnostic on purpose.
	// The weft pull and the warp advance share this kind, and Target tells them apart.
	// The Fabric Vocabulary Invariant polices host-compounded phrases, not the bare word "repo".
	// The use is therefore neither a build break nor a naming lapse.
	KindRepoAdvanced Kind = "repo_advanced"
	// KindMergeStaged records a merge verb's MergeStart call that observably changed a checkout's state.
	// It is recorded for a staged, conflicted or fast-forwarded checkout, never for an already-up-to-date no-op.
	KindMergeStaged Kind = "merge_staged"
	// KindMergeResolvedStaged records one side's staging of agent-resolved conflict paths.
	// Its Target is that side's checkout path, and it is recorded only after the staging call observably succeeded.
	// The empty-paths no-op is never recorded.
	KindMergeResolvedStaged Kind = "merge_resolved_staged"
	// KindMergeTrackedStaged records the warp side's staging of every tracked modification and deletion mid-merge;
	// Target is the warp checkout path.
	// It is recorded only after the staging call observably succeeded.
	KindMergeTrackedStaged Kind = "merge_tracked_staged"
	// KindMergeCommitted records a merge verb's conclude-commit landing on one side;
	// its Detail is the new SHA.
	KindMergeCommitted Kind = "merge_committed"
	// KindTagPushed records a push of an archive tag to the weft origin once it observably succeeded.
	// Its Target is the weft repo root and its Detail is the tag name.
	KindTagPushed Kind = "tag_pushed"
	// KindCommitsDropped records commits dropped from a checkout, once HEAD observably moved.
	// Its Target is the checkout and its Detail is the dropped commits' SHAs, space-separated.
	KindCommitsDropped Kind = "commits_dropped"
	// KindTagCreated records the write of a local set-aside tag, recorded only after the tag exists.
	// Its Target is the repository root and its Detail is the tag name.
	KindTagCreated Kind = "tag_created"
)

// Mutation is one flat entry in a mutation record: one primitive that observably changed state on disk or in git.
// Ordering across entries is carried by array order alone; there is no sequence field.
type Mutation struct {
	// Kind names which primitive was performed.
	Kind Kind `json:"kind"`
	// Target names what was mutated: a hub-relative, filepath.ToSlash'd path for anything at or below the hub root;
	// an absolute filepath.ToSlash'd path otherwise.
	// Target is a bare git ref name for a ref-recording entry.
	Target string `json:"target"`
	// Detail is optional kind-specific free text (e.g. the SHA a worktree_reset reset to).
	Detail string `json:"detail,omitempty"`
}

// Mutations is the record of the entries one verb call performed, in the order they happened, as a result carries it.
// The zero value is an empty record.
type Mutations struct {
	entries []Mutation
}

// MutationsOf returns a record holding a copy of entries.
func MutationsOf(entries []Mutation) Mutations {
	return Mutations{entries: append([]Mutation(nil), entries...)}
}

// Entries returns a copy of the record, in order.
// It never returns nil: an empty record returns an empty non-nil slice.
func (m Mutations) Entries() []Mutation {
	out := make([]Mutation, len(m.entries))
	copy(out, m.entries)
	return out
}

// Len reports the number of entries in the record, so a caller can test "record non-empty" without copying the slice.
func (m Mutations) Len() int {
	return len(m.entries)
}

// MarshalJSON implements json.Marshaler: a JSON array of the entries, [] rather than null for an empty record.
func (m Mutations) MarshalJSON() ([]byte, error) {
	entries := m.entries
	if entries == nil {
		entries = []Mutation{}
	}
	return json.Marshal(entries)
}

// MutationRecord is the carrier every mutating result type embeds,
// so a heterogeneous result can be read for its record through one accessor.
type MutationRecord struct {
	// Mutations holds the record accumulated over the call this result reports.
	Mutations Mutations `json:"mutations"`
}

// Mutated returns this result's accumulated mutation record.
func (r MutationRecord) Mutated() Mutations { return r.Mutations }
