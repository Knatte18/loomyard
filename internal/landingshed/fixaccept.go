// fixaccept.go implements AcceptFixReview, the operator's acceptance of a merge fixer change Finalize's fix reviews did not approve within their rounds.

package landingshed

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrNoFixReviewHalt reports a Finalize entry that is not halted at its fix review round cap, so there is no change to accept.
var ErrNoFixReviewHalt = errors.New("the Finalize entry is not halted at its fix review round cap")

// fixAcceptHeading opens the section AcceptFixReview appends to the change description.
const fixAcceptHeading = "## Unresolved fix review findings"

// AcceptFixReview records the operator's acceptance of the fixer change the Finalize entry halted on in the entry's attempt record under scratchDir.
// It appends the findings the last fix review left open to the change description at descriptionPath, so the run lands with them on record.
// The acceptance is bound to the task HEAD taskHead answers, so on resume Finalize lands the change at that HEAD as it stands and sends any later change to the fix review.
// An entry not halted at its fix review round cap is ErrNoFixReviewHalt, and nothing is written.
// A description that already holds the section is not appended to again, and an entry already accepted writes nothing, so a retry after a failed write is safe.
func AcceptFixReview(scratchDir string, entry int, descriptionPath string, taskHead func() (string, error)) error {
	path := FixerAttemptsPath(scratchDir, finalizeName)
	rec, err := ReadFixerAttempts(path, entry)
	if err != nil {
		return err
	}
	if rec.Halt != fixerHaltReviewCapped {
		return ErrNoFixReviewHalt
	}
	if rec.AcceptedHead != "" {
		return nil
	}
	head, err := taskHead()
	if err != nil {
		return fmt.Errorf("landingshed: read the task head: %w", err)
	}
	description, err := os.ReadFile(descriptionPath)
	if err != nil {
		return fmt.Errorf("landingshed: read the change description %s: %w", descriptionPath, err)
	}
	if !strings.Contains(string(description), "\n"+fixAcceptHeading+"\n") {
		appended := strings.TrimRight(string(description), "\n") + "\n\n" + fixAcceptSection(rec)
		if err := writeBytesAtomic(descriptionPath, "change description", []byte(appended)); err != nil {
			return err
		}
	}
	rec.AcceptedHead = head
	return WriteFixerAttempts(path, rec)
}

// fixAcceptSection renders the change description's section naming the fixer change the operator accepted and the findings left open on it.
func fixAcceptSection(rec FixerAttempts) string {
	var b strings.Builder
	b.WriteString(fixAcceptHeading + "\n\n")
	fmt.Fprintf(&b, "Finalize's fix review did not approve the merge fixer's change since %s within its rounds, and the operator accepted the change as it stands with these findings open:\n\n", rec.Baseline)
	for _, finding := range rec.Findings {
		b.WriteString("- " + finding + "\n")
	}
	return b.String()
}
