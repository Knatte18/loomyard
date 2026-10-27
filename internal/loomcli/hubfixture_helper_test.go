// hubfixture_helper_test.go holds the hub, config and verdict fixtures the tier0 and tier1 tests of this package share.
// It carries no build tag, so it compiles into every tag set.

package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// seedLoomConfig creates <anchorPath>/_lym/config/loom.yaml with the embedded template's contents.
// It seeds the one config load loomengine.LoadConfig performs strictly.
func seedLoomConfig(t *testing.T, anchorPath string) {
	t.Helper()
	configDir := filepath.Join(anchorPath, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "loom.yaml")
	if err := os.WriteFile(cfgPath, []byte(loomengine.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// hubDeploy is the deploy value the hub's landing.yaml carries in hubLocation, so a wired config reading a pair's copy instead differs from it.
const hubDeploy = "./deploy-hub.sh"

// seedLandingConfig creates <baseDir>/_lym/config/landing.yaml with the embedded template's contents, its deploy set to deploy.
// landingshed.LoadConfig is strict (an absent file is an error), so wire() fails on every test that wires a hub without this seed.
func seedLandingConfig(t *testing.T, baseDir, deploy string) {
	t.Helper()
	configDir := filepath.Join(baseDir, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	cfgPath := filepath.Join(configDir, "landing.yaml")
	contents := strings.Replace(landingshed.ConfigTemplate(), `deploy: ""`, fmt.Sprintf("deploy: %q", deploy), 1)
	if err := os.WriteFile(cfgPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", cfgPath, err)
	}
}

// hubLocation returns a *lymcwd.Location standing in for a real hub location.
// Its anchor path is seeded on disk with loom.yaml and a pair landing.yaml that blanks deploy,
// and its hub is seeded with loom's repo default, the hub-wide landing.yaml at the board dir and both loom stencils.
func hubLocation(t *testing.T, worktreeName, anchorRel string) *lymcwd.Location {
	t.Helper()
	hub := t.TempDir()
	loc := &lymcwd.Location{HubPath: hub, WorktreeName: worktreeName, AnchorRel: anchorRel}
	// hubgeom.ReedGeometry stats the worktree's .git entry to tell the prime from a task worktree.
	if err := os.MkdirAll(filepath.Join(loc.WorktreePath(), ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) = %v; want nil", err)
	}
	seedLoomConfig(t, loc.AnchorPath())
	seedLoomConfig(t, fabricengine.BoardDir(hub))
	seedLandingConfig(t, fabricengine.BoardDir(hub), hubDeploy)
	seedLandingConfig(t, loc.AnchorPath(), "")
	// stencilstore.Read hard-errors on a missing file, so the stencils the Spec closures read are seeded into the hub's stencils directory.
	stencilkit.SeedInto(t, fabricengine.StencilsDir(hub))
	return loc
}

// hubLocationAt returns a Location whose anchor is anchorPath, under a hub that holds the hub-wide language.yaml languageContent when that is non-empty and none otherwise.
// The hub is anchorPath's parent directory, so a test's plan fixtures and its language file share one temp tree.
func hubLocationAt(t *testing.T, anchorPath, languageContent string) *lymcwd.Location {
	t.Helper()
	loc := &lymcwd.Location{HubPath: filepath.Dir(anchorPath), WorktreeName: filepath.Base(anchorPath), AnchorRel: "."}
	if languageContent != "" {
		languagePath := configengine.ConfigFile(fabricengine.BoardDir(loc.HubPath), "language")
		if err := os.MkdirAll(filepath.Dir(languagePath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(languagePath), err)
		}
		if err := os.WriteFile(languagePath, []byte(languageContent), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", languagePath, err)
		}
	}
	return loc
}

// seedDarnHubConfig writes the hub-wide darn.yaml into hub's board directory with verifyAttempts substituted into the template.
// Each old, new pair of replacements is applied after it.
func seedDarnHubConfig(t *testing.T, hub, verifyAttempts string, replacements ...string) {
	t.Helper()
	configDir := filepath.Join(fabricengine.BoardDir(hub), "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	contents := strings.Replace(darnengine.ConfigTemplate(), "verify_attempts: 3", "verify_attempts: "+verifyAttempts, 1)
	contents = strings.NewReplacer(replacements...).Replace(contents)
	if err := os.WriteFile(filepath.Join(configDir, "darn.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile darn.yaml = %v; want nil", err)
	}
}

// writeHubGateConfig writes contents as the hub-wide gate.yaml in hub's board directory.
func writeHubGateConfig(t *testing.T, hub, contents string) {
	t.Helper()
	configDir := filepath.Join(fabricengine.BoardDir(hub), "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", configDir, err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "gate.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile gate.yaml = %v; want nil", err)
	}
}

// parityVerdict is the shared three-valued outcome both the producer side and the CLI side map onto before comparison.
// A string-typed constant set, rather than raw producer outcomes compared against raw exit codes, makes a mismatch failure message name the two sides' verdicts readably.
type parityVerdict string

const (
	verdictDone  parityVerdict = "done"
	verdictStuck parityVerdict = "stuck"
	verdictError parityVerdict = "error"
)

// producerVerdict maps a shuttleengine.Gate closure's raw (GateResult, error) onto a parityVerdict.
// A non-nil error is verdictError, GateResult.Passed == false is verdictStuck, and GateResult.Passed == true is verdictDone.
func producerVerdict(result shuttleengine.GateResult, err error) parityVerdict {
	if err != nil {
		return verdictError
	}
	if !result.Passed {
		return verdictStuck
	}
	return verdictDone
}

// cliVerdict maps a decoded verb envelope onto a parityVerdict.
// ok == true is verdictDone, ok == false with a "findings" key present is verdictStuck, and ok == false without a "findings" key is verdictError.
// The findings-key presence check is structural, never a matter of message wording.
func cliVerdict(env envelope.Envelope) parityVerdict {
	if env.OK {
		return verdictDone
	}
	if _, hasFindings := env.Raw["findings"]; hasFindings {
		return verdictStuck
	}
	return verdictError
}
