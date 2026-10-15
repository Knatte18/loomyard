// seam.go declares the delivery seam the engine reaches panes through, and the Sender a resolved caller is.

package creelengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
)

// Strand is one strand of the hub as the delivery seam reports it.
type Strand struct {
	// Name is the strand's full agent name.
	Name string
	// GUID is the strand's identity for its whole life.
	GUID string
	// Worktree is the absolute path of the worktree whose reed state holds the strand.
	Worktree string
	// Slug is the task slug of the worktree, empty in the prime.
	Slug string
	// Live reports whether the strand has a running pane; a dormant strand keeps its mail and queue.
	Live bool
	// PanePID is the process id of the strand's pane process, 0 when unknown.
	PanePID int
}

// Delivery is the seam through which the engine learns the hub's strands and reaches their panes.
// The engine decides what is typed and when; an implementation knows the providers and does the keystrokes.
type Delivery interface {
	// Strands lists every strand of the hub.
	// An error names what could not be read: the worktree list or one reed state.
	Strands() ([]Strand, error)
	// Idle reports whether the strand's agent is idle and may be typed into.
	Idle(strand Strand) (bool, error)
	// Paused reports whether a pause covers the strand; an error means the pause record could not be read.
	Paused(strand Strand) (bool, error)
	// Send types one line into the strand's pane and confirms it landed.
	// It returns ErrBusy when the pane guard held the write, and ErrNotLanded when the text was typed but not confirmed.
	Send(strand Strand, text string) error
	// Interrupt sends the guarded interrupt to the strand's running turn.
	// It returns ErrNoRunningTurn when there is none.
	Interrupt(strand Strand) error
}

var (
	// ErrBusy means the pane guard held a send: nothing was typed and the caller retries later.
	ErrBusy = errors.New("creel: the receiver is busy, nothing was typed")
	// ErrNotLanded means a send was typed but its landing was not confirmed, so it must never be typed again.
	ErrNotLanded = errors.New("creel: the text was typed but not confirmed")
	// ErrNoRunningTurn means an interrupt found no running turn and sent nothing.
	ErrNoRunningTurn = errors.New("creel: the receiver has no running turn")
)

// SenderKind tells the three kinds of caller apart.
type SenderKind int

const (
	// KindStrand is an agent in a reed strand.
	KindStrand SenderKind = iota
	// KindProgram is a Go program naming itself with an upper-case name.
	KindProgram
	// KindExternal is a caller creel cannot place: a terminal, or a session outside reed.
	KindExternal
)

// Sender is a resolved caller.
type Sender struct {
	// Name is the full agent name: a strand's, a program's, or "<shortname>:EXTERNAL".
	Name string
	Kind SenderKind
	// Terminal, ParentProcess and WorkingDir are the envelope creel sees of an EXTERNAL sender.
	Terminal      string
	ParentProcess string
	WorkingDir    string
}

// StrandSender returns the sender for the strand with the given full name.
func StrandSender(name string) Sender { return Sender{Name: name, Kind: KindStrand} }

// ProgramSender returns the sender for the program with the given full name, such as "ly:BATTEN".
func ProgramSender(name string) Sender { return Sender{Name: name, Kind: KindProgram} }

// ExternalSender returns the EXTERNAL sender of a hub with the envelope creel saw.
// It returns an error when the shortname is not a valid one.
func ExternalSender(shortname, terminal, parentProcess, workingDir string) (Sender, error) {
	name, err := agentname.FormatProgram(shortname, agentname.External)
	if err != nil {
		return Sender{}, fmt.Errorf("creel: cannot name the EXTERNAL sender: %w", err)
	}
	return Sender{
		Name: name, Kind: KindExternal,
		Terminal: terminal, ParentProcess: parentProcess, WorkingDir: workingDir,
	}, nil
}

// Envelope renders what creel saw of an EXTERNAL sender as "parent, terminal, directory", leaving out what is empty.
func (s Sender) Envelope() string {
	if s.Kind != KindExternal {
		return ""
	}
	var parts []string
	for _, part := range []string{s.ParentProcess, s.Terminal, s.WorkingDir} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}

// String renders the sender as a reader sees it: an EXTERNAL sender carries its envelope, as in "ly:EXTERNAL (claude, pts/4, ~/Code/…)".
func (s Sender) String() string {
	if envelope := s.Envelope(); envelope != "" {
		return s.Name + " (" + envelope + ")"
	}
	return s.Name
}

// isOrchName reports whether full names an orch strand, a role that matches orch or orch-N.
func isOrchName(full string) bool {
	name, err := agentname.Parse(full)
	return err == nil && agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// privileged reports whether the sender may inject, interrupt and hold on another strand: EXTERNAL, a program and an orch strand.
func (s Sender) privileged() bool {
	return s.Kind != KindStrand || isOrchName(s.Name)
}
