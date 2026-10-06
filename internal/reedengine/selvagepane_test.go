// selvagepane_test.go tests the seam helpers selvagepane.go owns: bottommostPaneID and
// ensureSelvagePaneLocked's create/heal path (relocated from lifecycle_test.go, unchanged), and
// planPaneTarget's split-target policy (relocated from spawn_test.go, adapted for its card 5
// signature change). All are pure/hermetic or driven through the e.tmux.execHook fake — no live tmux
// required.

package reedengine

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPlanPaneTarget(t *testing.T) {
	tests := []struct {
		name            string
		live            []LivePane
		selvagePaneID   string
		wantSplitTarget string
		wantInsertAbove bool
		wantErr         bool
	}{
		{
			// Collapses the old FreshSession_AdoptsTheAliveInitialPane and
			// AllStrandsPaneless_AdoptsFirstAlivePane cases, which differed
			// only in the (now-deleted) strand table they supplied: a sole
			// alive pane and no Selvage both reduce to the same input once
			// the strand table stops mattering.
			name:            "FreshSession_SplitsTheAliveInitialPane",
			live:            []LivePane{{ID: "%1", Height: 50}},
			wantSplitTarget: "%1",
		},
		{
			name: "SoleCorpseUnbound_NeverAdopted_SplitOffTheCorpse",
			// A corpse is never a valid target for anything but a split — the
			// remove-last-strand aftermath: kill-pane on a session's sole
			// pane corpses it (pane_dead=1, exit 0) instead of removing it,
			// and send-keys into a corpse is silently swallowed.
			live:            []LivePane{{ID: "%1", Dead: true, Height: 50}},
			wantSplitTarget: "%1",
		},
		{
			// Collapses the old OneStrandHoldsAPane_SplitsTheTallestAlive and
			// TinyActiveBand_SplitTargetsTheTallestNotTheFirst cases, which
			// differed only in the (now-deleted) strand table they supplied:
			// a 2-row pane beside a 47-row pane and no Selvage, in both.
			name: "TinyActiveBand_SplitTargetsTheTallestNotTheFirst",
			// The session-target split defect this planner replaces: tmux
			// splits the active pane, which select-layout can leave on a
			// 1-2 row band, and a too-small split fails silently. The
			// planner must always pick the tallest alive pane instead.
			live:            []LivePane{{ID: "%1", Height: 2}, {ID: "%2", Height: 47}},
			wantSplitTarget: "%2",
		},
		{
			name:            "DeadPaneNeverTheSplitTargetWhileAnyAlive",
			live:            []LivePane{{ID: "%1", Dead: true, Height: 47}, {ID: "%2", Height: 2}},
			wantSplitTarget: "%2",
		},
		{
			name:    "NoPanesAtAll_Errors",
			live:    nil,
			wantErr: true,
		},
		{
			name: "SelvagePresentNoStrandBound_NonSelvagePaneIsTheSplitTarget",
			// A live Selvage pane plus an alive non-Selvage pane: the split
			// target must land on the non-Selvage pane, never Selvage.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%1", Height: 50}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SelvagePresentWithStrand_SelvageNeverTheSplitTarget",
			// Selvage is tallest by raw Height here, but must still
			// never be chosen over a genuine (if shorter) non-Selvage
			// candidate.
			live:            []LivePane{{ID: "%selvage", Height: 90}, {ID: "%1", Height: 10}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SeveralUntrackedAlivePanes_SplitsRatherThanGuessingWhichToAdopt",
			// R4 review finding R4-F5, reproduced live and the reason this
			// seam was removed: after .lyx/reed.json was scrubbed from a
			// running session, no strand held a binding and several
			// untracked alive panes remained — one of them the previous
			// header pane, still running "lyx reed header --blocking" (the
			// pre-Selvage keepalive this batch removes). Adoption picked
			// it, send-keys typed the strand's command onto
			// a blocked pane's screen where it never executed (exit 0
			// throughout), and status then reported the strand live with no
			// such process on the box. With more than one candidate there
			// was no way to tell an idle shell from a busy one, so the
			// planner always splits a guaranteed-idle new pane instead — off
			// the tallest, %2 here.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%stale", Height: 12}, {ID: "%2", Height: 37}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%2",
		},
		{
			name: "SeveralAlivePanesButOnlyOneNonSelvageAlive_StillSplits",
			// A fresh boot's Selvage plus the sole new-session pane, with a
			// dead corpse also present. Exactly one alive non-Selvage pane —
			// it is the split target regardless.
			live:            []LivePane{{ID: "%selvage", Height: 1}, {ID: "%corpse", Dead: true, Height: 12}, {ID: "%1", Height: 37}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%1",
		},
		{
			name: "SelvageIsSolePane_SplitTargetFallsBackToSelvage",
			// Every strand has been removed: only Selvage remains. Selvage
			// must become the split target so a subsequent add still
			// has something to split (Selvage survives the split).
			live:            []LivePane{{ID: "%selvage", Height: 21}},
			selvagePaneID:   "%selvage",
			wantSplitTarget: "%selvage",
			wantInsertAbove: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &ReedState{SelvagePaneID: tt.selvagePaneID}
			splitTarget, insertAbove, err := planPaneTarget(st, tt.live)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("planPaneTarget(%+v, %q): expected error, got nil", tt.live, tt.selvagePaneID)
				}
				return
			}
			if err != nil {
				t.Fatalf("planPaneTarget: unexpected error: %v", err)
			}
			if splitTarget != tt.wantSplitTarget {
				t.Errorf("planPaneTarget(%+v, %q) splitTargetID = %q, want %q",
					tt.live, tt.selvagePaneID, splitTarget, tt.wantSplitTarget)
			}
			if insertAbove != tt.wantInsertAbove {
				t.Errorf("planPaneTarget(%+v, %q) insertAbove = %v, want %v",
					tt.live, tt.selvagePaneID, insertAbove, tt.wantInsertAbove)
			}
		})
	}
}

