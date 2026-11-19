// entries_harden.go implements hardenFixEntry, the Constructor for the "HardenFix" registry row.
// It also holds the refusal every Harden row gives when the run carries no hardener campaign.

package shedrecipe

import (
	"fmt"
	"slices"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// The fixed parts of the HardenFix row's gates.
const (
	// hardenVerifyGateKey is the hub gate.yaml key the row's verify gate runs.
	hardenVerifyGateKey = "harden-burler-fix"
	// hardenCleanAttempts is the attempt budget of the row's clean gate.
	hardenCleanAttempts = 3
)

// hardenFixKeys are the keys a HardenFix row's config may carry: a BurlerFix row's, bar the two that choose gates.
var hardenFixKeys = slices.DeleteFunc(slices.Clone(burlerFixKeys), func(key string) bool { return key == "gates" || key == "gate_key" })

// requireHarden refuses a nil Env.Harden, naming entry and what needs the campaign.
func requireHarden(entry, use string, env Env) error {
	if env.Harden == nil {
		return fmt.Errorf("shedrecipe: %s: %s needs Env.Harden, the run's hardener campaign, which only a hardener run's wiring builds; way forward: use the row in the hardener recipe, or remove %s from the row", entry, use, use)
	}
	return nil
}

// hardenFixEntry is the Constructor for the "HardenFix" registry row.
// The row is a BurlerFix row over Env.Harden whose gates are fixed.
// The `clean` gate has three attempts.
// The `verify` gate runs the hub key `harden-burler-fix` with the campaign's verify attempts.
// The `live` gate is the campaign's live gate, with its live attempts and PassOnCap.
// PassOnCap ends the row once the live attempts are spent and leaves the red live report for the facts.
// It refuses a `gates` or `gate_key` key, a profile whose fix-scope is not source, and a nil Env.Harden.
// It returns the producer wrapped in the campaign's fix round row.
func hardenFixEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	for _, key := range []string{"gates", "gate_key"} {
		if _, present := cfg[key]; present {
			return nil, fmt.Errorf("shedrecipe: HardenFix: row %q config key %q is refused: the HardenFix row's gates are fixed to clean, verify and live; way forward: remove the key from the recipe row", row.Name, key)
		}
	}
	if err := requireHarden("HardenFix", "the HardenFix row", env); err != nil {
		return nil, err
	}
	runSubdir, err := configString(cfg, "run_subdir", true)
	if err != nil {
		return nil, err
	}
	profileCfg, err := configMap(cfg, "profile", true)
	if err != nil {
		return nil, err
	}
	fixScope, err := configString(profileCfg, "fix-scope", false)
	if err != nil {
		return nil, err
	}
	if fixScope != string(burlerengine.FixScopeSource) {
		return nil, fmt.Errorf("shedrecipe: HardenFix: row %q profile fix-scope is %q; way forward: set the profile's fix-scope to %q, which the campaign's fixer commits through", row.Name, fixScope, burlerengine.FixScopeSource)
	}
	if err := requireAbsRoot("HardenFix", "RunRoot", env.RunRoot); err != nil {
		return nil, err
	}
	runDir, err := resolveUnderRoot("HardenFix", "run_subdir", env.RunRoot, runSubdir)
	if err != nil {
		return nil, err
	}

	producer, err := newBurlerFixProducer(row, cfg, env, func() (shuttleengine.GateSpec, error) {
		return hardenGateSpec(row, runDir, env)
	}, hardenFixKeys)
	if err != nil {
		return nil, err
	}
	return env.Harden.WrapRow(producer, row.Name, hardenshed.RowFix, runDir), nil
}

// hardenGateSpec returns the HardenFix row's fixed gates over the campaign, in the order they run.
func hardenGateSpec(row RowIdentity, runDir string, env Env) (shuttleengine.GateSpec, error) {
	clean, err := resolvePlainGateClosure("HardenFix", row.Name, hardenVerifyGateKey, 0, "clean", env)
	if err != nil {
		return nil, err
	}
	verify, err := resolvePlainGateClosure("HardenFix", row.Name, hardenVerifyGateKey, 1, "verify", env)
	if err != nil {
		return nil, err
	}
	cfg := env.Harden.Config
	return shuttleengine.GateSpec{
		{Name: "clean", Gate: clean, Attempts: hardenCleanAttempts},
		{Name: "verify", Gate: verify, Attempts: cfg.VerifyAttempts},
		{Name: "live", Gate: env.Harden.LiveGate(runDir), Attempts: cfg.LiveAttempts, PassOnCap: true},
	}, nil
}
