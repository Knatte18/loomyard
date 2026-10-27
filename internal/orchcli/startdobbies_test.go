//go:build tier0

// startdobbies_test.go pins that `lym beam start` starts the worktree's dobby once, before it touches a strand, and that a dobby that does not start only warns.

package orchcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

func TestStart_StartsTheDobbiesFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// recorded seeds a session id, so the bring-up of `lym beam start` is a resume.
		recorded string
		// seam, when false, leaves the start without a dobby seam.
		seam    bool
		entry   dobbyengine.StartEntry
		seamErr error
		// wantFirst is the first call start makes, empty when no dobby start is wired.
		wantFirst string
		// wantWarning is the text the report's warnings name, empty for no warning.
		wantWarning []string
	}{
		{name: "a successful dobby start runs before the strand is launched", seam: true, entry: dobbyengine.StartEntry{Worktree: "main", Outcome: dobbyengine.OutcomeStarted}, wantFirst: "dobby"},
		{
			name: "a failing dobby start still launches the strand and warns naming lym beam start", seam: true,
			entry:   dobbyengine.StartEntry{Worktree: "payments", Outcome: dobbyengine.OutcomeFailed, Detail: "start the dobby: spawn failed"},
			seamErr: errors.New("payments: start the dobby: spawn failed"), wantFirst: "dobby",
			wantWarning: []string{"payments", "spawn failed", "lym beam start"},
		},
		{
			name: "a refused dobby start warns with the engine's detail", seam: true,
			entry:     dobbyengine.StartEntry{Worktree: "payments", Outcome: dobbyengine.OutcomeRefused, Detail: "the landing hold stands; see `lym batten status payments`"},
			wantFirst: "dobby", wantWarning: []string{"the landing hold stands", "lym beam start"},
		},
		{name: "a resume start calls the seam first too", recorded: recordedTestSessionID, seam: true, entry: dobbyengine.StartEntry{Outcome: dobbyengine.OutcomeLive}, wantFirst: "dobby"},
		{name: "a nil seam starts nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t)
			if tt.recorded != "" {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", SessionID: tt.recorded})
			}
			if tt.seam {
				h.cli.startDobby = func() (dobbyengine.StartEntry, error) {
					h.calls = append(h.calls, "dobby")
					return tt.entry, tt.seamErr
				}
			}

			code, env := h.run(t)

			if code != 0 || env["ok"] != true {
				t.Fatalf("start = exit %d, %v; want a success", code, env)
			}
			if !slices.Contains(h.calls, "start") {
				t.Errorf("calls = %v; want the strand launched", h.calls)
			}
			if tt.wantFirst == "" && slices.Contains(h.calls, "dobby") || tt.wantFirst != "" && (len(h.calls) == 0 || h.calls[0] != tt.wantFirst) {
				t.Errorf("calls = %v; want %q first", h.calls, tt.wantFirst)
			}
			if count := slices.Index(h.calls, "dobby"); tt.seam && (count != 0 || slices.Contains(h.calls[1:], "dobby")) {
				t.Errorf("calls = %v; want the dobby started once, before the window and the orch", h.calls)
			}
			if got, _ := env["dobby"].(string); got != tt.entry.Outcome {
				t.Errorf("report dobby = %q; want %q", got, tt.entry.Outcome)
			}
			warning := warningText(env)
			for _, want := range tt.wantWarning {
				if !strings.Contains(warning, want) {
					t.Errorf("warning = %q; want it to name %q", warning, want)
				}
			}
			if len(tt.wantWarning) == 0 && warning != "" {
				t.Errorf("warning = %q; want none", warning)
			}
		})
	}
}
