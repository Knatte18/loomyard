// mutation.go declares the shared mutation-record vocabulary every mutating verb in this package
// accumulates into and every mutating result type embeds: the Kind enum, the flat Mutation entry,
// the Mutations accumulator, and the MutationRecord carrier a result type embeds to expose it.
// This is the single declarer of the Kind enum — a new member lands in the same commit as its
// recording site and its cmd/lyx/destructiveguard_test.go guard entry, and no other file in the
// tree declares a kind string literal.

package fabricengine

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Kind names the fixed, closed vocabulary of mutation kinds a Mutation entry can carry.
// Its string values are part of fabric's public JSON contract, so an existing constant's spelling
// must never change, and a new member lands in the same commit as its recording site and its
// cmd/lyx/destructiveguard_test.go guard entry — no other file declares a kind string literal.
type Kind string

// The fixed set of mutation kinds fabric records.
// Seven are auto-recorded by the destruction gate (destroy.go); the remaining eleven are
// hand-recorded at their success sites, since no chokepoint covers them.
const (
	// KindPathRemoved records removePath's deletion of a single path or a directory tree.
	KindPathRemoved Kind = "path_removed"
	// KindWorktreeRemoved records removeGitWorktree's `git worktree remove`.
	KindWorktreeRemoved Kind = "worktree_removed"
	// KindLinkRemoved records removeLink's fslink.Remove, including the removal half of a repoint.
	KindLinkRemoved Kind = "link_removed"
	// KindBranchDeleted records deleteBranch's `git branch -D`.
	KindBranchDeleted Kind = "branch_deleted"
	// KindWorktreeReset records resetHardTo's `reset --hard`.
	KindWorktreeReset Kind = "worktree_reset"
	// KindDirCreated records createExclusiveDir's directory mint.
	KindDirCreated Kind = "dir_created"
	// KindWorktreeCreated records createGitWorktree's `git worktree add`.
	KindWorktreeCreated Kind = "worktree_created"
	// KindBranchCreated records a verb success site that created a branch.
	KindBranchCreated Kind = "branch_created"
	// KindBranchPushed records a verb success site that pushed a branch.
	KindBranchPushed Kind = "branch_pushed"
	// KindCommitCreated records a verb success site that landed a commit.
	KindCommitCreated Kind = "commit_created"
	// KindLinkCreated records a verb success site that created a link, including the create half
	// of a repoint.
	KindLinkCreated Kind = "link_created"
	// KindFileWritten records a verb success site that rewrote a file (.git/info/exclude, config).
	KindFileWritten Kind = "file_written"
	// KindPushSpawned records that a push was launched via a detached child process, not observed
	// to completion by the current process.
	KindPushSpawned Kind = "push_spawned"
	// KindWorktreeSwitched records Checkout's `git switch` sites.
	KindWorktreeSwitched Kind = "worktree_switched"
	// KindRepoAdvanced records a repo advance: either the weft fast-forward pull or the warp
	// advance.
	//
	// "repo" is deliberately side-agnostic here and is not vocabulary drift: this kind is recorded
	// for both the weft fast-forward pull and the warp advance, and the entry's Target — the
	// advanced worktree root — is what tells the two sides apart.
	// The Fabric Vocabulary Invariant polices host-compounded phrases, not the bare word "repo", so
	// this neither breaks the build nor bends the rule; the comment exists so the reviewer of the
	// next slice reads it as the decision it is rather than as a warp/weft naming lapse.
	KindRepoAdvanced Kind = "repo_advanced"
	// KindMergeStaged records a merge verb's MergeStart call that observably changed a checkout's
	// state (staged, conflicted, or fast-forwarded) — never an already-up-to-date no-op.
	KindMergeStaged Kind = "merge_staged"
	// KindMergeResolvedStaged records one side's staging of agent-resolved conflict paths; Target is
	// that side's checkout path. It is recorded only after the staging call observably succeeded —
	// never on the empty-paths no-op.
	KindMergeResolvedStaged Kind = "merge_resolved_staged"
	// KindMergeCommitted records a merge verb's conclude-commit landing on one side; Detail is the
	// new SHA.
	KindMergeCommitted Kind = "merge_committed"
)

