// mutation.go declares the mutation recorder of this package, which every mutating verb accumulates into.
// Each mutating result type embeds the record it hands over.
// The entry and its kinds live in internal/fabriclayout, and the recorder here.

package fabricengine

import (
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Mutations accumulates the Mutation entries a single verb call performs, in the order they were recorded.
// It is constructed once per verb call and threaded as a pointer into everything that call performs.
//
// The mutating methods take a pointer receiver and must be nil-safe;
// a verb may install its populating defer before the recorder exists.
//
// Each appended entry is also written to the durable trace sink,
// so a killed process's trace keeps every mutation up to the last completed one.
type Mutations struct {
	// hubRoot is the absolute path Append converts a target against.
	// When it is empty, Append never converts and records the absolute slashed path.
	hubRoot string
	// entries holds the accumulated record, in append order.
	entries []fabriclayout.Mutation
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
func (m *Mutations) Append(kind fabriclayout.Kind, target, detail string) {
	if m == nil {
		return
	}
	recorded := hubRelativeTarget(m.hubRoot, target)
	m.entries = append(m.entries, fabriclayout.Mutation{Kind: kind, Target: recorded, Detail: detail})
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
// were already converted by whichever recorder produced them.
// A verb that calls another recording entry point folds that call's record in with Extend,
// and the CLI layer appends its own entries after the engine's record the same way.
// Extend logs nothing: its entries were already logged when first appended.
// A nil receiver is safe, and an empty other is a no-op.
func (m *Mutations) Extend(other fabriclayout.Mutations) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, other.Entries()...)
}

// AppendRef records one entry with Target set to ref verbatim, performing no path arithmetic.
// It is the recording path for git-ref kinds (branch_created, branch_deleted, branch_pushed).
// A nil receiver is safe and records nothing.
func (m *Mutations) AppendRef(kind fabriclayout.Kind, ref, detail string) {
	if m == nil {
		return
	}
	m.entries = append(m.entries, fabriclayout.Mutation{Kind: kind, Target: ref, Detail: detail})
	logger.Info("fabric: mutation", "kind", string(kind), "target", ref, "detail", detail)
}

// Entries returns a copy of the accumulated record, in append order.
// It never returns the internal slice, and never returns nil: an empty record returns an empty
// non-nil slice.
func (m Mutations) Entries() []fabriclayout.Mutation {
	out := make([]fabriclayout.Mutation, len(m.entries))
	copy(out, m.entries)
	return out
}

// Snapshot returns the record as a value with its own freshly allocated entries.
// Later appends through the recorder cannot change an already-returned result.
// A nil receiver is safe and returns the empty record.
func (m *Mutations) Snapshot() fabriclayout.Mutations {
	if m == nil {
		return fabriclayout.Mutations{}
	}
	return fabriclayout.MutationsOf(m.entries)
}

// Len reports the number of entries recorded so far.
func (m Mutations) Len() int {
	return len(m.entries)
}
