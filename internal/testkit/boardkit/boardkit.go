// Package boardkit builds board fixtures: a hub whose board directory holds only the board config, and a board file holding given entries.
//
// It gives tests that open the hub's board through boardengine.OpenHub a hub with no git and no other fixture.
// It gives tests that read the board's entries a board file without a Board.
// The kit imports only boardengine, configengine, fabricengine and the standard library.
package boardkit

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

var installReplayOnce sync.Once

// InstallReplay registers boardengine.FabricReplay as a fabricengine Bolt replayer, once per process.
// A test binary never runs the lym root pre-run that registers it, so a test that reaches a Bolt replay or the ended-set rule calls this first.
// The registration is never restored, so parallel tests share the one registration of the same value.
func InstallReplay(tb testing.TB) {
	tb.Helper()
	installReplayOnce.Do(func() {
		fabricengine.RegisterBoltReplayer(boardengine.FabricReplay{})
	})
}

// HubWithBoardConfig makes a temp hub whose Bolt checkout holds the default board config and nothing else, and returns the hub path.
// It fails the test on a setup error.
func HubWithBoardConfig(t *testing.T) string {
	t.Helper()
	hub := t.TempDir()
	configFile := configengine.ConfigFile(fabricengine.BoltDir(hub), "board")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(configFile), err)
	}
	if err := os.WriteFile(configFile, []byte(boardengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", configFile, err)
	}
	return hub
}

// WriteBoard writes the board file under boardDir holding entries, each one entry's JSON object, creating boardDir when it is absent and replacing any board file there.
// It fails the test on a setup error.
func WriteBoard(t *testing.T, boardDir string, entries ...string) {
	t.Helper()
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", boardDir, err)
	}
	path := filepath.Join(boardDir, boardengine.BoardFileRel())
	content := `{"version":1,"entries":[` + strings.Join(entries, ",") + `]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}
