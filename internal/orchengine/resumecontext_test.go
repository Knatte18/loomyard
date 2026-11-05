package orchengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// presenceFileIn returns the path of a new presence file holding presence, or of an absent file when presence is empty.
func presenceFileIn(t *testing.T, presence slackengine.Presence) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "presence.json")
	if presence == "" {
		return path
	}
	if _, err := slackengine.WritePresence(path, presence, time.Now()); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResumeContext_RendersPointerPerPhaseAndWritesMark(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	resumeText := func(t *testing.T, p Paths, stencils string) string {
		text, err := RenderResumePrompt(stencils, p.RolePath, "/handoffs/h.md")
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	tests := []struct {
		name         string
		state        State
		wantText     func(t *testing.T, p Paths, stencils string) string
		wantRoleFile bool
		// presenceFile tells the paths to read a presence file, which holds presence, or is absent when presence is empty.
		presenceFile bool
		presence     slackengine.Presence
		// wantLine says the rendered pointer is followed by the presence line.
		wantLine bool
	}{
		{
			name:     "resuming returns the pending pointer verbatim and renders no role file",
			state:    State{Strand: "s1", Phase: PhaseResuming, PendingResume: "read the note at /n"},
			wantText: func(*testing.T, Paths, string) string { return "read the note at /n" },
		},
		{
			name:         "compacting renders the resume prompt naming the note",
			state:        State{Strand: "s1", Phase: PhaseCompacting, LastHandoff: "/handoffs/h.md"},
			wantText:     resumeText,
			wantRoleFile: true,
		},
		{name: "idle renders the reload prompt", state: State{Strand: "s1", Phase: PhaseIdle}, wantRoleFile: true},
		{name: "handoff-requested renders the reload prompt", state: State{Strand: "s1", Phase: PhaseHandoffRequested}, wantRoleFile: true},
		{name: "clearing renders the reload prompt", state: State{Strand: "s1", Phase: PhaseClearing}, wantRoleFile: true},
		{name: "resuming with no pending pointer renders the reload prompt", state: State{Strand: "s1", Phase: PhaseResuming}, wantRoleFile: true},
		{
			name:         "presence slack ends the reload pointer with the presence line",
			state:        State{Strand: "s1", Phase: PhaseIdle},
			presenceFile: true, presence: slackengine.PresenceSlack, wantLine: true, wantRoleFile: true,
		},
		{
			name:         "presence slack ends the resume pointer with the presence line",
			state:        State{Strand: "s1", Phase: PhaseCompacting, LastHandoff: "/handoffs/h.md"},
			wantText:     resumeText,
			presenceFile: true, presence: slackengine.PresenceSlack, wantLine: true, wantRoleFile: true,
		},
		{
			name:         "presence terminal carries no line",
			state:        State{Strand: "s1", Phase: PhaseIdle},
			presenceFile: true, presence: slackengine.PresenceTerminal, wantRoleFile: true,
		},
		{name: "no presence file carries no line", state: State{Strand: "s1", Phase: PhaseIdle}, presenceFile: true, wantRoleFile: true},
		{
			name:         "resuming with a pending pointer that carries the line returns it unchanged",
			state:        State{Strand: "s1", Phase: PhaseResuming, PendingResume: "read the note at /n" + presenceLine},
			wantText:     func(*testing.T, Paths, string) string { return "read the note at /n" + presenceLine },
			presenceFile: true, presence: slackengine.PresenceSlack,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, stencils := testPaths(t), seedStencils(t)
			if tt.presenceFile {
				p.SlackPresencePath = presenceFileIn(t, tt.presence)
			}
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

			if tt.wantLine {
				want += presenceLine
			}

			got, err := ResumeContext(p, stencils, testIndex, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("ResumeContext = %q, want %q", got, want)
			}
			wantLines := 0
			if tt.presence == slackengine.PresenceSlack {
				wantLines = 1
			}
			if lines := strings.Count(got, presenceLine); lines != wantLines {
				t.Errorf("ResumeContext = %q carries the presence line %d times, want %d", got, lines, wantLines)
			}
			mark, found, err := ReadResumeMark(p)
			if err != nil || !found || mark.Text != want || !mark.At.Equal(now) {
				t.Errorf("mark = %+v, %v, %v; want the text %q at %v", mark, found, err, want, now)
			}
			if _, err := os.Stat(p.RolePath); (err == nil) != tt.wantRoleFile {
				t.Errorf("role file rendered = %v, want %v", err == nil, tt.wantRoleFile)
			}
			if tt.wantRoleFile {
				if role, err := os.ReadFile(p.RolePath); err != nil || !strings.Contains(string(role), testIndex) {
					t.Errorf("role file = %q, %v; want it to carry the index %q", role, err, testIndex)
				}
			}
		})
	}
}

func TestResumeContext_FailureReturnsErrorAndWritesNoMark(t *testing.T) {
	tiergate.Require(t, 0)
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
			text, err := ResumeContext(p, stencils, testIndex, time.Now())
			if err == nil {
				t.Fatalf("ResumeContext = %q, nil; want an error", text)
			}
			if _, found, err := ReadResumeMark(p); err != nil || found {
				t.Errorf("mark found = %v, %v; want none", found, err)
			}
		})
	}
}
