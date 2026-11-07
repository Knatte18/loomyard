// mutation.go declares the shared mutation-record vocabulary of this package.
// Every mutating verb accumulates into this vocabulary; each mutating result type embeds it.
// It holds the Kind enum, the flat Mutation entry, the Mutations accumulator, and the MutationRecord carrier.
// It is the only file that declares the Kind enum.
// A new member lands in the same commit as its recording site and its cmd/lym/destructiveguard_test.go guard entry,
// and no other file in the tree declares a kind string literal.

package fabricengine

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Kind names the fixed, closed vocabulary of mutation kinds a Mutation entry can carry.
// Its string values are part of fabric's public JSON contract, so an existing constant's spelling must never change.
type Kind string

// The fixed set of mutation kinds fabric records.
// The kinds the destruction gate performs are auto-recorded by its executors (destroy.go); the rest are hand-recorded at their success sites, since no chokepoint covers them.
const (
	// KindPathRemoved records removePath's deletion of a single path or a directory tree.
	KindPathRemoved Kind = "path_removed"
	// KindWorktreeRemoved records removeGitWorktree's `git worktree remove`.
	KindWorktreeRemoved Kind = "worktree_removed"
	// KindLinkRemoved records removeLink's fslink.Remove, including the removal half of a repoint.
	KindLinkRemoved Kind = "link_removed"
	// KindBranchDeleted records deleteBranch's `git branch -D`.
	KindBranchDeleted Kind = "branch_deleted"
	// KindRemoteBranchDeleted records deleteRemoteBranch's `git push <remote> --delete`.
	// It is recorded only for a delete that observably removed a ref on the remote.
	// It has its own kind because only the local deletion is recoverable,
	// and the remote copy is gone for good once pushed away.
	// A consumer switching on kind must be able to tell the two deletions apart.
	KindRemoteBranchDeleted Kind = "remote_branch_deleted"
	// KindRemoteBranchUpdated records updateRemoteBranch's leased push once it observably moved a task branch on the remote.
	// Its Target is the remote ref (`<remote>/<branch>`) and its Detail is the commit the branch was moved to.
	// The remote ref survives and only its tip moves.
	KindRemoteBranchUpdated Kind = "remote_branch_updated"
	// KindWorktreeReset records resetHardTo's `reset --hard`.
	KindWorktreeReset Kind = "worktree_reset"
	// KindDirCreated records a directory creation at a success site, including createExclusiveDir's mint.
	// It is recorded only for a directory that was absent beforehand.
	KindDirCreated Kind = "dir_created"
	// KindPathRenamed records a rename that moved a directory tree into place.
	// Its Target is the destination and its Detail is the hub-relative source path.
	KindPathRenamed Kind = "path_renamed"
	// KindWorktreeCreated records createGitWorktree's `git worktree add`.
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
	// KindTagPushed records archiveWeftTip's push of an archive tag to the weft origin once it observably succeeded.
	// Its Target is the weft repo root and its Detail is the tag name.
	KindTagPushed Kind = "tag_pushed"
	// KindCommitsDropped records dropSeedCommits' reset past the seed commits, once HEAD observably moved.
	// Its Target is the board checkout and its Detail is the dropped commits' SHAs, space-separated.
	KindCommitsDropped Kind = "commits_dropped"
	// KindTagCreated records setAsideTag's write of a local set-aside tag, recorded only after the tag exists.
	// Its Target is the repository root and its Detail is the tag name.
	KindTagCreated Kind = "tag_created"
)

// Mutation is one flat entry in a Mutations record: one primitive that observably changed state on disk or in git.
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

// Mutations accumulates the Mutation entries a single verb call performs, in the order they were recorded.
// It is constructed once per verb call and threaded as a pointer into everything that call performs.
//
// The mutating methods take a pointer receiver and must be nil-safe;
// a verb may install its populating defer before the recorder exists.
// The read-only methods take a value receiver, since a result type hands out its Mutations by value:
// that value is not addressable, so a pointer receiver would not compile on it.
//
// Each appended entry is also written to the durable trace sink,
// so a killed process's trace keeps every mutation up to the last completed one.
type Mutations struct {
	// hubRoot is the absolute path Append converts a target against.
	// When it is empty, Append never converts and records the absolute slashed path.
	hubRoot string
	// entries holds the accumulated record, in append order.
	entries []Mutation
}

