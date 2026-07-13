//go:build integration

// validate_test.go covers the validate verb's envelope shapes through
// RunCLI: a clean plan prints {"valid": true, "batches": N}; a plan with
// findings prints an error envelope carrying the findings array; an absent
// plan surfaces ParsePlan's own not-found error. Fixture plans are copied
// from internal/builderengine's own testdata plan fixtures into a scratch
// worktree's _lyx/plan, seeded via lyxtest + SeedConfig per the lyxtest
// Leaf Invariant.

package buildercli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/lyxtest"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// seedBuilderFixture returns a host-hub git fixture with shuttle/mux/
// builder config seeded, chdir'd into the host hub, ready for a builder CLI
// invocation. No weft-prime sibling is created: neither validate nor status
// ever weft-commits.
func seedBuilderFixture(t *testing.T) lyxtest.HostFixture {
	t.Helper()

	fixture := lyxtest.CopyHostHub(t)
	lyxtest.SeedConfig(t, fixture.Hub, map[string]string{
		"shuttle": shuttleengine.ConfigTemplate(),
		"mux":     muxengine.ConfigTemplate(),
		"builder": builderengine.ConfigTemplate(),
	})
	t.Chdir(fixture.Hub)
	return fixture
}

// builderengineTestdataDir and seedPlanFixture (the pure file-I/O plan
// fixture helpers, no git spawn) live in testdata_test.go, untagged, so
// Tier 1's run_test.go can use them without depending on this file.

func TestRunCLI_Validate_CleanPlan(t *testing.T) {
	fixture := seedBuilderFixture(t)
	seedPlanFixture(t, fixture.Hub, builderengineTestdataDir("plan-valid"))

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"validate"})

	if exitCode != 0 {
		t.Fatalf("RunCLI([validate]) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"valid":true`) {
		t.Errorf("output missing valid:true; got %q", got)
	}
	if !strings.Contains(got, `"batches":5`) {
		t.Errorf("output missing batches:5; got %q", got)
	}
}

func TestRunCLI_Validate_FindingsEnvelope(t *testing.T) {
	fixture := seedBuilderFixture(t)
	seedPlanFixture(t, fixture.Hub, builderengineTestdataDir("plan-unapproved"))

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"validate"})

	if exitCode != 1 {
		t.Fatalf("RunCLI([validate]) = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", got)
	}
	if !strings.Contains(got, `"findings"`) {
		t.Errorf("output missing findings array; got %q", got)
	}
	if !strings.Contains(got, "plan-unapproved") {
		t.Errorf("output missing plan-unapproved check name; got %q", got)
	}
}

func TestRunCLI_Validate_NoPlan(t *testing.T) {
	seedBuilderFixture(t)

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"validate"})
	if exitCode != 1 {
		t.Fatalf("RunCLI([validate]) = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "plan overview not found") {
		t.Errorf("output missing plan-not-found message; got %q", out.String())
	}
}
