// strand_test.go drives the strand-mutation *Locked helpers directly against a fixture .lyx: guid
// generation/uniqueness, unknown/cyclic parent rejection, the hidden-add no-launch path, the
// launch-path decision seam (needsLaunchOnAdd/needsLaunchOnSurface — the actual real-tmux launch
// itself is out of hermetic reach, see spawn_test.go), UpdateStrand's visible->hidden rejection,
// and RemoveStrand's non-leaf guard/cascade.
// None of the *Locked cases touch tmux: addStrandLocked/updateStrandLocked only reach tmux through
// launchStrandLocked,
// and every case here either stays hidden or is a rejection that never gets there;
// removeStrandLocked never touches tmux at all.
// The R5-F4 cases at the end of this file are the exception: they drive the pane-TARGETING half of
// RemoveStrand and the transport ops, which is tmux-facing by definition, through TmuxCmd's
// execHook seam rather than a live server.

package reedengine

import (
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

func TestAddStrandLocked_HiddenAdd_GuidUniqueRecordStoredNoLaunch(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}

	spec := AddSpec{Cmd: "claude --session-id abc", Display: render.Display{Anchor: render.AnchorHidden}}

	first, err := e.addStrandLocked(st, spec)
	if err != nil {
		t.Fatalf("addStrandLocked: %v", err)
	}
	second, err := e.addStrandLocked(st, spec)
	if err != nil {
		t.Fatalf("addStrandLocked: %v", err)
	}

	if len(first.GUID) != 32 || len(second.GUID) != 32 {
		t.Fatalf("guid lengths = %d, %d, want 32 hex chars each", len(first.GUID), len(second.GUID))
	}
	if first.GUID == second.GUID {
		t.Errorf("addStrandLocked produced duplicate guids: %q", first.GUID)
	}

	if len(st.Strands) != 2 {
		t.Fatalf("st.Strands has %d entries, want 2", len(st.Strands))
	}
	for _, s := range st.Strands {
		if s.PaneID != "" {
			t.Errorf("hidden-add strand %q PaneID = %q, want empty (launchStrandLocked must not run)", s.GUID, s.PaneID)
		}
		if s.Cmd != spec.Cmd {
			t.Errorf("hidden-add strand %q Cmd = %q, want %q stored verbatim though unrun", s.GUID, s.Cmd, spec.Cmd)
		}
	}
}

// TestAddStrandLocked_SessionIDRoundTripsThroughSaveLoad pins AddSpec.SessionID as opaque caller
// metadata: addStrandLocked stamps it verbatim into the appended Strand,
// and it survives a SaveState/LoadState round trip on disk exactly like every other carrier field
// (Cmd, ResumeCmd, Name).
func TestAddStrandLocked_SessionIDRoundTripsThroughSaveLoad(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}

	spec := AddSpec{SessionID: "caller-session-abc", Display: render.Display{Anchor: render.AnchorHidden}}
	strand, err := e.addStrandLocked(st, spec)
	if err != nil {
		t.Fatalf("addStrandLocked: %v", err)
	}
	if strand.SessionID != spec.SessionID {
		t.Fatalf("strand.SessionID = %q, want %q", strand.SessionID, spec.SessionID)
	}

	dotLyxDir := e.stateDir()
	if err := SaveState(dotLyxDir, st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	loaded, err := LoadState(dotLyxDir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	got, ok := strandByGUID(loaded.Strands, strand.GUID)
	if !ok {
		t.Fatalf("LoadState result missing strand %q", strand.GUID)
	}
	if got.SessionID != spec.SessionID {
		t.Errorf("loaded strand.SessionID = %q, want %q to survive SaveState/LoadState", got.SessionID, spec.SessionID)
	}
}

func TestAddStrandLocked_UnknownParentRejected(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}

	_, err := e.addStrandLocked(st, AddSpec{Parent: "does-not-exist", Display: render.Display{Anchor: render.AnchorHidden}})
	if err == nil {
		t.Fatal("addStrandLocked with unknown parent = nil error, want error")
	}
	if len(st.Strands) != 0 {
		t.Errorf("st.Strands = %+v, want no record registered on a rejected add", st.Strands)
	}
}

