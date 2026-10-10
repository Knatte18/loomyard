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
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// TestAddStrandLocked pins the hidden-add path and the engine-boundary rejections of one fresh state.
// Hidden adds mint unique 32-hex guids, store the record without launching (no PaneID) and keep Cmd verbatim though unrun;
// AddSpec.SessionID is opaque caller metadata, stamped verbatim and surviving a SaveState/LoadState round trip like every other carrier field (Cmd, ResumeCmd, Name);
// a known parent is accepted.
// An unknown parent, the deferred own-window anchor, a mistyped anchor or an empty one is rejected before any pane is launched or record registered —
// without that guard an in-process caller (shuttle) would persist the strand, launch its pane, and fail every subsequent apply in render until the strand was removed.
func TestAddStrandLocked(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}
	hidden := render.Display{Anchor: render.AnchorHidden}
	spec := AddSpec{Cmd: "claude --session-id abc", SessionID: "caller-session-abc", Display: hidden}

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
	for _, s := range st.Strands {
		if s.PaneID != "" {
			t.Errorf("hidden-add strand %q PaneID = %q, want empty (launchStrandLocked must not run)", s.GUID, s.PaneID)
		}
		if s.Cmd != spec.Cmd {
			t.Errorf("hidden-add strand %q Cmd = %q, want %q stored verbatim though unrun", s.GUID, s.Cmd, spec.Cmd)
		}
		if s.SessionID != spec.SessionID {
			t.Errorf("hidden-add strand %q SessionID = %q, want %q", s.GUID, s.SessionID, spec.SessionID)
		}
	}
	if err := SaveState(e.stateDir(), st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	loaded, err := LoadState(e.stateDir())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got, ok := strandByGUID(loaded.Strands, first.GUID); !ok || got.SessionID != spec.SessionID {
		t.Errorf("loaded strand = %+v (found %v), want SessionID %q to survive SaveState/LoadState", got, ok, spec.SessionID)
	}

	child, err := e.addStrandLocked(st, AddSpec{Parent: first.GUID, Display: hidden})
	if err != nil {
		t.Fatalf("addStrandLocked(known parent): %v", err)
	}
	if child.Parent != first.GUID {
		t.Errorf("strand.Parent = %q, want %q", child.Parent, first.GUID)
	}

	rejected := []struct {
		name string
		spec AddSpec
	}{
		{"UnknownParent", AddSpec{Parent: "does-not-exist", Display: hidden}},
		{"OwnWindowAnchor", AddSpec{Cmd: "x", Display: render.Display{Anchor: render.AnchorOwnWindow}}},
		{"MistypedAnchor", AddSpec{Cmd: "x", Display: render.Display{Anchor: render.Anchor("sideways")}}},
		{"EmptyAnchor", AddSpec{Cmd: "x", Display: render.Display{Anchor: render.Anchor("")}}},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			before := len(st.Strands)
			if _, err := e.addStrandLocked(st, tt.spec); err == nil {
				t.Fatal("addStrandLocked = nil error, want rejection")
			}
			if len(st.Strands) != before {
				t.Errorf("st.Strands = %+v, want no record registered on a rejected add", st.Strands)
			}
		})
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

//testtiming:keep pins the add-time launch decision: a hidden add never launches and a below-parent add does; its covering tests run this code without asserting it
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

//testtiming:keep pins the surface-time launch decision: only a hidden strand turned visible launches, a hidden-to-hidden or visible-to-visible update never does; its covering tests run this code without asserting it
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

