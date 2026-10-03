// reflect.go implements Reflect, the settle-scan-skip-spawn-archive step this package exists for.

package frictionengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// archiveTimestampFormat is the compact UTC timestamp layout the archive directory's suffix is
// rendered with.
const archiveTimestampFormat = "20060102-150405"

// coveredRecordFileName names the covered-notes record: a non-.md file in the friction directory,
// beside the report, holding the sorted note file names one reflection covers.
// scanNotes never reads it, because it reads only .md entries.
const coveredRecordFileName = "reflection-covered.json"

// coveredRecord is the covered-notes record's on-disk shape.
type coveredRecord struct {
	Notes []string `json:"notes"`
}

// recordState says what readRecord found.
type recordState int

const (
	// recordAbsent: no record file exists.
	recordAbsent recordState = iota
	// recordValid: the record parsed.
	recordValid
	// recordCorrupt: the record reads but does not parse.
	recordCorrupt
)

// Reflect scans deps.FrictionDir for friction notes and, when it finds any, spawns exactly one
// reflection agent over them through deps.Shuttle, archiving the covered files on a clean return
// only.
//
// Reflect is re-entrant across a killed driving process: a covered-notes record written before the
// spawn lets a re-invocation settle the prior reflection first, by attaching to a live agent or by
// archiving a finished one's files without spawning.
// One call spends at most one positive deps.Timeout, measured from entry through deps.Clock.
//
// Deps validation is Reflect's first act, and the only source of a non-nil error: every value on
// Deps is a wiring bug at the one call site when missing, and is surfaced loudly rather than failing
// silently at some later runtime step.
// Every failure below the validation line returns a nil error and a Report instead, per the decision
// that the reflection step can never change the run's own outcome.
func Reflect(deps Deps) (Report, error) {
	if err := validateDeps(deps); err != nil {
		return Report{}, err
	}

	clock := deps.Clock
	if clock == nil {
		clock = realClock{}
	}
	// A non-positive Timeout carries no budget: it reaches every spec unchanged,
	// so a zero one defers to shuttle's own run_timeout_min.
	budgeted := deps.Timeout > 0
	entered := clock.Now()
	remaining := func() time.Duration {
		if !budgeted {
			return deps.Timeout
		}
		return deps.Timeout - clock.Now().Sub(entered)
	}

	reportPath := filepath.Join(deps.FrictionDir, friction.ReportFileName)
	recordPath := filepath.Join(deps.FrictionDir, coveredRecordFileName)

	var archived Report
	didArchive := false

	covered, state, err := readRecord(recordPath)
	if err != nil {
		logger.Warn("frictionengine: could not read covered-notes record", "path", recordPath, "error", err)
		return Report{Status: StatusFailed}, nil
	}
	if state == recordCorrupt {
		logger.Warn("frictionengine: discarding unparsable covered-notes record", "path", recordPath)
		if err := removeIfExists(recordPath); err != nil {
			logger.Warn("frictionengine: could not delete covered-notes record", "path", recordPath, "error", err)
			return Report{Status: StatusFailed}, nil
		}
		state = recordAbsent
	}

	if state == recordValid {
		spec, err := buildReflectionSpec(withTimeout(deps, remaining()), covered, reportPath)
		if err != nil {
			logger.Warn("frictionengine: could not build reflection spec", "dir", deps.FrictionDir, "error", err)
			return Report{Status: StatusFailed, NoteCount: len(covered)}, nil
		}
		result, found, err := deps.Shuttle.Attach(spec)
		if err != nil {
			// A probe that cannot answer may be hiding a live agent, so nothing is spawned.
			logger.Warn("frictionengine: could not probe for a live reflection agent", "dir", deps.FrictionDir, "error", err)
			return Report{Status: StatusFailed, NoteCount: len(covered)}, nil
		}
		switch {
		case found:
			if result.Outcome != shuttleengine.OutcomeDone {
				logger.Warn("frictionengine: attached reflection run did not complete", "dir", deps.FrictionDir, "outcome", result.Outcome)
				return Report{Status: StatusFailed, NoteCount: len(covered)}, nil
			}
			logger.Info("frictionengine: attached reflection run complete", "dir", deps.FrictionDir)
		case fileExists(reportPath):
			logger.Info("frictionengine: prior reflection already reported; archiving without a spawn", "dir", deps.FrictionDir)
		default:
			// No live agent and no report: the covered notes are reflected again below.
			if err := removeIfExists(recordPath); err != nil {
				logger.Warn("frictionengine: could not delete covered-notes record", "path", recordPath, "error", err)
				return Report{Status: StatusFailed, NoteCount: len(covered)}, nil
			}
			state = recordAbsent
		}
		if state == recordValid {
			archived, err = archiveCovered(deps, clock, covered)
			if err != nil {
				logger.Warn("frictionengine: could not archive covered notes", "dir", deps.FrictionDir, "error", err)
				return Report{Status: StatusFailed, NoteCount: len(covered)}, nil
			}
			didArchive = true
		}
	}

	// With no record a leftover report is stale. A timed-out prior run can leave one behind, and
	// shuttleengine.Spec.validate rejects an OutputFiles entry that already exists, so this delete is
	// not optional.
	if err := removeIfExists(reportPath); err != nil {
		logger.Warn("frictionengine: could not delete stale reflection report", "path", reportPath, "error", err)
		return Report{Status: StatusFailed}, nil
	}

	notes, ok, err := scanNotes(deps.FrictionDir)
	if err != nil {
		logger.Warn("frictionengine: could not read friction directory", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed}, nil
	}
	if !ok {
		if didArchive {
			return archived, nil
		}
		logger.Info("frictionengine: nothing to reflect on", "dir", deps.FrictionDir)
		return Report{Status: StatusSkipped, NoteCount: 0}, nil
	}

	budget := remaining()
	if budgeted && budget <= 0 {
		logger.Warn("frictionengine: reflection budget spent before the spawn; notes left for the next reflection", "dir", deps.FrictionDir)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	if err := writeRecord(recordPath, notes); err != nil {
		logger.Warn("frictionengine: could not write covered-notes record", "path", recordPath, "error", err)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	spec, err := buildReflectionSpec(withTimeout(deps, budget), notes, reportPath)
	if err != nil {
		logger.Warn("frictionengine: could not build reflection spec", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	logger.Info("frictionengine: spawning reflection agent", "dir", deps.FrictionDir, "noteCount", len(notes))
	result, err := deps.Shuttle.Run(spec)
	if err != nil {
		logger.Warn("frictionengine: reflection run failed", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}
	if result.Outcome != shuttleengine.OutcomeDone {
		// died, timeout, asking, or any outcome this package does not recognize: the notes and the
		// record are left exactly as they are, so the next trigger in the same task settles or
		// reflects on the same notes again.
		logger.Warn("frictionengine: reflection run did not complete", "dir", deps.FrictionDir, "outcome", result.Outcome)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	archived, err = archiveCovered(deps, clock, notes)
	if err != nil {
		logger.Warn("frictionengine: could not archive covered notes", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}
	logger.Info("frictionengine: reflection complete", "dir", deps.FrictionDir, "reportPath", archived.ReportPath)
	return archived, nil
}

// withTimeout returns deps with its Timeout replaced, so a spec carries the budget left rather than
// the whole one.
func withTimeout(deps Deps, timeout time.Duration) Deps {
	deps.Timeout = timeout
	return deps
}

// validateDeps rejects a malformed Deps with a distinct error per field, in order: a nil
// Deps.Shuttle, an empty or non-absolute Deps.FrictionDir, an empty Deps.ArchivePrefix, and an empty
// Deps.StencilsDir, and an empty Deps.TaskSlug.
func validateDeps(deps Deps) error {
	if deps.Shuttle == nil {
		return fmt.Errorf("frictionengine: Reflect: Deps.Shuttle must not be nil")
	}
	if deps.FrictionDir == "" || !filepath.IsAbs(deps.FrictionDir) {
		return fmt.Errorf("frictionengine: Reflect: Deps.FrictionDir must be an absolute path")
	}
	if deps.ArchivePrefix == "" {
		return fmt.Errorf("frictionengine: Reflect: Deps.ArchivePrefix must not be empty")
	}
	if deps.StencilsDir == "" {
		return fmt.Errorf("frictionengine: Reflect: Deps.StencilsDir must not be empty")
	}
	if deps.TaskSlug == "" {
		return fmt.Errorf("frictionengine: Reflect: Deps.TaskSlug must not be empty")
	}
	return nil
}

// scanNotes reads dir and returns the sorted set of friction note file names -- every *.md entry
// except the one named friction.ReportFileName. Its second return value is false whenever there is
// nothing to reflect on: a missing directory, an empty directory, a directory whose entries are all
// non-.md, and a directory whose only .md entry is the reflection agent's own report file all report
// the same (nil, false, nil).
func scanNotes(dir string) ([]string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read friction directory: %w", err)
	}

	var notes []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if name == friction.ReportFileName {
			continue
		}
		notes = append(notes, name)
	}
	sort.Strings(notes)

	if len(notes) == 0 {
		return nil, false, nil
	}
	return notes, true, nil
}

// removeIfExists removes path, treating "does not exist" as success rather than an error.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", filepath.Base(path), err)
	}
	return nil
}

// fileExists reports whether path names an existing entry.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readRecord reads the covered-notes record at path.
// An absent record is recordAbsent with a nil error;
// a record that reads but does not parse is recordCorrupt with a nil error;
// any other read failure is an error.
func readRecord(path string) ([]string, recordState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, recordAbsent, nil
		}
		return nil, recordAbsent, fmt.Errorf("read covered-notes record: %w", err)
	}
	var rec coveredRecord
	if err := json.Unmarshal(data, &rec); err != nil || len(rec.Notes) == 0 {
		return nil, recordCorrupt, nil
	}
	sort.Strings(rec.Notes)
	return rec.Notes, recordValid, nil
}

// writeRecord writes the covered-notes record naming notes.
func writeRecord(path string, notes []string) error {
	data, err := json.Marshal(coveredRecord{Notes: notes})
	if err != nil {
		return fmt.Errorf("encode covered-notes record: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write covered-notes record: %w", err)
	}
	return nil
}

// archiveCovered moves the covered notes, the record and the report into the first free directory
// of deps.ArchivePrefix + timestamp, then -2, -3 and so on, and returns a StatusReflected Report for it.
// A covered file already gone is skipped.
// The friction directory itself is never renamed or removed, so a note written after the record
// stays behind for the next reflection.
func archiveCovered(deps Deps, clock Clock, covered []string) (Report, error) {
	base := deps.ArchivePrefix + clock.Now().UTC().Format(archiveTimestampFormat)
	archiveDir := base
	for n := 2; ; n++ {
		err := os.Mkdir(archiveDir, 0o755)
		if err == nil {
			break
		}
		if !os.IsExist(err) {
			return Report{}, fmt.Errorf("create archive directory: %w", err)
		}
		archiveDir = fmt.Sprintf("%s-%d", base, n)
	}

	names := append(append([]string{}, covered...), coveredRecordFileName, friction.ReportFileName)
	for _, name := range names {
		err := os.Rename(filepath.Join(deps.FrictionDir, name), filepath.Join(archiveDir, name))
		if err != nil && !os.IsNotExist(err) {
			return Report{}, fmt.Errorf("archive %s: %w", name, err)
		}
	}
	return Report{
		Status:     StatusReflected,
		NoteCount:  len(covered),
		ReportPath: filepath.Join(archiveDir, friction.ReportFileName),
	}, nil
}
