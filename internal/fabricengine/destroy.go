// destroy.go is the only file in package fabricengine permitted to perform a destructive primitive.
// The five primitives are: removing a path (os.RemoveAll/os.Remove), removing a git worktree (git
// worktree remove), removing or re-pointing a link (fslink.Remove), deleting a branch (git branch
// -D), and resetting a warp checkout hard (ResetHard). Every one of them is reached only through one
// of this file's executors, and every executor runs the shared check pipeline before performing its
// act — the gate executes, it does not merely approve.
//
// The pipeline runs four checks, always in this fixed order, stopping at the first failure:
// containment, ownership, dirtiness, force.
// --force answers dirtiness only: it never satisfies containment and never satisfies ownership, so a
// containment failure — the class of defect that once destroyed an entire hub — can never be
// overridden by a flag.
//
// See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant (added once this slice's guard test
// lands) for the machine-enforced half of this rule.

package fabricengine

import (
	"errors"
	"fmt"
)

// destructiveCheck enumerates the four checks the gate's pipeline runs, always in this fixed order.
type destructiveCheck int

const (
	checkContainment destructiveCheck = iota
	checkOwnership
	checkDirtiness
	checkForce
)

// String reports the check's name in prose, so a refusal message names the check that failed.
func (c destructiveCheck) String() string {
	switch c {
	case checkContainment:
		return "containment"
	case checkOwnership:
		return "ownership"
	case checkDirtiness:
		return "dirtiness"
	case checkForce:
		return "force"
	default:
		return "unknown"
	}
}

// destructiveRefusal is the gate's one error type: it names which of the four checks refused a
// destructive request, the requested act, the target (a path or a branch name), and a human reason.
// Every refusal in this file is returned as *destructiveRefusal, never as a bare fmt.Errorf, so a
// caller can always test for one with errors.As.
type destructiveRefusal struct {
	Check  destructiveCheck
	What   string
	Target string
	Reason string
}

// Error implements the error interface.
func (e *destructiveRefusal) Error() string {
	return fmt.Sprintf("refusing to %s: %s check failed for %s: %s", e.What, e.Check, e.Target, e.Reason)
}

// surfaceRefusal returns err unchanged when it carries a *destructiveRefusal, and nil otherwise.
// It is the single expression of the a-refusal-is-never-best-effort decision in _mill/discussion.md:
// every best-effort call site that can return an error wraps its executor call in surfaceRefusal, so
// an operational failure (git exited nonzero, the filesystem said no) stays discardable while a gate
// refusal never does.
func surfaceRefusal(err error) error {
	if errors.As(err, new(*destructiveRefusal)) {
		return err
	}
	return nil
}