// TestUpdateStrandLocked pins UpdateStrand's engine-boundary rules:
// a visible->hidden update, the deferred own-window anchor, a mistyped anchor and an unknown guid are rejected with the strand's display unchanged
// (a persisted own-window display would poison every later apply),
// while a hidden->hidden update is a no-op that launches nothing.
func TestUpdateStrandLocked(t *testing.T) {
	visible := render.Display{Anchor: render.AnchorBelowParent}
	hidden := render.Display{Anchor: render.AnchorHidden}
	tests := []struct {
		name    string
		strand  Strand
		guid    string
		display render.Display
		wantErr bool
	}{
		{"VisibleToHiddenRejected", Strand{GUID: "g1", PaneID: "%1", Display: visible}, "g1", hidden, true},
		{"OwnWindowAnchorRejected", Strand{GUID: "g1", PaneID: "%1", Display: visible}, "g1", render.Display{Anchor: render.AnchorOwnWindow}, true},
		{"MistypedAnchorRejected", Strand{GUID: "g1", PaneID: "%1", Display: visible}, "g1", render.Display{Anchor: render.Anchor("sideways")}, true},
		{"UnknownGuidRejected", Strand{GUID: "g1", PaneID: "%1", Display: visible}, "does-not-exist", render.Display{}, true},
		{"HiddenToHiddenIsANoOpWithNoLaunch", Strand{GUID: "g1", Display: hidden, Cmd: "claude"}, "g1", render.Display{Anchor: render.AnchorHidden, Focus: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			st := &ReedState{Strands: []Strand{tt.strand}}

			strand, err := e.updateStrandLocked(st, tt.guid, tt.display)

			if tt.wantErr {
				if err == nil {
					t.Fatal("updateStrandLocked = nil error, want rejection")
				}
				if st.Strands[0].Display.Anchor != tt.strand.Display.Anchor {
					t.Errorf("strand Display.Anchor = %v, want unchanged after a rejected update", st.Strands[0].Display.Anchor)
				}
				return
			}
			if err != nil {
				t.Fatalf("updateStrandLocked: %v", err)
			}
			if strand.PaneID != "" {
				t.Errorf("strand.PaneID = %q, want empty (still hidden, no launch)", strand.PaneID)
			}
		})
	}
}

// TestRemoveStrandLocked pins the non-leaf guard and the cascade on one table:
// a non-leaf removed without recursive is refused with the table unchanged,
// and the recursive remove cascades to every descendant, lists each removed strand with its name and leaves unrelated strands alone.
func TestRemoveStrandLocked(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{Strands: []Strand{
		{GUID: "root", Name: "root-name"},
		{GUID: "mid", Name: "mid-name", Parent: "root"},
		{GUID: "leaf", Name: "leaf-name", Parent: "mid"},
		{GUID: "unrelated", Name: "unrelated-name"},
	}}

	if _, _, err := e.removeStrandLocked(st, "root", false); err == nil {
		t.Fatal("removeStrandLocked(non-leaf, recursive=false) = nil error, want error")
	}
	if len(st.Strands) != 4 {
		t.Fatalf("st.Strands = %+v, want unchanged after a rejected remove", st.Strands)
	}

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
//
//testtiming:keep pins --if-absent requiring a name: rejected naming --name when IfAbsent is set without one, accepted otherwise; its covering tests run this code without asserting it
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

// installIfAbsentTmux installs a fakeTmux answering the tmux round trips AddStrand's --if-absent path
// makes before it ever reaches a no-op return: has-session (the session is up), display-message (a
// stable pane generation, so loadOrInitStateLocked's adoptPaneGenerationLocked stamp check never
// clears the fixture's bindings), and list-panes (paneLines, both the substrate snapshot
// ensureSessionLocked's sessionSubstrateLocked reads to decide the session is already usable and the
// alive-pane snapshot classifyIfAbsent decides against — the same list-panes call answers both, since
// both run against the identical fixture session). paneLines must therefore carry at least one pane
// line whenever a case wants AddStrand to see the session as already usable and skip the boot path,
// even when that pane is not any strand's own PaneID.
func installIfAbsentTmux(t *testing.T, e *Engine, paneLines string) *fakeTmux {
	t.Helper()
	fake := installFakeTmux(t, e)
	fake.answer("display-message", "$0|4321|1787000000", nil)
	fake.answer("list-panes", paneLines, nil)
	return fake
}

// TestAddStrand_IfAbsent_NoOps pins the no-op branches at the engine-call level:
// AddStrand must return the matched strand unchanged and persist nothing,
// even though the incoming spec carries a Focus:true Display and different Cmd/ResumeCmd/Parent than what is persisted.
// An alive match is a no-op, a hidden-only match is a no-op regardless of any pane being alive, and a role-segment --name matches on the full name it resolves to.
func TestAddStrand_IfAbsent_NoOps(t *testing.T) {
	tests := []struct {
		name      string
		paneLines string
		persisted Strand
	}{
		{
			name:      "MatchedAliveNoOps",
			paneLines: "%1 0 0 100 20 4321\n",
			persisted: Strand{
				GUID: "persisted-guid", Name: "tc:tslug:claude", PaneID: "%1",
				Cmd: "old-cmd", ResumeCmd: "old-resume", Parent: "old-parent",
				Display: render.Display{Anchor: render.AnchorBelowParent, Focus: false},
			},
		},
		{
			// A header-only pane line (no strand's own PaneID) so ensureSessionLocked's substrate probe finds
			// the session already usable and never boots: the hidden-only decision must not depend on any pane being alive.
			name:      "HiddenOnlyNoOps",
			paneLines: "%0 0 0 100 20 4321\n",
			persisted: Strand{
				GUID: "hidden-guid", Name: "tc:tslug:claude",
				Cmd: "old-cmd", ResumeCmd: "old-resume", Parent: "old-parent",
				Display: render.Display{Anchor: render.AnchorHidden},
			},
		},
		{
			name:      "RoleSegmentMatchesFullName",
			paneLines: "%1 0 0 100 20 4321\n",
			persisted: Strand{GUID: "persisted-guid", Name: "tc:tslug:claude", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			installIfAbsentTmux(t, e, tt.paneLines)
			if err := SaveState(e.stateDir(), &ReedState{Strands: []Strand{tt.persisted}}); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			got, err := e.AddStrand(AddSpec{
				IfAbsent: true, NameOverride: "claude",
				Cmd: "new-cmd", ResumeCmd: "new-resume", Parent: "new-parent",
				Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true},
			})
			if err != nil {
				t.Fatalf("AddStrand(--if-absent): %v", err)
			}

			if !reflect.DeepEqual(got, tt.persisted) {
				t.Errorf("AddStrand(--if-absent) = %+v, want unchanged persisted strand %+v", got, tt.persisted)
			}
			loaded, err := LoadState(e.stateDir())
			if err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			if len(loaded.Strands) != 1 || !reflect.DeepEqual(loaded.Strands[0], tt.persisted) {
				t.Errorf("persisted state after no-op = %+v, want unchanged single strand %+v", loaded.Strands, tt.persisted)
			}
		})
	}
}

