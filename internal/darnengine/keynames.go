// keynames.go declares the darn.yaml key paths the package's messages cite and renders a key's full name.

package darnengine

import "github.com/Knatte18/loomyard/internal/configengine"

// The darn.yaml key paths that messages cite.
const (
	writerKey    = "writer"
	gateFixerKey = "gate_fixer"
	reportDirKey = "report_dir"
	treeScopeKey = "tree_scope"
)

// keyName returns the full name of the darn.yaml key at path.
func keyName(path string) string {
	return configengine.KeyName(configModule, path)
}
