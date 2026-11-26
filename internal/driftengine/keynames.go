// keynames.go renders the full name of a drift.yaml key.

package driftengine

import "github.com/Knatte18/loomyard/internal/configengine"

// judgeParentTimeoutKey is the drift.yaml key bounding the judge's ask to the parent orch.
const judgeParentTimeoutKey = "judge_parent_timeout_min"

// keyName returns the full name of the drift.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
