// shed.go reads a finished run's archived shed seed and status from its newest archive tag.
// It decides from them whether the run is a full loom run, sets the run's span and splits that span by shed history entry into the Producers table.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// ArchiveTrees reads a file as a commit's tree holds it.
type ArchiveTrees interface {
	FileAtRevision(rev, relPath string) ([]byte, error)
}

// ShedEntry is one shed history entry: its producer and the time it finished.
type ShedEntry struct {
	Producer string
	// End is the entry's last_at when it folded repeated calls, else its at.
	End time.Time
}

// ShedRecord is what a run's newest archive tag holds of its seed and shed status.
type ShedRecord struct {
	// Tag is the archive tag the record was read from.
	Tag ArchiveTag
	// Older is the commit date of the slug's next-older tag, zero when the slug has one tag.
	Older time.Time
	// Recipe is the seed's recipe, empty when the seed is absent.
	Recipe string
	// StartedAt is the seed's started_at, zero when the seed or the field is absent.
	StartedAt time.Time
	// Entries are the status's history entries in recorded order.
	Entries []ShedEntry
}

// ProducerRow is one producer's share of a run's span.
type ProducerRow struct {
	Producer string
	Entries  int
	Elapsed  time.Duration
	// Idle is the part of Elapsed in which no session of the run was in a turn.
	Idle time.Duration
}

// readShedRecord reads the seed and shed status of the slug's newest tag, tags being ordered newest first.
// It returns nil and no error when there are no tags, when status.json is absent at the tag, or when its history is empty;
// an absent seed.json leaves StartedAt zero.
// A seed or status that does not decode, or a timestamp in either that does not parse, is an error naming the slug, the tag and the file.
func readShedRecord(trees ArchiveTrees, slug string, tags []ArchiveTag) (*ShedRecord, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	tag := tags[0]
	fail := func(file string, err error) error {
		return fmt.Errorf("shed record of %s from tag %s, %s: %w", slug, tag.Name, file, err)
	}
	seedRel, statusRel, err := shedrun.TreeRunFileRels(slug)
	if err != nil {
		return nil, fmt.Errorf("shed record of %s from tag %s, seed.json and status.json: %w", slug, tag.Name, err)
	}

	statusData, err := trees.FileAtRevision(tag.Commit, statusRel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, nil
	}
	if err != nil {
		return nil, fail("status.json", err)
	}
	var status shedengine.Status
	if err := json.Unmarshal(statusData, &status); err != nil {
		return nil, fail("status.json", err)
	}
	if len(status.History) == 0 {
		return nil, nil
	}
	record := &ShedRecord{Tag: tag}
	if len(tags) > 1 {
		record.Older = tags[1].Committed
	}
	for _, entry := range status.History {
		end, err := time.Parse(time.RFC3339, entry.At)
		if err == nil && entry.Repeats > 0 {
			end, err = time.Parse(time.RFC3339, entry.LastAt)
		}
		if err != nil {
			return nil, fail("status.json", err)
		}
		record.Entries = append(record.Entries, ShedEntry{Producer: entry.Producer, End: end})
	}

	seedData, err := trees.FileAtRevision(tag.Commit, seedRel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return record, nil
	}
	if err != nil {
		return nil, fail("seed.json", err)
	}
	seed, err := shedrun.ParseSeed(seedData)
	if err != nil {
		return nil, fail("seed.json", err)
	}
	record.Recipe = seed.Recipe
	if seed.StartedAt != "" {
		if record.StartedAt, err = time.Parse(time.RFC3339, seed.StartedAt); err != nil {
			return nil, fail("seed.json", err)
		}
	}
	return record, nil
}

// inFlight reports whether the slug's worktree under hub exists.
// Teardown removes a finished run's worktree, so one that still exists is in flight and its archive tags belong to an older run of the slug.
func inFlight(hub, slug string) (bool, error) {
	_, err := os.Stat(filepath.Join(hub, slug))
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return err == nil, nil
}

