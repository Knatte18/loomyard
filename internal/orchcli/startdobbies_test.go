//go:build tier0

// startdobbies_test.go pins that `lyx orch start` starts the dobbies before it touches a strand, and that a failing dobby start only warns.

package orchcli

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
)

func TestStart_StartsTheDobbiesFirst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		// seam, when false, leaves the start without a dobby seam.
		seam    bool
		seamErr error
		// wantFirst is the first call start makes, empty when no dobby start is wired.
		wantFirst string
		// wantWarning is the text the envelope's warning names, empty for no warning.
		wantWarning []string
	}{
		{name: "a successful dobby start runs before the strand is launched", seam: true, wantFirst: "dobbies"},
		{
			name: "a failing dobby start still launches the strand and warns naming lyx beam start", seam: true,
			seamErr: errors.New("payments: start the dobby: spawn failed"), wantFirst: "dobbies",
			wantWarning: []string{"payments", "spawn failed", "lyx beam start"},
		},
		{name: "a continue start calls the seam first too", args: []string{"--continue"}, seam: true, wantFirst: "dobbies"},
		{name: "a nil seam starts nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStartHarness(t)
			if slices.Contains(tt.args, "--continue") {
				saveSeedState(t, h, orchengine.State{Phase: orchengine.PhaseIdle, Strand: "g1", SessionID: adoptTestSessionID})
			}
			if tt.seam {
				h.cli.startDobbies = func() error {
					h.calls = append(h.calls, "dobbies")
					return tt.seamErr
				}
			}

			code, env := h.run(t, tt.args...)

			if code != 0 || env["ok"] != true {
				t.Fatalf("start = exit %d, %v; want a success", code, env)
			}
			if !slices.Contains(h.calls, "start") {
				t.Errorf("calls = %v; want the strand launched", h.calls)
			}
			if tt.wantFirst == "" && slices.Contains(h.calls, "dobbies") || tt.wantFirst != "" && (len(h.calls) == 0 || h.calls[0] != tt.wantFirst) {
				t.Errorf("calls = %v; want %q first", h.calls, tt.wantFirst)
			}
			warning, _ := env["warning"].(string)
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
