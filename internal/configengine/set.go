// set.go implements the non-interactive `lyx config <module> --set key=value` write path:
// one layer's file is written with a single yamlengine mutation, with no editor invocation and no
// validation loop.
// The base layer scaffolds from the template like Edit does, so a fresh default-valued file is created and rolled back identically;
// every layer above it is sparse and holds only the keys set.
// A local write is checked against the local rule before anything is written.

package configengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// layerPath returns the file of layer under dirs and the dir whose _lyx/ must exist for it.
// A layer that does not apply under dirs is an error.
func layerPath(dirs Dirs, spec Spec, layer Layer) (path, owner string, err error) {
	path, applies := LayerFile(dirs, spec, layer)
	if !applies {
		return "", "", fmt.Errorf("the %s layer of the %s config does not apply here", layer, spec.Name)
	}
	owner = dirs.Anchor
	if layer == LayerDefault {
		owner = dirs.Board
	}
	return path, owner, nil
}

// Set writes pairs into the layer's file of spec under dirs.
// It never opens an editor and never loops on validation failure.
// The base layer is scaffolded from the template when absent and written with yamlengine.SetValues, and a freshly-scaffolded file is removed on error, mirroring Edit's contract;
// a layer above the base is sparse, so only the keys set are written through yamlengine.SetSparse and nothing is scaffolded.
// A write to the local layer is checked with [ValidateLocal] first, and a refused one writes nothing.
// Returns the sorted list of pre-existing keys not in the template that were preserved verbatim;
// nil on any error.
func Set(dirs Dirs, spec Spec, layer Layer, pairs []yamlengine.KV) ([]string, error) {
	path, owner, err := layerPath(dirs, spec, layer)
	if err != nil {
		return nil, err
	}
	if _, err := FindBaseDir(owner); err != nil {
		return nil, err
	}
	template := string(spec.Template)

	if layer != BaseLayer(dirs, spec) {
		return setSparse(spec, layer, path, pairs)
	}

	scaffolded, err := scaffoldIfMissing(path, filepath.Dir(path), template)
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

	result, err := yamlengine.SetValues(spec.Template, existingBytes, pairs, spec.OpenMaps...)
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

// setSparse writes pairs into the sparse layer file at path, creating it when absent.
// The merged document is checked against the local rule before it is written, so a refused write leaves the file as it was.
func setSparse(spec Spec, layer Layer, path string, pairs []yamlengine.KV) ([]string, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result, err := yamlengine.SetSparse(spec.Template, existing, pairs, spec.OpenMaps...)
	if err != nil {
		return nil, err
	}
	if len(result.Unknown) > 0 {
		return nil, fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))
	}
	if layer == LayerLocal {
		if err := ValidateLocal(spec, result.Merged); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, result.Merged, 0o644); err != nil {
		return nil, err
	}
	return result.Preserved, nil
}