// NewMutations constructs the one recorder a verb call threads through everything it performs.
// hubRoot may be empty, in which case Append records the absolute slashed path without converting.
func NewMutations(hubRoot string) *Mutations {
	return &Mutations{hubRoot: hubRoot}
}

// Append records one entry, converting an absolute target to a hub-relative, filepath.ToSlash'd Target.
// It falls back to the absolute slashed path in three cases:
// filepath.Rel errors, the relative path escapes the hub root, or hubRoot is empty.
// A target that resolves to the hub root itself is recorded as the literal ".", matching filepath.Rel's output.
// A nil receiver is safe: the call records nothing and does not panic.
func (m *Mutations) Append(kind Kind, target, detail string) {
	if m == nil {
		return
	}
	recorded := hubRelativeTarget(m.hubRoot, target)
	m.entries = append(m.entries, Mutation{Kind: kind, Target: recorded, Detail: detail})
	logger.Info("fabric: mutation", "kind", string(kind), "target", recorded, "detail", detail)
}

// hubRelativeTarget returns target relative to hubRoot, with forward slashes, when target lies under hubRoot.
// Otherwise it returns target with forward slashes.
func hubRelativeTarget(hubRoot, target string) string {
	if hubRoot == "" {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(hubRoot, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return filepath.ToSlash(target)
	}
	return rel
}

// refDetail builds the Detail of a branch_created or branch_pushed entry: "side=<side> repo=<repo>",
// plus " remote=<remote>" when remote is non-empty.
// side is "code" or "records"; repo is made absolute (kept as given on error) and slashed.
func refDetail(side, repo, remote string) string {
	if !filepath.IsAbs(repo) {
		if abs, err := filepath.Abs(repo); err == nil {
			repo = abs
		}
	}
	detail := "side=" + side + " repo=" + filepath.ToSlash(repo)
	if remote != "" {
		detail += " remote=" + remote
	}
	return detail
}

// Extend appends every entry of other to m verbatim, performing no path conversion: other's entries
// were already converted by whichever Mutations produced them.
// A verb that calls another recording entry point folds that call's record in with Extend,
// and the CLI layer appends its own entries after the engine's record the same way.
// Extend logs nothing: its entries were already logged when first appended.
// A nil receiver is safe, and an empty other is a no-op.
func (m *Mutations) Extend(other Mutations) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, other.entries...)
}

// AppendRef records one entry with Target set to ref verbatim, performing no path arithmetic.
// It is the recording path for git-ref kinds (branch_created, branch_deleted, branch_pushed).
// A nil receiver is safe and records nothing.
func (m *Mutations) AppendRef(kind Kind, ref, detail string) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, Mutation{Kind: kind, Target: ref, Detail: detail})
	logger.Info("fabric: mutation", "kind", string(kind), "target", ref, "detail", detail)
}

// Entries returns a copy of the accumulated record, in append order.
// It never returns the internal slice, and never returns nil: an empty record returns an empty
// non-nil slice.
func (m Mutations) Entries() []Mutation {
	out := make([]Mutation, len(m.entries))
	copy(out, m.entries)
	return out
}

// Snapshot returns a value copy of m with its own freshly allocated entries.
// Later appends through the recorder cannot change an already-returned result.
// A nil receiver is safe and returns a zero-value Mutations.
func (m *Mutations) Snapshot() Mutations {
	if m == nil {
		return Mutations{}
	}
	entries := make([]Mutation, len(m.entries))
	copy(entries, m.entries)
	return Mutations{hubRoot: m.hubRoot, entries: entries}
}

// Len reports the number of entries in the record, so a caller can test "record non-empty" without
// copying the slice.
func (m Mutations) Len() int {
	return len(m.entries)
}

// MarshalJSON marshals m to a JSON array of its entries, emitting [] rather than null for an empty
// or zero-value record. It is declared on the value receiver so both Mutations and *Mutations
// marshal identically.
func (m Mutations) MarshalJSON() ([]byte, error) {
	entries := m.entries
	if entries == nil {
		entries = []Mutation{}
	}
	return json.Marshal(entries)
}

// MutationRecord is the carrier every mutating result type embeds, so a heterogeneous result can be
// read for its record through one accessor.
type MutationRecord struct {
	// Mutations holds the record accumulated over the call this result reports.
	Mutations Mutations `json:"mutations"`
}

// Mutated returns this result's accumulated mutation record.
// It is the one accessor the package's live-state tests read a heterogeneous result through.
func (r MutationRecord) Mutated() Mutations { return r.Mutations }