func TestAddStrandLocked_KnownParentAccepted(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{{GUID: "parent-guid", Display: render.Display{Anchor: render.AnchorHidden}}}}

	strand, err := e.addStrandLocked(st, AddSpec{Parent: "parent-guid", Display: render.Display{Anchor: render.AnchorHidden}})
	if err != nil {
		t.Fatalf("addStrandLocked: %v", err)
	}
	if strand.Parent != "parent-guid" {
		t.Errorf("strand.Parent = %q, want %q", strand.Parent, "parent-guid")
	}
}

func TestWouldFormCycle(t *testing.T) {
	strands := []Strand{
		{GUID: "root", Parent: ""},
		{GUID: "mid", Parent: "root"},
		{GUID: "leaf", Parent: "mid"},
	}

	tests := []struct {
		name   string
		guid   string
		parent string
		want   bool
	}{
		{"NoCycle_LeafParentsRoot", "new", "root", false},
		{"NoCycle_UnrelatedParent", "new", "leaf", false},
		{"Cycle_ParentIsGuidItself", "mid", "mid", true},
		{"Cycle_ParentChainWalksBackToGuid", "root", "leaf", true},
		{"NoCycle_EmptyParent", "new", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wouldFormCycle(strands, tt.guid, tt.parent); got != tt.want {
				t.Errorf("wouldFormCycle(strands, %q, %q) = %v, want %v", tt.guid, tt.parent, got, tt.want)
			}
		})
	}
}

func TestNeedsLaunchOnAdd(t *testing.T) {
	tests := []struct {
		name   string
		anchor render.Anchor
		want   bool
	}{
		{"Hidden_NoLaunch", render.AnchorHidden, false},
		{"BelowParent_Launches", render.AnchorBelowParent, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsLaunchOnAdd(render.Display{Anchor: tt.anchor})
			if got != tt.want {
				t.Errorf("needsLaunchOnAdd(anchor=%v) = %v, want %v", tt.anchor, got, tt.want)
			}
		})
	}
}

func TestNeedsLaunchOnSurface(t *testing.T) {
	tests := []struct {
		name      string
		wasHidden bool
		anchor    render.Anchor
		want      bool
	}{
		{"HiddenToVisible_Surfaces", true, render.AnchorBelowParent, true},
		{"HiddenToHidden_NoOpNotASurface", true, render.AnchorHidden, false},
		{"VisibleToVisible_NotASurface", false, render.AnchorBelowParent, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsLaunchOnSurface(tt.wasHidden, render.Display{Anchor: tt.anchor})
			if got != tt.want {
				t.Errorf("needsLaunchOnSurface(%v, anchor=%v) = %v, want %v", tt.wasHidden, tt.anchor, got, tt.want)
			}
		})
	}
}

func TestUpdateStrandLocked_VisibleToHiddenRejected(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{
		{GUID: "g1", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
	}}

	_, err := e.updateStrandLocked(st, "g1", render.Display{Anchor: render.AnchorHidden})
	if err == nil {
		t.Fatal("updateStrandLocked(visible->hidden) = nil error, want error")
	}
	if st.Strands[0].Display.Anchor != render.AnchorBelowParent {
		t.Errorf("strand Display.Anchor = %v, want unchanged after a rejected update", st.Strands[0].Display.Anchor)
	}
}

func TestUpdateStrandLocked_HiddenToHidden_NoOpNoLaunch(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{
		{GUID: "g1", Display: render.Display{Anchor: render.AnchorHidden}, Cmd: "claude"},
	}}

	strand, err := e.updateStrandLocked(st, "g1", render.Display{Anchor: render.AnchorHidden, Focus: true})
	if err != nil {
		t.Fatalf("updateStrandLocked(hidden->hidden): %v", err)
	}
	if strand.PaneID != "" {
		t.Errorf("strand.PaneID = %q, want empty (still hidden, no launch)", strand.PaneID)
	}
}

