// hub_helper_test.go holds the hub, plan-directory and gate-config seeds the tier0 and tier1 tests of this package share.
// The fixtures are package-local: the internal and external test packages deliberately do not share a test-helper package.

package websterengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// testGateConfig declares tier0 and tier1 and lists them in the `card` and `batch` keys, tier0 alone in `webster-fix`.
// Each call returns a fresh config, so a test may edit its keys.
func testGateConfig() gateslot.Config {
	return gateslot.Config{
		Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}, "tier1": {About: "integration"}},
		Gates: map[string]gateslot.GateKey{
			"card":        {Tiers: []int{0}, Scope: "card"},
			"batch":       {Tiers: []int{0, 1}, Scope: "card"},
			"webster-fix": {Tiers: []int{0}, Scope: "diff"},
		},
	}
}

// seedHubStencils seeds every registry stencil under hub's real fabricengine.StencilsDir(hub) location, the geometry the render functions derive internally before reading through stencilstore.Read.
func seedHubStencils(t *testing.T, hub string) {
	t.Helper()
	stencilkit.SeedInto(t, fabricengine.StencilsDir(hub))
}

// seedPlanDir creates a t.TempDir() seeded with a two-card plan, so the fingerprint gate has something real to hash.
// BeginBatch's fingerprint gate reads planDir directly, never deps.Batches, so no actual plan-format parsing is needed for these tests.
func seedPlanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "go",
		Cards: []plankit.Card{
			{Number: 1, Slug: "json-flag", Summary: "add the json flag", Intent: "placeholder card.", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}},
			{Number: 2, Slug: "list-tests", Summary: "list the tests", Intent: "placeholder card.", Groups: []plankit.Group{{Label: "Prosa", Targets: []string{"base.txt"}}}},
		},
	})
	return dir
}

// mustFingerprint replicates fingerprint.go's own algorithm exactly: SHA-256 over every "*.md" file's name and contents, sorted and NUL-separated.
// A test can then seed a State.PlanFingerprint that matches what BeginBatch will independently recompute.
// The algorithm is package-private and these tests deliberately stay in the external websterengine_test package.
func mustFingerprint(t *testing.T, planDir string) string {
	t.Helper()
	entries, err := os.ReadDir(planDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", planDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// fileSHA is the hex SHA-256 of the file at path, the hash State.PlanFileHashes and BatchState.CardHashes record.
func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
