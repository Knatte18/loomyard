// round.go decorates a Harden round row so it reaps the sandbox around its work and writes the round's Go files.

package hardenshed

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// hardenFilePrefix opens the name of every file the campaign writes into the run directory, `harden-<kind>-<round>.json`.
const hardenFilePrefix = "harden-"

// The ways forward a round row's errors end with.
const (
	resumeWayForward = "way forward: run lym loom resume"
	reapWayForward   = "way forward: run lym harden sandbox clear in the task worktree, then lym loom resume"
)

// RowKind says which half of a round a decorated row is.
type RowKind string

// The kinds of round row.
const (
	// RowReview is the review half, which writes the round's review file.
	RowReview RowKind = "review"
	// RowFix is the fix half, which writes the round's fixer report.
	RowFix RowKind = "fix"
)

// RoundModels is the record of the models one round runs, written as `harden-models-<round>.json`.
type RoundModels struct {
	// Round is the round the record is of.
	Round int `json:"round"`
	// Review is the model-spec of the round's reviewer.
	Review string `json:"review"`
	// Fix is the model-spec of the round's fixer.
	Fix string `json:"fix"`
}

// RoundCommits is the record of the commits one fix round made, written as `harden-commits-<round>.json`.
type RoundCommits struct {
	// Round is the round the record is of.
	Round int `json:"round"`
	// Head is the commit the fix row first started at.
	Head string `json:"head"`
	// Commits are the commits the row made past Head.
	Commits []string `json:"commits"`
}

// RoundHead is the record of the commit a round's fix row first started at, written as `harden-head-<round>.json`.
type RoundHead struct {
	// Round is the round the record is of.
	Round int `json:"round"`
	// Head is the commit the worktree was at when the round's fix row first started.
	Head string `json:"head"`
}

// StrayReport is the record of what the reaps around one round found, written as `harden-strays-<round>.json`.
type StrayReport struct {
	// Round is the round the report is of.
	Round int `json:"round"`
	// Entries are the strays every reap of the round found, in the order the reaps ran.
	Entries []StrayEntry `json:"entries"`
}

// RowDeps is what a decorated round row needs told.
type RowDeps struct {
	// Row is the name of the row, recorded in the strays its reaps find.
	Row string
	// Kind is the half of the round the row is.
	Kind RowKind
	// RunDir is the run directory the round's files are written in.
	RunDir string
	// Reap reaps the sandbox at point for row and returns the strays it found.
	Reap func(row string, point ReapPoint) ([]StrayEntry, error)
	// Models returns the reviewer and fixer model-specs of round.
	Models func(round int) (review, fix string)
	// Head returns the commit the worktree is at.
	Head func() (string, error)
	// Commits lists the commits made past from.
	Commits func(from string) ([]string, error)
	// Archive is what the fix row's archive of its round needs.
	Archive ArchiveDeps
}

// roundRow is a producer decorated with the Harden round's reaps and files.
type roundRow struct {
	inner shedengine.ShedProducer
	deps  RowDeps
}

// WrapRow returns a producer that calls inner between a reap at the start and a reap at the end.
// Its round is fixed once, at the start of the call: the round after the last fixed one for a review row,
// so a review row resumed after it wrote its review keeps that review's round, and the last reviewed one for a fix row.
// A review row writes the round's models file.
// A fix row records the head it first started at in the round's head file, and a call resumed after a crash reuses it,
// so the commits file lists every commit the round's calls made.
// A fix row writes the round's commits file when inner returned no error, and then archives the round.
// Every reap adds its strays to the round's stray report.
// A reap, file or archive error is joined to inner's error and names its way forward.
// It returns inner's outcome and pointer.
func WrapRow(inner shedengine.ShedProducer, deps RowDeps) shedengine.ShedProducer {
	return &roundRow{inner: inner, deps: deps}
}

