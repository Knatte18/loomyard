// replayer.go holds heddle's Bolt replayer: the by-entry merge of a local heddle.json commit onto origin's store.

package heddleengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/entrymerge"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// Replayer is heddle's Bolt replayer, named heddle.
// It holds no state, claims the store file alone and is no run ledger.
type Replayer struct{}

var _ fabricengine.BoltReplayer = Replayer{}

// Name returns the replayer's registry name.
func (Replayer) Name() string { return "heddle" }

// Claims reports whether path, a Bolt-root-relative slash path, is the store file.
// The lym override file is left unclaimed, since its entries carry no order or targets for the by-entry merge.
func (Replayer) Claims(path string) bool { return path == FileName }

// entryFields is the merge's table over Entry, keyed by slug.
// Unknown keys are not fields: they stay as origin holds them.
var entryFields = entrymerge.Table[Entry]{
	Key: func(e Entry) string { return e.Slug },
	Fields: []entrymerge.Field[Entry]{
		{Name: "text", Get: func(e Entry) any { return e.Text }, Set: func(dst *Entry, src Entry) { dst.Text = src.Text }},
		{Name: "order", Get: func(e Entry) any { return e.Order }, Set: func(dst *Entry, src Entry) { dst.Order = src.Order }},
		{Name: "targets", Get: func(e Entry) any {
			if e.Targets == nil {
				return []string{}
			}
			return e.Targets
		}, Set: func(dst *Entry, src Entry) { dst.Targets = slices.Clone(src.Targets) }},
	},
}

// replayRun is the state of one Apply: origin's store, what the commit's three sides held and what has been applied or set aside so far.
type replayRun struct {
	store   Store
	base    Store
	result  fabricengine.ReplayResult
	applied bool
}

// Apply re-applies the request's heddle.json commit onto origin's store in req.Dir by entry, and reports what it kept and what it set aside.
// A changed entry merges field by field with origin winning a field both sides changed, an added entry is appended unless origin added the slug too, and a removed entry is dropped only while origin still holds it as the commit's parent did.
// A change to an entry origin no longer holds is reported lost.
// An entry that fails ValidateEntry is reported refused and the rest of the commit still lands.
// It saves through Save, so every unknown key stays as origin holds it, and writes nothing when no change survived.
// It takes no lock, since its caller holds the Bolt write and push locks.
func (Replayer) Apply(req fabricengine.ReplayRequest) (fabricengine.ReplayResult, error) {
	var baseBytes, parentBytes, commitBytes []byte
	for _, file := range req.Files {
		if file.Path == FileName {
			baseBytes, parentBytes, commitBytes = file.Base, file.Before, file.After
		}
	}
	base, err := Decode(baseBytes)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("merge base %s: %w", FileName, err)
	}
	parent, err := Decode(parentBytes)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("parent %s: %w", FileName, err)
	}
	commit, err := Decode(commitBytes)
	if err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("commit %s: %w", FileName, err)
	}
	origin, err := Load(req.Dir)
	if err != nil {
		return fabricengine.ReplayResult{}, err
	}

	run := &replayRun{store: origin, base: base}
	run.store.Entries = slices.Clone(origin.Entries)
	for _, entry := range commit.Entries {
		if before, existed := parent.Get(entry.Slug); existed {
			run.changeEntry(before, entry)
		} else {
			run.addEntry(entry)
		}
	}
	for _, before := range parent.Entries {
		if _, kept := commit.Get(before.Slug); !kept {
			run.removeEntry(before)
		}
	}

	run.result.Empty = !run.applied
	if run.result.Empty {
		return run.result, nil
	}
	if err := Save(req.Dir, run.store); err != nil {
		return fabricengine.ReplayResult{}, fmt.Errorf("save %s: %w", FileName, err)
	}
	return run.result, nil
}

func (r *replayRun) lost(entry, field, reason, local string) {
	r.result.Lost = append(r.result.Lost, fabricengine.ReplayFinding{Entry: entry, Field: field, Reason: reason, Local: local})
}

func (r *replayRun) refuse(entry, field string, err error) {
	r.result.Refused = append(r.result.Refused, fabricengine.ReplayFinding{Entry: entry, Field: field, Reason: err.Error()})
}

// originIndex returns the index of the entry origin holds under slug, or -1.
func (r *replayRun) originIndex(slug string) int {
	return slices.IndexFunc(r.store.Entries, func(entry Entry) bool { return entry.Slug == slug })
}

// changeEntry applies the fields commit changed against before onto origin's entry.
// A field origin already holds is skipped, a field origin moved to a third value is reported lost, and an entry origin no longer holds reports the change lost.
func (r *replayRun) changeEntry(before, commit Entry) {
	if len(entryFields.Changed(before, commit)) == 0 {
		return
	}
	index := r.originIndex(commit.Slug)
	if index < 0 {
		r.lost(commit.Slug, "", "origin does not hold the entry", "")
		return
	}
	origin := r.store.Entries[index]
	origin.Targets = slices.Clone(origin.Targets)
	base, _ := r.base.Get(commit.Slug)
	merged, applied, losses := entryFields.Change(base, before, origin, commit)
	for _, loss := range losses {
		r.lost(loss.Entry, loss.Field, loss.Reason, loss.Local)
	}
	if len(applied) == 0 {
		return
	}
	if err := ValidateEntry(merged); err != nil {
		r.refuse(commit.Slug, strings.Join(applied, ", "), err)
		return
	}
	r.store.Entries[index] = merged
	r.applied = true
}

// addEntry appends commit's entry when origin holds none under its slug.
// Where origin holds one, each field the commit's entry carries differently is reported lost.
func (r *replayRun) addEntry(commit Entry) {
	if index := r.originIndex(commit.Slug); index >= 0 {
		for _, clash := range entryFields.AddClash(r.store.Entries[index], commit) {
			r.lost(clash.Entry, clash.Field, clash.Reason, clash.Local)
		}
		return
	}
	if err := ValidateEntry(commit); err != nil {
		r.refuse(commit.Slug, "", err)
		return
	}
	r.store.Entries = append(r.store.Entries, commit)
	r.applied = true
}

// removeEntry drops before's entry from origin's store while origin still holds it as the commit's parent did.
// An entry origin changed is kept and reported lost, and one origin already dropped needs nothing.
func (r *replayRun) removeEntry(before Entry) {
	index := r.originIndex(before.Slug)
	if index < 0 {
		return
	}
	if !entryFields.Same(r.store.Entries[index], before) {
		r.lost(before.Slug, "", "origin changed the entry this commit removes", "")
		return
	}
	r.store.Entries = slices.Delete(r.store.Entries, index, index+1)
	r.applied = true
}
