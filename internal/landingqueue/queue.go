// queue.go holds the landing queue's records and its store: the entries, their outcomes, the move mark and the backstop.

package landingqueue

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	recordSuffix = ".json"
	lockSuffix   = ".lock"
	markName     = "move-mark" + recordSuffix
)

// Outcome is what became of a queued run's landing; the empty Outcome means it is still undecided.
type Outcome string

// The outcomes a queued run can be given.
const (
	// OutcomeLanded means the run's work is on the parent branch.
	OutcomeLanded Outcome = "landed"
	// OutcomeConflict means the run's work does not merge cleanly.
	OutcomeConflict Outcome = "conflict"
	// OutcomeFailed means the run lands alone, with its own gate and fixer:
	// a batch's gate failed on its work, or its own fixer or fix review took it.
	OutcomeFailed Outcome = "failed"
)

// Entry is one run waiting to land on a parent branch.
type Entry struct {
	Slug string `json:"slug"`
	// Pair is the path of the run's pair.
	Pair      string `json:"pair"`
	RunID     string `json:"run_id"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	TaskHead  string `json:"task_head"`
	// RecordsHead is the head of the run's records when it joined.
	RecordsHead string `json:"records_head"`
	// Message is the landing commit's message, its Lym-Landing trailer included.
	Message string `json:"message"`
	// TrueMerge keeps the task head as a second parent of the landing commit; otherwise the landing squashes.
	TrueMerge bool `json:"true_merge"`
	// JoinedAt is the UTC time the run joined, which orders the queue.
	JoinedAt time.Time `json:"joined_at"`
	Outcome  Outcome   `json:"outcome,omitempty"`
	// Commit is the landing commit of a landed run, or the commit a failed run is held at, and empty otherwise.
	// A run a batch's gate failed is held at the chain commit that failed.
	// A run its own fixer or fix review took is held at its task head.
	Commit string `json:"commit,omitempty"`
}

// MoveMark records a move of a parent branch from Base to a chain of landing commits.
type MoveMark struct {
	Base string `json:"base"`
	// Members are the runs the move lands, in chain order.
	Members []MarkMember `json:"members"`
}

// MarkMember is one run a MoveMark lands.
type MarkMember struct {
	Slug string `json:"slug"`
	// Commit is the run's landing commit.
	Commit string `json:"commit"`
	// TaskHead is the task head the landing commit lands.
	TaskHead string `json:"task_head"`
	// Records is the records commit the run's ledger landing holds.
	Records string `json:"records"`
}

// BackstopSeams are the reads and the write a backstop makes for each member of a standing move mark.
type BackstopSeams struct {
	// ParentHolds reports whether the parent branch holds commit.
	ParentHolds func(commit string) (bool, error)
	// UpstreamLacks reports whether the parent branch's upstream lacks commit.
	UpstreamLacks func(commit string) (bool, error)
	// WriteLanding records the landing of slug at tip, holding the records commit.
	WriteLanding func(slug, tip, records string) error
}

// Store reads and writes the landing queue of one parent branch.
type Store struct {
	// Dir is the directory holding every branch's queue.
	Dir string
	// Branch is the parent branch the queue lands on.
	Branch string
	// Alive reports whether an owner process lives; nil judges by the pid and start time of a running process.
	Alive func(pid int, startTime string) bool
}

// Dir returns the hub-wide landing queue directory under boardDir.
func Dir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, lymdirs.LandingQueueDirName)
}

func (s *Store) branchDir() string {
	return filepath.Join(s.Dir, hex.EncodeToString([]byte(s.Branch)))
}

func (s *Store) entryPath(slug string) string {
	return filepath.Join(s.branchDir(), hex.EncodeToString([]byte(slug))+recordSuffix)
}

func (s *Store) markPath() string {
	return filepath.Join(s.branchDir(), markName)
}

func lockPath(path string) string {
	return lock.TrackedFileLockPath(path, lockSuffix)
}

// Join writes entry, replacing the run's standing one.
// A standing failed or conflict outcome is kept with its commit whatever task head entry names.
// A standing landed outcome is kept only while the task head is unchanged.
func (s *Store) Join(entry Entry) error {
	path := s.entryPath(entry.Slug)
	err := state.UpdateJSON(path, lockPath(path), func(standing Entry, found bool) (Entry, error) {
		kept := standing.Outcome == OutcomeFailed || standing.Outcome == OutcomeConflict ||
			(standing.Outcome == OutcomeLanded && standing.TaskHead == entry.TaskHead)
		if found && kept {
			entry.Outcome, entry.Commit = standing.Outcome, standing.Commit
		}
		return entry, nil
	})
	if err != nil {
		return fmt.Errorf("join the landing queue as %s: %w", entry.Slug, err)
	}
	return nil
}

// Read returns the entry of slug, found false when none stands.
func (s *Store) Read(slug string) (Entry, bool, error) {
	path := s.entryPath(slug)
	entry, found, err := state.ReadJSON[Entry](path, lockPath(path))
	if err != nil {
		return Entry{}, false, fmt.Errorf("read landing queue entry of %s: %w", slug, err)
	}
	return entry, found, nil
}

// Entries returns every entry of the queue in join order, entries joined at one time ordered by slug.
func (s *Store) Entries() ([]Entry, error) {
	files, err := os.ReadDir(s.branchDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read landing queue directory: %w", err)
	}
	var entries []Entry
	for _, file := range files {
		name, isRecord := strings.CutSuffix(file.Name(), recordSuffix)
		if file.IsDir() || !isRecord || file.Name() == markName {
			continue
		}
		slug, err := hex.DecodeString(name)
		if err != nil {
			return nil, fmt.Errorf("landing queue entry name %q is not a hex slug: %w", file.Name(), err)
		}
		entry, found, err := s.Read(string(slug))
		if err != nil {
			return nil, err
		}
		if found {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if order := a.JoinedAt.Compare(b.JoinedAt); order != 0 {
			return order
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	return entries, nil
}

// Live reports whether the process that owns entry lives.
func (s *Store) Live(entry Entry) bool {
	if s.Alive != nil {
		return s.Alive(entry.PID, entry.StartTime)
	}
	return parentlock.ProcessAlive(entry.PID, entry.StartTime)
}

// SetOutcome gives the entry of slug an outcome and its commit; an empty outcome clears both.
// An absent entry is an error.
func (s *Store) SetOutcome(slug string, outcome Outcome, commit string) error {
	if outcome == "" {
		commit = ""
	}
	return s.update(slug, func(entry *Entry) { entry.Outcome, entry.Commit = outcome, commit })
}

// SetLanded gives the entry of slug the landed outcome with the landing commit, only while the entry names taskHead.
// An entry that joined again with another head holds work the commit does not land, and keeps its outcome.
// An absent entry is an error.
func (s *Store) SetLanded(slug, taskHead, commit string) error {
	return s.update(slug, func(entry *Entry) {
		if entry.TaskHead == taskHead {
			entry.Outcome, entry.Commit = OutcomeLanded, commit
		}
	})
}

// update rewrites the standing entry of slug through change, under the entry's write lock.
func (s *Store) update(slug string, change func(*Entry)) error {
	path := s.entryPath(slug)
	err := state.UpdateJSON(path, lockPath(path), func(entry Entry, found bool) (Entry, error) {
		if !found {
			return Entry{}, errors.New("no entry stands")
		}
		change(&entry)
		return entry, nil
	})
	if err != nil {
		return fmt.Errorf("set the landing queue outcome of %s: %w", slug, err)
	}
	return nil
}

// Remove deletes the entry of slug; an absent one is no error.
func (s *Store) Remove(slug string) error {
	_, err := removeRecord(s.entryPath(slug), func(Entry) bool { return true })
	if err != nil {
		return fmt.Errorf("remove landing queue entry of %s: %w", slug, err)
	}
	return nil
}

// Prune removes every entry that has no outcome and whose owner is dead, and returns their slugs.
// An entry with an outcome stays until its own run reads it.
// The caller runs it only after Backstop, which gives a landed outcome to the entries a dead mover had already landed.
func (s *Store) Prune() ([]string, error) {
	entries, err := s.Entries()
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, entry := range entries {
		// Judged again on the record read under its lock, so an entry rejoined since the listing stays.
		removed, err := removeRecord(s.entryPath(entry.Slug), func(current Entry) bool {
			return current.Outcome == "" && !s.Live(current)
		})
		if err != nil {
			return pruned, fmt.Errorf("prune landing queue entry of %s: %w", entry.Slug, err)
		}
		if removed {
			pruned = append(pruned, entry.Slug)
		}
	}
	return pruned, nil
}

// removeRecord deletes the record at path when drop accepts the record read under the write lock.
// It reports whether it deleted the record.
func removeRecord[T any](path string, drop func(T) bool) (bool, error) {
	// Checked before the lock is taken, since an absent directory has nowhere to hold one.
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	held, err := lock.AcquireWriteLock(lockPath(path))
	if err != nil {
		return false, err
	}
	defer held.Release()

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record T
	if err := json.Unmarshal(data, &record); err != nil {
		return false, err
	}
	if !drop(record) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

// WriteMark records mark, replacing a standing one.
func (s *Store) WriteMark(mark MoveMark) error {
	path := s.markPath()
	if err := state.WriteJSON(path, lockPath(path), mark); err != nil {
		return fmt.Errorf("write landing move mark: %w", err)
	}
	return nil
}

// ReadMark returns the standing move mark, found false when none stands.
func (s *Store) ReadMark() (MoveMark, bool, error) {
	path := s.markPath()
	mark, found, err := state.ReadJSON[MoveMark](path, lockPath(path))
	if err != nil {
		return MoveMark{}, false, fmt.Errorf("read landing move mark: %w", err)
	}
	return mark, found, nil
}

// RemoveMark deletes the standing move mark; an absent one is no error.
func (s *Store) RemoveMark() error {
	if _, err := removeRecord(s.markPath(), func(MoveMark) bool { return true }); err != nil {
		return fmt.Errorf("remove landing move mark: %w", err)
	}
	return nil
}

// Backstop finishes the records of a move whose mover did not, and returns how many landings it wrote.
// A member of a standing mark counts when the parent holds its commit and the parent's upstream lacks it.
// Each such member gets its landing written.
// Its entry, when one stands and still names the task head the mark does, gets the landed outcome.
// The mark is then removed, also when the parent holds none of its commits.
// An error keeps the mark, so a later Backstop retries it.
func (s *Store) Backstop(seams BackstopSeams) (int, error) {
	mark, found, err := s.ReadMark()
	if err != nil || !found {
		return 0, err
	}
	written := 0
	for _, member := range mark.Members {
		holds, err := seams.ParentHolds(member.Commit)
		if err != nil {
			return written, fmt.Errorf("landing backstop: does the parent hold %s: %w", member.Commit, err)
		}
		if !holds {
			continue
		}
		lacks, err := seams.UpstreamLacks(member.Commit)
		if err != nil {
			return written, fmt.Errorf("landing backstop: does the upstream lack %s: %w", member.Commit, err)
		}
		if !lacks {
			continue
		}
		if err := seams.WriteLanding(member.Slug, member.Commit, member.Records); err != nil {
			return written, fmt.Errorf("landing backstop: write the landing of %s: %w", member.Slug, err)
		}
		written++
		_, queued, err := s.Read(member.Slug)
		if err != nil {
			return written, err
		}
		if !queued {
			continue
		}
		if err := s.SetLanded(member.Slug, member.TaskHead, member.Commit); err != nil {
			return written, err
		}
	}
	return written, s.RemoveMark()
}