// TestAddStrandUnless_NamedStrandSkips pins that a named strand skips the add whether it is live, dormant (its pane gone) or hidden, and that a skip saves, launches and moves nothing.
func TestAddStrandUnless_NamedStrandSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		before []Strand
		orch   Strand
	}{
		{"Live", nil, Strand{GUID: "orch-guid", Name: "tc:tslug:orch", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}},
		{"AfterAnotherStrand", []Strand{{GUID: "other-guid", Name: "tc:tslug:other", Display: render.Display{Anchor: render.AnchorBelowParent}}}, Strand{GUID: "orch-guid", Name: "tc:tslug:orch", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}},
		{"DormantPaneGone", nil, Strand{GUID: "orch-guid", Name: "tc:tslug:orch", PaneID: "%9", Display: render.Display{Anchor: render.AnchorBelowParent}}},
		{"Hidden", nil, Strand{GUID: "orch-guid", Name: "tc:tslug:orch", Display: render.Display{Anchor: render.AnchorHidden}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newTestEngine(t)
			fake := installIfAbsentTmux(t, e, "%1 0 0 100 20 4321\n")

			if err := SaveState(e.stateDir(), &ReedState{Strands: append(slices.Clone(tt.before), tt.orch)}); err != nil {
				t.Fatalf("SaveState: %v", err)
			}
			// The seeded state carries no socket, session or pane-generation stamp,
			// and every load stamps them in memory,
			// so a SaveState on the skip path would change these bytes.
			statePath := filepath.Join(e.stateDir(), reedStateFileName)
			stateBefore, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read state: %v", err)
			}

			got, skipped, err := e.AddStrandUnless(AddSpec{NameOverride: "claude", Display: render.Display{Anchor: render.AnchorBelowParent, Focus: true}}, "orch")
			if err != nil {
				t.Fatalf("AddStrandUnless: %v", err)
			}
			if !skipped || !reflect.DeepEqual(got, tt.orch) {
				t.Errorf("AddStrandUnless = (%+v, %v), want (%+v, true)", got, skipped, tt.orch)
			}
			for _, c := range fake.Sequence() {
				if c == "split-window" || c == "select-layout" || c == "select-pane" || c == "kill-pane" {
					t.Errorf("tmux %s issued by a skipped add", c)
				}
			}
			stateAfter, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read state: %v", err)
			}
			if string(stateAfter) != string(stateBefore) {
				t.Errorf("persisted state after skip = %s, want unchanged %s", stateAfter, stateBefore)
			}
		})
	}
}

