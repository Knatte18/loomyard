// projectrelease_test.go covers the release of a project with its draft member tasks in projectrelease.go.

package boardengine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newReleaseBoard returns a board holding entries, already marked draft so its first write drafts no released task.
func newReleaseBoard(t *testing.T, entries ...string) *boardengine.Board {
	t.Helper()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) { boardPath = cfg.Path })
	content := `{"version":1,"draft_status":true,"entries":[` + strings.Join(entries, ",") + `]}`
	if err := os.WriteFile(filepath.Join(boardPath, "board.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed board.json: %v", err)
	}
	return b
}

// statusOf returns the status of the entry slug names, "<none>" when it has none.
func statusOf(t *testing.T, b *boardengine.Board, slug string) string {
	t.Helper()
	task, found, err := b.GetTask(slug)
	if err != nil || !found {
		t.Fatalf("GetTask(%s) = found %v, %v; want the entry", slug, found, err)
	}
	if task.Status == nil {
		return "<none>"
	}
	return *task.Status
}

// TestSetStatusReleasingProject_ReleasesDraftMembers pins the release of a project with an empty status.
// A null status releases the project and its direct draft member tasks, in one write.
// Released, run-held and done members, a nested project's members and outsiders stay as they were.
func TestSetStatusReleasingProject_ReleasesDraftMembers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	b := newReleaseBoard(t,
		`{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"],"status":""}`,
		`{"id":1,"slug":"d1","title":"D1","kind":"task","labels":["bug"],"project":"p","status":"draft"}`,
		`{"id":2,"slug":"released","title":"R","kind":"task","labels":["bug"],"project":"p"}`,
		`{"id":3,"slug":"running","title":"Run","kind":"task","labels":["bug"],"project":"p","status":"running · Darn-Write"}`,
		`{"id":4,"slug":"landed","title":"L","kind":"task","labels":["bug"],"project":"p","status":"done"}`,
		`{"id":5,"slug":"nested","title":"N","kind":"project","labels":["bug"],"project":"p"}`,
		`{"id":6,"slug":"nested-member","title":"NM","kind":"task","labels":["bug"],"project":"nested","status":"draft"}`,
		`{"id":7,"slug":"outsider","title":"O","kind":"task","labels":["bug"],"status":"draft"}`,
		`{"id":8,"slug":"d2","title":"D2","kind":"task","labels":["bug"],"project":"p","status":"draft"}`,
	)

	released, err := b.SetStatusReleasingProject("p", nil)
	if err != nil {
		t.Fatalf("SetStatusReleasingProject(p, nil) error = %v; want nil", err)
	}
	if want := []string{"d1", "d2"}; !slices.Equal(released, want) {
		t.Errorf("SetStatusReleasingProject(p, nil) = %v; want %v", released, want)
	}
	wantStatus := map[string]string{
		"p":             "<none>",
		"d1":            "<none>",
		"d2":            "<none>",
		"released":      "<none>",
		"running":       "running · Darn-Write",
		"landed":        "done",
		"nested":        "<none>",
		"nested-member": "draft",
		"outsider":      "draft",
	}
	for slug, want := range wantStatus {
		if got := statusOf(t, b, slug); got != want {
			t.Errorf("status of %s = %q; want %q", slug, got, want)
		}
	}
}

