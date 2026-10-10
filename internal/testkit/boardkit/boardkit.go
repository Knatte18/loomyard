// Package boardkit builds a hub fixture whose board directory holds only the board config.
//
// It gives tests that open the hub's board through boardengine.OpenHub a hub with no git and no other fixture.
// The kit imports only boardengine, configengine, fabricengine and the standard library.
package boardkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// HubWithBoardConfig makes a temp hub whose board directory holds the default board config and nothing else, and returns the hub path.
// It fails the test on a setup error.
func HubWithBoardConfig(t *testing.T) string {
	t.Helper()
	hub := t.TempDir()
	configFile := configengine.ConfigFile(fabricengine.BoardDir(hub), "board")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(configFile), err)
	}
	if err := os.WriteFile(configFile, []byte(boardengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", configFile, err)
	}
	return hub
}
