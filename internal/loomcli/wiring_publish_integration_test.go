//go:build tier1

// wiring_publish_integration_test.go drives wireLanding over a real hub and pair, so the landing deps carry what wire loaded from the hub.

package loomcli

import (
	"os"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWireLanding_TellsLanguageSkills asserts wireLanding hands the hub's language skills, as wire loaded them, to the landing deps over a real hub and pair.
func TestWireLanding_TellsLanguageSkills(t *testing.T) {
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "languagelanding"
	hubforge.AddPair(t, hub, slug)
	location, err := lyxcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	boardDir := fabricengine.BoardDir(location.HubPath)
	if err := os.MkdirAll(configengine.ConfigDir(boardDir), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v; want nil", err)
	}
	if err := os.WriteFile(configengine.ConfigFile(boardDir, "language"), []byte("skills:\n  - scribe:golang-testing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(language.yaml) error = %v; want nil", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	if err := c.wireLanding(); err != nil {
		t.Fatalf("wireLanding() = %v; want nil", err)
	}

	if want := []string{"scribe:golang-testing"}; !slices.Equal(c.env.Landing.LanguageSkills, want) {
		t.Errorf("env.Landing.LanguageSkills = %v; want %v", c.env.Landing.LanguageSkills, want)
	}
}
