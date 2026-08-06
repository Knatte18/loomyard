// set.go implements the non-interactive `lyx config <module> --set key=value`
// write path: scaffold-if-missing plus a single yamlengine.SetValues mutation,
// with no editor invocation and no validation loop. It shares scaffoldIfMissing
// with Edit so both entry points create and roll back a fresh default-valued
// file identically.

package configengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// Set writes pairs into module's config file under baseDir, scaffolding from
// template if needed. Unlike Edit, it never opens an editor and never loops on
// validation failure. Removes freshly-scaffolded files on error, mirroring Edit's
// contract. Returns the sorted list of pre-existing keys not in template that
// were preserved verbatim; nil on any error.
func Set(baseDir, module, template string, pairs []yamlengine.KV) ([]string, error) {
	// Check that baseDir is initialized.
	if _, err := FindBaseDir(baseDir); err != nil {
		return nil, err
	}

	path := ConfigFile(baseDir, module)
	configDir := ConfigDir(baseDir)

	scaffolded, err := scaffoldIfMissing(path, configDir, template)
	if err != nil {
		return nil, err
	}

	removeIfScaffolded := func() {
		if scaffolded {
			_ = os.Remove(path)
		}
	}

	existingBytes, err := os.ReadFile(path)
	if err != nil {
		removeIfScaffolded()
		return nil, err
	}

	result, err := yamlengine.SetValues([]byte(template), existingBytes, pairs)
	if err != nil {
		removeIfScaffolded()
		return nil, err
	}

	if len(result.Unknown) > 0 {
		removeIfScaffolded()
		return nil, fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))
	}

	if err := os.WriteFile(path, result.Merged, 0o644); err != nil {
		removeIfScaffolded()
		return nil, err
	}

	return result.Preserved, nil
}
