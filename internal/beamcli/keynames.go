// keynames.go renders the full names of the beam.yaml keys the package's texts cite.

package beamcli

import (
	"github.com/Knatte18/loomyard/internal/beamsetting"
	"github.com/Knatte18/loomyard/internal/configengine"
)

// pullRequestName returns the full name of the beam key that decides how a landing goes.
func pullRequestName() string {
	return configengine.KeyName(beamsetting.Module, beamsetting.PullRequestKey)
}

// maxRunsName returns the full name of the beam key bounding the runs a dobby drives at once.
func maxRunsName() string {
	return configengine.KeyName(beamsetting.Module, "max_runs")
}
