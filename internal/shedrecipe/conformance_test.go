// conformance_test.go holds the checks every registry entry must pass, ranged over Names() so a new entry cannot skip them silently.

package shedrecipe

import (
	"testing"
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
		"Preflight": {newTestEnv, emptyConfig},
		"Publish":   {withLanding, emptyConfig},
		"PRGate": {
			env: func(t *testing.T) Env {
				env := withLanding(t)
				armPRGate(&env.Landing)
				return env
			},
			cfg: emptyConfig,
		},
		"PRRework":        {reworkTestEnv, emptyConfig},
		"Finalize":        {withLanding, emptyConfig},
		"FrictionReflect": {newTestEnv, emptyConfig},
		"LoomPreflight":   {newTestEnv, emptyConfig},
		"Batchifier":      {newTestEnv, emptyConfig},
		"DiscussionWrite": {newTestEnv, emptyConfig},
		"DiscussionSeats": {newTestEnv, emptyConfig},
		"Describe": {
			env: newDescribeTestEnv,
			cfg: func(*testing.T, Env) Config { return gatesCfg("description", 3) },
		},
		"PlanWrite": {newTestEnv, emptyConfig},
		"Stub":      {newTestEnv, emptyConfig},
		"Webster":   {newTestEnv, emptyConfig},
		"SingleLLM": {
			env: func(t *testing.T) Env {
				env, _ := validSingleLLMEnv(t)
				return env
			},
			cfg: func(*testing.T, Env) Config { return validSingleLLMConfig("singlellm-valid") },
		},
		"Bouncer": {
			env: newTestEnv,
			cfg: minimalBouncerConfig,
		},
		"BurlerRound": {
			env: newTestEnv,
			cfg: func(*testing.T, Env) Config { return minimalBurlerConfig() },
		},
		"MultiLLM": {
			env: func(t *testing.T) Env {
				env := newTestEnv(t)
				writeStencilFile(t, env.StencilsDir, "multillm-seat", "Body text with no markers.\n")
				return env
			},
			cfg: func(*testing.T, Env) Config { return validMultiLLMConfig() },
		},
		"WorktreeCreate":   {newTestEnv, emptyConfig},
		"InnerRun":         {newTestEnv, emptyConfig},
		"SeedChild":        {newTestEnvWithSeedChild, emptyConfig},
		"WorktreeTeardown": {newTestEnv, emptyConfig},
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
			if producer, err := constructor("row-name", row.cfg(t, env), env); err != nil || producer == nil {
				t.Fatalf("%s() with its minimal config = %v, %v; want a producer", name, producer, err)
			}

			cfg := row.cfg(t, env)
			cfg["bogus_key"] = "x"
			_, err = constructor("row-name", cfg, env)
			assertErrContains(t, err, "bogus_key")
		})
	}
}
