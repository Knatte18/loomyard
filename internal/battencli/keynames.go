// keynames.go renders the full names of the config keys the package's texts cite.

package battencli

import (
	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// maxRunsName returns the full name of the beam key bounding the runs a dobby drives at once.
func maxRunsName() string {
	return configengine.KeyName(beamsetting.Module, "max_runs")
}

// battenKeyName returns the full name of the batten.yaml key at path.
func battenKeyName(path string) string {
	return configengine.KeyName("batten", path)
}

// childDriverFallback names, for a text, the settings a child run's driver comes from when no flag sets it.
func childDriverFallback() string {
	return "the child recipe's driver_kind, then " + configengine.KeyName("shed", "driver_kind")
}
