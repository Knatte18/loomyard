// settlerecord.go implements the Bouncer's two durable records beside its run directory's round files:
// the settle record that says a segment's verdict settled over which subject, and the generation record that numbers the review generations.
// It also holds the subject identity both records carry, the settle record's liveness test and the carry of the records across a run-directory archive.

package shedadapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

const (
	settleRecordFile     = "settle-record.json"
	settlePatchFile      = "settle-record.patch"
	generationRecordFile = "generation-record.json"
)

// The kinds of subject a review segment judges.
const (
	// SubjectDocument is a document segment's subject: the reviewed artifacts, identified by the hash of their identity text.
	SubjectDocument = "document"
	// SubjectCode is a code segment's subject: the task's diff against its merge base.
	SubjectCode = "code"
	// SubjectMerge is a merge-in's subject, identified by the merge-in's own identity hash.
	SubjectMerge = "merge"
)

// Subject is the identity of what a review segment judges, compared to tell a re-entry over an unchanged subject from one over a changed subject.
// Kind is one of the Subject constants.
// Hash is a document's identity-text hash or a merge-in's identity.
// MergeBase, Head, FullHash and PatchID describe a code segment: the merge base and the branch head the diff was taken at, the hash of the full diff and the diff's patch-id.
// Base is the amendment base read with the subject, the record head a document settled at or a code segment's merge base, and Patch is a code segment's full diff.
// Base and Patch are never encoded: a record holds them beside the subject.
type Subject struct {
	Kind      string `json:"kind"`
	Hash      string `json:"hash,omitempty"`
	MergeBase string `json:"merge_base,omitempty"`
	Head      string `json:"head,omitempty"`
	FullHash  string `json:"full_hash,omitempty"`
	PatchID   string `json:"patch_id,omitempty"`
	Base      string `json:"-"`
	Patch     []byte `json:"-"`
}

// Unchanged reports whether s is the same subject as settled.
// A document or a merge-in is unchanged when its Kind and Hash are equal.
// A code segment is unchanged when its merge base is the settled one and its FullHash is equal.
// It is also unchanged when the merge base moved, mergeInOnly(settled.Head) reports that the history since then holds only merge-ins, and the PatchID is equal.
// A nil mergeInOnly reads a moved merge base as changed, and a mergeInOnly error is returned.
//
// Bound: a moved merge base with a merge-in-only history and an equal patch-id reads as unchanged.
// That lets through, unreviewed by this segment, a parent change that interacts with the task's change without sharing a line with it.
// The verify tiers bound it.
func (s Subject) Unchanged(settled Subject, mergeInOnly func(since string) (bool, error)) (bool, error) {
	if s.Kind != settled.Kind {
		return false, nil
	}
	switch s.Kind {
	case SubjectDocument, SubjectMerge:
		return s.Hash == settled.Hash, nil
	case SubjectCode:
		if s.MergeBase == settled.MergeBase {
			return s.FullHash == settled.FullHash, nil
		}
		if mergeInOnly == nil {
			return false, nil
		}
		only, err := mergeInOnly(settled.Head)
		if err != nil || !only {
			return false, err
		}
		return s.PatchID == settled.PatchID, nil
	}
	return false, nil
}

// settleRecord is the record of one settled review verdict, kept in the run directory as settle-record.json.
// Base is the amendment base, Generation and Round are the generation and round that settled, and SettledAt is the settle time in RFC 3339.
// Settled is true once the segment's Commit returned, so a record with it false is a settle that was interrupted before the commit.
// A code segment's full diff lives beside it in settle-record.patch.
// A Generation below 1 reads as 1.
type settleRecord struct {
	Subject    Subject `json:"subject"`
	Base       string  `json:"base,omitempty"`
	Generation int     `json:"generation,omitempty"`
	Round      int     `json:"round"`
	SettledAt  string  `json:"settled_at"`
	Settled    bool    `json:"settled"`
}

// generationRecord is the record of a run directory's review generation, kept as generation-record.json.
// Number is the generation, Amendment says the generation reviews only a change under the amendment allowance, and Base is its amendment base.
// RejectReview is the review file of the PR-Gate reject a re-entry is told.
// JudgeSubject, JudgeBase and JudgeAt (RFC 3339) are the subject, base and time the latest judge spawn recorded.
type generationRecord struct {
	Number       int     `json:"number"`
	Amendment    bool    `json:"amendment,omitempty"`
	Base         string  `json:"base,omitempty"`
	RejectReview string  `json:"reject_review,omitempty"`
	JudgeSubject Subject `json:"judge_subject"`
	JudgeBase    string  `json:"judge_base,omitempty"`
	JudgeAt      string  `json:"judge_at,omitempty"`
}

