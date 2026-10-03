// retiring_test.go pins MarkRetiring's persistence, its Status projection, and the ErrUnknownStrand sentinel the unknown-guid refusals wrap.

package reedengine

import (
	"errors"
	"testing"
)

func TestMarkRetiring_RoundTripsThroughStateAndStatus(t *testing.T) {
	const selvagePane = "%1"
	const strandPane = "%2"
	liveGeneration := PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "4321", Created: "1787000000"}

	e := newTestEngine(t)
	fake := installFakeTmux(t, e)
	fake.answer("display-message", "$0|4321|1787000000", nil)
	fake.answer("list-sessions", "worktree\n", nil)
	fake.answer("list-panes", selvagePane+" 0 0 100 3 4322\n"+strandPane+" 0 3 100 20 4323\n", nil)
	st := &ReedState{
		SelvagePaneID:  selvagePane,
		PaneGeneration: liveGeneration,
		Strands: []Strand{
			{GUID: "marked", Name: "marked", PaneID: strandPane},
			{GUID: "other", Name: "other"},
		},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if err := e.MarkRetiring("marked", true); err != nil {
		t.Fatalf("MarkRetiring(true): %v", err)
	}
	loaded, err := LoadState(e.stateDir())
	if err != nil || loaded == nil {
		t.Fatalf("LoadState = %v, %v", loaded, err)
	}
	if !loaded.Strands[0].Retiring || loaded.Strands[1].Retiring {
		t.Fatalf("persisted Retiring = [%v %v]; want [true false]", loaded.Strands[0].Retiring, loaded.Strands[1].Retiring)
	}

	result, err := e.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !result.Strands[0].Retiring || result.Strands[1].Retiring {
		t.Fatalf("Status Retiring = [%v %v]; want [true false]", result.Strands[0].Retiring, result.Strands[1].Retiring)
	}

	if err := e.MarkRetiring("marked", false); err != nil {
		t.Fatalf("MarkRetiring(false): %v", err)
	}
	loaded, err = LoadState(e.stateDir())
	if err != nil || loaded == nil {
		t.Fatalf("LoadState = %v, %v", loaded, err)
	}
	if loaded.Strands[0].Retiring {
		t.Fatal("Retiring still set after MarkRetiring(false)")
	}
}

func TestMarkRetiring_UnknownGUIDWrapsErrUnknownStrand(t *testing.T) {
	e := newTestEngine(t)
	if err := SaveState(e.stateDir(), &ReedState{Strands: []Strand{{GUID: "a", Name: "a"}}}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := e.MarkRetiring("missing", true); !errors.Is(err, ErrUnknownStrand) {
		t.Fatalf("MarkRetiring(unknown) = %v; want it to wrap ErrUnknownStrand", err)
	}
}

func TestMarkRetiring_NoStateFileWrapsErrUnknownStrand(t *testing.T) {
	e := newTestEngine(t)
	if err := e.MarkRetiring("missing", true); !errors.Is(err, ErrUnknownStrand) {
		t.Fatalf("MarkRetiring(no state) = %v; want it to wrap ErrUnknownStrand", err)
	}
}

func TestRemoveStrandLocked_UnknownGUIDWrapsErrUnknownStrand(t *testing.T) {
	e := newTestEngine(t)
	_, _, err := e.removeStrandLocked(&ReedState{}, "missing", false)
	if !errors.Is(err, ErrUnknownStrand) {
		t.Fatalf("removeStrandLocked(unknown) = %v; want it to wrap ErrUnknownStrand", err)
	}
}