// TestNewReapPolicy_ThreeQuestionsAssertedIndependently drives newReapPolicy's three consulting
// questions independently rather than through a single combined predicate, so a future
// fold-together fails here. The corpse case is the one that pins the distinction: it is exempt from
// both kills and authorizes neither reap.
//
//testtiming:keep pins the reap policy's three questions apart: an alive Selvage is exempt from both kills and authorizes the reap, a present corpse is exempt from both but authorizes none, and an unrelated pane is never exempt; its covering tests run this code without asserting it
func TestNewReapPolicy_ThreeQuestionsAssertedIndependently(t *testing.T) {
	const selvagePane = "%selvage"
	const otherPane = "%other"

	tests := []struct {
		name                        string
		st                          *ReedState
		live                        []LivePane
		wantExemptFromDeadKill      bool
		wantExemptFromUntrackedReap bool
		wantAuthorizesReap          bool
	}{
		{
			name:                        "AliveSelvage",
			st:                          &ReedState{SelvagePaneID: selvagePane},
			live:                        []LivePane{{ID: selvagePane, Dead: false}},
			wantExemptFromDeadKill:      true,
			wantExemptFromUntrackedReap: true,
			wantAuthorizesReap:          true,
		},
		{
			name:                        "DeadButPresentSelvageCorpse",
			st:                          &ReedState{SelvagePaneID: selvagePane},
			live:                        []LivePane{{ID: selvagePane, Dead: true}},
			wantExemptFromDeadKill:      true,
			wantExemptFromUntrackedReap: true,
			wantAuthorizesReap:          false,
		},
		{
			name:                        "SelvagePaneIDNamingAnAbsentPane",
			st:                          &ReedState{SelvagePaneID: selvagePane},
			live:                        []LivePane{{ID: otherPane, Dead: false}},
			wantExemptFromDeadKill:      true,
			wantExemptFromUntrackedReap: true,
			wantAuthorizesReap:          false,
		},
		{
			name:                        "EmptySelvagePaneID",
			st:                          &ReedState{},
			live:                        []LivePane{{ID: otherPane, Dead: false}},
			wantExemptFromDeadKill:      false,
			wantExemptFromUntrackedReap: false,
			wantAuthorizesReap:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := newReapPolicy(tt.st, tt.live)

			if got := policy.exemptFromDeadKill(selvagePane); got != tt.wantExemptFromDeadKill {
				t.Errorf("exemptFromDeadKill(selvagePane) = %v, want %v", got, tt.wantExemptFromDeadKill)
			}
			if got := policy.exemptFromDeadKill(otherPane); got {
				t.Errorf("exemptFromDeadKill(otherPane) = %v, want false (never exempts an unrelated pane)", got)
			}
			if got := policy.exemptFromUntrackedReap(selvagePane); got != tt.wantExemptFromUntrackedReap {
				t.Errorf("exemptFromUntrackedReap(selvagePane) = %v, want %v", got, tt.wantExemptFromUntrackedReap)
			}
			if got := policy.exemptFromUntrackedReap(otherPane); got {
				t.Errorf("exemptFromUntrackedReap(otherPane) = %v, want false (never exempts an unrelated pane)", got)
			}
			if got := policy.authorizesReap(); got != tt.wantAuthorizesReap {
				t.Errorf("authorizesReap() = %v, want %v", got, tt.wantAuthorizesReap)
			}
		})
	}
}

