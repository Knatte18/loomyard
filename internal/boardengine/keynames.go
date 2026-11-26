// keynames.go renders the full names of the board.yaml label lists the package's refusals cite.

package boardengine

import "github.com/Knatte18/loomyard/internal/configengine"

// typesName returns the full name of the board.yaml list of type labels.
func typesName() string {
	return configengine.KeyName("board", "types")
}

// labelsName returns the full name of the board.yaml list of other labels.
func labelsName() string {
	return configengine.KeyName("board", "labels")
}
