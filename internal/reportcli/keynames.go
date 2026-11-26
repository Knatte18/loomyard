// keynames.go renders the full name of the report.yaml key the package's help cites.

package reportcli

import "github.com/Knatte18/loomyard/internal/configengine"

// costMinRunsName returns the full name of the key giving the fewest runs a cost cell reports.
func costMinRunsName() string {
	return configengine.KeyName("report", "cost_min_runs")
}
