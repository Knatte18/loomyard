// finding.go declares the Finding value type and its Kind discriminator, the vocabulary Check
// reports its results in.

package shedcheck

import "fmt"

// Kind is the stable, machine-readable discriminator a caller filters or allowlists on.
type Kind string

// The eight finding kinds, in the fixed order Check reports them: this const block's declaration
// order is also Check's own fixed check order.
const (
	KindBadEntry           Kind = "bad-entry"
	KindNoTerminals        Kind = "no-terminals"
	KindBadTerminal        Kind = "bad-terminal"
	KindDanglingTarget     Kind = "dangling-target"
	KindUnreachable        Kind = "unreachable"
	KindUnexpectedTerminal Kind = "unexpected-terminal"
	KindDoneCycle          Kind = "done-cycle"
	KindBlindGate          Kind = "blind-gate"
)

// Finding is one defect Check reports. There is no severity field: every Finding is a defect, and
// Kind is what a caller discriminates on.
//
// Kind, Producer, and Target are the pinned contract every test asserts against literally.
// Message is human-readable prose whose wording is deliberately not pinned by any test.
type Finding struct {
	Kind     Kind
	Producer string
	Target   string
	Message  string
}

// String returns a single line suitable for a test-failure message, containing the Kind, the
// Producer, the Target, and the Message. Its exact format is not part of the contract.
func (f Finding) String() string {
	return fmt.Sprintf("%s: producer=%q target=%q: %s", f.Kind, f.Producer, f.Target, f.Message)
}
