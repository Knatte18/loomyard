// strand_test.go drives the strand-mutation *Locked helpers directly
// against a fixture .lyx: guid generation/uniqueness, unknown/cyclic parent
// rejection, the hidden-add no-launch path, the launch-path decision seam
// (needsLaunchOnAdd/needsLaunchOnSurface — the actual real-psmux launch
// itself is out of hermetic reach, see spawn_test.go), UpdateStrand's
// visible->hidden rejection, and RemoveStrand's non-leaf guard/cascade.
// None of these touch psmux: addStrandLocked/updateStrandLocked only reach
// psmux through launchStrandLocked, and every case here either stays
// hidden or is a rejection that never gets there; removeStrandLocked never
// touches psmux at all.

package muxengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

func TestAddStrandLocked_HiddenAdd_GuidUniqueRecordStoredNoLaunch(t *testing.T) {
	e := newTestEngine(t)
	st := &MuxState{}

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

// TestAddStrandLocked_SessionIDRoundTripsThroughSaveLoad pins AddSpec.SessionID
// as opaque caller metadata: addStrandLocked stamps it verbatim into the
// appended Strand, and it survives a SaveState/LoadState round trip on disk
// exactly like every other carrier field (Cmd, ResumeCmd, Name).
func TestAddStrandLocked_SessionIDRoundTripsThroughSaveLoad(t *testing.T) {
	e := newTestEngine(t)
	st := &MuxState{}

	spec := AddSpec{SessionID: "caller-session-abc", Display: render.Display{Anchor: render.AnchorHidden}}
	strand, err := e.addStrandLocked(st, spec)
	if err != nil {
		t.Fatalf("addStrandLocked: %v", err)
	}
	if strand.SessionID != spec.SessionID {
		t.Fatalf("strand.SessionID = %q, want %q", strand.SessionID, spec.SessionID)
	}

	dotLyxDir := e.layout.DotLyxDir()
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
	st := &MuxState{}

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
	st := &MuxState{Strands: []Strand{{GUID: "parent-guid", Display: render.Display{Anchor: render.AnchorHidden}}}}

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
		{"Top_Launches", render.AnchorTop, true},
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
	st := &MuxState{Strands: []Strand{
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
	st := &MuxState{Strands: []Strand{
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
	st := &MuxState{}

	if _, err := e.updateStrandLocked(st, "does-not-exist", render.Display{}); err == nil {
		t.Fatal("updateStrandLocked(unknown guid) = nil error, want error")
	}
}

func TestRemoveStrandLocked_NonLeafWithoutRecursiveErrors(t *testing.T) {
	e := newTestEngine(t)
	st := &MuxState{Strands: []Strand{
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
	st := &MuxState{Strands: []Strand{
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
	st := &MuxState{}

	if _, _, err := e.removeStrandLocked(st, "does-not-exist", true); err == nil {
		t.Fatal("removeStrandLocked(unknown guid) = nil error, want error")
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
			got := resolveStrandName(tpl, tt.spec, guid, `C:\Code\loomyard\wts\internal-mux`)
			if got != tt.want {
				t.Errorf("resolveStrandName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAddStrandLocked_AnchorValidatedAtEngineBoundary pins the engine-API
// guard the CLI cannot provide: an in-process caller (shuttle) passing the
// deferred own-window anchor or a mistyped anchor must be rejected BEFORE
// any pane is launched or record registered — without this, the strand
// would persist, its pane would launch, and every subsequent apply would
// fail in render until the strand was removed.
func TestAddStrandLocked_AnchorValidatedAtEngineBoundary(t *testing.T) {
	e := newTestEngine(t)

	for _, anchor := range []render.Anchor{render.AnchorOwnWindow, render.Anchor("sideways"), render.Anchor("")} {
		st := &MuxState{}
		_, err := e.addStrandLocked(st, AddSpec{Cmd: "x", Display: render.Display{Anchor: anchor}})
		if err == nil {
			t.Fatalf("addStrandLocked(anchor=%q) = nil error, want rejection", anchor)
		}
		if len(st.Strands) != 0 {
			t.Errorf("anchor %q: st.Strands = %+v, want no record registered on a rejected add", anchor, st.Strands)
		}
	}
}

// TestUpdateStrandLocked_AnchorValidatedAtEngineBoundary mirrors the add
// guard for UpdateStrand: flipping a live strand's anchor to own-window (or
// garbage) must be rejected with the strand's display unchanged — a
// persisted own-window display would poison every later apply.
func TestUpdateStrandLocked_AnchorValidatedAtEngineBoundary(t *testing.T) {
	e := newTestEngine(t)

	for _, anchor := range []render.Anchor{render.AnchorOwnWindow, render.Anchor("sideways")} {
		st := &MuxState{Strands: []Strand{
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

// TestAlivePanePIDs pins RemoveStrand's reap-root selection: only panes that
// are being removed AND are present AND not dead contribute their pane pid —
// a dead pane's recorded pid may already have been reused by an unrelated
// process, so it must never seed the descendant closure the reap force-kills.
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
