// amend.go — the parent's amendment of the sealed expectation and the drift records: the rules `lym drift amend` runs, over the records guard.

package driftengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// Who made an amendment.
const (
	// AmendedByParent marks an amendment the run's parent orch made.
	AmendedByParent = "parent"
	// AmendedByOperator marks an amendment the operator made from an unnamed shell.
	AmendedByOperator = "operator"
)

// recheckRow is the recipe row that checks the code a review perch's fixes committed.
const recheckRow = "Drift-Recheck"

// amendmentKindRestore is the Kind of an amendment that wrote back what failed its check.
const amendmentKindRestore = "restore"

// amendForms names the three forms of an amendment as the verb's flags spell them.
const amendForms = "--entries <file>, --restore or --discard"

// amendRows are the rows an amendment is admitted at while blocked: Drift-Expect for Restore alone, the writer rows, Drift-Verify and Drift-Recheck.
var amendRows = []string{expectRow, darnWriteRow, websterRow, verifyRow, recheckRow}

// AmendRequest is one amendment the run's parent asks for.
type AmendRequest struct {
	// By is AmendedByParent or AmendedByOperator.
	By string
	// Entries is the added entries as YAML in expected.yaml's shape.
	Entries string
	// Restore writes back whatever failed its check.
	Restore bool
	// Discard sets the whole expectation aside.
	Discard bool
}

// Check refuses a request that holds none or several of Entries, Restore and Discard, naming the three flags.
func (r AmendRequest) Check() error {
	held := 0
	for _, form := range []bool{r.Entries != "", r.Restore, r.Discard} {
		if form {
			held++
		}
	}
	if held != 1 {
		return fmt.Errorf("drift amend refused: the request holds %d of %s; way forward: pass exactly one of them", held, amendForms)
	}
	return nil
}

// Amend records one amendment of the sealed expectation or the records in one WriteRecords commit, and never edits the seal commit.
// It is admitted only at a blocked writer row, Drift-Verify or Drift-Recheck, and at a blocked Drift-Expect for Restore alone, and refuses a request that holds none or several of the three forms before it reads anything.
// Entries are parsed by ParseExpectation under the recorded mode against the baseline's listing, so an entry the parser refuses is refused with its message, and the stored rulings of every cell they cover, or under diff command of every case they name, are dropped so they are judged again.
// Restore writes back whatever failed its check, the seal commit's copy over an edited expected.yaml and RestoreRecords over records that differ from their commit, and is refused when neither failed.
// Discard records that the expectation is set aside, so the seal check is skipped and containment reads an empty expectation and every amendment.
// An amendment only adds coverage, restores the sealed file and records, or sets the whole expectation aside; it cannot remove one entry or change the seal.
func Amend(d Deps, st VerbStatus, req AmendRequest) error {
	if err := req.Check(); err != nil {
		return err
	}
	if err := admitAmend(st, req); err != nil {
		return err
	}
	records, err := ReadRecords(d)
	if err != nil {
		return err
	}
	switch {
	case !records.ScopeFound:
		return errNoScope
	case records.Scope.Scope == ScopeOff && !req.Restore:
		return fmt.Errorf("drift amend refused: the recorded scope is off, so nothing is checked and nothing needs amending; way forward: lym loom resume")
	}

	amendment := Amendment{By: req.By, At: stamp(d.now())}
	var dropRulings func(judged *JudgedRecord)
	switch {
	case req.Entries != "":
		expectation, err := parseAmendedEntries(d, records, req.Entries)
		if err != nil {
			return err
		}
		amendment.Kind, amendment.Entries = amendmentKindEntries, req.Entries
		dropRulings = func(judged *JudgedRecord) { dropCoveredRulings(judged, expectation, records.Scope.Diff) }
	case req.Restore:
		if err := restoreFailed(d, records); err != nil {
			return err
		}
		amendment.Kind = amendmentKindRestore
	default:
		amendment.Kind = amendmentKindDiscard
	}

	return WriteRecords(d, func(scope *ScopeRecord, judged *JudgedRecord) error {
		scope.Amendments = append(scope.Amendments, amendment)
		if dropRulings != nil {
			dropRulings(judged)
		}
		return nil
	})
}

