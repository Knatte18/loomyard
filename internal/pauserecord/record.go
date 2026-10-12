// record.go declares the pause record's types and constants, its locked read-modify-write, and the coverage and interval queries over it.

package pauserecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
)

// RecordFileName is the pause record's file name within its directory.
const RecordFileName = "record.json"

// ScopeHub is the scope of an entry that covers every pair and the prime.
const ScopeHub = "hub"

// Retention is how long a resumed entry stays in the record, so a reader that was away across a pause still sees its interval.
const Retention = 7 * 24 * time.Hour

// ResumeLine is the one line a session interrupted by a pause is given when the pause ends.
const ResumeLine = "[lyx] A pause interrupted your last turn and is now over. Carry on where you stopped: rerun the tool call or fork the pause cut. If nothing was cut, end your turn."

// Geometry is the told pair every reader of the record is handed.
type Geometry struct {
	// RecordPath is the record file.
	RecordPath string
	// Slug is the reading process's own pair slug, empty in the prime and in standalone mode.
	Slug string
}

// Session is one session an entry owns.
type Session struct {
	// Strand is the reed strand name.
	Strand string `json:"strand"`
	GUID   string `json:"guid"`
	// Slug is empty for the orch.
	Slug string `json:"slug"`
	Orch bool   `json:"orch"`
	// ResumeNoticed is set once the orch watcher has queued the orch's resume line.
	ResumeNoticed bool `json:"resume_noticed"`
}

// Entry is one pause: its scope, its timing and the sessions it interrupted.
type Entry struct {
	// Scope is ScopeHub or the covered pair's slug.
	Scope    string    `json:"scope"`
	PausedAt time.Time `json:"paused_at"`
	GraceEnd time.Time `json:"grace_end"`
	// Caller is the pausing strand name, or EXTERNAL.
	Caller   string    `json:"caller"`
	Sessions []Session `json:"sessions"`
	// ResumedAt is zero while the entry is live.
	ResumedAt time.Time `json:"resumed_at"`
}

// Record is the whole file: at most one live entry per scope.
type Record struct {
	Live    []Entry `json:"live"`
	Resumed []Entry `json:"resumed"`
}

// Interval is a span of time, from Start to End.
type Interval struct {
	Start time.Time
	End   time.Time
}

// Coverage is what a record says about one slug.
type Coverage struct {
	// Covered reports whether a live entry covers the slug.
	Covered bool
	// Entry is the covering live entry, zero when uncovered.
	Entry Entry
	// Intervals holds every interval of every entry covering the slug, live and resumed.
	Intervals []Interval
}

// Read returns the record at path: the empty record when the file is missing, and an error naming the path when it cannot be read or decoded.
func Read(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, nil
	}
	if err != nil {
		return Record{}, fmt.Errorf("read pause record %s: %w", path, err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode pause record %s: %w", path, err)
	}
	return record, nil
}

// Update applies mutate to the record at path under the write lock beside it, drops resumed entries older than Retention before now, and replaces the file atomically.
// A mutate error writes nothing and is returned.
func Update(path string, now time.Time, mutate func(*Record) error) (Record, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Record{}, fmt.Errorf("create pause record directory: %w", err)
	}
	fileLock, err := lock.AcquireWriteLock(path + ".lock")
	if err != nil {
		return Record{}, fmt.Errorf("lock pause record %s: %w", path, err)
	}
	defer fileLock.Release()

	record, err := Read(path)
	if err != nil {
		return Record{}, err
	}
	if err := mutate(&record); err != nil {
		return Record{}, err
	}
	record.Resumed = withoutExpired(record.Resumed, now)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, fmt.Errorf("encode pause record: %w", err)
	}
	if err := writeAtomically(path, data); err != nil {
		return Record{}, err
	}
	return record, nil
}

// withoutExpired returns the entries resumed within Retention before now.
func withoutExpired(entries []Entry, now time.Time) []Entry {
	cutoff := now.Add(-Retention)
	var kept []Entry
	for _, entry := range entries {
		if !entry.ResumedAt.Before(cutoff) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// writeAtomically writes data to a temp file beside path and renames it over path.
func writeAtomically(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// covers reports whether an entry of this scope covers slug; an empty slug is covered by a hub entry only.
func (e Entry) covers(slug string) bool {
	return e.Scope == ScopeHub || (slug != "" && e.Scope == slug)
}

// Covering returns the oldest live entry covering slug.
func (r Record) Covering(slug string) (Entry, bool) {
	var oldest Entry
	found := false
	for _, entry := range r.Live {
		if !entry.covers(slug) {
			continue
		}
		if !found || entry.PausedAt.Before(oldest.PausedAt) {
			oldest = entry
			found = true
		}
	}
	return oldest, found
}

// Owner returns the live entry whose sessions include guid.
func (r Record) Owner(guid string) (Entry, bool) {
	for _, entry := range r.Live {
		for _, session := range entry.Sessions {
			if session.GUID == guid {
				return entry, true
			}
		}
	}
	return Entry{}, false
}

// CoverageOf returns the covering live entry for slug and the interval of every entry covering it.
// A live entry's interval runs from its pause to now, a resumed one's to its resume.
func (r Record) CoverageOf(slug string, now time.Time) Coverage {
	var coverage Coverage
	coverage.Entry, coverage.Covered = r.Covering(slug)
	for _, entry := range r.Live {
		if entry.covers(slug) {
			coverage.Intervals = append(coverage.Intervals, Interval{Start: entry.PausedAt, End: now})
		}
	}
	for _, entry := range r.Resumed {
		if entry.covers(slug) {
			coverage.Intervals = append(coverage.Intervals, Interval{Start: entry.PausedAt, End: entry.ResumedAt})
		}
	}
	return coverage
}

// ReadCoverage reads the record at path and returns its coverage of slug at now.
func ReadCoverage(path, slug string, now time.Time) (Coverage, error) {
	record, err := Read(path)
	if err != nil {
		return Coverage{}, err
	}
	return record.CoverageOf(slug, now), nil
}

// Excluded returns the length of the union of intervals clipped to [from, to], so overlapping intervals count once.
func Excluded(intervals []Interval, from, to time.Time) time.Duration {
	var clipped []Interval
	for _, interval := range intervals {
		start, end := interval.Start, interval.End
		if start.Before(from) {
			start = from
		}
		if end.After(to) {
			end = to
		}
		if end.After(start) {
			clipped = append(clipped, Interval{Start: start, End: end})
		}
	}
	sort.Slice(clipped, func(i, j int) bool { return clipped[i].Start.Before(clipped[j].Start) })

	var total time.Duration
	var reached time.Time
	for _, interval := range clipped {
		start := interval.Start
		if start.Before(reached) {
			start = reached
		}
		if interval.End.After(start) {
			total += interval.End.Sub(start)
			reached = interval.End
		}
	}
	return total
}

// CoveredError renders the refusal an entry point returns when entry covers its run, naming the resume that lifts the hold and the retry to make after it.
func CoveredError(e Entry, slug, retry string) error {
	pause, resume := "a hub-wide pause", `"lyx resume"`
	if e.Scope != ScopeHub {
		pause, resume = fmt.Sprintf("the pause of %s", slug), fmt.Sprintf(`"lyx resume %s"`, slug)
	}
	return fmt.Errorf("pause: %s has held this run since %s; way forward: run %s, then %s",
		pause, e.PausedAt.Format(time.RFC3339), resume, retry)
}
