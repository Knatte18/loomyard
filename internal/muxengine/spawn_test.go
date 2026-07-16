// spawn_test.go table-tests planPaneTarget's adopt-vs-split decision —
// including the corpse-pane rules tmux forces (never adopt a dead pane;
// split the tallest alive pane, or the kept corpse when nothing is alive)
// and the header-pane exclusion (never adopted, never the preferred split
// target, but the sole-pane fallback) — and verifies loadOrInitStateLocked's
// fresh-worktree bootstrap. Both are pure/hermetic, no live tmux required.
// launchStrandLocked itself always makes a real tmux round trip
// (list-panes/split-window + send-keys), so it is exercised only through
// this decision seam, not invoked directly here; the composed live behavior
// is covered by the smoke tests.

package muxengine

import "testing"

func TestPlanPaneTarget(t *testing.T) {
	tests := []struct {
		name            string
		strands         []Strand
		live            []LivePane
		headerPaneID    string
		wantAdoptID     string
		wantSplitTarget string
		wantErr         bool
	}{
		{
			name:        "FreshSession_AdoptsTheAliveInitialPane",
			strands:     nil,
			live:        []LivePane{{ID: "%1", Height: 50}},
			wantAdoptID: "%1",
		},
		{
			name:        "AllStrandsPaneless_AdoptsFirstAlivePane",
			strands:     []Strand{{GUID: "a"}, {GUID: "b"}},
			live:        []LivePane{{ID: "%1", Height: 50}},
			wantAdoptID: "%1",
		},
		{
			name: "SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse",
			// The remove-last-strand aftermath: kill-pane on a session's
			// sole pane corpses it (pane_dead=1, exit 0) instead of removing
			// it, and send-keys into a corpse is silently swallowed — so the
			// next add must split, not adopt, even though no strand holds a
			// binding.
			strands:         []Strand{{GUID: "a"}},
			live:            []LivePane{{ID: "%1", Dead: true, Height: 50}},
			wantSplitTarget: "%1",
		},
		{
			name:            "OneStrandHoldsAPane_SplitsTheTallestAlive",
			strands:         []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b"}},
			live:            []LivePane{{ID: "%1", Height: 2}, {ID: "%2", Height: 47}},
			wantSplitTarget: "%2",
		},
		{
			name: "TinyActiveBand_SplitTargetsTheTallestNotTheFirst",
			// The session-target split defect this planner replaces: tmux
			// splits the active pane, which select-layout can leave on a
			// 1-2 row band, and a too-small split fails silently. The
			// planner must always pick the tallest alive pane instead.
			strands:         []Strand{{GUID: "a", PaneID: "%1"}, {GUID: "b", PaneID: "%2"}},
			live:            []LivePane{{ID: "%1", Height: 2}, {ID: "%2", Height: 47}},
			wantSplitTarget: "%2",
		},
		{
			name:            "DeadPaneNeverTheSplitTargetWhileAnyAlive",
			strands:         []Strand{{GUID: "a", PaneID: "%1"}},
			live:            []LivePane{{ID: "%1", Dead: true, Height: 47}, {ID: "%2", Height: 2}},
			wantSplitTarget: "%2",
		},
		{
			name:    "NoPanesAtAll_Errors",
			strands: []Strand{{GUID: "a"}},
			live:    nil,
			wantErr: true,
		},
		{
			name: "HeaderPresentNoStrandBound_HeaderNeverAdopted",
			// A live header pane plus an alive non-header pane: adoption
			// must land on the non-header pane, never the header, even
			// though no strand holds a binding yet.
			strands:      nil,
			live:         []LivePane{{ID: "%header", Height: 1}, {ID: "%1", Height: 50}},
			headerPaneID: "%header",
			wantAdoptID:  "%1",
		},
		{
			name: "HeaderPresentWithStrand_HeaderNeverTheSplitTarget",
			// The header is tallest by raw Height here, but must still
			// never be chosen over a genuine (if shorter) non-header
			// candidate.
			strands:         []Strand{{GUID: "a", PaneID: "%1"}},
			live:            []LivePane{{ID: "%header", Height: 90}, {ID: "%1", Height: 10}},
			headerPaneID:    "%header",
			wantSplitTarget: "%1",
		},
		{
			name: "HeaderIsSolePane_SplitTargetFallsBackToHeader",
			// Every strand has been removed: only the header remains. The
			// header must become the split target so a subsequent add still
			// has something to split (the header survives the split).
			strands:         nil,
			live:            []LivePane{{ID: "%header", Height: 21}},
			headerPaneID:    "%header",
			wantSplitTarget: "%header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adoptID, splitTarget, err := planPaneTarget(tt.strands, tt.live, tt.headerPaneID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("planPaneTarget(%+v, %+v, %q): expected error, got nil", tt.strands, tt.live, tt.headerPaneID)
				}
				return
			}
			if err != nil {
				t.Fatalf("planPaneTarget: unexpected error: %v", err)
			}
			if adoptID != tt.wantAdoptID || splitTarget != tt.wantSplitTarget {
				t.Errorf("planPaneTarget(%+v, %+v, %q) = (adopt %q, split %q), want (adopt %q, split %q)",
					tt.strands, tt.live, tt.headerPaneID, adoptID, splitTarget, tt.wantAdoptID, tt.wantSplitTarget)
			}
		})
	}
}

