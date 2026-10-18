// identity.go resolves who is calling a request from the connecting peer's process ancestry and what the client claims.

package creelengine

import (
	"errors"
	"fmt"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// maxAncestryDepth bounds the walk up a peer's parents, so a cyclic or absurdly deep ancestry ends the walk.
const maxAncestryDepth = 256

// ErrStrandClaimOutsidePane is what a refusal of a strand claim from a process in no strand pane matches under errors.Is.
// The Go client returns it for a refusal carrying CodeStrandClaimOutsidePane, with the daemon's message as its text.
var ErrStrandClaimOutsidePane = errors.New("creel: the strand claim comes from a process in no strand pane")

// codedRefusal is a refusal whose text is the daemon's message and that matches the sentinel its code stands for.
type codedRefusal struct {
	message  string
	sentinel error
}

func (e *codedRefusal) Error() string { return e.message }

func (e *codedRefusal) Unwrap() error { return e.sentinel }

// Claim is what a client sends with each request.
// It states who the client says it is; the daemon decides who it is from the peer's process ancestry where the platform allows.
type Claim struct {
	// StrandName is the client's $LYX_STRAND_NAME.
	StrandName string `json:"strand_name,omitempty"`
	// Program is the full name of a program sender, set only through the Go client API; the CLI has no flag for it.
	Program string `json:"program,omitempty"`
	// Terminal, ParentProcess and WorkingDir are the envelope an EXTERNAL sender is shown with.
	Terminal      string `json:"terminal,omitempty"`
	ParentProcess string `json:"parent_process,omitempty"`
	WorkingDir    string `json:"working_dir,omitempty"`
}

// resolveSender decides who sent a request.
// peerPID is the connecting process, 0 when the platform cannot tell; strands and strandsErr are Delivery.Strands' answer; parentPID reads a process's parent.
// On Linux a peer inside a live strand's pane process tree is that strand, whatever its environment says, and a claim never raises a pane's privilege.
// Elsewhere identity is the environment alone.
// The refusal of a strand claim naming a listed strand from a process in no pane wraps ErrStrandClaimOutsidePane; no other refusal does.
func resolveSender(peerPID int, claim Claim, shortname string, strands []Strand, strandsErr error, parentPID func(int) (int, bool), goos string) (Sender, error) {
	program, err := checkProgramClaim(claim, shortname)
	if err != nil {
		return Sender{}, err
	}
	if goos != "linux" {
		return resolveFromEnvironment(claim, shortname, program)
	}
	pane, unreadable := paneOfPeer(peerPID, strands, strandsErr, parentPID)
	if pane != nil {
		if claim.StrandName != "" && claim.StrandName != pane.Name {
			return Sender{}, fmt.Errorf("creel: LYX_STRAND_NAME names %s but the calling process runs in the pane of %s", claim.StrandName, pane.Name)
		}
		if program == "" {
			return StrandSender(pane.Name), nil
		}
		if !isOrchName(pane.Name) {
			return Sender{}, fmt.Errorf("creel: %s may not sign as the program %s: only an orch strand's pane may claim a program name", pane.Name, program)
		}
		return ProgramSender(program), nil
	}
	if unreadable != "" {
		return Sender{}, fmt.Errorf("creel: the calling process is in no known strand pane and %s, so it cannot be told from a strand; nothing was done", unreadable)
	}
	if _, named := findStrand(strands, claim.StrandName); named && claim.StrandName != "" {
		return Sender{}, &codedRefusal{
			message:  fmt.Sprintf("creel: LYX_STRAND_NAME names %s but the calling process runs in no strand pane", claim.StrandName),
			sentinel: ErrStrandClaimOutsidePane,
		}
	}
	if program != "" {
		return ProgramSender(program), nil
	}
	return externalFromClaim(claim, shortname)
}

// checkProgramClaim validates a program claim: a well-formed upper-case name under this hub's shortname that is not EXTERNAL.
// It returns the empty string when the client claims none.
func checkProgramClaim(claim Claim, shortname string) (string, error) {
	if claim.Program == "" {
		return "", nil
	}
	claimed, name, err := agentname.ParseProgram(claim.Program)
	if err != nil {
		return "", fmt.Errorf("creel: refused the program claim: %w", err)
	}
	if name == agentname.External {
		return "", fmt.Errorf("creel: refused the program claim %s: %s is reserved for senders creel cannot place", claim.Program, agentname.External)
	}
	if claimed != shortname {
		return "", fmt.Errorf("creel: refused the program claim %s: this hub's shortname is %s", claim.Program, shortname)
	}
	return claim.Program, nil
}

// resolveFromEnvironment is identity off Linux: the strand name when it is one of this hub, else the program claim, else EXTERNAL.
func resolveFromEnvironment(claim Claim, shortname, program string) (Sender, error) {
	if name, err := agentname.Parse(claim.StrandName); err == nil && name.Shortname == shortname {
		return StrandSender(claim.StrandName), nil
	}
	if program != "" {
		return ProgramSender(program), nil
	}
	return externalFromClaim(claim, shortname)
}

func externalFromClaim(claim Claim, shortname string) (Sender, error) {
	return ExternalSender(shortname, claim.Terminal, claim.ParentProcess, claim.WorkingDir)
}

// paneOfPeer walks the peer's ancestry, the peer itself first, and returns the live strand whose pane process it meets.
// unreadable says what keeps a peer in no known pane from being told from a strand, and is empty when every live strand's pane process is known.
func paneOfPeer(peerPID int, strands []Strand, strandsErr error, parentPID func(int) (int, bool)) (pane *Strand, unreadable string) {
	if strandsErr != nil {
		return nil, fmt.Sprintf("the hub's strands could not be read (%v)", strandsErr)
	}
	panes := map[int]*Strand{}
	for i := range strands {
		strand := &strands[i]
		if !strand.Live {
			continue
		}
		if strand.PanePID <= 0 {
			unreadable = fmt.Sprintf("the pane process of %s is unknown", strand.Name)
			continue
		}
		panes[strand.PanePID] = strand
	}
	pid := peerPID
	for depth := 0; pid > 0 && depth < maxAncestryDepth; depth++ {
		if strand, ok := panes[pid]; ok {
			return strand, ""
		}
		parent, ok := parentPID(pid)
		if !ok || parent == pid {
			break
		}
		pid = parent
	}
	return nil, unreadable
}
