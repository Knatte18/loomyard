// pane.go is the pane check every state-changing verb runs first: which strand's pane the process sits in, and whether that strand may run the verb.

package slackcli

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

// ancestryPlatform is the GOOS on which a process is placed in a pane by its process ancestry; elsewhere creel places it by environment.
const ancestryPlatform = "linux"

// paneCaller is what the pane check knows of the process that runs a verb.
type paneCaller struct {
	// PID is the process's id.
	PID int
	// Strands is the hub's strand listing, and StrandsErr the error reading it returned.
	Strands    []creelengine.Strand
	StrandsErr error
	// ParentPID reads a process's parent.
	ParentPID func(pid int) (int, bool)
	// GOOS names the platform the process runs on.
	GOOS string
	// StrandName is the process's $LYM_STRAND_NAME, which places it in a strand's pane off Linux.
	StrandName string
}

// paneRefusal refuses a caller that runs in a live strand's pane, unless allowOrch holds and that strand is an orch.
// It also refuses a caller in no known pane while a live strand's pane process, or the hub's strand listing, is unknown, since it then cannot be told from a strand's process.
// wayForward is the closing clause of the first refusal, and the second names its own.
// A process outside every pane while every live pane process is known passes.
func paneRefusal(caller paneCaller, allowOrch bool, wayForward string) error {
	strand, unreadable := callerPane(caller)
	switch {
	case strand != "" && allowOrch && creelengine.IsOrchStrand(strand):
		return nil
	case strand != "":
		return fmt.Errorf("slack: this runs in the pane of the strand %s, which may not run it; way forward: %s", strand, wayForward)
	case unreadable != "":
		return fmt.Errorf("slack: this runs in no known strand pane and %s, so it cannot be told from a strand's process; way forward: wait until it is known or that strand ends, then run the verb again", unreadable)
	}
	return nil
}

// callerPane returns the strand whose pane the caller sits in, empty when it sits in none, and what keeps a caller in no known pane from being told from a strand's.
// On Linux the strand comes from the process ancestry, creel's own walk.
// Elsewhere it is the strand $LYM_STRAND_NAME names among the hub's, as creel resolves a sender there, and nothing is unreadable.
func callerPane(caller paneCaller) (strand, unreadable string) {
	if caller.GOOS == ancestryPlatform {
		pane, unreadable := creelengine.PaneOf(caller.PID, caller.Strands, caller.StrandsErr, caller.ParentPID)
		if pane == nil {
			return "", unreadable
		}
		return pane.Name, ""
	}
	named := func(strand creelengine.Strand) bool { return strand.Name == caller.StrandName }
	if caller.StrandName != "" && slices.ContainsFunc(caller.Strands, named) {
		return caller.StrandName, ""
	}
	return "", ""
}
