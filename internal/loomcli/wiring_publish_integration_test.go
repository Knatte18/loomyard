// wiring_publish_integration_test.go drives wireLanding over a real hub and pair, so the landing deps carry what wire loaded from the hub.

package loomcli

import (
	"os"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestWireLanding_TellsLanguageSkills asserts wireLanding hands the hub's language skills, as wire loaded them, to the landing deps over a real hub and pair.
func TestWireLanding_TellsLanguageSkills(t *testing.T) {
	tiergate.Require(t, 1)
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "languagelanding"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
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

// TestWireLanding_FillsSamplerSessionSeams asserts wireLanding fills the Sampler-Session row's session, liveness, registration and record watch for a sampler seed, and leaves them nil for a darn seed, over a real hub and pair.
func TestWireLanding_FillsSamplerSessionSeams(t *testing.T) {
	tiergate.Require(t, 1)
	tests := []struct {
		name    string
		recipe  string
		wantSet bool
	}{
		{name: "a sampler seed fills the session seams", recipe: shedrun.RecipeSampler, wantSet: true},
		{name: "a darn seed leaves them nil", recipe: shedrun.RecipeDarn, wantSet: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			const slug = "samplerlanding"
			hubforge.AddPair(t, hub, slug)
			location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
			if err != nil {
				t.Fatalf("ResolveWorktree error = %v; want nil", err)
			}
			configDir := configengine.ConfigDir(fabricengine.BoardDir(location.HubPath))
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatalf("MkdirAll error = %v; want nil", err)
			}
			for module, template := range map[string]string{"darn": darnengine.ConfigTemplate(), "sampler": samplerengine.ConfigTemplate()} {
				if err := os.WriteFile(configengine.ConfigFile(fabricengine.BoardDir(location.HubPath), module), []byte(template), 0o644); err != nil {
					t.Fatalf("WriteFile(%s.yaml) error = %v; want nil", module, err)
				}
			}

			c := &loomCLI{runID: shedrun.SelfRunID, recipe: tt.recipe}
			if err := c.wire(location, location.AnchorPath()); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}
			if err := c.wireLanding(); err != nil {
				t.Fatalf("wireLanding() = %v; want nil", err)
			}

			sampler := c.env.Sampler
			set := sampler.Sessions != nil && sampler.SessionLive != nil && sampler.RegisterSession != nil && sampler.Watch != nil
			unset := sampler.Sessions == nil && sampler.SessionLive == nil && sampler.RegisterSession == nil && sampler.Watch == nil
			if tt.wantSet && !set || !tt.wantSet && !unset {
				t.Errorf("after wireLanding of a %s seed: Sessions nil = %v, SessionLive nil = %v, RegisterSession nil = %v, Watch nil = %v; want them set = %v", tt.recipe, sampler.Sessions == nil, sampler.SessionLive == nil, sampler.RegisterSession == nil, sampler.Watch == nil, tt.wantSet)
			}
		})
	}
}

// TestWire_DescribeTableCarriesTheDriftVerdict asserts the Describe table wire builds holds the regression check rendered from the run's scope record as its drift_verdict value, over a real hub and pair.
func TestWire_DescribeTableCarriesTheDriftVerdict(t *testing.T) {
	tiergate.Require(t, 1)
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	const slug = "describeverdict"
	hubforge.AddPair(t, hub, slug)
	location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree error = %v; want nil", err)
	}
	stencilkit.SeedInto(t, fabricengine.StencilsDir(location.HubPath))
	err = driftengine.UpdateScope(driftengine.NewPaths(location.AnchorPath()), func(driftengine.ScopeRecord, bool) (driftengine.ScopeRecord, error) {
		return driftengine.ScopeRecord{Scope: driftengine.ScopeFull}, nil
	})
	if err != nil {
		t.Fatalf("UpdateScope() error = %v; want nil", err)
	}

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(location, location.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}
	table, err := c.env.DescribeTable()
	if err != nil {
		t.Fatalf("DescribeTable() error = %v; want nil", err)
	}

	const want = "## Regression check\n\n- Scope: full.\n- Compare: not run.\n- Accepted drift: none."
	if got := table.Values["drift_verdict"]; got != want {
		t.Errorf("DescribeTable() drift_verdict = %q; want %q", got, want)
	}
}
