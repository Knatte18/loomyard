// amendment.go implements the Bouncer's new review generation over a changed subject.
// It holds the archive and the generation record that start it, the amendment allowance that bounds an amendment's rounds, and AmendmentNote, the text that scopes an amendment's review to the change.

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

// startNextGeneration starts the generation after settled's over a changed subject.
// It reads the review input first, keeping its path when the input was written after settled.SettledAt, and returns a read error before anything is archived.
// It then archives the run directory, the records travelling along, and writes the generation record with the next number.
// An amendment carries the Amendment mark, base and the kept review input path; a full generation, which cfg.FullGeneration selects, carries the number alone.
// An amendment with an allowance of 0 settles through approveWithoutReview, and every other generation seeds.
// A failed archive degrades, as the clear does.
// A Call that fails after the archive leaves the generation record naming a generation no seed has run for, and the next Call's seed takes it from there.
func (b *Bouncer) startNextGeneration(ctx context.Context, settled settleRecord, base string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	amendment := !b.cfg.FullGeneration
	inputPath := ""
	if amendment {
		var err error
		if inputPath, err = b.keptReviewInput(settled.SettledAt); err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}

	next := settled.Generation + 1
	logger.Warn("shedadapters: bouncer starting a new review generation over a changed subject", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "generation", next, "amendment", amendment, "runDir", b.cfg.RunDir)
	if err := archiveRunDir(b.cfg.RunDir, b.cfg.Now); err != nil {
		return b.degrade(ctx, "shedadapters: bouncer failed to archive the run directory for a new generation", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "generation", next, "cause", err)
	}

	record := generationRecord{Number: next}
	if amendment {
		record.Amendment = true
		record.Base = base
		record.ReviewInput = inputPath
	}
	if err := writeGenerationRecord(b.cfg.RunDir, record); err != nil {
		return "", shedengine.OutputPointer{}, b.recordFailure("generation record", err)
	}

	if amendment && b.cfg.AmendAllowance == 0 {
		return b.approveWithoutReview()
	}
	return b.seedCall(ctx)
}

// keptReviewInput returns the path of the review input when the ReviewInput seam reports one written after settledAt, and "" otherwise.
// An unset or unparsable settledAt reads as the zero time, so any input is later.
func (b *Bouncer) keptReviewInput(settledAt string) (string, error) {
	if b.cfg.ReviewInput == nil {
		return "", nil
	}
	path, writtenAt, found, err := b.cfg.ReviewInput()
	if err != nil {
		return "", fmt.Errorf("shedadapters: %s (%s): read the review input: %w", b.cfg.Name, bouncerEngineLabel, err)
	}
	settledTime, _ := time.Parse(time.RFC3339, settledAt)
	if !found || !writtenAt.After(settledTime) {
		return "", nil
	}
	return path, nil
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

// AmendmentNote renders from the run directory's generation record the note the seed, the reviewers and the judge read, scoping an amendment's review to its change.
// It is `none` outside an amendment.
// For a document the note limits the review to the artifacts' change since the commit the record's base names.
// For a code segment the settled side is the stored settle-record.patch and the current side is the working-tree diff against the current merge base, untracked files outside the exclusion set included,
// so a change already uncommitted at the settle is never shown as new and parent changes a Parent-Align brought are left out.
// The note also names the review input file when the record holds one, as an input the amendment reviews besides the change.
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
	if generation.ReviewInput != "" {
		fmt.Fprintf(&note, "\nThe review input is %s; review it besides the change.", generation.ReviewInput)
	}
	return note.String()
}