func TestLoadOrInitStateLocked_AbsentFileInitializesFromEngineIdentity(t *testing.T) {
	e := newTestEngine(t)

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st == nil {
		t.Fatal("loadOrInitStateLocked() = nil, want a fresh MuxState")
	}
	if st.Socket != e.Socket() {
		t.Errorf("fresh state Socket = %q, want %q", st.Socket, e.Socket())
	}
	if st.Session != e.SessionName() {
		t.Errorf("fresh state Session = %q, want %q", st.Session, e.SessionName())
	}
	if len(st.Strands) != 0 {
		t.Errorf("fresh state Strands = %v, want empty", st.Strands)
	}
}

func TestLoadOrInitStateLocked_ExistingFileLoadsVerbatim(t *testing.T) {
	e := newTestEngine(t)

	want := &MuxState{
		Socket:  "some-other-server",
		Session: "some-other-session",
		Strands: []Strand{{GUID: "g1", PaneID: "%1"}},
	}
	if err := SaveState(e.layout.DotLyxDir(), want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	st, err := e.loadOrInitStateLocked()
	if err != nil {
		t.Fatalf("loadOrInitStateLocked: %v", err)
	}
	if st.Socket != want.Socket || st.Session != want.Session {
		t.Errorf("loadOrInitStateLocked() = %+v, want the persisted state, not a freshly initialized one", st)
	}
	if len(st.Strands) != 1 || st.Strands[0].GUID != "g1" {
		t.Errorf("loadOrInitStateLocked() Strands = %+v, want the persisted strand", st.Strands)
	}
}

// TestSendKeysLiteralArg pins the dash-escape rule for tmux send-keys -l:
// tmux parses a '-'-leading literal argument as flags and silently drops
// it (exit 0, nothing typed; '--' does not stop the parsing), so a
// dash-leading opaque cmd must be sent with one leading space — which the
// pane shell ignores — while every other text passes through verbatim.
func TestSendKeysLiteralArg(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"claude --continue", "claude --continue"},
		{"-join('a','b')", " -join('a','b')"},
		{"--flag-first", " --flag-first"},
		{" -already-spaced", " -already-spaced"},
		{"", ""},
		{"echo one; echo Enter", "echo one; echo Enter"},
	}
	for _, tt := range tests {
		if got := sendKeysLiteralArg(tt.text); got != tt.want {
			t.Errorf("sendKeysLiteralArg(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}