//testtiming:keep pins the add going ahead when no strand carries the named name, with the new strand named from the told geometry and persisted after the existing ones; its covering tests run this code without asserting it
func TestAddStrandUnless_NoNamedStrandAdds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		persisted []Strand
	}{
		{"OtherStrandOnly", []Strand{{GUID: "other-guid", Name: "tc:tslug:other", PaneID: "%1", Display: render.Display{Anchor: render.AnchorBelowParent}}}},
		{"NoOrch", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newTestEngine(t)
			installIfAbsentTmux(t, e, "%1 0 0 100 20 4321\n")
			if err := SaveState(e.stateDir(), &ReedState{Strands: tt.persisted}); err != nil {
				t.Fatalf("SaveState: %v", err)
			}

			got, skipped, err := e.AddStrandUnless(AddSpec{NameOverride: "claude", Segment: "review", Display: render.Display{Anchor: render.AnchorHidden}}, "orch")
			if err != nil {
				t.Fatalf("AddStrandUnless: %v", err)
			}
			if skipped {
				t.Error("AddStrandUnless skipped, want an add")
			}
			if got.Name != "tc:tslug:claude" {
				t.Errorf("added strand name = %q, want tc:tslug:claude", got.Name)
			}
			if got.Segment != "review" || got.Color != segmentcolor.Orange {
				t.Errorf("added strand segment/color = %q/%q, want review/orange", got.Segment, got.Color)
			}
			loaded, err := LoadState(e.stateDir())
			if err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			if saved, ok := strandByGUID(loaded.Strands, got.GUID); !ok || saved.Segment != "review" || saved.Color != "" {
				t.Errorf("persisted strand = %+v (found %v), want Segment review persisted and Color never persisted", saved, ok)
			}
			if len(loaded.Strands) != len(tt.persisted)+1 {
				t.Errorf("strand count = %d, want %d", len(loaded.Strands), len(tt.persisted)+1)
			}
		})
	}
}

func TestAddStrandUnless_UnformableNameRefusesBeforeTmux(t *testing.T) {
	e := newTestEngine(t)
	fake := installIfAbsentTmux(t, e, "%1 0 0 100 20 4321\n")

	if _, _, err := e.AddStrandUnless(AddSpec{Display: render.Display{Anchor: render.AnchorHidden}}, "Bad Name"); err == nil {
		t.Fatal("AddStrandUnless(unformable name) = nil error, want a refusal")
	}
	if cmds := fake.Sequence(); len(cmds) != 0 {
		t.Errorf("tmux commands issued before refusal: %v", cmds)
	}
}

// TestAddStrand_IfAbsentWithoutName_FailsBeforeAnyTmuxContact pins that validateIfAbsent's config
// rejection still precedes any tmux contact at all, now that AddStrand's pre-flight is
// ensureSessionLocked rather than requireSessionLocked: a --if-absent call with no name override must
// fail with validateIfAbsent's own error, not the nonexistent multiplexer binary's.
func TestAddStrand_IfAbsentWithoutName_FailsBeforeAnyTmuxContact(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.AddStrand(AddSpec{IfAbsent: true, Display: render.Display{Anchor: render.AnchorHidden}})
	if err == nil {
		t.Fatal("AddStrand(--if-absent, no name) = nil error, want validateIfAbsent's rejection")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("AddStrand(--if-absent, no name) error = %q, want it to name the --name requirement (validateIfAbsent must run before any tmux contact)", err)
	}
}

// TestAddStrand_ColdEngine_NoLongerReturnsNoSessionMessage pins the self-heal behaviour change at the
// engine-call level: AddStrand against a cold engine (no live session, nothing booted) no longer
// refuses with noSessionMessage's friendly text. The returned error IS still non-nil — the fixture's
// configured tmux/shell binaries do not exist on disk, so ensureSessionLocked's own
// sessionSubstrateLocked probe fails with that nonexistent binary's exec error, reached before
// anything else runs. Asserting that binary error's exact text would pin an OS-specific string, so
// this only asserts what the self-heal change actually promises: neither the no-session phrase nor the
// `lyx reed up` remedy noSessionMessage names appears in the error AddStrand now returns.
func TestAddStrand_ColdEngine_NoLongerReturnsNoSessionMessage(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.AddStrand(AddSpec{NameOverride: "claude", Display: render.Display{Anchor: render.AnchorHidden}})
	if err == nil {
		t.Fatal("AddStrand against a cold engine = nil error, want the nonexistent tmux binary's error")
	}
	if strings.Contains(err.Error(), "no reed session") {
		t.Errorf("AddStrand against a cold engine error = %q, want it to no longer carry noSessionMessage's no-session phrase", err)
	}
	if strings.Contains(err.Error(), "lyx reed up") {
		t.Errorf("AddStrand against a cold engine error = %q, want it to no longer name the `lyx reed up` remedy", err)
	}
}