// TestSelvageRenderParams asserts the present, absent and empty-id cases, and that HeightRows comes
// through from the engine's config rather than being defaulted.
//
//testtiming:keep pins the render parameters: a present Selvage pane id passes through, an absent or empty one is blanked, and HeightRows comes from the engine's config; its covering tests run this code without asserting it
func TestSelvageRenderParams(t *testing.T) {
	const selvagePane = "%selvage"
	const heightRows = 4

	tests := []struct {
		name       string
		st         *ReedState
		presentIDs map[string]bool
		want       string
	}{
		{
			name:       "PresentPane_IDPassesThrough",
			st:         &ReedState{SelvagePaneID: selvagePane},
			presentIDs: map[string]bool{selvagePane: true},
			want:       selvagePane,
		},
		{
			name:       "AbsentPane_IDBlanked",
			st:         &ReedState{SelvagePaneID: selvagePane},
			presentIDs: map[string]bool{},
			want:       "",
		},
		{
			name:       "EmptySelvagePaneID_StaysEmpty",
			st:         &ReedState{},
			presentIDs: map[string]bool{},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.cfg.Selvage.HeightRows = heightRows

			got := e.selvageRenderParams(tt.st, tt.presentIDs)
			if got.PaneID != tt.want {
				t.Errorf("selvageRenderParams().PaneID = %q, want %q", got.PaneID, tt.want)
			}
			if got.HeightRows != heightRows {
				t.Errorf("selvageRenderParams().HeightRows = %d, want %d (from the engine's config, not defaulted)", got.HeightRows, heightRows)
			}
		})
	}
}

// TestSeedSelvageClaim asserts it adds the id when non-empty and leaves the map untouched when empty.
//
//testtiming:keep pins the seeded claim: the Selvage pane id is added to the claimed set when set and an absent Selvage claims nothing; its covering tests run this code without asserting it
func TestSeedSelvageClaim(t *testing.T) {
	t.Run("NonEmptyID_Added", func(t *testing.T) {
		st := &ReedState{SelvagePaneID: "%selvage"}
		claimed := map[string]bool{}
		seedSelvageClaim(st, claimed)
		if !claimed["%selvage"] {
			t.Errorf("claimed = %v, want it to contain %q", claimed, "%selvage")
		}
	})

	t.Run("EmptyID_MapUntouched", func(t *testing.T) {
		st := &ReedState{}
		claimed := map[string]bool{}
		seedSelvageClaim(st, claimed)
		if len(claimed) != 0 {
			t.Errorf("claimed = %v, want empty (an absent Selvage claims nothing)", claimed)
		}
	})
}

