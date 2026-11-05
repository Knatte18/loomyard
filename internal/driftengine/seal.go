// seal.go — the expectation's seal and the baseline's digest: the two checks every drift check runs first, over the records commits that hold their trailers.

package driftengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// The commit trailers that hold the seal and the baseline digest, and the prefix that marks a halt only `lym drift defer` settles.
const (
	// SealTrailerKey carries the sha256 of expected.yaml as Drift-Expect sealed it.
	SealTrailerKey = "Drift-Seal"
	// BaselineTrailerKey carries the BaselineDigest of the baseline directory as it was when its capture counted.
	BaselineTrailerKey = "Drift-Baseline"
	// DeferHaltPrefix marks a Stuck reason whose way forward is `lym drift defer`; it follows the decision halt prefix a row's Stuck opens with, so the verb tells such a halt from another on the same row.
	DeferHaltPrefix = "drift defer halt: "
)

// expectRow is the recipe row that seals the expectation.
const expectRow = "Drift-Expect"

// amendmentKindDiscard is the Kind of an amendment that drops the expectation.
const amendmentKindDiscard = "discard"

// PastRow reports whether current comes after row in rows, the run's recipe row order.
// A row absent from rows, or an absent current, is not past.
func PastRow(rows []string, current, row string) bool {
	currentIndex, rowIndex := slices.Index(rows, current), slices.Index(rows, row)
	return currentIndex >= 0 && rowIndex >= 0 && currentIndex > rowIndex
}

// BaselineDigest hashes the baseline directory's tree with sha256: every regular file's slash-separated relative path and content, in path order.
// A missing directory is an error wrapping fs.ErrNotExist.
func BaselineDigest(dir string) (string, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("drift: digest the baseline directory %s: %w", dir, err)
	}
	sort.Strings(names)

	hash := sha256.New()
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return "", fmt.Errorf("drift: digest the baseline file %s: %w", name, err)
		}
		fmt.Fprintf(hash, "%d:%s\n%d:", len(name), name, len(content))
		hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// BaselineState is what CheckBaseline found.
type BaselineState struct {
	// Fault is the reason the baseline cannot be trusted, opening with DeferHaltPrefix and naming `lym drift defer` as the way forward, and empty when the baseline holds.
	Fault string
	// Unguarded is true when no commit carries BaselineTrailerKey, so only the digest compare was skipped: a capture that counted before the digest existed.
	Unguarded bool
}

// CheckBaseline recomputes the digest of the baseline directory and compares it with the one the oldest commit carrying BaselineTrailerKey recorded, so an edit of the baseline after its capture counted is found.
// A missing directory is a Fault whether or not a commit carries the trailer.
// With no such commit it is Unguarded and only the compare is skipped.
func CheckBaseline(d Deps) (BaselineState, error) {
	if err := requireSeams("the baseline check", seam{"TrailerCommit", d.TrailerCommit != nil}); err != nil {
		return BaselineState{}, err
	}
	commit, found, err := d.TrailerCommit(BaselineTrailerKey, false)
	if err != nil {
		return BaselineState{}, fmt.Errorf("drift: find the oldest %s commit: %w", BaselineTrailerKey, err)
	}
	state := BaselineState{Unguarded: !found}

	digest, err := BaselineDigest(d.Paths.BaselineDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		state.Fault = DeferHaltPrefix + fmt.Sprintf("the baseline directory %s is missing, so the counted capture cannot be compared; way forward: the parent runs lym drift defer, then lym loom resume", d.Paths.BaselineDir)
	case err != nil:
		return BaselineState{}, err
	case found && digest != commit.Value:
		state.Fault = DeferHaltPrefix + fmt.Sprintf("the baseline under %s no longer matches the digest recorded when its capture counted; way forward: the parent runs lym drift defer, then lym loom resume", d.Paths.BaselineDir)
	}
	return state, nil
}

