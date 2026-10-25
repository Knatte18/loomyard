//go:build tier0

// conformance_test.go holds the checks every registry entry must pass, ranged over Names() so a new entry cannot skip them silently.

package shedrecipe

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
)

// minimalEntry builds the Env and Config an entry needs to construct successfully.
type minimalEntry struct {
	env func(t *testing.T) Env
	cfg func(t *testing.T, env Env) Config
}

// emptyConfig is the Config of an entry that requires no keys.
func emptyConfig(*testing.T, Env) Config { return Config{} }

// minimalEntries maps every registry name to a valid construction input.
func minimalEntries() map[string]minimalEntry {
	withLanding := func(t *testing.T) Env {
		env := newTestEnv(t)
		env.Landing = validLandingDeps(t)
		return env
	}
	return map[string]minimalEntry{
		"Preflight":       {newTestEnv, emptyConfig},
		"ParentAlign":     {withLanding, emptyConfig},
		"DarnWrite":       {darnTestEnv, emptyConfig},
		"Finalize":        {withLanding, emptyConfig},
		"FrictionReflect": {newTestEnv, emptyConfig},
		"LoomPreflight":   {newTestEnv, emptyConfig},
		"Implementer":     {newImplementerTestEnv, implementerTestConfig},
		"DiscussionWrite": {newTestEnv, emptyConfig},
		"Describe": {
			env: newDescribeTestEnv,
			cfg: func(*testing.T, Env) Config { return gatesCfg("description", 3) },
		},
		"BeamDescribe": {
			env: func(t *testing.T) Env {
				env := newTestEnv(t)
				env.BeamDescribe = landingshed.BeamDescribeDeps{
					Slug:            "demo",
					DescriptionPath: filepath.Join(t.TempDir(), "summary.md"),
					Entry:           func() (string, string, error) { return "Title", "Brief", nil },
					CommitSubjects:  func() ([]string, error) { return nil, nil },
					Commit:          func() error { return nil },
				}
				return env
			},
			cfg: emptyConfig,
		},
		"PlanWrite": {newTestEnv, emptyConfig},
		"Stub":      {newTestEnv, emptyConfig},
		"Bouncer": {
			env: newTestEnv,
			cfg: minimalBouncerConfig,
		},
		"BurlerReview": {
			env: newTestEnv,
			cfg: func(*testing.T, Env) Config { return minimalReviewConfig() },
		},
		"BurlerFix": {
			env: newTestEnv,
			cfg: func(*testing.T, Env) Config { return minimalFixConfig() },
		},
		"MultiLLM": {
			env: func(t *testing.T) Env {
				env := newTestEnv(t)
				writeMultiLLMStencils(t, env)
				return env
			},
			cfg: func(*testing.T, Env) Config { return validMultiLLMConfig() },
		},
		"WorktreeCreate":   {newTestEnv, emptyConfig},
		"InnerRun":         {newTestEnv, emptyConfig},
		"Land":             {newTestEnv, emptyConfig},
		"SeedChild":        {newTestEnvWithSeedChild, emptyConfig},
		"WorktreeTeardown": {newTestEnv, emptyConfig},
		"Command":          {newCommandTestEnv, commandTestConfig},
		"PRReview":         {newPRReviewTestEnv, prReviewTestConfig},
		"DriftBaseline": {
			env: driftTestEnv,
			cfg: func(*testing.T, Env) Config { return Config{"subject": "decision-record"} },
		},
		"DriftAwait": {driftTestEnv, emptyConfig},
		"DriftVerify": {
			env: driftTestEnv,
			cfg: func(*testing.T, Env) Config {
				return Config{"subject": "decision-record", "gates": []any{map[string]any{"name": "verify", "attempts": 3}}}
			},
		},
	}
}

func TestEveryEntry_RejectsUnknownConfigKey(t *testing.T) {
	minimal := minimalEntries()
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			row, ok := minimal[name]
			if !ok {
				t.Fatalf("registry entry %q has no row in minimalEntries; add one so its unknown-config-key rejection is checked", name)
			}
			constructor, err := Lookup(name)
			if err != nil {
				t.Fatalf("Lookup(%q) error = %v; want nil", name, err)
			}

			env := row.env(t)
			if producer, err := constructor(reviewRow("row-name"), row.cfg(t, env), env); err != nil || producer == nil {
				t.Fatalf("%s() with its minimal config = %v, %v; want a producer", name, producer, err)
			}

			cfg := row.cfg(t, env)
			cfg["bogus_key"] = "x"
			_, err = constructor(reviewRow("row-name"), cfg, env)
			assertErrContains(t, err, "bogus_key")
		})
	}
}
