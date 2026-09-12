// status_test.go is a table over renderStatusLine, pinning its exact rendered line for each shape of
// Activity the composed status file can carry, plus a table over printStatusLinesOnChange's
// suppress-an-unchanged-line rule.

package loomcli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestRenderStatusLine covers renderStatusLine's three shapes: an empty last and wait, a populated
// last only, and both populated -- each asserting the exact expected line, since the format is pinned
// rather than left to judgment.
func TestRenderStatusLine(t *testing.T) {
	tests := []struct {
		name string
		st   shedengine.Status
		want string
	}{
		{
			name: "EmptyLastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Preflight", Last: "", Wait: ""},
			},
			want: "loom running | now Preflight",
		},
		{
			name: "LastOnly",
			st: shedengine.Status{
				State:    shedengine.StateRunning,
				Activity: shedengine.Activity{Now: "Discussion-Write", Last: "Preflight → done", Wait: ""},
			},
			want: "loom running | now Discussion-Write | last Preflight → done",
		},
		{
			name: "LastAndWait",
			st: shedengine.Status{
				State:    shedengine.StateBlocked,
				Activity: shedengine.Activity{Now: "Plan-Validate", Last: "Plan-Validate → stuck", Wait: "plan validation failed"},
			},
			want: "loom blocked | now Plan-Validate | last Plan-Validate → stuck | wait plan validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderStatusLine(tt.st); got != tt.want {
				t.Errorf("renderStatusLine(%+v) = %q; want %q", tt.st, got, tt.want)
			}
		})
	}
}

// TestPrintStatusLinesOnChange covers the suppress-an-unchanged-line rule the watch tail rests on.
// The UnchangedRepeats row is the direct regression guard: before this rule the tail printed one
// line per poll, so a producer call that lasts minutes emitted hundreds of byte-identical lines into
// the strand pane and evicted its own scrollback.
func TestPrintStatusLinesOnChange(t *testing.T) {
	tests := []struct {
		name  string
		polls []string
		want  []string
	}{
		{
			name:  "UnchangedRepeats",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Write", "loom running | now Plan-Write"},
			want:  []string{"loom running | now Plan-Write"},
		},
		{
			name:  "PrintsEveryTransition",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Write", "loom running | now Plan-Validate", "loom blocked | now Plan-Validate"},
			want:  []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom blocked | now Plan-Validate"},
		},
		{
			name:  "ReprintsAfterReturningToAnEarlierLine",
			polls: []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom running | now Plan-Write"},
			want:  []string{"loom running | now Plan-Write", "loom running | now Plan-Validate", "loom running | now Plan-Write"},
		},
		{
			name:  "TransientUnavailableIsAlsoDeduped",
			polls: []string{statusUnavailableLine, statusUnavailableLine, "loom running | now Plan-Write"},
			want:  []string{statusUnavailableLine, "loom running | now Plan-Write"},
		},
		{
			name:  "FirstLineIsAlwaysPrinted",
			polls: []string{"loom running | now Preflight"},
			want:  []string{"loom running | now Preflight"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			next := 0
			poll := func() string {
				line := tt.polls[next]
				next++
				return line
			}
			slept := 0
			printStatusLinesOnChange(&out, poll, func() { slept++ }, len(tt.polls))

			got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if out.Len() == 0 {
				got = nil
			}
			if len(got) != len(tt.want) {
				t.Fatalf("printStatusLinesOnChange(%v) printed %d line(s) %q; want %d line(s) %q", tt.polls, len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("printStatusLinesOnChange(%v) line %d = %q; want %q", tt.polls, i, got[i], tt.want[i])
				}
			}
			if slept != len(tt.polls) {
				t.Errorf("printStatusLinesOnChange(%v) slept %d time(s); want %d -- the tail must keep polling at its interval even while suppressing output", tt.polls, slept, len(tt.polls))
			}
		})
	}
}

// TestStatusCmd_EnvelopeKeySet drives statusCmd()'s RunE in-process against a hand-populated
// receiver whose shedPaths point at a seeded status file under t.TempDir(), and asserts the emitted
// envelope's top-level key set equals exactly the nine payload keys plus "ok" -- in both directions,
// so the interrupt_policy addition can neither quietly rename an existing key nor let a later
// removal go unnoticed. It also tables interrupt_policy's own value across the three shapes
// InterruptPolicyFor returns: "handback" for NameWebster, "reinvoke" for another named row, and the
// empty string for a current_producer naming no row at all.
func TestStatusCmd_EnvelopeKeySet(t *testing.T) {
	tests := []struct {
		name            string
		currentProducer string
		wantPolicy      string
	}{
		{"HandbackForWebster", loomshed.NameWebster, loomshed.InterruptPolicyHandback},
		{"ReinvokeForAnotherRow", loomshed.NameDiscussionWrite, loomshed.InterruptPolicyReinvoke},
		{"EmptyForUnknownRow", "not-a-real-row", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			statusLockPath := filepath.Join(dir, "status.json.lock")

			seeded := shedengine.Status{
				CurrentProducer: tt.currentProducer,
				State:           shedengine.StateRunning,
				Activity:        shedengine.Activity{Now: "Preflight"},
			}
			if err := state.WriteJSON(statusPath, statusLockPath, seeded); err != nil {
				t.Fatalf("state.WriteJSON(%q) = %v; want nil", statusPath, err)
			}

			c := &loomCLI{
				shedPaths: loomrecipe.ShedPaths{
					StatusPath:     statusPath,
					StatusLockPath: statusLockPath,
				},
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(c.statusCmd(), &out, nil)
			if exitCode != 0 {
				t.Fatalf("statusCmd() exit code = %d; want 0; output: %q", exitCode, out.String())
			}

			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("json.Unmarshal(%q) = %v; want nil", out.String(), err)
			}

			wantKeys := map[string]bool{
				"ok": true, "current_producer": true, "state": true, "error": true,
				"pause_requested": true, "activity": true, "history_length": true,
				"slug": true, "parent": true, "interrupt_policy": true,
			}
			for k := range wantKeys {
				if _, ok := envelope[k]; !ok {
					t.Errorf("statusCmd() envelope missing key %q; got keys %v", k, envelope)
				}
			}
			for k := range envelope {
				if !wantKeys[k] {
					t.Errorf("statusCmd() envelope has unexpected key %q; got keys %v", k, envelope)
				}
			}

			if got, _ := envelope["interrupt_policy"].(string); got != tt.wantPolicy {
				t.Errorf("statusCmd() envelope[\"interrupt_policy\"] = %q; want %q", got, tt.wantPolicy)
			}
		})
	}
}