func TestUpdateStrandLocked_UnknownGuidRejected(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}

	if _, err := e.updateStrandLocked(st, "does-not-exist", render.Display{}); err == nil {
		t.Fatal("updateStrandLocked(unknown guid) = nil error, want error")
	}
}

func TestRemoveStrandLocked_NonLeafWithoutRecursiveErrors(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{
		{GUID: "parent"},
		{GUID: "child", Parent: "parent"},
	}}

	_, _, err := e.removeStrandLocked(st, "parent", false)
	if err == nil {
		t.Fatal("removeStrandLocked(non-leaf, recursive=false) = nil error, want error")
	}
	if len(st.Strands) != 2 {
		t.Errorf("st.Strands = %+v, want unchanged after a rejected remove", st.Strands)
	}
}

func TestRemoveStrandLocked_RecursiveCascadesAndListsEveryRemoved(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{
		{GUID: "root", Name: "root-name"},
		{GUID: "mid", Name: "mid-name", Parent: "root"},
		{GUID: "leaf", Name: "leaf-name", Parent: "mid"},
		{GUID: "unrelated", Name: "unrelated-name"},
	}}

	removed, _, err := e.removeStrandLocked(st, "root", true)
	if err != nil {
		t.Fatalf("removeStrandLocked(recursive=true): %v", err)
	}

	wantGUIDs := map[string]string{"root": "root-name", "mid": "mid-name", "leaf": "leaf-name"}
	if len(removed.Strands) != len(wantGUIDs) {
		t.Fatalf("removed.Strands = %+v, want %d entries", removed.Strands, len(wantGUIDs))
	}
	for _, r := range removed.Strands {
		if wantGUIDs[r.GUID] != r.Name {
			t.Errorf("removed entry %+v does not match expected name %q", r, wantGUIDs[r.GUID])
		}
	}

	if len(st.Strands) != 1 || st.Strands[0].GUID != "unrelated" {
		t.Errorf("st.Strands after cascade = %+v, want only the unrelated strand left", st.Strands)
	}
}

func TestRemoveStrandLocked_UnknownGuidRejected(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}

	if _, _, err := e.removeStrandLocked(st, "does-not-exist", true); err == nil {
		t.Fatal("removeStrandLocked(unknown guid) = nil error, want error")
	}
}

// TestRemovalEmptiedSession pins the four-way classification removalEmptiedSession makes: the
// success-swallow in RemoveStrand may only fire when the session is confirmed gone AND no remaining
// strand is expected to still own a live pane (mirroring anyPlacedStrand's Anchor !=
// render.AnchorHidden filter).
func TestRemovalEmptiedSession(t *testing.T) {
	tests := []struct {
		name        string
		remaining   []Strand
		sessionGone bool
		want        bool
	}{
		{
			name:        "SessionGone_EmptyRemaining_True",
			remaining:   nil,
			sessionGone: true,
			want:        true,
		},
		{
			name: "SessionGone_AllRemainingHidden_True",
			remaining: []Strand{
				{GUID: "a", Display: render.Display{Anchor: render.AnchorHidden}},
				{GUID: "b", Display: render.Display{Anchor: render.AnchorHidden}},
			},
			sessionGone: true,
			want:        true,
		},
		{
			name: "SessionGone_OneRemainingNonHidden_False",
			remaining: []Strand{
				{GUID: "a", Display: render.Display{Anchor: render.AnchorHidden}},
				{GUID: "b", Display: render.Display{Anchor: render.AnchorBelowParent}},
			},
			sessionGone: true,
			want:        false,
		},
		{
			name: "SessionNotGone_AnyRemaining_False",
			remaining: []Strand{
				{GUID: "a", Display: render.Display{Anchor: render.AnchorHidden}},
			},
			sessionGone: false,
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removalEmptiedSession(tt.remaining, tt.sessionGone); got != tt.want {
				t.Errorf("removalEmptiedSession(%+v, sessionGone=%v) = %v, want %v", tt.remaining, tt.sessionGone, got, tt.want)
			}
		})
	}
}

