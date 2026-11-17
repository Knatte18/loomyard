// stencilpins_integration_test.go pins that a run's first seed records its stencil pins and a re-entry keeps them.
// It runs git, so it is a tier-1 file apart from the tier-0 tests of the pins' use on resume.

package loomcli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestSeedAndCommitBootstrap_PinsStencilsOnTheFirstSeedOnly asserts that the first seed pins every installed stencil,
// and that a re-entry after a stencil changed keeps the first seed's pins.
// It is not parallel: it sets FABRIC_SKIP_GIT, so the bootstrap's commit needs no git repository.
func TestSeedAndCommitBootstrap_PinsStencilsOnTheFirstSeedOnly(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("FABRIC_SKIP_GIT", "1")
	dir := t.TempDir()
	stencilsDir := filepath.Join(dir, "stencils")
	stencilkit.SeedInto(t, stencilsDir)
	location := &lymcwd.Location{HubPath: dir, WorktreeName: "pair", AnchorRel: "."}
	// Writing the origin record seeds the exclude file of the records sibling, which must be a repository.
	records := fabricengine.RecordsWorktree(location)
	if err := os.MkdirAll(records, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", records, err)
	}
	gitkit.Git(t, records, "init")
	c := &loomCLI{
		location: location,
		runDeps:  websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: stencilsDir}},
		shedPaths: shedbuild.ShedPaths{
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}
	wantPins, err := stencilstore.Pins(stencilsDir, stencils.Registry())
	if err != nil {
		t.Fatalf("Pins() error: %v", err)
	}

	if _, _, stage, err := c.seedAndCommitBootstrap("pair", "main"); err != nil {
		t.Fatalf("first seedAndCommitBootstrap() = stage %d, %v; want success", stage, err)
	}
	editInstalledStencil(t, stencilsDir, stencils.Registry().Names()[0])
	if _, _, stage, err := c.seedAndCommitBootstrap("pair", "main"); err != nil {
		t.Fatalf("second seedAndCommitBootstrap() = stage %d, %v; want success", stage, err)
	}

	got, found, err := shedrun.ReadStencilPins(c.location, shedrun.SelfRunID)
	if err != nil || !found {
		t.Fatalf("ReadStencilPins() = found %v, err %v; want the first seed's pins", found, err)
	}
	if !reflect.DeepEqual(got.Stencils, wantPins) {
		t.Errorf("stencil pins after a re-entry differ from the first seed's: got %d entries, want %d", len(got.Stencils), len(wantPins))
	}
}