// TestClearSelvagePaneBinding asserts it empties a set id and is a no-op on an already-empty one.
//
//testtiming:keep pins the Selvage binding being emptied when set and left empty when already empty; its covering tests run this code without asserting it
func TestClearSelvagePaneBinding(t *testing.T) {
	t.Run("SetID_Cleared", func(t *testing.T) {
		st := &ReedState{SelvagePaneID: "%selvage"}
		clearSelvagePaneBinding(st)
		if st.SelvagePaneID != "" {
			t.Errorf("SelvagePaneID = %q, want empty", st.SelvagePaneID)
		}
	})

	t.Run("AlreadyEmpty_NoOp", func(t *testing.T) {
		st := &ReedState{}
		clearSelvagePaneBinding(st)
		if st.SelvagePaneID != "" {
			t.Errorf("SelvagePaneID = %q, want empty", st.SelvagePaneID)
		}
	})
}

// TestBottommostPaneID asserts the Selvage split target is chosen by pane_top rather than by
// list-panes order, which tmux does not guarantee is top-to-bottom.
//
//testtiming:keep pins the split target being chosen by pane_top rather than list-panes order, which tmux does not guarantee is top to bottom; its covering tests run this code without asserting it
func TestBottommostPaneID(t *testing.T) {
	tests := []struct {
		name string
		live []LivePane
		want string
	}{
		{"sole pane", []LivePane{{ID: "%0", Top: 0}}, "%0"},
		{"already last", []LivePane{{ID: "%0", Top: 0}, {ID: "%1", Top: 2}}, "%1"},
		{"not last in list order", []LivePane{{ID: "%1", Top: 26}, {ID: "%0", Top: 0}}, "%1"},
		{"three panes, tallest-top listed first", []LivePane{{ID: "%2", Top: 30}, {ID: "%0", Top: 10}, {ID: "%1", Top: 0}}, "%2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bottommostPaneID(tt.live); got != tt.want {
				t.Errorf("bottommostPaneID(%v) = %q; want %q", tt.live, got, tt.want)
			}
		})
	}
}

// TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure pins the validateSplitCreatedNewPane
// guard at its call site (against regression).
func TestEnsureSelvagePaneLocked_RebuildRejectsSilentSplitFailure(t *testing.T) {
	e := newTestEngine(t)

	// One alive, non-Selvage pane (%0) — the new-session initial pane a fresh
	// boot leaves before Selvage exists. It is the only pane, so it is both
	// the bottommost split target and the id psmux's silent-split shape re-prints.
	const existingPaneID = "%0"
	listPanesOut := existingPaneID + " 0 0 100 20 4321\n"

	// Every other verb (send-keys, kill-pane) is only reached if the guard is (wrongly) bypassed;
	// the fake answers it empty so the missing-guard regression returns nil and this test's error assertion catches it.
	fake := installFakeTmux(t, e)
	fake.answer("list-panes", listPanesOut, nil)
	// psmux silent failure: exit 0, no new pane, an EXISTING pane's id
	// printed on stdout. Trusting it would bind Selvage to %0.
	fake.answer("split-window", existingPaneID+"\n", nil)

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	err := e.ensureSelvagePaneLocked(st)
	if err == nil {
		t.Fatalf("ensureSelvagePaneLocked accepted a silent-split failure (bound Selvage to pre-existing pane %q); the validateSplitCreatedNewPane guard at this call site is missing or bypassed", existingPaneID)
	}
	if !strings.Contains(err.Error(), "split Selvage pane") {
		t.Errorf("error = %v, want it to name the Selvage split failure", err)
	}
	if st.SelvagePaneID != "" {
		t.Errorf("SelvagePaneID = %q, want unchanged (never bound to the pre-existing strand pane on a rejected rebuild)", st.SelvagePaneID)
	}
}

// TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit is the regression guard for
// the R4 review's R4-F4: an untracked one-row Selvage band at the physical bottom of the window made
// every Selvage rebuild impossible, wedging up and resume permanently with "no space for new pane"
// while status kept reporting the session healthy.
//
// Reproduced live before the fix: with a session up and the default one-row Selvage band laid out,
// removing .lyx/reed.json — a never-tracked machine-local tree, exactly what `git clean -xdf` in the
// worktree deletes — left `lyx reed up` and `lyx reed resume` failing identically on every
// subsequent invocation, with `lyx reed down` the only (unnamed) escape.
//
// The scripted substrate below is the shape that produced it: a one-row pane at the largest pane_top
// that tmux refuses to split, and a tall pane above it. The assertions are that the even-vertical
// re-tile is actually issued and that the retried split's pane becomes Selvage — a fix that only
// improved the error message fails both.
func TestEnsureSelvagePaneLocked_RecoversWhenTheBottomPaneIsTooSmallToSplit(t *testing.T) {
	e := newTestEngine(t)

	const oneRowBottomPaneID = "%1"
	const tallPaneID = "%0"
	const rebuiltSelvagePaneID = "%7"
	// pane_id pane_dead pane_top pane_width pane_height pane_pid
	wedged := tallPaneID + " 0 0 100 48 4321\n" + oneRowBottomPaneID + " 0 48 100 1 4322\n"
	retiled := tallPaneID + " 0 0 100 24 4321\n" + oneRowBottomPaneID + " 0 25 100 25 4322\n"

	reTiled := false
	fake := installFakeTmux(t, e)
	fake.answerFunc("list-panes", func([]string) (string, error) {
		if reTiled {
			return retiled, nil
		}
		return wedged, nil
	})
	fake.answerFunc("select-layout", func(args []string) (string, error) {
		if len(args) < 2 || args[len(args)-1] != "even-vertical" {
			return "", fmt.Errorf("unexpected select-layout args %v; want the built-in even-vertical layout", args)
		}
		reTiled = true
		return "", nil
	})
	fake.answerFunc("split-window", func([]string) (string, error) {
		if !reTiled {
			// tmux's real refusal against a one-row pane: exit 1, no pane.
			return "", errors.New("exit status 1: no space for new pane")
		}
		return rebuiltSelvagePaneID + "\n", nil
	})

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked() = %v; want nil (the Selvage rebuild must recover from a one-row bottom pane, not wedge the worktree)", err)
	}
	if !reTiled {
		t.Errorf("ensureSelvagePaneLocked never issued the even-vertical re-tile; without it the retried split has no room either")
	}
	if splitAttempts := fake.Count("split-window"); splitAttempts != 2 {
		t.Errorf("split-window attempts = %d; want exactly 2 (one refused, one retried behind the re-tile)", splitAttempts)
	}
	if st.SelvagePaneID != rebuiltSelvagePaneID {
		t.Errorf("SelvagePaneID = %q; want %q (the pane the retried split created)", st.SelvagePaneID, rebuiltSelvagePaneID)
	}
	// The retried split carries the launch command too: a retry that dropped it would boot a recovered Selvage as a commandless shell.
	retriedSplitArgs := fake.LastArgv("split-window")
	if launchArg := retriedSplitArgs[len(retriedSplitArgs)-1]; launchArg != e.cfg.Shell {
		t.Errorf("retried split-window trailing argument = %q, want %q (a retried Selvage must never boot commandless)", launchArg, e.cfg.Shell)
	}
}

