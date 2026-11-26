// keynames.go declares the gate.yaml key paths the package's texts cite and renders a key's full name.

package gatecli

import "github.com/Knatte18/loomyard/internal/configengine"

// configModule is the config module whose file is gate.yaml.
const configModule = "gate"

// The gate.yaml key paths that texts cite.
const (
	busyQuitSecKey            = "busy_quit_sec"
	memoryQuitMBKey           = "memory_quit_mb"
	inertPathsKey             = "inert_paths"
	refuseRawKey              = "refuse_raw"
	scoperKey                 = "scoper"
	gatesKey                  = "gates"
	redundancyBuildTimeoutKey = "redundancy_build_timeout_min"
	redundancyTestTimeoutKey  = "redundancy_test_timeout_sec"
	redundancyTestMemoryMBKey = "redundancy_test_memory_mb"
	redundancyMaxTestsKey     = "redundancy_max_tests"
	redundancyBudgetMinKey    = "redundancy_budget_min"
	reachBudgetMinKey         = "reach_budget_min"
	mutationMaxMutantsKey     = "mutation_max_mutants"
	mutationBudgetMinKey      = "mutation_budget_min"
	tiersKey                  = "tiers"
	// tierMemoryMBLeaf, tierTimeoutMinLeaf and tierFilterLeaf are keys of one tier's mapping.
	tierMemoryMBLeaf   = "memory_mb"
	tierTimeoutMinLeaf = "timeout_min"
	tierFilterLeaf     = "filter"
)

// keyName returns the full name of the gate.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}

// tierKey returns the full name of the gate.yaml mapping of the tier named tier.
func tierKey(tier string) string {
	return keyName(tiersKey + "." + tier)
}

// tierKeyName returns the full name of the key leaf of the gate.yaml tier named tier.
func tierKeyName(tier, leaf string) string {
	return keyName(tiersKey + "." + tier + "." + leaf)
}
