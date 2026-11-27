// keynames.go declares the landing.yaml key paths the package's messages cite and renders a key's full name.

package landingshed

import (
	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// configModule is the config module whose file is landing.yaml.
const configModule = "landing"

// The landing.yaml key paths that messages cite.
const (
	conflictKey                 = "conflict"
	describeKey                 = "describe"
	fixerKey                    = "fixer"
	squashKey                   = "squash"
	fixerTimeoutMinKey          = "fixer_timeout_min"
	forbiddenTermsKey           = "describe_forbidden_terms"
	describeReviewKey           = "describe_review"
	describeReviewTimeoutMinKey = "describe_review_timeout_min"
	finalizeLostRacesKey        = "finalize_lost_races"
)

// beamPullRequestName returns the full name of the beam key that decides how a landing goes.
func beamPullRequestName() string {
	return configengine.KeyName(beamsetting.Module, beamsetting.PullRequestKey)
}

// gateKeyName returns the full name of the gate.yaml key at path.
func gateKeyName(path string) string {
	return configengine.KeyName("gate", path)
}

// keyName returns the full name of the landing.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