// Mutation is one flat entry in a Mutations record, naming one primitive that observably changed
// state on disk or in git. Ordering across entries is carried by array order alone — there is
// deliberately no sequence field.
type Mutation struct {
	// Kind names which primitive was performed.
	Kind Kind `json:"kind"`
	// Target names what was mutated: a hub-relative, filepath.ToSlash'd path for anything at or
	// below the hub root, an absolute filepath.ToSlash'd path otherwise, or a bare git ref name for
	// a ref-recording entry.
	Target string `json:"target"`
	// Detail is optional kind-specific free text (e.g. the SHA a worktree_reset reset to).
	Detail string `json:"detail,omitempty"`
}

// Mutations accumulates the Mutation entries a single verb call performs, in the order they were
// recorded. It is constructed once per verb call via NewMutations and threaded as a *Mutations
// into everything that call performs.
//
// Receiver rule, and why it is split: the mutating and snapshotting methods (Append, AppendRef,
// Extend, Snapshot) take a pointer receiver and must be nil-safe, since CloneHub and Unwire both
// install their populating defer before the recorder exists. The read-only methods (Entries, Len,
// MarshalJSON) take a value receiver and carry no nil-safety obligation, because they are called on
// the Mutations value a result type carries (via MutationRecord.Mutated), never on a recorder
// pointer — Mutated returns a non-addressable Mutations value, so a pointer receiver would not
// compile against res.Mutated().Entries()/.Len().
type Mutations struct {
	// hubRoot is the absolute path Append converts a target against. It may be empty, in which
	// case Append never converts and records the absolute slashed path.
	hubRoot string
	// entries holds the accumulated record, in append order.
	entries []Mutation
}

// NewMutations constructs the one recorder a verb call threads through everything it performs.
// hubRoot may be empty, in which case Append never converts a target and records the absolute
// slashed path instead.
func NewMutations(hubRoot string) *Mutations {
	return &Mutations{hubRoot: hubRoot}
}

// Append records one entry, converting target from an absolute filesystem path to a hub-relative,
// filepath.ToSlash'd Target.
// The conversion falls back to the absolute slashed path when filepath.Rel errors, when the result
// escapes the hub root (a ".." first segment), or when hubRoot is empty.
// A target that resolves to the hub root itself is recorded as the literal ".", matching
// filepath.Rel's own output — this is CloneHub's hub-minting case, deliberately kept rather than
// special-cased away.
// A nil receiver is safe: it returns without panicking, so a not-yet-threaded call site degrades to
// recording nothing rather than crashing.
func (m *Mutations) Append(kind Kind, target, detail string) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, Mutation{
		Kind:   kind,
		Target: hubRelativeTarget(m.hubRoot, target),
		Detail: detail,
	})
}

// hubRelativeTarget converts target to a hub-relative, filepath.ToSlash'd path when it descends
// from hubRoot, falling back to the absolute filepath.ToSlash'd path otherwise.
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

// Extend appends every entry of other to m verbatim, performing no path conversion: other's entries
// were already converted by whichever Mutations produced them.
// This is the composition primitive for a verb that calls another recording entry point (Unwire
// over UnwireJunctions), and for the CLI layer's concatenate-engine-record-then-CLI-entries rule.
// A nil receiver is safe, and an empty other is a no-op.
func (m *Mutations) Extend(other Mutations) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, other.entries...)
}

// AppendRef records one entry with Target set to ref verbatim, performing no path arithmetic at
// all. This is the git-ref recording path (branch_created, branch_deleted, branch_pushed).
// A nil receiver is safe: it returns without panicking, so a not-yet-threaded call site degrades to
// recording nothing rather than crashing.
func (m *Mutations) AppendRef(kind Kind, ref, detail string) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, Mutation{Kind: kind, Target: ref, Detail: detail})
}

// Entries returns a copy of the accumulated record, in append order.
// It never returns the internal slice, and never returns nil: an empty record returns an empty
// non-nil slice.
func (m Mutations) Entries() []Mutation {
	out := make([]Mutation, len(m.entries))
	copy(out, m.entries)
	return out
}

// Snapshot returns a value copy of m whose entries is a freshly allocated copy of the current
// entries, so later appends through the recorder cannot mutate an already-returned result.
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
// It is the one accessor the package's own live-state harness (the livestate_-prefixed
// package fabricengine_test files, built on internal/hubforge) reads a heterogeneous result through.
func (r MutationRecord) Mutated() Mutations { return r.Mutations }