// TestClassifyIfAbsent drives every one of the four branch rows plus the extra cases the discussion's
// Testing section enumerates: an empty-PaneID candidate, a candidate bound to a pane present but not
// alive, and a hidden strand sharing a name with a not-alive visible one.
func TestClassifyIfAbsent(t *testing.T) {
	visible := render.Display{Anchor: render.AnchorBelowParent}
	hidden := render.Display{Anchor: render.AnchorHidden}

	tests := []struct {
		name       string
		strands    []Strand
		targetName string
		aliveIDs   map[string]bool
		wantDec    ifAbsentDecision
		wantIdx    int
	}{
		{
			name:       "NoMatch_Add",
			strands:    []Strand{{GUID: "a", Name: "other", Display: visible}},
			targetName: "claude",
			wantDec:    ifAbsentAdd,
			wantIdx:    -1,
		},
		{
			name:       "MatchedAlive_NoOp",
			strands:    []Strand{{GUID: "a", Name: "claude", PaneID: "%1", Display: visible}},
			targetName: "claude",
			aliveIDs:   map[string]bool{"%1": true},
			wantDec:    ifAbsentNoOpAlive,
			wantIdx:    0,
		},
		{
			name:       "MatchedNotAlive_Relaunch",
			strands:    []Strand{{GUID: "a", Name: "claude", PaneID: "%1", Display: visible}},
			targetName: "claude",
			aliveIDs:   map[string]bool{},
			wantDec:    ifAbsentRelaunch,
			wantIdx:    0,
		},
		{
			name:       "MatchedHiddenOnly_NoOpHidden",
			strands:    []Strand{{GUID: "a", Name: "claude", Display: hidden}},
			targetName: "claude",
			aliveIDs:   map[string]bool{},
			wantDec:    ifAbsentNoOpHidden,
			wantIdx:    0,
		},
		{
			name:       "EmptyPaneID_NotAlive_Relaunch",
			strands:    []Strand{{GUID: "a", Name: "claude", PaneID: "", Display: visible}},
			targetName: "claude",
			aliveIDs:   map[string]bool{"": true},
			wantDec:    ifAbsentRelaunch,
			wantIdx:    0,
		},
		{
			name:       "PanePresentButNotInAliveSet_Relaunch",
			strands:    []Strand{{GUID: "a", Name: "claude", PaneID: "%1", Display: visible}},
			targetName: "claude",
			aliveIDs:   map[string]bool{"%1": false},
			wantDec:    ifAbsentRelaunch,
			wantIdx:    0,
		},
		{
			name: "TwoCandidates_SecondAlive_SelectsSecond",
			strands: []Strand{
				{GUID: "a", Name: "claude", PaneID: "%1", Display: visible},
				{GUID: "b", Name: "claude", PaneID: "%2", Display: visible},
			},
			targetName: "claude",
			aliveIDs:   map[string]bool{"%2": true},
			wantDec:    ifAbsentNoOpAlive,
			wantIdx:    1,
		},
		{
			name: "TwoCandidates_NeitherAlive_SelectsFirst",
			strands: []Strand{
				{GUID: "a", Name: "claude", PaneID: "%1", Display: visible},
				{GUID: "b", Name: "claude", PaneID: "%2", Display: visible},
			},
			targetName: "claude",
			aliveIDs:   map[string]bool{},
			wantDec:    ifAbsentRelaunch,
			wantIdx:    0,
		},
		{
			name: "HiddenSharesNameWithNotAliveVisible_RelaunchAgainstVisible",
			strands: []Strand{
				{GUID: "a", Name: "claude", Display: hidden},
				{GUID: "b", Name: "claude", PaneID: "%1", Display: visible},
			},
			targetName: "claude",
			aliveIDs:   map[string]bool{},
			wantDec:    ifAbsentRelaunch,
			wantIdx:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDec, gotIdx := classifyIfAbsent(tt.strands, tt.targetName, tt.aliveIDs)
			if gotDec != tt.wantDec || gotIdx != tt.wantIdx {
				t.Errorf("classifyIfAbsent() = (%v, %d), want (%v, %d)", gotDec, gotIdx, tt.wantDec, tt.wantIdx)
			}
		})
	}
}

