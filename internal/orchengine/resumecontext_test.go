package orchengine

import (
	"os"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/stencilstore"
)

func TestResumeContext_RendersPointerPerPhaseAndWritesMark(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name         string
		state        State
		wantText     func(t *testing.T, p Paths, stencils string) string
		wantRoleFile bool
	}{
		{
			name:     "resuming returns the pending pointer verbatim and renders no role file",
			state:    State{Strand: "s1", Phase: PhaseResuming, PendingResume: "read the note at /n"},
			wantText: func(*testing.T, Paths, string) string { return "read the note at /n" },
		},
		{
			name:  "compacting renders the resume prompt naming the note",
			state: State{Strand: "s1", Phase: PhaseCompacting, LastHandoff: "/handoffs/h.md"},
			wantText: func(t *testing.T, p Paths, stencils string) string {
				text, err := RenderResumePrompt(stencils, p.RolePath, "/handoffs/h.md")
				if err != nil {
					t.Fatal(err)
				}
				return text
			},
			wantRoleFile: true,
		},
		{name: "idle renders the reload prompt", state: State{Strand: "s1", Phase: PhaseIdle}, wantRoleFile: true},
		{name: "handoff-requested renders the reload prompt", state: State{Strand: "s1", Phase: PhaseHandoffRequested}, wantRoleFile: true},
		{name: "clearing renders the reload prompt", state: State{Strand: "s1", Phase: PhaseClearing}, wantRoleFile: true},
		{name: "resuming with no pending pointer renders the reload prompt", state: State{Strand: "s1", Phase: PhaseResuming}, wantRoleFile: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, stencils := testPaths(t), seedStencils(t)
			if err := SaveState(p, tt.state); err != nil {
				t.Fatal(err)
			}
			want := ""
			if tt.wantText != nil {
				want = tt.wantText(t, p, stencils)
			} else {
				var err error
				if want, err = RenderReloadPrompt(stencils, p.RolePath); err != nil {
					t.Fatal(err)
				}
			}

			got, err := ResumeContext(p, stencils, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("ResumeContext = %q, want %q", got, want)
			}
			mark, found, err := ReadResumeMark(p)
			if err != nil || !found || mark.Text != want || !mark.At.Equal(now) {
				t.Errorf("mark = %+v, %v, %v; want the text %q at %v", mark, found, err, want, now)
			}
			if _, err := os.Stat(p.RolePath); (err == nil) != tt.wantRoleFile {
				t.Errorf("role file rendered = %v, want %v", err == nil, tt.wantRoleFile)
			}
		})
	}
}

func TestResumeContext_FailureReturnsErrorAndWritesNoMark(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(t *testing.T, p Paths, stencils string)
	}{
		{"missing stencil", func(t *testing.T, p Paths, stencils string) {
			if err := SaveState(p, State{Strand: "s1", Phase: PhaseIdle}); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(stencilstore.Path(stencils, reloadStencilName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"no state file", func(*testing.T, Paths, string) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, stencils := testPaths(t), seedStencils(t)
			tt.setup(t, p, stencils)
			if text, err := ResumeContext(p, stencils, time.Now()); err == nil {
				t.Fatalf("ResumeContext = %q, nil; want an error", text)
			}
			if _, found, err := ReadResumeMark(p); err != nil || found {
				t.Errorf("mark found = %v, %v; want none", found, err)
			}
		})
	}
}
