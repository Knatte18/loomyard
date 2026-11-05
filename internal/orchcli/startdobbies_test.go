// startdobbies_test.go pins that `lym beam start` starts the worktree's dobby once, before it touches a strand, that a dobby that does not start only warns, and that a bring-up for another verb reads a dobby that did not start as an error.

package orchcli

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStart_StartsTheDobbiesFirst(t *testing.T) {
	tiergate.Require(t, 0)
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
		// relay, when true, wires the relay seam, which answers relayErr; when false the start has no relay seam.
		relay    bool
		relayErr error
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
		{
			name: "a relay start runs once, after the dobby and before the strand", seam: true, relay: true,
			entry: dobbyengine.StartEntry{Outcome: dobbyengine.OutcomeLive}, wantFirst: "dobby",
		},
		{
			name: "a failing relay start warns naming lym slack start and still launches the strand", seam: true, relay: true,
			entry: dobbyengine.StartEntry{Outcome: dobbyengine.OutcomeLive}, relayErr: errors.New("no per-user directory for the token file"), wantFirst: "dobby",
			wantWarning: []string{"no per-user directory for the token file", "lym slack start"},
		},
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
			if tt.relay {
				h.cli.startRelay = func() (slackengine.StartOutcome, error) {
					h.calls = append(h.calls, "relay")
					return slackengine.OutcomeStarted, tt.relayErr
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
			wantOrder := []string{}
			if tt.seam {
				wantOrder = append(wantOrder, "dobby")
			}
			if tt.relay {
				wantOrder = append(wantOrder, "relay")
			}
			wantOrder = append(wantOrder, "start")
			stepsOfStart := slices.DeleteFunc(slices.Clone(h.calls), func(call string) bool { return !slices.Contains([]string{"dobby", "relay", "start"}, call) })
			if !slices.Equal(stepsOfStart, wantOrder) {
				t.Errorf("start steps = %v; want %v, so the relay runs once, after the dobby and before the strand, and a nil seam calls nothing", stepsOfStart, wantOrder)
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

	t.Run("relayStarter answers nil for a beam and a starter for the prime", func(t *testing.T) {
		t.Parallel()
		location := &lymcwd.Location{HubPath: filepath.Join("hubs", "app.hub"), WorktreeName: "main", AnchorRel: "."}
		if starter := relayStarter(location, true); starter != nil {
			t.Error("relayStarter(beam) is not nil; want none, since a beam's bring-up never starts the hub's relay")
		}
		if starter := relayStarter(location, false); starter == nil {
			t.Error("relayStarter(prime) is nil; want a starter")
		}
	})
}

// TestBringUpLocation pins that the prime's name brings up the prime without resolving anything, and that any other name brings up the location resolved from that worktree's directory.
func TestBringUpLocation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	prime := &lymcwd.Location{HubPath: filepath.Join("hubs", "app.hub"), WorktreeName: "main", AnchorRel: "."}
	beam := &lymcwd.Location{HubPath: prime.HubPath, WorktreeName: "some-beam", AnchorRel: "."}
	tests := []struct {
		name       string
		worktree   string
		resolveErr error
		want       *lymcwd.Location
		// wantResolved is the directory resolve was asked for, empty when it was not called.
		wantResolved string
		wantErr      string
	}{
		{name: "Prime", worktree: "main", want: prime},
		{name: "Beam", worktree: "some-beam", want: beam, wantResolved: fabricengine.WorktreePath(prime, "some-beam")},
		{name: "BeamResolveFails", worktree: "some-beam", resolveErr: errors.New("no such worktree"), wantResolved: fabricengine.WorktreePath(prime, "some-beam"), wantErr: "orch: resolve the worktree of some-beam: no such worktree"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ""
			resolve := func(worktreeRoot string) (*lymcwd.Location, error) {
				resolved = worktreeRoot
				return beam, tt.resolveErr
			}

			got, err := bringUpLocation(prime, tt.worktree, resolve)

			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != tt.wantErr {
				t.Errorf("bringUpLocation(%s) error = %q; want %q", tt.worktree, gotErr, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("bringUpLocation(%s) = %+v; want %+v", tt.worktree, got, tt.want)
			}
			if resolved != tt.wantResolved {
				t.Errorf("bringUpLocation(%s) resolved %q; want %q", tt.worktree, resolved, tt.wantResolved)
			}
		})
	}
}

// TestDobbyStartError pins that a bring-up whose dobby started or was live passes, and that a refused, failed or unreported dobby start is an error naming the worktree and its cause.
func TestDobbyStartError(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		report BeamUpReport
		// wantErr is the error's text, empty for no error.
		wantErr string
	}{
		{"Started", BeamUpReport{Dobby: dobbyengine.OutcomeStarted}, ""},
		{"Live", BeamUpReport{Dobby: dobbyengine.OutcomeLive}, ""},
		{"Refused", BeamUpReport{Dobby: dobbyengine.OutcomeRefused, DobbyDetail: "owned by elsewhere"}, "orch: the dobby of some-beam was not started: owned by elsewhere"},
		{"Failed", BeamUpReport{Dobby: dobbyengine.OutcomeFailed, DobbyDetail: "exec failed"}, "orch: the dobby of some-beam was not started: exec failed"},
		{"Unreported_CarriesTheWarnings", BeamUpReport{Warnings: []string{"first", "second"}}, "orch: the dobby of some-beam was not started: first; second"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := dobbyStartError("some-beam", tt.report)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tt.wantErr {
				t.Errorf("dobbyStartError(some-beam, %+v) = %q; want %q", tt.report, got, tt.wantErr)
			}
		})
	}
}