// sealDigest returns the sha256 of an expectation file's bytes, as the seal trailer carries it.
func sealDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SealExpectation reads expected.yaml, hashes it and commits it alone through CommitWithTrailers with `Drift-Seal: <sha256>` and then any extra trailer, such as the baseline digest of a counted capture that has none yet.
func SealExpectation(d Deps, extra ...fabricengine.Trailer) error {
	if err := requireSeams("the seal", seam{"CommitWithTrailers", d.CommitWithTrailers != nil}); err != nil {
		return err
	}
	data, err := os.ReadFile(d.Paths.Expected)
	if err != nil {
		return fmt.Errorf("drift: read the expectation to seal it: %w", err)
	}
	trailers := append([]fabricengine.Trailer{{Key: SealTrailerKey, Value: sealDigest(data)}}, extra...)
	if err := d.CommitWithTrailers([]string{durableRel(expectedFileName)}, trailers); err != nil {
		return fmt.Errorf("drift: commit the sealed expectation: %w", err)
	}
	return nil
}

// SealState is the expectation containment reads, as CheckSeal decided it.
// Exactly one of Sealed, Empty, NotYet and Fault is the verdict, except a Fault after an edit, which keeps the seal's copy in Sealed.
type SealState struct {
	// Sealed is the expectation's bytes as the seal commit holds them.
	Sealed []byte
	// Empty is true when containment reads the empty expectation: nothing was written, the parent discarded it, or the run passed Drift-Expect unsealed.
	Empty bool
	// NotYet is true when the run has not reached Drift-Expect's end, so no expectation exists to check.
	NotYet bool
	// Fault names the failed check and its way forward; a gate or verb that only reports checks against Sealed regardless.
	Fault string
}

// CheckSeal decides the expectation containment reads, in this order:
// a `discard` amendment in rec, or an Unsealed record, is Empty with no check;
// a status row not past Drift-Expect is NotYet;
// with no seal commit, a missing file is Empty and a present one is a Fault naming `--discard`;
// otherwise the oldest commit carrying SealTrailerKey is the seal, and its copy must hash to the trailer's value and its code head must equal baselineHead, else a Fault naming `--discard`,
// and the current file must hash to the trailer's value, else a Fault naming `--restore` with Sealed still the seal's copy.
func CheckSeal(d Deps, rec Records, baselineHead string) (SealState, error) {
	if err := requireSeams("the seal check", seam{"Status", d.Status != nil}, seam{"TrailerCommit", d.TrailerCommit != nil}, seam{"FileAt", d.FileAt != nil}); err != nil {
		return SealState{}, err
	}
	for _, amendment := range rec.Scope.Amendments {
		if amendment.Kind == amendmentKindDiscard {
			return SealState{Empty: true}, nil
		}
	}
	if rec.Scope.Unsealed {
		return SealState{Empty: true}, nil
	}
	status, err := d.Status()
	if err != nil {
		return SealState{}, fmt.Errorf("drift: read the run status for the seal check: %w", err)
	}
	if !PastRow(d.Rows, status.CurrentProducer, expectRow) {
		return SealState{NotYet: true}, nil
	}

	current, fileFound, err := readRecordFile(d.Paths.Expected)
	if err != nil {
		return SealState{}, err
	}
	seal, sealFound, err := d.TrailerCommit(SealTrailerKey, false)
	if err != nil {
		return SealState{}, fmt.Errorf("drift: find the oldest %s commit: %w", SealTrailerKey, err)
	}
	if !sealFound {
		if !fileFound {
			return SealState{Empty: true}, nil
		}
		return SealState{Fault: "expected.yaml exists but no commit sealed it; way forward: the parent runs lym drift amend <slug> --discard"}, nil
	}

	sealed, copyFound, err := d.FileAt(seal.SHA, durableRel(expectedFileName))
	if err != nil {
		return SealState{}, fmt.Errorf("drift: read the sealed expectation at %s: %w", seal.SHA, err)
	}
	if !copyFound || sealDigest(sealed) != seal.Value {
		return SealState{Fault: "the sealed copy of expected.yaml fails its own seal hash; way forward: the parent runs lym drift amend <slug> --discard"}, nil
	}
	if seal.CodeHead != baselineHead {
		return SealState{Fault: fmt.Sprintf("the seal was made at code head %s, not at the baseline's %s; way forward: the parent runs lym drift amend <slug> --discard", seal.CodeHead, baselineHead)}, nil
	}
	if !fileFound || sealDigest(current) != seal.Value {
		return SealState{Sealed: sealed, Fault: "expected.yaml was changed after Drift-Expect sealed it; way forward: the parent runs lym drift amend <slug> --restore"}, nil
	}
	return SealState{Sealed: sealed}, nil
}
