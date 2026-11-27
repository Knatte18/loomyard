// builder.go holds the chainable builder of made-up snapshots.

package fabrickit

import (
	"maps"
	"slices"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// Builder collects the facts of one snapshot; every setter records its value and returns the builder.
type Builder struct {
	snapshot fabriccore.Snapshot
}

// NewSnapshot returns an empty builder.
func NewSnapshot() *Builder {
	return &Builder{}
}

// Phases sets the phases read.
func (b *Builder) Phases(phases ...fabriccore.Phase) *Builder {
	b.snapshot.Phases = slices.Clone(phases)
	return b
}

// LocalRef sets one local ref of store.
func (b *Builder) LocalRef(store fabriccore.Store, ref, sha string) *Builder {
	refs := b.snapshot.Stores[store]
	if refs.Local == nil {
		refs.Local = fabriccore.Refs{}
	}
	refs.Local[ref] = sha
	if b.snapshot.Stores == nil {
		b.snapshot.Stores = map[fabriccore.Store]fabriccore.StoreRefs{}
	}
	b.snapshot.Stores[store] = refs
	return b
}

// OriginRef sets one ref of store on origin and marks the store's origin as read.
func (b *Builder) OriginRef(store fabriccore.Store, ref, sha string) *Builder {
	refs := b.snapshot.Stores[store]
	if refs.Origin == nil {
		refs.Origin = fabriccore.Refs{}
	}
	refs.Origin[ref] = sha
	refs.OriginRead = true
	if b.snapshot.Stores == nil {
		b.snapshot.Stores = map[fabriccore.Store]fabriccore.StoreRefs{}
	}
	b.snapshot.Stores[store] = refs
	return b
}

// Worktree appends one worktree of the code repository.
func (b *Builder) Worktree(entry fabriclayout.WorktreeEntry) *Builder {
	b.snapshot.Worktrees = append(b.snapshot.Worktrees, entry)
	return b
}

// Anchor sets the anchor marker of the worktree at the path worktree.
func (b *Builder) Anchor(worktree, anchor string) *Builder {
	if b.snapshot.Anchors == nil {
		b.snapshot.Anchors = map[string]string{}
	}
	b.snapshot.Anchors[worktree] = anchor
	return b
}

// RecordsStore sets the description of the records store.
func (b *Builder) RecordsStore(store fabriccore.RecordsStore) *Builder {
	b.snapshot.RecordsStore = store
	return b
}

// PendingTeardown appends one unfinished remote teardown.
func (b *Builder) PendingTeardown(record fabriclayout.PendingTeardown) *Builder {
	b.snapshot.PendingTeardowns = append(b.snapshot.PendingTeardowns, record)
	return b
}

// Outcome appends one action outcome.
func (b *Builder) Outcome(outcome fabriccore.Outcome) *Builder {
	b.snapshot.Outcomes = append(b.snapshot.Outcomes, outcome)
	return b
}

// ReadFailure appends one failed read.
func (b *Builder) ReadFailure(failure fabriccore.ReadFailure) *Builder {
	b.snapshot.ReadFailures = append(b.snapshot.ReadFailures, failure)
	return b
}

// Build returns the snapshot of what is set so far.
// It shares no map or slice with the builder, so a later setter call never changes it.
func (b *Builder) Build() fabriccore.Snapshot {
	built := b.snapshot
	built.Phases = slices.Clone(built.Phases)
	built.Stores = maps.Clone(built.Stores)
	for store, refs := range built.Stores {
		refs.Local = maps.Clone(refs.Local)
		refs.Origin = maps.Clone(refs.Origin)
		built.Stores[store] = refs
	}
	built.Worktrees = slices.Clone(built.Worktrees)
	built.Anchors = maps.Clone(built.Anchors)
	built.PendingTeardowns = slices.Clone(built.PendingTeardowns)
	built.Outcomes = slices.Clone(built.Outcomes)
	for i := range built.Outcomes {
		built.Outcomes[i].Action.Paths = slices.Clone(built.Outcomes[i].Action.Paths)
	}
	built.ReadFailures = slices.Clone(built.ReadFailures)
	return built
}