// TestValidateIfAbsent pins the one requirement --if-absent adds: rejected (naming --name) whenever
// IfAbsent is true and NameOverride is empty, and accepted otherwise — including the ordinary
// IfAbsent-false case with no name at all.
func TestValidateIfAbsent(t *testing.T) {
	tests := []struct {
		name    string
		spec    AddSpec
		wantErr bool
	}{
		{"IfAbsentTrue_NoName_Rejected", AddSpec{IfAbsent: true}, true},
		{"IfAbsentTrue_WithName_Accepted", AddSpec{IfAbsent: true, NameOverride: "claude"}, false},
		{"IfAbsentFalse_NoName_Accepted", AddSpec{}, false},
		{"IfAbsentFalse_WithName_Accepted", AddSpec{NameOverride: "claude"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIfAbsent(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateIfAbsent(%+v) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "--name") {
				t.Errorf("validateIfAbsent(%+v) error = %v, want it to name the --name requirement", tt.spec, err)
			}
		})
	}
}

// addIfAbsentHook builds an execHook answering the tmux round trips AddStrand's --if-absent path
// makes before it ever reaches a no-op return: has-session (the session is up), display-message (a
// stable pane generation, so loadOrInitStateLocked's adoptPaneGenerationLocked stamp check never
// clears the fixture's bindings), and list-panes (paneLines, the alive-pane snapshot classifyIfAbsent
// decides against).
func addIfAbsentHook(paneLines string) func(capture bool, args ...string) (string, error) {
	return func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", nil
		case "display-message":
			return "$0|4321|1787000000", nil
		case "list-panes":
			return paneLines, nil
		default:
			return "", nil
		}
	}
}

// TestAddStrand_IfAbsent_MatchedAliveNoOps pins the alive no-op branch at the engine-call level:
// AddStrand must return the matched strand unchanged and persist nothing, even though the incoming
// spec carries a Focus:true Display and different Cmd/ResumeCmd/Parent than what is persisted.
func TestAddStrand_IfAbsent_MatchedAliveNoOps(t *testing.T) {
	e := newTestEngine(t)
	e.tmux.execHook = addIfAbsentHook("%1 0 0 100 20 4321\n")

	persisted := Strand{
		GUID: "persisted-guid", Name: "claude", PaneID: "%1",
		Cmd: "old-cmd", ResumeCmd: "old-resume", Parent: "old-parent",
		Display: render.Display{Anchor: render.AnchorBelowParent, Focus: false},
	}
	if err := SaveState(e.stateDir(), &ReedState{Strands: []Strand{persisted}}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := e.AddStrand(AddSpec{
		IfAbsent: true, NameOverride: "claude",
		Cmd: "new-cmd", ResumeCmd: "new-resume", Parent: "new-parent",
		Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true},
	})
	if err != nil {
		t.Fatalf("AddStrand(--if-absent, alive match): %v", err)
	}
	if got != persisted {
		t.Errorf("AddStrand(--if-absent, alive match) = %+v, want unchanged persisted strand %+v", got, persisted)
	}

	loaded, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded.Strands) != 1 || loaded.Strands[0] != persisted {
		t.Errorf("persisted state after alive no-op = %+v, want unchanged single strand %+v", loaded.Strands, persisted)
	}
}

