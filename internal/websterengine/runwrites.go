// runwrites.go reads the write history of one run: every Write, Edit or NotebookEdit a session of the run made.
// Each event records its write's time and result.
// It is the one reader behind the contract-file evidence and the own-path set of a reset;
// it also feeds the uncommitted-path classing of a recovery prompt.

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
	// Recoveries holds the own writes of every recovery and cold verify-gate fixer session, each in transcript order;
	// a contract file last written by either is reported as written by "a recovery or verify-fixer session".
	Recoveries []shuttleengine.WriteEvent
}

// loadRunWrites audits every session the state records and collects the write events.
// The Master sessions are State.MasterSessionID, then each batch record's SessionID in batch-number order.
// The recovery sessions follow: each batch's RecoverySessions in batch-number order;
// the cold verify-gate fixers' sessions come last.
// Both lists are deduplicated, an empty id is skipped, and each id is audited once.
// Each session is audited through the engine its own state entry records,
// so a later change to the configured engine never changes it.
// It reads only; an engine or audit error is returned naming the session.
// paneCwd is the directory the sessions ran in, which the audit resolves relative write paths against.
// A nil engines yields an empty history.
func loadRunWrites(engines AuditEngines, st *State, paneCwd string) (RunWrites, error) {
	var writes RunWrites
	if st == nil || engines == nil {
		return writes, nil
	}
	master, recovery := recordedSessions(st)
	for _, session := range master {
		engine, err := sessionAuditEngine(engines, session.ID, session.Engine)
		if err != nil {
			return RunWrites{}, err
		}
		audit, err := engine.AuditForks(session.ID, paneCwd)
		if err != nil {
			return RunWrites{}, fmt.Errorf("audit session %s for run writes: %w", session.ID, err)
		}
		writes.Master = append(writes.Master, audit.ParentWriteEvents...)
		for _, fork := range audit.Forks {
			writes.Forks = append(writes.Forks, fork.WriteEvents...)
		}
	}
	for _, session := range recovery {
		engine, err := sessionAuditEngine(engines, session.ID, session.Engine)
		if err != nil {
			return RunWrites{}, err
		}
		audit, err := engine.AuditForks(session.ID, paneCwd)
		if err != nil {
			return RunWrites{}, fmt.Errorf("audit recovery session %s for run writes: %w", session.ID, err)
		}
		writes.Recoveries = append(writes.Recoveries, audit.ParentWriteEvents...)
		for _, fork := range audit.Forks {
			writes.Forks = append(writes.Forks, fork.WriteEvents...)
		}
	}
	return writes, nil
}

// writtenWorktreePaths returns the sorted, deduplicated paths that some session of the run wrote successfully.
// Each path is slash-separated and relative to worktree.
// It keeps untracked files, and a failed write is not evidence.
// A relative write path is resolved against paneCwd.
// A path outside the worktree is dropped, and the error is a link-resolution failure.
func writtenWorktreePaths(writes RunWrites, paneCwd, worktree string) ([]string, error) {
	root, err := canonicalPath(worktree)
	if err != nil {
		return nil, err
	}
	var written []string
	for _, ev := range slices.Concat(writes.Master, writes.Forks, writes.Recoveries) {
		if !ev.Succeeded {
			continue
		}
		canon, err := canonicalPath(resolveWritePath(paneCwd, ev.Path))
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

// recordedSession is one session id with the engine name its state entry records.
// The engine is empty for an entry written before engines were recorded.
type recordedSession struct {
	ID     string
	Engine string
}

// recordedSessions returns the distinct, non-empty Master sessions st records and the recovery sessions.
// The recovery sessions come from each batch in batch-number order;
// the cold verify-gate fixers' sessions follow, in spawn order.
// Both lists are in a stable order.
// Master's id takes MasterEngine, each batch's SessionID takes its Engine;
// a recovery session takes its RecoveryEngines entry, and a fixer session its record's Engine.
// An id is listed once, at its first sighting, which fixes its engine;
// an id recorded as a Master session is never listed again as a recovery session.
func recordedSessions(st *State) (master, recovery []recordedSession) {
	listed := func(id string) bool {
		match := func(s recordedSession) bool { return s.ID == id }
		return slices.ContainsFunc(master, match) || slices.ContainsFunc(recovery, match)
	}
	add := func(list *[]recordedSession, id, engine string) {
		if id != "" && !listed(id) {
			*list = append(*list, recordedSession{ID: id, Engine: engine})
		}
	}
	add(&master, st.MasterSessionID, st.MasterEngine)
	numbers := make([]int, 0, len(st.Batches))
	for n := range st.Batches {
		numbers = append(numbers, n)
	}
	slices.Sort(numbers)
	for _, n := range numbers {
		if b := st.Batches[n]; b != nil {
			add(&master, b.SessionID, b.Engine)
		}
	}
	for _, n := range numbers {
		if b := st.Batches[n]; b != nil {
			for _, id := range b.RecoverySessions {
				add(&recovery, id, b.RecoveryEngines[id])
			}
		}
	}
	for _, rec := range st.VerifyFixers {
		add(&recovery, rec.SessionID, rec.Engine)
	}
	return master, recovery
}
