// generation_test.go covers the pane-generation stamp: parsing a display-message answer, the two
// comparison predicates, and adoptPaneGenerationLocked's three outcomes (adopt, clear, refuse)
// driven through TmuxCmd's execHook seam so no live server is required.

package reedengine

import (
	"errors"
	"strings"
	"testing"
)

func TestParsePaneGeneration(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    PaneGeneration
		wantErr bool
	}{
		{
			name: "a well-formed answer",
			out:  "$0|2569636|1787043918\n",
			want: PaneGeneration{SessionName: "svc", TmuxSessionID: "$0", ServerPID: "2569636", Created: "1787043918"},
		},
		{
			name: "surrounding whitespace is trimmed",
			out:  "  $3|17|99  ",
			want: PaneGeneration{SessionName: "svc", TmuxSessionID: "$3", ServerPID: "17", Created: "99"},
		},
		{
			name:    "an empty answer is an error, never a zero stamp",
			out:     "",
			wantErr: true,
		},
		{
			name:    "too few fields is an error",
			out:     "$0|2569636",
			wantErr: true,
		},
		{
			name:    "too many fields is an error",
			out:     "$0|2569636|1787043918|extra",
			wantErr: true,
		},
		{
			name:    "a blank field is an error, since a partial stamp would mismatch every future probe",
			out:     "$0||1787043918",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePaneGeneration("svc", tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePaneGeneration(%q) error = %v; wantErr %v", tt.out, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("parsePaneGeneration(%q) = %+v; want %+v", tt.out, got, tt.want)
			}
		})
	}
}

func TestPaneGeneration_RecordedAndSameIncarnation(t *testing.T) {
	base := PaneGeneration{SessionName: "svc", TmuxSessionID: "$0", ServerPID: "100", Created: "5"}

	if (PaneGeneration{}).Recorded() {
		t.Error("(PaneGeneration{}).Recorded() = true; want false")
	}
	if !base.Recorded() {
		t.Error("base.Recorded() = false; want true")
	}

	tests := []struct {
		name  string
		other PaneGeneration
		want  bool
	}{
		{"identical", base, true},
		{
			name:  "a renamed session is still the same incarnation",
			other: PaneGeneration{SessionName: "svc-renamed", TmuxSessionID: "$0", ServerPID: "100", Created: "5"},
			want:  true,
		},
		{
			name:  "a different session on the same server is not",
			other: PaneGeneration{SessionName: "svc", TmuxSessionID: "$1", ServerPID: "100", Created: "5"},
			want:  false,
		},
		{
			name:  "the same session id on a reborn server is not",
			other: PaneGeneration{SessionName: "svc", TmuxSessionID: "$0", ServerPID: "200", Created: "5"},
			want:  false,
		},
		{
			name:  "the same session id and server with a different creation time is not",
			other: PaneGeneration{SessionName: "svc", TmuxSessionID: "$0", ServerPID: "100", Created: "6"},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.SameIncarnation(tt.other); got != tt.want {
				t.Errorf("base.SameIncarnation(%+v) = %v; want %v", tt.other, got, tt.want)
			}
		})
	}
}

// generationHook returns an execHook answering display-message with the generation recorded in
// generationsBySessionTarget, and erroring for any session not in it — the shape tmux itself has for
// a session that does not exist.
// Every other subcommand answers empty/nil, since adoptPaneGenerationLocked issues none of them.
func generationHook(t *testing.T, generationsBySessionTarget map[string]string) func(bool, ...string) (string, error) {
	t.Helper()
	return func(capture bool, args ...string) (string, error) {
		if args[0] != "display-message" {
			return "", nil
		}
		target := args[len(args)-2]
		out, ok := generationsBySessionTarget[target]
		if !ok {
			return "", errors.New("exit status 1: can't find session")
		}
		return out, nil
	}
}

