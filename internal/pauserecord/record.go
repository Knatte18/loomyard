// record.go declares the pause record's types and constants, its locked updates, and its coverage and interval queries.

package pauserecord

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// RecordFileName is the pause record's file name within its directory.
const RecordFileName = "record.json"

// ScopeHub is the scope of an entry that covers every pair and the prime.
const ScopeHub = "hub"

// OwnerCaller is the Caller of every pause entry that holds a run owned by another machine.
const OwnerCaller = "owner"

// HoldCaller is the Caller of every pause entry that holds a run, or a released task, that `lym adopt` took over.
// Only the green adopt check of the entry's subject lifts it by itself.
const HoldCaller = "adopt-hold"

// Retention is how long a resumed entry stays in the record.
// It lets a reader that was away across a pause still see its interval.
const Retention = 7 * 24 * time.Hour

// ResumeLine is the one line a session interrupted by a pause is given when the pause ends.
const ResumeLine = "[lym] A pause interrupted your last turn and is now over. Carry on where you stopped: rerun the tool call or fork the pause cut. If nothing was cut, end your turn."

// Geometry is what every reader of the record is handed: the record's file and the reader's own pair slug.
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
	// OwnerHeld reports whether a live entry of the slug's own scope carries OwnerCaller.
	// It is set even when another entry is the covering one.
	OwnerHeld bool
	// Held reports whether a live entry of the slug's own scope carries HoldCaller.
	// It is set even when another entry is the covering one.
	Held bool
}

// Read returns the record at path, or the empty record when the file does not exist.
// A file that cannot be read or decoded returns an error naming the path.
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

// Update applies mutate to the record at path under the write lock beside it.
// It drops resumed entries older than Retention before now,
// and it replaces the file atomically, keeping every key of the replaced file that the record types do not declare.
// A mutate error or an undecodable file writes nothing and is returned.
func Update(path string, now time.Time, mutate func(*Record) error) (Record, error) {
	var updated Record
	err := state.UpdateJSON(path, path+".lock", func(current Record, _ bool) (Record, error) {
		if err := mutate(&current); err != nil {
			return Record{}, err
		}
		current.Resumed = withoutExpired(current.Resumed, now)
		updated = current
		return current, nil
	})
	if err != nil {
		return Record{}, fmt.Errorf("update pause record %s: %w", path, err)
	}
	return updated, nil
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

// LatestResumed returns the resumed entry covering slug that resumed last.
// It lets a reader that was away learn of a pause it never saw, for as long as the entry is retained.
func (r Record) LatestResumed(slug string) (Entry, bool) {
	var latest Entry
	found := false
	for _, entry := range r.Resumed {
		if entry.covers(slug) && (!found || entry.ResumedAt.After(latest.ResumedAt)) {
			latest = entry
			found = true
		}
	}
	return latest, found
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

// ClaimSession records session in the oldest live entry covering slug, under the record lock.
// That entry's resume gives the session its resume line.
// An uncovered slug records nothing and returns the zero entry and false.
// A covered slug returns true and the entry that owns the session after the call;
// a session a live entry already owns is recorded nowhere again, and that owning entry is returned.
// An undecodable record returns an error and records nothing.
func ClaimSession(path, slug string, session Session, now time.Time) (Entry, bool, error) {
	var owner Entry
	var covered bool
	_, err := Update(path, now, func(r *Record) error {
		covering, found := r.Covering(slug)
		if !found {
			return nil
		}
		covered = true
		if existing, owned := r.Owner(session.GUID); owned {
			owner = existing
			return nil
		}
		for i := range r.Live {
			if r.Live[i].Scope == covering.Scope {
				r.Live[i].Sessions = append(r.Live[i].Sessions, session)
				owner = r.Live[i]
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return Entry{}, false, err
	}
	return owner, covered, nil
}

// CoverageOf returns the covering live entry for slug and the interval of every entry covering it.
// A live entry's interval runs from its pause to now, a resumed one's to its resume.
// OwnerHeld is set when a live entry of slug's own scope carries OwnerCaller, and Held when one carries HoldCaller.
func (r Record) CoverageOf(slug string, now time.Time) Coverage {
	var coverage Coverage
	coverage.Entry, coverage.Covered = r.Covering(slug)
	for _, entry := range r.Live {
		if slug != "" && entry.Scope == slug {
			switch entry.Caller {
			case OwnerCaller:
				coverage.OwnerHeld = true
			case HoldCaller:
				coverage.Held = true
			}
		}
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

// CoveredError renders the refusal an entry point returns when entry covers its run.
// The refusal names the resume that lifts the hold and the retry to make after it.
func CoveredError(e Entry, slug, retry string) error {
	pause, resume := "a hub-wide pause", `"lym tender resume"`
	if e.Scope != ScopeHub {
		pause, resume = fmt.Sprintf("the pause of %s", slug), fmt.Sprintf(`"lym tender resume %s"`, slug)
	}
	return fmt.Errorf("pause: %s has held this run since %s; way forward: run %s, then %s",
		pause, e.PausedAt.Format(time.RFC3339), resume, retry)
}