// admitAmend refuses an amendment at a row it is not admitted at, at an admitted row that is not blocked, and a form other than Restore at Drift-Expect, where no expectation is sealed yet.
func admitAmend(st VerbStatus, req AmendRequest) error {
	row, runState := st.Status.CurrentProducer, st.Status.State
	switch {
	case !slices.Contains(amendRows, row):
		return fmt.Errorf("drift amend refused: the run is at %s (%s), and amend is admitted only at a blocked %s; way forward: lym loom status shows where the run is", orNone(row), runState, strings.Join(amendRows, ", "))
	case runState != shedengine.StateBlocked:
		return fmt.Errorf("drift amend refused: %s is %s, and amend is admitted only at its halt; way forward: wait for the row's halt, which lym loom status shows, then run lym drift amend again", row, runState)
	case row == expectRow && !req.Restore:
		return fmt.Errorf("drift amend refused: at %s only --restore is admitted, since no expectation is sealed yet; way forward: lym loom status shows the halt's own ways forward", row)
	}
	return nil
}

// parseAmendedEntries parses entries under the recorded diff mode against the baseline's listing.
// A refusal of the parser is wrapped with the way forward of correcting the entries file.
func parseAmendedEntries(d Deps, records Records, entries string) (Expectation, error) {
	mode := records.Scope.Diff
	if mode == "" {
		return Expectation{}, fmt.Errorf("drift amend refused: the scope record holds no diff mode yet; way forward: lym loom resume records it, then run lym drift amend again")
	}
	var baseline []CaseListing
	if mode == DiffLines {
		patterns, err := CompileKeyPatterns(records.Scope.KeyPatterns)
		if err != nil {
			return Expectation{}, fmt.Errorf("drift: %w", err)
		}
		if baseline, err = ListCaptures(d.Paths.BaselineDir, patterns); err != nil {
			return Expectation{}, err
		}
	}
	expectation, err := ParseExpectation([]byte(entries), mode, baseline)
	if err != nil {
		return Expectation{}, fmt.Errorf("drift amend refused: %w; way forward: correct the entry in the --entries file and run lym drift amend <slug> again", err)
	}
	return expectation, nil
}

// dropCoveredRulings drops from judged the stored rulings the amended expectation reopens:
// under diff command each case ruling of a case the expectation names, otherwise each cell ruling the expectation covers.
func dropCoveredRulings(judged *JudgedRecord, expectation Expectation, mode string) {
	if mode == DiffCommand {
		judged.Cases = slices.DeleteFunc(judged.Cases, func(ruled JudgedCase) bool {
			_, named := expectation.Cases[ruled.Case]
			return named
		})
		return
	}
	judged.Tuples = slices.DeleteFunc(judged.Tuples, func(ruled JudgedTuple) bool {
		_, covered := expectation.Cover(Cell{Case: ruled.Case, Identity: ruled.Identity, Kind: ruled.Kind, Old: ruled.Old, New: ruled.New})
		return covered
	})
}

// restoreFailed writes back whatever failed its check: the records from their newest commit when they differ from it, and the seal commit's copy over an expected.yaml edited after the seal.
// It refuses when neither failed, naming where the halt's own ways forward are.
func restoreFailed(d Deps, records Records) error {
	restored := false
	if records.Mismatch {
		if err := RestoreRecords(d); err != nil {
			return err
		}
		restored = true
	}
	head, err := baselineHead(d)
	if err != nil {
		return err
	}
	seal, err := CheckSeal(d, records, head)
	if err != nil {
		return err
	}
	if seal.Fault != "" && seal.Sealed != nil {
		lock, err := recordLock(d.Paths, expectedFileName)
		if err != nil {
			return err
		}
		if err := state.WriteFile(d.Paths.Expected, lock, seal.Sealed); err != nil {
			return fmt.Errorf("drift: restore %s: %w", expectedFileName, err)
		}
		restored = true
	}
	if !restored {
		return fmt.Errorf("drift amend refused: --restore found nothing to restore, since the records match their commit and the sealed expectation is unedited; way forward: lym loom status shows the halt's own ways forward, such as --entries <file> for a change the expectation does not cover")
	}
	return nil
}
