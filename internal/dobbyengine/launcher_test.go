// launcher_test.go pins when a dobby start adds the hub's desktop launcher and when a catch-up removes it.
// The tests read XDG_DESKTOP_DIR, which tmuxkit.Main points at the run directory.
// Each row keeps a shortname of its own there.

package dobbyengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/desktoplauncher"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeOwnersBoard writes a board.json whose prime owner is primeOwner in a new board directory.
// A beamOwner that is not empty adds a beam project named pay owned by it.
// It returns the directory.
func writeOwnersBoard(t *testing.T, primeOwner, beamOwner string) string {
	t.Helper()
	boardDir := filepath.Join(t.TempDir(), "board")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entries := ""
	if beamOwner != "" {
		entries = fmt.Sprintf(`{"id":1,"slug":"pay","title":"pay","kind":"project","beam":true,"owner_machine":%q}`, beamOwner)
	}
	board := fmt.Sprintf(`{"version":1,"entries":[%s],"prime_owner":%q}`, entries, primeOwner)
	if err := os.WriteFile(filepath.Join(boardDir, "board.json"), []byte(board), 0o644); err != nil {
		t.Fatal(err)
	}
	return boardDir
}

// testDesktop returns the desktop XDG_DESKTOP_DIR names, and removes the launcher of shortname when the test ends.
func testDesktop(t *testing.T, shortname string) desktoplauncher.Desktop {
	t.Helper()
	desktop, err := desktoplauncher.Locate()
	if err != nil {
		t.Fatalf("desktoplauncher.Locate() = %v; want the desktop tmuxkit.Main set", err)
	}
	t.Cleanup(func() {
		if err := desktop.Remove(shortname); err != nil {
			t.Errorf("remove the launcher of %s: %v", shortname, err)
		}
	})
	return desktop
}

// launcherStands reports whether the launcher of shortname stands on desktop.
func launcherStands(t *testing.T, desktop desktoplauncher.Desktop, shortname string) bool {
	t.Helper()
	shortnames, err := desktop.Shortnames()
	if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(shortnames, shortname)
}

func TestRun_StartAddsTheDesktopLauncherOfAWorktreeOwnedHere(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	here, err := machinename.ReadIdentity()
	if err != nil {
		t.Fatal(err)
	}

	// Every row's board has this machine own something the catch-up keeps the launcher for,
	// so the launcher stands afterwards only when the start added it.
	tests := []struct {
		name      string
		shortname string
		// owner and elsewhere are what the worktree's owner read answers.
		owner     string
		elsewhere bool
		// boardPrimeOwner and boardBeamOwner are the owners the hub's board records.
		boardPrimeOwner string
		boardBeamOwner  string
		want            bool
	}{
		{name: "OwnedHere", shortname: "start-owned-here", owner: here.Name, boardPrimeOwner: here.Name, want: true},
		{name: "OwnedElsewhere", shortname: "start-owned-elsewhere", owner: "far-away", elsewhere: true, boardPrimeOwner: here.Name},
		{name: "NoRecordedOwner", shortname: "start-no-owner", boardPrimeOwner: here.Name},
		{name: "BeamOwnedHereUnderAPrimeOwnedElsewhere", shortname: "start-beam-here", owner: here.Name, boardPrimeOwner: "far-away", boardBeamOwner: here.Name, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desktop := testDesktop(t, tt.shortname)
			h := newLoopHarness(t, 1)
			h.geom.LymExecutable = "/opt/lym"
			h.deps.Owner = func() (string, bool, error) { return tt.owner, tt.elsewhere, nil }
			h.deps.CatchUp = fakeCatchUpDeps(func() error { return nil })
			h.deps.CatchUp.Launcher = Launcher{BoardDir: writeOwnersBoard(t, tt.boardPrimeOwner, tt.boardBeamOwner), Shortname: tt.shortname}

			if err := h.run(t); err != nil {
				t.Fatalf("Run() = %v; want nil", err)
			}

			if got := launcherStands(t, desktop, tt.shortname); got != tt.want {
				t.Errorf("launcher of %s stands after the start and its catch-up = %v; want %v", tt.shortname, got, tt.want)
			}
		})
	}
}

//testtiming:keep pins the catch-up removing a launcher that stands, which the start test never plants
func TestCatchUp_RemovesTheDesktopLauncherOnceThisMachineOwnsNothing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	here, err := machinename.ReadIdentity()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		shortname  string
		primeOwner string
		want       bool
	}{
		{name: "PrimeOwnedHere", shortname: "catch-up-owned-here", primeOwner: here.Name, want: true},
		{name: "OwnsNothing", shortname: "catch-up-owns-nothing", primeOwner: "far-away"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			desktop := testDesktop(t, tt.shortname)
			if err := desktop.Add("/opt/lym", tt.shortname); err != nil {
				t.Fatal(err)
			}
			deps := fakeCatchUpDeps(func() error { return nil })
			deps.Launcher = Launcher{BoardDir: writeOwnersBoard(t, tt.primeOwner, ""), Shortname: tt.shortname}

			if _, err := CatchUp(deps); err != nil {
				t.Fatalf("CatchUp() = %v; want nil", err)
			}

			if got := launcherStands(t, desktop, tt.shortname); got != tt.want {
				t.Errorf("launcher of %s stands after the catch-up = %v; want %v", tt.shortname, got, tt.want)
			}
		})
	}
}

func TestOwnsAnything_ReadsEveryOwnerThroughTheRegistry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	here := machinename.Identity{ID: "id-here", Name: "osl46"}
	registry := boardengine.Registry{
		"id-here":  {Name: "osl46", Former: []string{"osl-1046"}},
		"id-other": {Name: "neve"},
	}

	tests := []struct {
		name   string
		owners boardengine.Owners
		want   bool
	}{
		{name: "PrimeOwnedHere", owners: boardengine.Owners{Prime: "osl46", Registry: registry}, want: true},
		{name: "PrimeOwnedUnderAFormerName", owners: boardengine.Owners{Prime: "osl-1046", Registry: registry}, want: true},
		{
			name:   "BeamOwnedHere",
			owners: boardengine.Owners{Prime: "neve", Registry: registry, Entries: map[string]boardengine.OwnerEntry{"pay": {Owner: "osl46", IsProject: true, Beam: true}}},
			want:   true,
		},
		{
			name:   "DoneBeamOwnedHere",
			owners: boardengine.Owners{Prime: "neve", Registry: registry, Entries: map[string]boardengine.OwnerEntry{"pay": {Owner: "osl46", IsProject: true, Beam: true, Done: true}}},
		},
		{name: "EverythingOwnedElsewhere", owners: boardengine.Owners{Prime: "neve", Registry: registry, Entries: map[string]boardengine.OwnerEntry{"pay": {Owner: "neve"}}}},
		{name: "NoOwnerRecorded", owners: boardengine.Owners{Registry: registry}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OwnsAnything(tt.owners, here); got != tt.want {
				t.Errorf("OwnsAnything(%+v) = %v; want %v", tt.owners, got, tt.want)
			}
		})
	}
}
