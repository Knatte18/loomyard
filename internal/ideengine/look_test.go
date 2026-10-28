//go:build tier0

// look_test.go covers lookFor over a hub of plain directories: the window title from the hub shortname,
// and the project and worktree colours of the prime, a beam and a task under each.

package ideengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// Colours of the palette the hub below uses, in palette order after the prime's green.
const (
	purple = "#7d2d6b"
	blue   = "#2d4f7d"
	brown  = "#7d5c2d"
	violet = "#4a2d7d"
)

// lookHub builds a hub of plain directories: a prime, a beam bm recording violet, a beam bn recording no colour, tasks t1 and t2 under bm, and tasks t3, t4 and t5 under the prime, with settings for t2, t4 and t5.
// A non-empty shortname is recorded on the board.
func lookHub(t *testing.T, shortname string) *lymcwd.Location {
	t.Helper()
	hub := t.TempDir()
	for _, name := range []string{"prime", "bm", "bn", "t1", "t2", "t3", "t4", "t5"} {
		if err := os.MkdirAll(filepath.Join(hub, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	board := fabricengine.BoardDir(hub)
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	boardJSON := `{"version":1,"entries":[` +
		`{"id":0,"slug":"bm","title":"BM","kind":"project","labels":["bug"],"beam":true,"beam_color":"` + violet + `"},` +
		`{"id":1,"slug":"bn","title":"BN","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":2,"slug":"gone","title":"Gone","kind":"project","labels":["bug"],"beam":true,"beam_color":"` + purple + `","status":"done"},` +
		`{"id":3,"slug":"t1","title":"T1","kind":"task","labels":["bug"],"project":"bm"},` +
		`{"id":4,"slug":"t2","title":"T2","kind":"task","labels":["bug"],"project":"bm"}]}`
	if err := os.WriteFile(filepath.Join(board, "board.json"), []byte(boardJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if shortname != "" {
		if err := fabricengine.WriteShortname(board, shortname); err != nil {
			t.Fatal(err)
		}
	}
	writeBars(t, filepath.Join(hub, "t2"), violet, vscode.PrimeColor)
	writeBars(t, filepath.Join(hub, "t4"), vscode.PrimeColor, purple)
	writeBars(t, filepath.Join(hub, "t5"), vscode.PrimeColor, brown)
	return &lymcwd.Location{HubPath: hub, WorktreeName: "prime", AnchorRel: "."}
}

// writeBars writes <dir>/.vscode/settings.json holding title and activity as the title-bar and activity-bar backgrounds.
func writeBars(t *testing.T, dir, title, activity string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"workbench.colorCustomizations": map[string]any{"titleBar.activeBackground": title, "activityBar.background": activity}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vscode", "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLookFor(t *testing.T) {
	t.Parallel()
	l := lookHub(t, "tst")
	tests := []struct {
		slug string
		want vscode.Look
	}{
		{"prime", vscode.Look{Title: "TST: prime", ProjectColor: vscode.PrimeColor, WorktreeColor: vscode.PrimeColor}},
		{"bm", vscode.Look{Title: "TST: bm", ProjectColor: violet, WorktreeColor: violet}},
		{"bn", vscode.Look{Title: "TST: bn", ProjectColor: purple, WorktreeColor: purple}},
		{"t1", vscode.Look{Title: "TST: t1", ProjectColor: violet, WorktreeColor: purple}},
		{"t3", vscode.Look{Title: "TST: t3", ProjectColor: vscode.PrimeColor, WorktreeColor: blue}},
		{"t5", vscode.Look{Title: "TST: t5", ProjectColor: vscode.PrimeColor, WorktreeColor: brown}},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			t.Parallel()
			own, err := lymcwd.NamedWorktree(l.HubPath, tt.slug)
			if err != nil {
				t.Fatal(err)
			}
			if got := lookFor(l, own, tt.slug, "prime"); got != tt.want {
				t.Errorf("lookFor(%s) = %+v; want %+v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestLookForDegradesToATaskUnderThePrime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		slug   string
		damage func(t *testing.T, l *lymcwd.Location)
	}{
		{
			name: "UnreadableBoard",
			slug: "t1",
			damage: func(t *testing.T, l *lymcwd.Location) {
				if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(l.HubPath), "board.json"), []byte("not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "UnresolvableParent",
			slug: "t1",
			damage: func(t *testing.T, l *lymcwd.Location) {
				marker := filepath.Join(fabricengine.BoardDir(l.HubPath), lymcwd.AnchorFileName)
				if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(marker, []byte("../escape"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := lookHub(t, "tst")
			own, err := lymcwd.NamedWorktree(l.HubPath, tt.slug)
			if err != nil {
				t.Fatal(err)
			}
			tt.damage(t, l)

			got := lookFor(l, own, tt.slug, "prime")

			if got.ProjectColor != vscode.PrimeColor || got.WorktreeColor == vscode.PrimeColor {
				t.Errorf("lookFor(%s) = %+v; want green on the title bar and a non-green activity bar", tt.slug, got)
			}
		})
	}
}

func TestLookForWithoutShortnameTitlesTheSlugAlone(t *testing.T) {
	t.Parallel()
	l := lookHub(t, "")
	own, err := lymcwd.NamedWorktree(l.HubPath, "t3")
	if err != nil {
		t.Fatal(err)
	}
	if got := lookFor(l, own, "t3", "prime").Title; got != "t3" {
		t.Errorf("lookFor(t3).Title = %q; want %q", got, "t3")
	}
}
