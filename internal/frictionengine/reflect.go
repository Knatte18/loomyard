// reflect.go implements Reflect, the scan-skip-spawn-archive step this package exists for.

package frictionengine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// archiveTimestampFormat is the compact UTC timestamp layout the archive directory's suffix is
// rendered with.
const archiveTimestampFormat = "20060102-150405"

// Reflect scans deps.FrictionDir for friction notes and, when it finds any, spawns exactly one
// reflection agent over them through deps.Shuttle, archiving the friction directory on a clean
// return only.
//
// Deps validation is Reflect's first act, and the only source of a non-nil error: every value on
// Deps is a wiring bug at the one call site when missing, and is surfaced loudly rather than failing
// silently at some later runtime step. Every failure below the validation line returns a nil error
// and a Report instead, per the decision that the reflection step can never change the run's own
// outcome.
func Reflect(deps Deps) (Report, error) {
	if err := validateDeps(deps); err != nil {
		return Report{}, err
	}

	notes, ok, err := scanNotes(deps.FrictionDir)
	if err != nil {
		logger.Warn("frictionengine: could not read friction directory", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed}, nil
	}
	if !ok {
		logger.Info("frictionengine: nothing to reflect on", "dir", deps.FrictionDir)
		return Report{Status: StatusSkipped, NoteCount: 0}, nil
	}

	reportPath := filepath.Join(deps.FrictionDir, friction.ReportFileName)

	// A timed-out prior run can leave a stale report file behind. shuttleengine.Spec.validate
	// rejects an OutputFiles entry that already exists, so this delete is not optional: without it,
	// every subsequent Reflect against the same directory would fail at spec validation rather than
	// at the agent.
	if err := deleteStaleReport(reportPath); err != nil {
		logger.Warn("frictionengine: could not delete stale reflection report", "path", reportPath, "error", err)
		return Report{Status: StatusFailed}, nil
	}

	spec, err := buildReflectionSpec(deps, notes, reportPath)
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
		// died, timeout, asking, or any outcome this package does not recognize: the friction
		// directory is left exactly as it is, not archived, so the next trigger in the same task
		// reflects on the same notes again.
		logger.Warn("frictionengine: reflection run did not complete", "dir", deps.FrictionDir, "outcome", result.Outcome)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	archivedPath, err := archiveFrictionDir(deps)
	if err != nil {
		logger.Warn("frictionengine: could not archive friction directory", "dir", deps.FrictionDir, "error", err)
		return Report{Status: StatusFailed, NoteCount: len(notes)}, nil
	}

	archivedReportPath := filepath.Join(archivedPath, friction.ReportFileName)
	logger.Info("frictionengine: reflection complete", "dir", deps.FrictionDir, "reportPath", archivedReportPath)
	return Report{Status: StatusReflected, NoteCount: len(notes), ReportPath: archivedReportPath}, nil
}

// validateDeps rejects a malformed Deps with a distinct error per field, in order: a nil
// Deps.Shuttle, an empty or non-absolute Deps.FrictionDir, an empty Deps.ArchivePrefix, and an empty
// Deps.StencilsDir.
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

// deleteStaleReport removes reportPath if it exists, treating "does not exist" as success rather
// than an error.
func deleteStaleReport(reportPath string) error {
	if err := os.Remove(reportPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete stale reflection report: %w", err)
	}
	return nil
}

// archiveFrictionDir renames deps.FrictionDir onto a timestamped sibling under deps.ArchivePrefix and
// recreates deps.FrictionDir empty, returning the archive directory's path.
//
// friction.EnsureDir is what recreates the directory: it is the one place that removed the directory,
// making it the cheapest, hardest-to-get-wrong place to put it back, rather than teaching every
// future caller to re-ensure it.
func archiveFrictionDir(deps Deps) (string, error) {
	clock := deps.Clock
	if clock == nil {
		clock = realClock{}
	}

	archiveDir := deps.ArchivePrefix + clock.Now().UTC().Format(archiveTimestampFormat)
	if err := os.Rename(deps.FrictionDir, archiveDir); err != nil {
		return "", fmt.Errorf("archive friction directory: %w", err)
	}
	friction.EnsureDir(deps.FrictionDir)
	return archiveDir, nil
}
