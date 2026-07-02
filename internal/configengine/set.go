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

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// Set writes pairs into module's config file under baseDir, scaffolding the
// file from template first if it does not yet exist.
//
// Unlike Edit, Set never opens an editor and never loops on validation
// failure: it is the fully non-interactive counterpart used by the --set CLI
// flag. Every error path removes a freshly-scaffolded file before returning,
// mirroring Edit's abort-removes-scaffold contract, so a failed --set never
// leaves a fresh default-valued file behind on disk.
func Set(baseDir, module, template string, pairs []yamlengine.KV) error {
	// Check that baseDir is initialized.
	if _, err := FindBaseDir(baseDir); err != nil {
		return err
	}

	path := hubgeometry.ConfigFile(baseDir, module)
	configDir := hubgeometry.ConfigDir(baseDir)

	scaffolded, err := scaffoldIfMissing(path, configDir, template)
	if err != nil {
		return err
	}

	// removeIfScaffolded restores the pre-call filesystem state on any later
	// failure, exactly as Edit's abort path does: a failed --set must never
	// leave a fresh default-valued file behind.
	removeIfScaffolded := func() {
		if scaffolded {
			_ = os.Remove(path)
		}
	}

	existingBytes, err := os.ReadFile(path)
	if err != nil {
		removeIfScaffolded()
		return err
	}

	result, err := yamlengine.SetValues([]byte(template), existingBytes, pairs)
	if err != nil {
		removeIfScaffolded()
		return err
	}

	if len(result.Unknown) > 0 {
		removeIfScaffolded()
		return fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))
	}

	if err := os.WriteFile(path, result.Merged, 0o644); err != nil {
		removeIfScaffolded()
		return err
	}

	return nil
}