// fullLoomRun reports whether the slug's latest run went through the whole loom lifecycle:
// its worktree under hub is torn down, and the shed record at its newest archive tag, tags being ordered newest first, names the loom recipe and holds a Finalize entry.
// A run stopped partway, such as a bench pair parked after Discussion-Write or an abandoned run, is not one.
func fullLoomRun(trees ArchiveTrees, hub, slug string, tags []ArchiveTag) (bool, error) {
	running, err := inFlight(hub, slug)
	if err != nil || running {
		return false, err
	}
	record, err := readShedRecord(trees, slug, tags)
	if err != nil || record == nil || record.Recipe != shedrun.RecipeLoom {
		return false, err
	}
	return slices.ContainsFunc(record.Entries, func(entry ShedEntry) bool { return entry.Producer == loomshed.NameFinalize }), nil
}

// useShedRecord stores rec and sets the run's span from it.
// The span ends at the latest entry end.
// It starts at the seed's started_at, or at the run's first transcript line without one.
// That start is raised to the older tag's date when that is later, then lowered to the first entry's end when that is earlier.
func (run *RunTally) useShedRecord(rec ShedRecord) {
	run.Shed = &rec
	from := rec.StartedAt
	if from.IsZero() {
		from = run.First
	}
	if rec.Older.After(from) {
		from = rec.Older
	}
	if first := rec.Entries[0].End; first.Before(from) {
		from = first
	}
	to := from
	for _, entry := range rec.Entries {
		if entry.End.After(to) {
			to = entry.End
		}
	}
	run.Span = Interval{from, to}
}

// Producers splits the run's span by shed history entry, nil for a run with no shed record.
// An entry owns the interval from the latest end among the entries before it, the span's start for the first, to its own end, empty when its end is not later.
// Rows sum the entries per producer and are ordered by elapsed time, longest first, then by name.
func (run RunTally) Producers() []ProducerRow {
	if run.Shed == nil {
		return nil
	}
	rows := map[string]*ProducerRow{}
	latest := run.Span.From
	for _, entry := range run.Shed.Entries {
		row := rows[entry.Producer]
		if row == nil {
			row = &ProducerRow{Producer: entry.Producer}
			rows[entry.Producer] = row
		}
		row.Entries++
		interval := Interval{latest, entry.End}
		row.Elapsed += interval.Duration()
		row.Idle += interval.Duration() - covered(run.Working, interval)
		if entry.End.After(latest) {
			latest = entry.End
		}
	}
	return sortedProducerRows(rows)
}

// sortedProducerRows returns the rows ordered by elapsed time, longest first, then by producer name.
func sortedProducerRows(rows map[string]*ProducerRow) []ProducerRow {
	sorted := make([]ProducerRow, 0, len(rows))
	for _, row := range rows {
		sorted = append(sorted, *row)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Elapsed != sorted[j].Elapsed {
			return sorted[i].Elapsed > sorted[j].Elapsed
		}
		return sorted[i].Producer < sorted[j].Producer
	})
	return sorted
}

// sumProducers sums the rows of every run with a shed record per producer, and the spans of those runs.
func sumProducers(runs []RunTally) ([]ProducerRow, time.Duration) {
	rows := map[string]*ProducerRow{}
	var span time.Duration
	for _, run := range runs {
		if run.Shed == nil {
			continue
		}
		span += run.Span.Duration()
		for _, row := range run.Producers() {
			sum := rows[row.Producer]
			if sum == nil {
				sum = &ProducerRow{Producer: row.Producer}
				rows[row.Producer] = sum
			}
			sum.Entries += row.Entries
			sum.Elapsed += row.Elapsed
			sum.Idle += row.Idle
		}
	}
	return sortedProducerRows(rows), span
}

// writeProducers writes the Producers table, each row's share being its elapsed time over span.
func writeProducers(w io.Writer, rows []ProducerRow, span time.Duration) {
	fmt.Fprintln(w, "### Producers")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| producer | entries | elapsed | idle | share |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, row := range rows {
		share := 0.0
		if span > 0 {
			share = 100 * float64(row.Elapsed) / float64(span)
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %.1f%% |\n", row.Producer, row.Entries, row.Elapsed.Round(time.Second), row.Idle.Round(time.Second), share)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "A row's elapsed is the gap between finish times, so it includes any wait before the row started, and `Publish` and `Finalize` record no inner breakdown.")
	fmt.Fprintln(w)
}