// TestEnsureSelvagePaneLocked_SplitsOnceBelowTheBottommostPane pins the Selvage create path on a scripted two-pane session:
//   - the split targets the bottommost pane and carries no -b, so the new pane lands below it, where render.Rules emits the band cell last;
//   - it pins its pane to Geometry.PaneCwd, not AnchorPath (the two are distinct on newTestEngine's fixture, so this cannot pass by coincidence);
//   - it launches the shell as split-window's own trailing shell-command argument, never by typing into the pane with send-keys;
//   - it records the new pane's id onto state even though the fake tmux never runs a real shell.
//
// The new-session spawn site is not reachable from this seam, since it builds its argv and runs it
// through the os/exec package's Command function directly rather than through e.tmux; the tagged reed suites cover that half.
//
//testtiming:keep pins the Selvage create path's split-window call: targeting the bottommost pane with no -b, pinned to PaneCwd, launching the shell as the trailing argument and never via send-keys, then recording the new pane id; its covering tests run this code without asserting it
func TestEnsureSelvagePaneLocked_SplitsOnceBelowTheBottommostPane(t *testing.T) {
	e := newTestEngine(t)
	const topPaneID = "%0"
	const bottomPaneID = "%1"
	const newPaneID = "%2"
	fake := installFakeTmux(t, e)
	fake.answer("list-panes", topPaneID+" 0 0 100 10 4321\n"+bottomPaneID+" 0 10 100 10 4322\n", nil)
	// A genuinely new pane id, distinct from the pre-split live set, so the silent-split guard (validateSplitCreatedNewPane) does not reject the call.
	fake.answer("split-window", newPaneID+"\n", nil)

	st := &ReedState{Socket: e.Socket(), Session: e.SessionName()}
	if err := e.ensureSelvagePaneLocked(st); err != nil {
		t.Fatalf("ensureSelvagePaneLocked: %v", err)
	}

	splitArgs := fake.LastArgv("split-window")
	flagValue := func(flag string) string {
		t.Helper()
		for i, arg := range splitArgs {
			if arg == flag {
				if i+1 >= len(splitArgs) {
					t.Fatalf("split-window argv %v has a trailing %s with no value", splitArgs, flag)
				}
				return splitArgs[i+1]
			}
		}
		t.Fatalf("split-window argv %v has no %s flag", splitArgs, flag)
		return ""
	}
	for _, arg := range splitArgs {
		if arg == "-b" {
			t.Errorf("split-window argv %v carries -b; want no -b (Selvage splits below, not above)", splitArgs)
		}
	}
	if got := flagValue("-t"); got != bottomPaneID {
		t.Errorf("split-window -t value = %q, want %q (the bottommost pane)", got, bottomPaneID)
	}
	if got := flagValue("-c"); got != e.geom.PaneCwd || got == e.geom.AnchorPath {
		t.Errorf("split-window -c value = %q, want Geometry.PaneCwd %q and not AnchorPath %q", got, e.geom.PaneCwd, e.geom.AnchorPath)
	}
	// The value after -F is the format; the launch line is the one argument after it.
	formatIdx := -1
	for i, arg := range splitArgs {
		if arg == "-F" {
			formatIdx = i
		}
	}
	if formatIdx == -1 || formatIdx+2 >= len(splitArgs) {
		t.Fatalf("split-window argv %v carries no trailing command argument after the -F value; want the launch line appended as split-window's own trailing shell-command argument", splitArgs)
	}
	if got := splitArgs[formatIdx+2]; got != e.cfg.Shell {
		t.Errorf("split-window trailing command argument = %q, want %q (e.cfg.Shell, launched the same way new-session launches the session's first pane)", got, e.cfg.Shell)
	}
	if sendKeysCalls := fake.Count("send-keys"); sendKeysCalls != 0 {
		t.Errorf("send-keys calls = %d, want 0 (Selvage must launch its own command on the split, not be typed into via send-keys)", sendKeysCalls)
	}
	if st.SelvagePaneID != newPaneID {
		t.Errorf("SelvagePaneID = %q, want %q", st.SelvagePaneID, newPaneID)
	}
}
