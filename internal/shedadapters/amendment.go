// amendment.go implements the Bouncer's new review generation over a changed subject or a newer review input.
// It holds the archive and the generation record that start it.
// It also holds the amendment allowance that bounds an amendment's rounds.
// AmendmentNote is the text that scopes an amendment's review to the change.
// ReviewInputNote is the text that names the inputs a full generation was opened over.

package shedadapters

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// ReviewInput is a file a re-entered review is told to read besides the subject's change.
// WrittenAt is when the file was written.
// Full marks an input that forces a full generation, reviewing the whole range, instead of an amendment.
type ReviewInput struct {
	Path      string
	WrittenAt time.Time
	Full      bool
}

// startNextGeneration starts the generation that follows settled.
// It reads the review inputs first, keeping those written after settled.SettledAt.
// A read error is returned before anything is archived.
// It then archives the run directory with the records travelling along.
// It writes the generation record with the next number and the kept inputs' paths.
// A generation is full when cfg.FullGeneration selects it or a kept input is marked full;
// it then carries no Amendment mark and no base.
// Any other generation is an amendment and carries the Amendment mark and base.
// An amendment with an allowance of 0 settles through approveWithoutReview, and every other generation seeds.
// A failed archive degrades.
// A Call that fails after the archive leaves the generation record naming a generation no seed has run for,
// and the next Call's seed takes it from there.
func (b *Bouncer) startNextGeneration(ctx context.Context, settled settleRecord, base string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	kept, err := b.keptReviewInput(settled.SettledAt)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	amendment := !b.cfg.FullGeneration && !hasFullInput(kept)

	next := settled.Generation + 1
	logger.Warn("shedadapters: bouncer starting a new review generation", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "generation", next, "amendment", amendment, "runDir", b.cfg.RunDir)
	if err := archiveRunDir(b.cfg.RunDir, b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive the run directory for a new generation", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "generation", next, "cause", err)
	}

	record := generationRecord{Number: next}
	for _, input := range kept {
		record.ReviewInputs = append(record.ReviewInputs, input.Path)
	}
	if amendment {
		record.Amendment = true
		record.Base = base
	}
	if err := writeGenerationRecord(b.cfg.RunDir, record); err != nil {
		return "", shedengine.OutputPointer{}, b.recordFailure("generation record", err)
	}

	if amendment && b.cfg.AmendAllowance == 0 {
		return b.approveWithoutReview()
	}
	return b.seedCall(ctx)
}

// keptReviewInput returns the inputs the ReviewInputs seam reports that were written after settledAt.
// They come in the seam's order.
// It returns none when the seam is unset.
// An unset or unparsable settledAt reads as the zero time, so any input is later.
func (b *Bouncer) keptReviewInput(settledAt string) ([]ReviewInput, error) {
	if b.cfg.ReviewInputs == nil {
		return nil, nil
	}
	inputs, err := b.cfg.ReviewInputs()
	if err != nil {
		return nil, fmt.Errorf("shedadapters: %s (%s): read the review inputs: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	settledTime, _ := time.Parse(time.RFC3339, settledAt)
	var kept []ReviewInput
	for _, input := range inputs {
		if input.WrittenAt.After(settledTime) {
			kept = append(kept, input)
		}
	}
	return kept, nil
}

// hasFullInput reports whether any of inputs is marked full.
func hasFullInput(inputs []ReviewInput) bool {
	for _, input := range inputs {
		if input.Full {
			return true
		}
	}
	return false
}

// amendment returns the run directory's generation record and true when it marks an amendment.
// An unreadable record warns and reports false, so the budget and hard cap fall back to the perch's own.
func (b *Bouncer) amendment() (generationRecord, bool) {
	generation, found, err := readGenerationRecord(b.cfg.RunDir)
	if err != nil {
		logger.Warn("shedadapters: bouncer generation record unreadable; treating the generation as no amendment", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "cause", err)
		return generationRecord{}, false
	}
	return generation, found && generation.Amendment
}

// AmendmentNote renders the note that scopes an amendment's review to its change.
// The note is built from the run directory's generation record, and the seed, the reviewers and the judge read it.
// The note is `none` outside an amendment.
// For a document the note limits the review to the artifacts' change since the commit the record's base names.
// For a code segment the settled side is the stored settle-record.patch.
// The current side is the working-tree diff against the current merge base.
// It includes untracked files outside the exclusion set.
// A change already uncommitted at the settle is never shown as new;
// parent changes a Parent-Align brought are left out.
// When the record holds review inputs, the note names each file as an input the amendment reviews besides the change.
func AmendmentNote(runDir string) string {
	generation, found, err := readGenerationRecord(runDir)
	if err != nil {
		logger.Warn("shedadapters: generation record unreadable; rendering no amendment note", "runDir", runDir, "cause", err)
		return noNote
	}
	if !found || !generation.Amendment {
		return noNote
	}

	var note strings.Builder
	if settled, settledFound, _ := readSettleRecord(runDir); settledFound && settled.Subject.Kind == SubjectCode {
		fmt.Fprintf(&note, "This generation is an amendment: the task's change was reviewed and settled before, so review only what changed since.\n")
		fmt.Fprintf(&note, "The settled side is the diff stored at %s, taken at merge base %s.\n", filepath.Join(runDir, settlePatchFile), generation.Base)
		note.WriteString("The current side is the working-tree diff against the current merge base, untracked files outside the exclusion set included.\n")
		note.WriteString("A change already uncommitted at the settle is not new, and parent changes a Parent-Align brought are left out.")
	} else {
		fmt.Fprintf(&note, "This generation is an amendment: the artifacts were reviewed and settled before, so review only the artifacts' change since commit %s.", generation.Base)
	}
	for _, path := range generation.inputPaths() {
		fmt.Fprintf(&note, "\nThe review input is %s; review it besides the change.", path)
	}
	return note.String()
}

// ReviewInputNote renders the note that names the review inputs a full generation was opened over.
// The note is built from the run directory's generation record, and the rubric's review_input marker is filled from it.
// It lists each input's path, which the generation reads besides the diff.
// The note is `none` for an amendment, for a generation whose record holds no input, and when no record exists.
func ReviewInputNote(runDir string) string {
	generation, found, err := readGenerationRecord(runDir)
	if err != nil {
		logger.Warn("shedadapters: generation record unreadable; rendering no review input note", "runDir", runDir, "cause", err)
		return noNote
	}
	paths := generation.inputPaths()
	if !found || generation.Amendment || len(paths) == 0 {
		return noNote
	}

	var note strings.Builder
	note.WriteString("This generation reviews the whole range; it was opened by review inputs written after the perch last settled, which you read besides the diff:")
	for _, path := range paths {
		fmt.Fprintf(&note, "\n- %s", path)
	}
	return note.String()
}