// readRecord decodes the JSON record at path, tolerating keys the type does not know.
// An absent file, and one in an absent directory, is not found; reading an absent record creates neither a directory nor a lock file.
func readRecord[T any](path string) (T, bool, error) {
	if _, err := os.Stat(path); err != nil {
		var zero T
		if errors.Is(err, os.ErrNotExist) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("shedadapters: stat record %s: %w", path, err)
	}
	record, found, err := state.ReadJSONNoMkdir[T](path, path+".lock")
	if err != nil {
		return record, false, fmt.Errorf("shedadapters: read record %s: %w", path, err)
	}
	return record, found, nil
}

// writeRecord writes the JSON record at path through internal/state under a lock beside it.
func writeRecord[T any](path string, record T) error {
	if err := state.WriteJSON(path, path+".lock", record); err != nil {
		return fmt.Errorf("shedadapters: write record %s: %w", path, err)
	}
	return nil
}

// readSettleRecord reads the run directory's settle record, reporting false when it holds none.
func readSettleRecord(runDir string) (settleRecord, bool, error) {
	record, found, err := readRecord[settleRecord](filepath.Join(runDir, settleRecordFile))
	if record.Generation < 1 {
		record.Generation = 1
	}
	return record, found, err
}

// writeSettleRecord writes the run directory's settle record, and a non-nil patch beside it first, so a record never names a patch that is absent.
func writeSettleRecord(runDir string, rec settleRecord, patch []byte) error {
	if patch != nil {
		patchPath := filepath.Join(runDir, settlePatchFile)
		if err := state.WriteFile(patchPath, patchPath+".lock", patch); err != nil {
			return fmt.Errorf("shedadapters: write settle patch %s: %w", patchPath, err)
		}
	}
	return writeRecord(filepath.Join(runDir, settleRecordFile), rec)
}

// readGenerationRecord reads the run directory's generation record, reporting false when it holds none.
func readGenerationRecord(runDir string) (generationRecord, bool, error) {
	return readRecord[generationRecord](filepath.Join(runDir, generationRecordFile))
}

// writeGenerationRecord writes the run directory's generation record.
func writeGenerationRecord(runDir string, rec generationRecord) error {
	return writeRecord(filepath.Join(runDir, generationRecordFile), rec)
}

// liveSettleRecord returns the run directory's settle record when it is live, and false otherwise.
// A record is live when it names the directory's current generation, which is the generation record's number or 1 when there is none,
// and no `round-<n>-review.md` is on disk for a round past the settled one.
func liveSettleRecord(runDir string) (settleRecord, bool, error) {
	record, found, err := readSettleRecord(runDir)
	if err != nil || !found {
		return settleRecord{}, false, err
	}
	generation, generationFound, err := readGenerationRecord(runDir)
	if err != nil {
		return settleRecord{}, false, err
	}
	current := 1
	if generationFound && generation.Number > 1 {
		current = generation.Number
	}
	if record.Generation != current {
		return settleRecord{}, false, nil
	}

	entries, err := os.ReadDir(runDir)
	if err != nil {
		return settleRecord{}, false, fmt.Errorf("shedadapters: read run dir %s: %w", runDir, err)
	}
	for _, entry := range entries {
		round, kind, ok := parseRoundFileName(entry.Name())
		if ok && kind == "review.md" && round > record.Round {
			return settleRecord{}, false, nil
		}
	}
	return record, true, nil
}

// SettleLive reports whether the run directory holds a live settle record, and when that record settled.
// A record is live when it names the directory's current generation and no later round's review is on disk.
// A live record whose settle time does not parse is an error.
func SettleLive(runDir string) (settledAt time.Time, live bool, err error) {
	record, live, err := liveSettleRecord(runDir)
	if err != nil || !live {
		return time.Time{}, false, err
	}
	settledAt, err = time.Parse(time.RFC3339, record.SettledAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("shedadapters: settle record in %s: settled_at %q: %w", runDir, record.SettledAt, err)
	}
	return settledAt, true, nil
}

// CarryRecords copies the settle record, its patch and the generation record that exist in fromDir into toDir, byte for byte.
// An absent file is skipped, and an absent fromDir holds none.
// It copies nothing when toDir already holds a generation record, since every seed and settle that has a subject writes one.
// The generation record is copied last, so a carry that failed part way leaves none and a repeat completes it.
func CarryRecords(fromDir, toDir string) error {
	generationPath := filepath.Join(toDir, generationRecordFile)
	if _, err := os.Stat(generationPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("shedadapters: stat %s: %w", generationPath, err)
	}

	for _, name := range []string{settleRecordFile, settlePatchFile, generationRecordFile} {
		data, err := os.ReadFile(filepath.Join(fromDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("shedadapters: read %s to carry: %w", filepath.Join(fromDir, name), err)
		}
		target := filepath.Join(toDir, name)
		if err := state.WriteFile(target, target+".lock", data); err != nil {
			return fmt.Errorf("shedadapters: carry %s: %w", target, err)
		}
	}
	return nil
}