// TestSetStatusReleasingProject_Refusals pins that a refused project release writes nothing.
// A member failing its own release's validation is named in the refusal.
// A project with a run status or done is refused too.
// That refusal names each draft member's release as the way forward, or says there is none to release.
func TestSetStatusReleasingProject_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	draftMember := func(id int, slug string) string {
		return fmt.Sprintf(`{"id":%d,"slug":%q,"title":"T","kind":"task","labels":["bug"],"project":"p","status":"draft"}`, id, slug)
	}
	tests := []struct {
		name string
		// project is the entry "p"; members and extra follow it on the board, and members stay draft after the refusal.
		project string
		members []string
		extra   []string
		wantHas []string
	}{
		{
			name:    "MemberFailingValidation",
			project: `{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"]}`,
			members: []string{"d1"},
			extra:   []string{`{"id":2,"slug":"bad","title":"Bad","kind":"task","labels":["bug"],"project":"p","status":"draft","depends_on":["ghost"]}`},
			wantHas: []string{`"p"`, `member "bad"`, "dangling dependency"},
		},
		{
			name:    "RunStatusProjectNamesEveryMember",
			project: `{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"],"beam":true,"status":"running · Webster"}`,
			members: []string{"d1", "d2"},
			wantHas: []string{`"p"`, `lym board set-status '{"slug":"d1","status":null}'; lym board set-status '{"slug":"d2","status":null}'`},
		},
		{
			name:    "DoneProject",
			project: `{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"],"status":"done"}`,
			members: []string{"d1"},
			wantHas: []string{`"p"`, `lym board set-status '{"slug":"d1","status":null}'`},
		},
		{
			name:    "DoneProjectWithoutDraftMember",
			project: `{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"],"status":"done"}`,
			wantHas: []string{`"p"`, "has no draft member task to release"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries := []string{tt.project}
			for i, slug := range tt.members {
				entries = append(entries, draftMember(i+1, slug))
			}
			b := newReleaseBoard(t, append(entries, tt.extra...)...)
			wantProjectStatus := statusOf(t, b, "p")
			released, err := b.SetStatusReleasingProject("p", nil)
			if err == nil {
				t.Fatalf("SetStatusReleasingProject(p, nil) = %v, nil; want a refusal", released)
			}
			for _, want := range tt.wantHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("SetStatusReleasingProject(p, nil) error = %q; want it to contain %q", err, want)
				}
			}
			for _, slug := range tt.members {
				if got := statusOf(t, b, slug); got != "draft" {
					t.Errorf("status of %s after the refusal = %q; want draft", slug, got)
				}
			}
			if got := statusOf(t, b, "p"); got != wantProjectStatus {
				t.Errorf("status of p after the refusal = %q; want %q", got, wantProjectStatus)
			}
		})
	}
}

// TestSetStatusReleasingProject_OtherWritesReleaseNoMember pins which writes release members.
// Only a null status on a project with an empty status does.
// A plain status write, an upsert, a non-null status and a null status on a task release none.
// Nor does a project with a hand-set status.
func TestSetStatusReleasingProject_OtherWritesReleaseNoMember(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	paused := "paused"
	writes := []struct {
		name          string
		projectStatus string
		write         func(b *boardengine.Board) ([]string, error)
		wantProject   string
		wantD1        string
		wantD3        string
	}{
		{"SetStatus", `""`, func(b *boardengine.Board) ([]string, error) { return nil, b.SetStatus("p", nil) }, "<none>", "draft", "done"},
		{"UpsertTask", `""`, func(b *boardengine.Board) ([]string, error) {
			_, err := b.UpsertTask(map[string]any{"slug": "p", "status": nil})
			return nil, err
		}, "<none>", "draft", "done"},
		{"ProjectWithAHandSetStatus", `"paused"`, func(b *boardengine.Board) ([]string, error) {
			return b.SetStatusReleasingProject("p", nil)
		}, "<none>", "draft", "done"},
		{"NonNullStatusOnProject", `""`, func(b *boardengine.Board) ([]string, error) {
			return b.SetStatusReleasingProject("p", &paused)
		}, "paused", "draft", "done"},
		{"NullStatusOnTask", `""`, func(b *boardengine.Board) ([]string, error) {
			return b.SetStatusReleasingProject("d1", nil)
		}, "", "<none>", "done"},
		// A done task is not refused as a done project is: only a project takes the refusal.
		{"NullStatusOnDoneTask", `""`, func(b *boardengine.Board) ([]string, error) {
			return b.SetStatusReleasingProject("d3", nil)
		}, "", "draft", "<none>"},
	}
	for _, tt := range writes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newReleaseBoard(t,
				`{"id":0,"slug":"p","title":"P","kind":"project","labels":["bug"],"status":`+tt.projectStatus+`}`,
				`{"id":1,"slug":"d1","title":"D1","kind":"task","labels":["bug"],"project":"p","status":"draft"}`,
				`{"id":2,"slug":"d2","title":"D2","kind":"task","labels":["bug"],"project":"p","status":"draft"}`,
				`{"id":3,"slug":"d3","title":"D3","kind":"task","labels":["bug"],"project":"p","status":"done"}`,
			)
			released, err := tt.write(b)
			if err != nil {
				t.Fatalf("write error = %v; want nil", err)
			}
			if len(released) != 0 {
				t.Errorf("released members = %v; want none", released)
			}
			if got := statusOf(t, b, "p"); got != tt.wantProject {
				t.Errorf("status of p = %q; want %q", got, tt.wantProject)
			}
			if got := statusOf(t, b, "d1"); got != tt.wantD1 {
				t.Errorf("status of d1 = %q; want %q", got, tt.wantD1)
			}
			if got := statusOf(t, b, "d2"); got != "draft" {
				t.Errorf("status of d2 = %q; want draft", got)
			}
			if got := statusOf(t, b, "d3"); got != tt.wantD3 {
				t.Errorf("status of d3 = %q; want %q", got, tt.wantD3)
			}
		})
	}
}
