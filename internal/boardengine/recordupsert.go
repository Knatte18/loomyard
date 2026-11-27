// recordupsert.go holds the upsert that replaces an entry's stored decision record in the write that lands its payload.

package boardengine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fsx"
)

// UpsertTaskWithRecord upserts fields onto the existing entry fields["slug"] names.
// It replaces the entry's stored decision record with record.
// Both land in one write under the board lock, and it returns the written entry and the siblings the write created.
// It refuses, with nothing written and each refusal naming its way forward:
// a run agent's session, as the board's strand name classifies it;
// a slug the board does not hold, an entry with no stored record and a done entry;
// an entry a run holds, and an entry whose task worktree stands in the hub;
// and an entry whose weft branch weftLive reports live, or whose probe fails.
// A family project whose raddle filing the raddle store records is exempt from the worktree and weft refusals.
// Its run reads no record again.
// weftLive may read origin, so it runs before the board lock is taken.
// The entry refusals are judged again under the lock.
// The payload is validated before the record file is written, and the record file is written before board.json is saved.
// The write stamps the entry's record_body_sha256 with the sha256 of the body it leaves.
// When the write fails after the record file was written, the entry's former record is written back.
func (b *Board) UpsertTaskWithRecord(fields map[string]any, record []byte, weftLive func(slug string) (bool, error)) (Task, []Sibling, error) {
	if isRunAgentSession(b.strandName) {
		return Task{}, nil, fmt.Errorf("the run session %s cannot replace a stored decision record: the run reads its own record, never the board's; "+
			"way forward: change the record on the run with `lym loom decision replace <slug>`, or ask the operator or an orch to run the upsert", b.strandName)
	}
	slug, _ := fields["slug"].(string)
	exempt, err := b.judgeRecordReplace(slug)
	if err != nil {
		return Task{}, nil, err
	}
	if !exempt {
		live, err := weftLive(slug)
		if err != nil {
			return Task{}, nil, fmt.Errorf("the weft branch of %q could not be probed: %w; nothing was written; way forward: resolve the cause above, then re-run the upsert", slug, err)
		}
		if live {
			return Task{}, nil, fmt.Errorf("the weft branch of %q is live, so a raddle start would adopt it and keep the record its run committed, never the stored one; nothing was written; "+
				"way forward: wait for the pair's teardown to finish, or, when its remote step failed, carry out the way forward its failure mail names, then re-run the upsert", slug)
		}
	}

	name := DecisionFileName(slug)
	path := filepath.Join(b.boardPath, name)
	var former []byte
	var siblings []Sibling
	recordWritten, saved := false, false
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		if _, err := b.judgeRecordReplaceIn(store, slug); err != nil {
			return nil, err
		}
		loaded := cloneTasks(store.Tasks())
		task, err := store.UpsertTask(fields)
		if err != nil {
			return nil, err
		}
		if former, err = os.ReadFile(path); err != nil {
			return nil, fmt.Errorf("read decision record %s: %w", name, err)
		}
		if err := fsx.AtomicWrite(b.boardPath, name, string(record)); err != nil {
			return nil, fmt.Errorf("write decision record %s: %w", name, err)
		}
		recordWritten = true
		i, _ := store.index(slug)
		store.tasks[i].RecordBodySHA256 = bodySHA256(store.tasks[i].Body)
		siblings = addedSiblings(loaded, store.Tasks(), b.vocab)
		return task, nil
	}, func() error {
		saved = true
		return nil
	})
	if err != nil {
		if recordWritten && !saved {
			_ = fsx.AtomicWriteBytes(path, former)
		}
		return Task{}, nil, err
	}
	return result.(Task), siblings, nil
}

// judgeRecordReplace loads the board without the lock and judges the entry slug as judgeRecordReplaceIn does.
func (b *Board) judgeRecordReplace(slug string) (bool, error) {
	store, err := b.loadStore()
	if err != nil {
		return false, err
	}
	return b.judgeRecordReplaceIn(store, slug)
}

// judgeRecordReplaceIn refuses an entry of store whose stored decision record an upsert may not replace.
// It refuses a slug store does not hold, an entry with no stored record, a done entry and an entry a run holds.
// It also refuses an entry whose task worktree stands in the hub.
// A family project whose filing is done is exempt from that refusal.
// It reports whether the entry is such a family project, which the weft refusal exempts too.
func (b *Board) judgeRecordReplaceIn(store *Store, slug string) (bool, error) {
	task, found := store.GetTask(slug)
	switch {
	case !found:
		return false, fmt.Errorf("no board entry %q: --record-file replaces the stored decision record of an existing entry; nothing was written; "+
			"way forward: file the entry with `lym board upsert` without --record-file, and lym stores its record when its run's Discussion is approved", slug)
	case !task.Discussed || !b.recordStored(slug):
		return false, fmt.Errorf("entry %q has no stored decision record to replace; nothing was written; "+
			"way forward: drop --record-file, since lym stores an entry's record when its run's Discussion is approved", slug)
	case isDone(task):
		return false, fmt.Errorf("entry %q is done, and a landed entry keeps the decision record it last stored; nothing was written; "+
			"way forward: file the change as a new entry with `lym board upsert`", slug)
	case RunHolds(task):
		return false, fmt.Errorf("entry %q is held by a run (status %q), which reads its own record and never the stored one; nothing was written; "+
			"way forward: change the record on the run with `lym loom decision replace %s`", slug, *task.Status, slug)
	}
	raddle, err := ReadRaddleStore(b.boardPath)
	if err != nil {
		return false, err
	}
	filed, held := raddle.Holds(slug)
	familyFiled := task.Kind == KindProject && held && filed.Project == slug
	if !familyFiled && b.pairExists != nil && b.pairExists(slug) {
		return false, fmt.Errorf("entry %q has a task worktree in the hub, and Record-Write never overwrites the record a worktree holds, so a resumed run there would keep the old one; nothing was written; "+
			"way forward: run `lym loom decision replace %s` in that worktree, or end the run first with `lym batten cancel %s` from the hub's prime worktree", slug, slug, slug)
	}
	return familyFiled, nil
}
