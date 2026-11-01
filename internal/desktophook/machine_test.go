//go:build tier0 || tier1

// machine_test.go holds the fixture helper both test tiers use: a hub's desktop machine override.

package desktophook_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// newDirsWithMachineConfig returns the Dirs of a fresh board and anchor whose board holds contents as its desktop machine override.
// The board holds no hub file, as on a hub not yet reconciled.
func newDirsWithMachineConfig(t *testing.T, contents string) configengine.Dirs {
	t.Helper()
	dirs := configengine.Dirs{Board: t.TempDir(), Anchor: t.TempDir()}
	if err := os.MkdirAll(configengine.ScratchDir(dirs.Board), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configengine.ScratchDir(dirs.Board), err)
	}
	machinePath := configengine.MachineFile(dirs.Board, "desktop")
	if err := os.WriteFile(machinePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", machinePath, err)
	}
	return dirs
}