// TestAddStrand_IfAbsent_HiddenOnlyNoOps mirrors the alive no-op for the hidden-only branch: the same
// four fields stay unchanged, plus the strand count is unchanged (nothing was added) and the returned
// strand is the hidden one, carrying a non-empty GUID and Name.
func TestAddStrand_IfAbsent_HiddenOnlyNoOps(t *testing.T) {
	e := newTestEngine(t)
	e.tmux.execHook = addIfAbsentHook("")

	persisted := Strand{
		GUID: "hidden-guid", Name: "claude",
		Cmd: "old-cmd", ResumeCmd: "old-resume", Parent: "old-parent",
		Display: render.Display{Anchor: render.AnchorHidden},
	}
	if err := SaveState(e.stateDir(), &ReedState{Strands: []Strand{persisted}}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got, err := e.AddStrand(AddSpec{
		IfAbsent: true, NameOverride: "claude",
		Cmd: "new-cmd", ResumeCmd: "new-resume", Parent: "new-parent",
		Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true},
	})
	if err != nil {
		t.Fatalf("AddStrand(--if-absent, hidden match): %v", err)
	}
	if got.GUID == "" || got.Name == "" {
		t.Fatalf("AddStrand(--if-absent, hidden match) = %+v, want non-empty GUID and Name", got)
	}
	if got != persisted {
		t.Errorf("AddStrand(--if-absent, hidden match) = %+v, want unchanged persisted strand %+v", got, persisted)
	}

	loaded, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(loaded.Strands) != 1 {
		t.Errorf("strand count after hidden no-op = %d, want 1 (nothing added)", len(loaded.Strands))
	}
	if loaded.Strands[0] != persisted {
		t.Errorf("persisted state after hidden no-op = %+v, want unchanged single strand %+v", loaded.Strands, persisted)
	}
}