// Call runs the decorated row; see WrapRow.
func (r *roundRow) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	round, err := r.round()
	if err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("harden row %s: %w; %s", r.deps.Row, err, resumeWayForward)
	}
	if err := r.reap(round, ReapStart); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	var head string
	if r.deps.Kind == RowReview {
		review, fix := r.deps.Models(round)
		models := RoundModels{Round: round, Review: review, Fix: fix}
		if err := writeRoundRecord(r.deps.RunDir, "models", round, models); err != nil {
			return "", shedengine.OutputPointer{}, r.fileError(err)
		}
	} else if head, err = r.startHead(round); err != nil {
		return "", shedengine.OutputPointer{}, r.fileError(err)
	}

	outcome, pointer, callErr := r.inner.Call(ctx)
	failures := []error{callErr}
	fixed := r.deps.Kind == RowFix && callErr == nil
	if fixed {
		failures = append(failures, r.writeCommits(round, head))
	}
	failures = append(failures, r.reap(round, ReapEnd))
	if fixed {
		if err := ArchiveRound(r.deps.Archive, round); err != nil {
			failures = append(failures, r.fileError(err))
		}
	}
	return outcome, pointer, errors.Join(failures...)
}

// round returns the round the row works on.
func (r *roundRow) round() (int, error) {
	if r.deps.Kind == RowFix {
		return shedadapters.ReviewRound(r.deps.RunDir)
	}
	fixed, err := shedadapters.FixedRound(r.deps.RunDir)
	return fixed + 1, err
}

// reap reaps at point and adds what it found to the round's stray report.
// A start reap's entries are marked as left by an interrupted row.
func (r *roundRow) reap(round int, point ReapPoint) error {
	strays, reapErr := r.deps.Reap(r.deps.Row, point)
	if point == ReapStart {
		for i := range strays {
			strays[i].Interrupted = true
		}
	}
	var failures []error
	if reapErr != nil {
		failures = append(failures, fmt.Errorf("harden row %s: reap at %s: %w; %s", r.deps.Row, point, reapErr, reapWayForward))
	}
	if err := appendStrays(r.deps.RunDir, round, strays); err != nil {
		failures = append(failures, r.fileError(err))
	}
	return errors.Join(failures...)
}

// startHead returns the commit the round's fix row first started at.
// A call after the first reads it from the round's head file; the first call reads the worktree's head and records it.
func (r *roundRow) startHead(round int) (string, error) {
	recorded, found, err := readRoundRecord[RoundHead](r.deps.RunDir, "head", round)
	if err != nil {
		return "", err
	}
	if found {
		return recorded.Head, nil
	}
	head, err := r.deps.Head()
	if err != nil {
		return "", fmt.Errorf("read the head: %w", err)
	}
	if err := writeRoundRecord(r.deps.RunDir, "head", round, RoundHead{Round: round, Head: head}); err != nil {
		return "", err
	}
	return head, nil
}

// writeCommits writes the round's commits file, the commits made past head.
func (r *roundRow) writeCommits(round int, head string) error {
	commits, err := r.deps.Commits(head)
	if err != nil {
		return r.fileError(fmt.Errorf("list the commits past %s: %w", head, err))
	}
	if err := writeRoundRecord(r.deps.RunDir, "commits", round, RoundCommits{Round: round, Head: head, Commits: commits}); err != nil {
		return r.fileError(err)
	}
	return nil
}

// fileError names the row and the way forward of an error that is no reap's.
func (r *roundRow) fileError(err error) error {
	return fmt.Errorf("harden row %s: %w; %s", r.deps.Row, err, resumeWayForward)
}

// roundRecordPath returns the path of the round's `harden-<kind>-<round>.json` file in runDir.
func roundRecordPath(runDir, kind string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("%s%s-%d.json", hardenFilePrefix, kind, round))
}

// readRoundRecord reads the round's `harden-<kind>-<round>.json` file, and reports whether it exists.
func readRoundRecord[T any](runDir, kind string, round int) (T, bool, error) {
	path := roundRecordPath(runDir, kind, round)
	record, found, err := state.ReadJSON[T](path, lock.TrackedFileLockPath(path, ".lock"))
	if err != nil {
		return record, false, fmt.Errorf("read %s: %w", path, err)
	}
	return record, found, nil
}

// writeRoundRecord writes record as the round's `harden-<kind>-<round>.json` file.
func writeRoundRecord[T any](runDir, kind string, round int, record T) error {
	path := roundRecordPath(runDir, kind, round)
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, ".lock"), record); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// appendStrays adds strays to the round's stray report, which it creates when the round has none.
func appendStrays(runDir string, round int, strays []StrayEntry) error {
	path := roundRecordPath(runDir, "strays", round)
	err := state.UpdateJSON(path, lock.TrackedFileLockPath(path, ".lock"), func(cur StrayReport, _ bool) (StrayReport, error) {
		cur.Round = round
		cur.Entries = append(cur.Entries, strays...)
		return cur, nil
	})
	if err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	return nil
}
