// landingdeps_test.go drift-guards landingDeps: it asserts every field of the returned
// landingshed.Deps is populated, via a reflection-based walk rather than an enumerated list of field
// assertions, so a newly added field is caught automatically rather than silently passing. This test
// needs no fixture, no hubforge, and no git init -- landingDeps performs no I/O -- so it stays Tier 1.

package loomcli

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestLandingDeps_EveryFieldPopulated asserts landingDeps populates every field of the
// landingshed.Deps it returns, walking the struct by reflection so a sixteenth field added later is
// caught automatically rather than silently passing an enumerated list of assertions.
func TestLandingDeps_EveryFieldPopulated(t *testing.T) {
	t.Parallel()

	loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}
	geom := websterengine.Geometry{
		WebsterDir:  "/webster",
		StencilsDir: "/stencils",
	}
	pushBranch := func() error { return nil }
	registry := modelspec.Registry{"claude/sonnet-5": {}}
	runner := &shuttleengine.Runner{}
	cfg := landingshed.Config{Squash: true}

	deps := landingDeps(
		loc,
		geom,
		"task/foo",
		"https://example.com/origin.git",
		"main",
		true,
		pushBranch,
		registry,
		runner,
		cfg,
	)

	v := reflect.ValueOf(deps)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		if field.IsZero() {
			t.Errorf("landingDeps(...).%s is the zero value; want it populated", typ.Field(i).Name)
		}
	}
}