// TestAdoptPaneGenerationLocked is the regression guard for the R5 review's R5-F2 (stale bindings
// from a previous session incarnation reported live:true and blocked resume) and R5-F5 / R5-F4
// (a renamed worktree, or a hand-copied .lyx, silently double-launching every strand or targeting
// another worktree's live panes).
func TestAdoptPaneGenerationLocked(t *testing.T) {
	const liveAnswer = "$4|900|1787043918"
	liveGeneration := PaneGeneration{SessionName: "worktree", TmuxSessionID: "$4", ServerPID: "900", Created: "1787043918"}

	tests := []struct {
		name string
		// generations maps an exact-match window target to the display-message answer for it.
		generations     map[string]string
		recorded        PaneGeneration
		wantErrFragment string
		wantCleared     bool
		wantGeneration  PaneGeneration
	}{
		{
			name:           "an unstamped state adopts the live generation without clearing",
			generations:    map[string]string{"=worktree:": liveAnswer},
			recorded:       PaneGeneration{},
			wantCleared:    false,
			wantGeneration: liveGeneration,
		},
		{
			name:           "a matching stamp keeps the bindings",
			generations:    map[string]string{"=worktree:": liveAnswer},
			recorded:       liveGeneration,
			wantCleared:    false,
			wantGeneration: liveGeneration,
		},
		{
			name:           "a stamp from an earlier incarnation of the same session clears the bindings",
			generations:    map[string]string{"=worktree:": liveAnswer},
			recorded:       PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "100", Created: "1787000000"},
			wantCleared:    true,
			wantGeneration: liveGeneration,
		},
		{
			name:           "a stamp naming a session that no longer exists clears the bindings",
			generations:    map[string]string{"=worktree:": liveAnswer},
			recorded:       PaneGeneration{SessionName: "svc-orig", TmuxSessionID: "$0", ServerPID: "100", Created: "1787000000"},
			wantCleared:    true,
			wantGeneration: liveGeneration,
		},
		{
			name: "a stamp naming a DIFFERENT session that is still live refuses the operation",
			generations: map[string]string{
				"=worktree:": liveAnswer,
				"=svc-orig:": "$0|900|1787000000",
			},
			recorded:        PaneGeneration{SessionName: "svc-orig", TmuxSessionID: "$0", ServerPID: "900", Created: "1787000000"},
			wantErrFragment: "STILL RUNNING",
		},
		{
			name: "a namesake session that is a different incarnation is not mistaken for the orphan",
			generations: map[string]string{
				"=worktree:": liveAnswer,
				"=svc-orig:": "$7|900|1787043999",
			},
			recorded:       PaneGeneration{SessionName: "svc-orig", TmuxSessionID: "$0", ServerPID: "100", Created: "1787000000"},
			wantCleared:    true,
			wantGeneration: liveGeneration,
		},
		{
			name:           "a probe failure leaves the bindings exactly as loaded",
			generations:    map[string]string{},
			recorded:       PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "100", Created: "1787000000"},
			wantCleared:    false,
			wantGeneration: PaneGeneration{SessionName: "worktree", TmuxSessionID: "$0", ServerPID: "100", Created: "1787000000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			e.tmux.execHook = generationHook(t, tt.generations)

			st := &ReedState{
				HeaderPaneID:   "%1",
				Strands:        []Strand{{GUID: "a", PaneID: "%2"}},
				PaneGeneration: tt.recorded,
			}

			err := e.adoptPaneGenerationLocked(st)
			if tt.wantErrFragment != "" {
				if err == nil {
					t.Fatalf("adoptPaneGenerationLocked() = nil; want an error naming the still-live recorded session")
				}
				if !strings.Contains(err.Error(), tt.wantErrFragment) {
					t.Errorf("adoptPaneGenerationLocked() error = %v; want it to contain %q", err, tt.wantErrFragment)
				}
				if !strings.Contains(err.Error(), "kill-session") {
					t.Errorf("adoptPaneGenerationLocked() error = %v; want it to name the exact remedy command", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("adoptPaneGenerationLocked() = %v; want nil", err)
			}

			gotCleared := st.HeaderPaneID == "" && findStrandPaneID(st.Strands, "a") == ""
			if gotCleared != tt.wantCleared {
				t.Errorf("bindings cleared = %v (HeaderPaneID=%q, strand a=%q); want cleared = %v",
					gotCleared, st.HeaderPaneID, findStrandPaneID(st.Strands, "a"), tt.wantCleared)
			}
			if st.PaneGeneration != tt.wantGeneration {
				t.Errorf("PaneGeneration = %+v; want %+v", st.PaneGeneration, tt.wantGeneration)
			}
		})
	}
}