func TestResolveStrandName(t *testing.T) {
	const tpl = "<ROLE>:<ROUND>:<SHORT_GUID>"
	guid := "abc1234500000000000000000000000"

	tests := []struct {
		name string
		spec AddSpec
		want string
	}{
		{"NameOverrideWinsVerbatim", AddSpec{NameOverride: "custom-name", Role: "main"}, "custom-name"},
		{"RoleFillsTemplate", AddSpec{Role: "main", Round: "1"}, "main:1:abc12345"},
		{"NeitherNameNorRole_BareShortGuid", AddSpec{}, "abc12345"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStrandName(tpl, tt.spec, guid, `C:\Code\loomyard\wts\internal-reed`)
			if got != tt.want {
				t.Errorf("resolveStrandName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAddStrandLocked_AnchorValidatedAtEngineBoundary pins the engine-API guard the CLI cannot
// provide: an in-process caller (shuttle) passing the deferred own-window anchor or a mistyped
// anchor must be rejected BEFORE any pane is launched or record registered — without this, the
// strand would persist, its pane would launch, and every subsequent apply would fail in render
// until the strand was removed.
func TestAddStrandLocked_AnchorValidatedAtEngineBoundary(t *testing.T) {
	e := newTestEngine(t)

	for _, anchor := range []render.Anchor{render.AnchorOwnWindow, render.Anchor("sideways"), render.Anchor("")} {
		st := &ReedState{}
		_, err := e.addStrandLocked(st, AddSpec{Cmd: "x", Display: render.Display{Anchor: anchor}})
		if err == nil {
			t.Fatalf("addStrandLocked(anchor=%q) = nil error, want rejection", anchor)
		}
		if len(st.Strands) != 0 {
			t.Errorf("anchor %q: st.Strands = %+v, want no record registered on a rejected add", anchor, st.Strands)
		}
	}
}

// TestUpdateStrandLocked_AnchorValidatedAtEngineBoundary mirrors the add guard for UpdateStrand:
// flipping a live strand's anchor to own-window (or garbage) must be rejected with the strand's
// display unchanged — a persisted own-window display would poison every later apply.
func TestUpdateStrandLocked_AnchorValidatedAtEngineBoundary(t *testing.T) {
	e := newTestEngine(t)

	for _, anchor := range []render.Anchor{render.AnchorOwnWindow, render.Anchor("sideways")} {
		st := &ReedState{Strands: []Strand{
			{GUID: "g1", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		}}
		_, err := e.updateStrandLocked(st, "g1", render.Display{Anchor: anchor})
		if err == nil {
			t.Fatalf("updateStrandLocked(anchor=%q) = nil error, want rejection", anchor)
		}
		if st.Strands[0].Display.Anchor != render.AnchorBelowParent {
			t.Errorf("anchor %q: strand Display.Anchor = %v, want unchanged after a rejected update", anchor, st.Strands[0].Display.Anchor)
		}
	}
}

// TestAlivePanePIDs pins RemoveStrand's reap-root selection: only panes that are being removed AND
// are present AND not dead contribute their pane pid — a dead pane's recorded pid may already have
// been reused by an unrelated process, so it must never seed the descendant closure the reap
// force-kills.
func TestAlivePanePIDs(t *testing.T) {
	live := []LivePane{
		{ID: "%1", Dead: false, PID: 100},
		{ID: "%2", Dead: true, PID: 200},
		{ID: "%3", Dead: false, PID: 300},
		{ID: "%4", Dead: false, PID: 0},
	}

	got := alivePanePIDs([]string{"%1", "%2", "%4", "%9"}, live)
	if len(got) != 1 || got[0] != 100 {
		t.Fatalf("alivePanePIDs = %v, want [100] (alive+requested only; dead %%2 excluded, pid-less %%4 excluded, absent %%9 excluded)", got)
	}

	if got := alivePanePIDs(nil, live); got != nil {
		t.Errorf("alivePanePIDs(no panes) = %v, want nil", got)
	}
}

// TestSessionReapRoots is the regression guard for the R2 review's R2-F2: Down snapshotted its
// descendant-closure roots from EVERY pane the session listed, dead ones included, while
// RemoveStrand correctly filtered to alive panes only. tmux keeps reporting a dead pane's recorded
// #{pane_pid} indefinitely (remain-on-exit is on for every reed session, and reconcile deliberately
// KEEPS the last dead pane and any dead Selvage corpse), so once the OS recycled that pid, Down would
// expand an unrelated process's whole subtree, block on it for the full reapExitTimeout, and then
// SIGKILL it.
// The dead-pane row is the assertion that matters; the pid-less row pins that the two forms share
// one predicate rather than each re-deriving it.
func TestSessionReapRoots(t *testing.T) {
	tests := []struct {
		name string
		live []LivePane
		want []int
	}{
		{
			name: "dead pane's recorded pid is never a reap root",
			live: []LivePane{
				{ID: "%1", Dead: false, PID: 100},
				{ID: "%2", Dead: true, PID: 200},
				{ID: "%3", Dead: false, PID: 300},
			},
			want: []int{100, 300},
		},
		{
			name: "pid-less pane contributes nothing",
			live: []LivePane{{ID: "%1", Dead: false, PID: 0}},
			want: nil,
		},
		{
			name: "every pane dead (the kept-corpse session shape)",
			live: []LivePane{
				{ID: "%1", Dead: true, PID: 100},
				{ID: "%2", Dead: true, PID: 200},
			},
			want: nil,
		},
		{
			name: "no panes at all",
			live: nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionReapRoots(tt.live)
			if !slices.Equal(got, tt.want) {
				t.Errorf("sessionReapRoots(%v) = %v; want %v", tt.live, got, tt.want)
			}
		})
	}
}

// TestPaneIDsInSession is the regression guard for the R5 review's R5-F4: RemoveStrand spent every
// persisted pane id as a kill-pane target with no check that it belonged to this worktree's
// session. The -L socket is per hub and tmux pane ids are server-global, so a stale or copied
// reed.json routinely carries a valid, addressable id belonging to a SIBLING worktree's live
// session — reproduced live, one worktree's remove killed another's strand pane and its process
// while reporting ok:true.
func TestPaneIDsInSession(t *testing.T) {
	live := []LivePane{
		{ID: "%1", Dead: false},
		{ID: "%2", Dead: true},
	}

	tests := []struct {
		name    string
		paneIDs []string
		want    []string
	}{
		{
			name:    "every id is a pane of this session",
			paneIDs: []string{"%1", "%2"},
			want:    []string{"%1", "%2"},
		},
		{
			name:    "a sibling worktree's live pane id is dropped",
			paneIDs: []string{"%1", "%7"},
			want:    []string{"%1"},
		},
		{
			name:    "a dead-but-present pane is kept, since membership and not aliveness is the filter",
			paneIDs: []string{"%2"},
			want:    []string{"%2"},
		},
		{
			name:    "no id belongs to this session",
			paneIDs: []string{"%7", "%8"},
			want:    []string{},
		},
		{
			name:    "an empty request stays empty",
			paneIDs: nil,
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paneIDsInSession(tt.paneIDs, live)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("paneIDsInSession(%v, live) = %v; want %v", tt.paneIDs, got, tt.want)
			}
		})
	}
}

// TestResolvePaneInThisSessionLocked is R5-F4's guard for the transport half: SendText, SendKey and
// CapturePane share this resolution, and a pane id belonging to another worktree's session on the
// shared socket is a target send-keys accepts — one agent's input typed into another agent's pane.
func TestResolvePaneInThisSessionLocked(t *testing.T) {
	st := &ReedState{Strands: []Strand{{GUID: "a", PaneID: "%7"}}}

	tests := []struct {
		name            string
		listPanesOut    string
		wantErrFragment string
	}{
		{
			name:         "a pane of this session resolves",
			listPanesOut: "%7 0 0 100 20 4321\n",
		},
		{
			name:            "a pane belonging to another session on the shared socket is refused",
			listPanesOut:    "%1 0 0 100 20 4321\n",
			wantErrFragment: "not a pane of this worktree's session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] == "list-panes" {
					return tt.listPanesOut, nil
				}
				return "", nil
			}

			paneID, err := e.resolvePaneInThisSessionLocked(st, "a")
			if tt.wantErrFragment == "" {
				if err != nil {
					t.Fatalf("resolvePaneInThisSessionLocked() error = %v; want nil", err)
				}
				if paneID != "%7" {
					t.Errorf("resolvePaneInThisSessionLocked() = %q; want %q", paneID, "%7")
				}
				return
			}
			if err == nil {
				t.Fatalf("resolvePaneInThisSessionLocked() = (%q, nil); want a refusal", paneID)
			}
			if !strings.Contains(err.Error(), tt.wantErrFragment) {
				t.Errorf("resolvePaneInThisSessionLocked() error = %v; want it to contain %q", err, tt.wantErrFragment)
			}
		})
	}
}