// Deliberately NOT tested hermetically: a validation-ordering test shaped like
// TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact, pinning that a config error (e.g. a bad header
// template) fails AddStrand before any tmux contact. ensureSessionLocked's first act is
// sessionSubstrateLocked's own session probe, so — unlike Up, whose pre-tmux validation block runs
// ahead of any session check — AddStrand's cold path always contacts tmux first, and the config
// validation buried inside upLocked's delegate only runs, if at all, after that probe. That ordering
// is the point of the seam (a warm AddStrand must cost only the one extra probe round trip, never a
// config validation it did not already perform — Shared Decision "the warm path changes only by
// adding probe round trips"), not an ordering defect to fix. Cold-path validation ordering for
// AddStrand is covered by the smoke tier instead (batch 5).

// hiddenSpec is a hidden add, which never launches a pane, so addStrandLocked reaches no tmux.
func hiddenSpec(role, nameOverride string) AddSpec {
	return AddSpec{Role: role, NameOverride: nameOverride, Display: render.Display{Anchor: render.AnchorHidden}}
}

// TestStrandNameLocked_Names pins how an add forms its strand name:
// the role is numbered by how many strands already hold it, a dormant strand still holds its role,
// a legacy-shaped name holds nothing, an empty slug gives two segments and an empty role defaults to "strand".
//
//testtiming:keep pins how an add forms its strand name: the role numbered by the strands holding it, a dormant strand still counting, a legacy name holding nothing, an empty slug giving two segments and an empty role defaulting to strand; its covering tests run this code without asserting it
func TestStrandNameLocked_Names(t *testing.T) {
	tests := []struct {
		name      string
		existing  []Strand
		emptySlug bool
		roles     []string
		want      []string
	}{
		{
			name:  "FormsAndNumbersRoles",
			roles: []string{"worker", "worker", "reviewer"},
			want:  []string{"tc:tslug:worker", "tc:tslug:worker-2", "tc:tslug:reviewer"},
		},
		{
			name:     "DormantStrandStillCounts",
			existing: []Strand{{GUID: "g1", Name: "tc:tslug:worker", Display: render.Display{Anchor: render.AnchorHidden}}},
			roles:    []string{"worker"},
			want:     []string{"tc:tslug:worker-2"},
		},
		{
			name:     "LegacyNameHoldsNothing",
			existing: []Strand{{GUID: "g1", Name: "worker:1:abc12345"}},
			roles:    []string{"worker"},
			want:     []string{"tc:tslug:worker"},
		},
		{
			name:      "EmptySlugGivesTwoSegments",
			emptySlug: true,
			roles:     []string{"orch"},
			want:      []string{"tc:orch"},
		},
		{
			name:  "DefaultRole",
			roles: []string{"", ""},
			want:  []string{"tc:tslug:strand", "tc:tslug:strand-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			if tt.emptySlug {
				e.geom.NameSlug = ""
			}
			st := &ReedState{Strands: append([]Strand(nil), tt.existing...)}

			var got []string
			for _, role := range tt.roles {
				s, err := e.addStrandLocked(st, hiddenSpec(role, ""))
				if err != nil {
					t.Fatalf("addStrandLocked(role=%q): %v", role, err)
				}
				got = append(got, s.Name)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("names = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStrandNameLocked_ExplicitNameHeldRefuses(t *testing.T) {
	e := newTestEngine(t)
	st := &ReedState{}
	held, err := e.addStrandLocked(st, hiddenSpec("", "driver"))
	if err != nil {
		t.Fatalf("addStrandLocked(explicit driver): %v", err)
	}
	if held.Name != "tc:tslug:driver" {
		t.Fatalf("explicit role segment Name = %q, want tc:tslug:driver", held.Name)
	}

	for _, override := range []string{"driver", "tc:tslug:driver"} {
		_, err := e.addStrandLocked(st, hiddenSpec("", override))
		if err == nil {
			t.Fatalf("addStrandLocked(NameOverride=%q) = nil error, want the held-name refusal", override)
		}
		for _, want := range []string{held.GUID, "lyx reed remove --name driver frees it, or pass another role"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("NameOverride=%q error = %q, want it to contain %q", override, err, want)
			}
		}
	}
	if len(st.Strands) != 1 {
		t.Errorf("strands = %d, want 1 (a refused add registers nothing)", len(st.Strands))
	}
}

func TestStrandNameLocked_ForeignPrefixRefuses(t *testing.T) {
	e := newTestEngine(t)

	for _, override := range []string{"other:tslug:driver", "tc:otherslug:driver"} {
		if _, err := e.addStrandLocked(&ReedState{}, hiddenSpec("", override)); err == nil {
			t.Errorf("addStrandLocked(NameOverride=%q) = nil error, want a foreign-prefix refusal", override)
		}
	}
}

// TestAddStrand_UnformableName_RefusesBeforeAnyTmuxCommand pins both up-front refusals, for add and replace alike:
// the exact way-forward text, and no tmux command issued.
func TestAddStrand_UnformableName_RefusesBeforeAnyTmuxCommand(t *testing.T) {
	tests := []struct {
		name      string
		shortname string
		slug      string
		wantText  string
	}{
		{"MissingShortname", "", "tslug", "no strand name can be formed: this hub records no shortname; way forward: lyx fabric shortname <shortname> records it, then retry; a session lyx refuses the verb from reports status: FAILED and the orch runs it"},
		{"BadSlug", "tc", "Bad_Slug", "way forward: lyx fabric add <slug> creates the task under a slug that fits; a session lyx refuses the verb from reports status: FAILED and the orch runs it"},
		{"BadShortname", "T-C", "tslug", `"T-C"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.geom.NameShortname, e.geom.NameSlug = tt.shortname, tt.slug
			fake := installFakeTmux(t, e)

			spec := hiddenSpec("worker", "")
			_, addErr := e.AddStrand(spec)
			_, replaceErr := e.ReplaceStrand("any-guid", spec)
			for op, err := range map[string]error{"AddStrand": addErr, "ReplaceStrand": replaceErr} {
				if err == nil || !strings.Contains(err.Error(), tt.wantText) {
					t.Errorf("%s error = %v, want it to contain %q", op, err, tt.wantText)
				}
			}
			if calls := len(fake.Calls()); calls != 0 {
				t.Errorf("tmux commands issued = %d, want 0 (a refused call never boots tmux)", calls)
			}
		})
	}
}

// TestAlivePanePIDs pins RemoveStrand's reap-root selection: only panes that are being removed AND
// are present AND not dead contribute their pane pid — a dead pane's recorded pid may already have
// been reused by an unrelated process, so it must never seed the descendant closure the reap
// force-kills.
//
//testtiming:keep pins the reap-root selection: only a pane that is requested, present and alive contributes its pid, a dead, pid-less or absent one never does since a dead pane's recorded pid may have been reused; its covering tests run this code without asserting it
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
//
//testtiming:keep pins that Down's reap roots exclude a dead pane's recorded pid, a pid-less pane and a corpse-only session; its covering tests run this code without asserting it
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
//
//testtiming:keep pins the kill-pane targets being filtered to panes of this worktree's session, a dead but present pane kept and a sibling worktree's pane id dropped; its covering tests run this code without asserting it
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
			if !slices.Equal(got, tt.want) {
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
			installFakeTmux(t, e).answer("list-panes", tt.listPanesOut, nil)

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
// It drives the whole op through the fake tmux, recording every kill-pane target.
//
//testtiming:keep pins RemoveStrand's kill-pane loop consulting the session filter, so a stale reed.json naming a sibling worktree's pane kills nothing there; its covering tests run this code without asserting it
func TestRemoveStrand_NeverKillsAPaneOutsideThisSession(t *testing.T) {
	e := newTestEngine(t)

	// Only %1 is a pane of this session; %7 is a sibling worktree's live pane on the shared socket,
	// which is what a stale or hand-copied reed.json records.
	const thisSessionPane = "%1"
	const siblingPane = "%7"

	fake := installIfAbsentTmux(t, e, thisSessionPane+" 0 0 100 20 4321\n")

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

	var killed []string
	for _, argv := range fake.ArgvFor("kill-pane") {
		killed = append(killed, argv[len(argv)-1])
	}
	for _, id := range killed {
		if id == siblingPane {
			t.Fatalf("RemoveStrand issued kill-pane against %s, a pane outside this worktree's session; killed=%v", siblingPane, killed)
		}
	}
}
