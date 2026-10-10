// runwrites.go reads the run's own write history: every Write, Edit or NotebookEdit a Master session
// or one of its forks or recovery sessions made, with each write's time and result.
// It is the one reader behind the contract-file evidence, the own-path set of a reset and the uncommitted-path classing of a recovery prompt.

package websterengine

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// RunWrites is the write history of one run's sessions.
type RunWrites struct {
	// Master holds every Master session's own writes, session by session, each in transcript order.
	Master []shuttleengine.WriteEvent
	// Forks holds every fork's writes, session by session, each in discovery then transcript order.
	Forks []shuttleengine.WriteEvent
	// Recoveries holds every recovery session's own writes, session by session, each in transcript order.
	Recoveries []shuttleengine.WriteEvent
}

// loadRunWrites audits every session the state records and collects the write events.
// The Master sessions are State.MasterSessionID then each batch record's SessionID in batch-number order;
// the recovery sessions follow, each batch's RecoverySessions in batch-number order; both are deduplicated, an empty id skipped, and each id is audited once.
// It reads only; an audit error is returned naming the session.
func loadRunWrites(engine shuttleengine.Engine, st *State, worktree string) (RunWrites, error) {
	var writes RunWrites
	if st == nil {
		return writes, nil
	}
	master, recovery := recordedSessions(st)
	for _, session := range master {
		audit, err := engine.AuditForks(session, worktree)
		if err != nil {
			return RunWrites{}, fmt.Errorf("audit session %s for run writes: %w", session, err)
		}
		writes.Master = append(writes.Master, audit.ParentWriteEvents...)
		for _, fork := range audit.Forks {
			writes.Forks = append(writes.Forks, fork.WriteEvents...)
		}
	}
	for _, session := range recovery {
		audit, err := engine.AuditForks(session, worktree)
		if err != nil {
			return RunWrites{}, fmt.Errorf("audit recovery session %s for run writes: %w", session, err)
		}
		writes.Recoveries = append(writes.Recoveries, audit.ParentWriteEvents...)
		for _, fork := range audit.Forks {
			writes.Forks = append(writes.Forks, fork.WriteEvents...)
		}
	}
	return writes, nil
}

// writtenWorktreePaths returns, sorted and deduplicated, the slash-separated paths relative to worktree that some session of the run wrote successfully.
// Unlike the tracked own paths of a reset it keeps untracked files, and a failed write is not evidence.
// A path outside the worktree is dropped, and the error is a link-resolution failure.
func writtenWorktreePaths(writes RunWrites, worktree string) ([]string, error) {
	root, err := canonicalPath(worktree)
	if err != nil {
		return nil, err
	}
	var written []string
	for _, ev := range slices.Concat(writes.Master, writes.Forks, writes.Recoveries) {
		if !ev.Succeeded {
			continue
		}
		canon, err := canonicalPath(resolveWritePath(worktree, ev.Path))
		if err != nil {
			return nil, err
		}
		if !pathWithin(root, canon) {
			continue
		}
		rel, err := filepath.Rel(root, canon)
		if err != nil {
			return nil, fmt.Errorf("websterengine: relate %s to %s: %w", canon, root, err)
		}
		if rel = filepath.ToSlash(rel); rel != "." && !slices.Contains(written, rel) {
			written = append(written, rel)
		}
	}
	slices.Sort(written)
	return written, nil
}

// recordedSessions lists the distinct non-empty Master session ids st records, and separately the distinct recovery session ids of every batch in batch-number order, each in a stable order.
// An id recorded as a Master session is never listed again as a recovery session.
func recordedSessions(st *State) (master, recovery []string) {
	add := func(list *[]string, id string) {
		if id != "" && !slices.Contains(master, id) && !slices.Contains(recovery, id) {
			*list = append(*list, id)
		}
	}
	add(&master, st.MasterSessionID)
	numbers := make([]int, 0, len(st.Batches))
	for n := range st.Batches {
		numbers = append(numbers, n)
	}
	slices.Sort(numbers)
	for _, n := range numbers {
		if b := st.Batches[n]; b != nil {
			add(&master, b.SessionID)
		}
	}
	for _, n := range numbers {
		if b := st.Batches[n]; b != nil {
			for _, id := range b.RecoverySessions {
				add(&recovery, id)
			}
		}
	}
	return master, recovery
}