// TestRemoveStrand_NeverKillsAPaneOutsideThisSession pins R5-F4 at the CALL SITE rather than at the
// helper: paneIDsInSession is only worth having if RemoveStrand's kill-pane loop actually consults
// it, and the destructive shape the R5 review reproduced live was a kill-pane issued against a
// sibling worktree's live pane.
// It drives the whole op through TmuxCmd's execHook seam, recording every kill-pane target.
func TestRemoveStrand_NeverKillsAPaneOutsideThisSession(t *testing.T) {
	e := newTestEngine(t)

	// Only %1 is a pane of this session; %7 is a sibling worktree's live pane on the shared socket,
	// which is what a stale or hand-copied reed.json records.
	const thisSessionPane = "%1"
	const siblingPane = "%7"

	var killed []string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		switch args[0] {
		case "has-session":
			return "", nil
		case "display-message":
			return "$0|4321|1787000000", nil
		case "list-panes":
			return thisSessionPane + " 0 0 100 20 4321\n", nil
		case "kill-pane":
			killed = append(killed, args[len(args)-1])
			return "", nil
		default:
			return "", nil
		}
	}

	st := &ReedState{
		SelvagePaneID: thisSessionPane,
		Strands:       []Strand{{GUID: "copied", Name: "copied", PaneID: siblingPane, Display: render.Display{Anchor: render.AnchorBelowParent}}},
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if _, err := e.RemoveStrand("copied", false); err != nil {
		t.Fatalf("RemoveStrand: %v", err)
	}

	for _, id := range killed {
		if id == siblingPane {
			t.Fatalf("RemoveStrand issued kill-pane against %s, a pane outside this worktree's session; killed=%v", siblingPane, killed)
		}
	}
}
