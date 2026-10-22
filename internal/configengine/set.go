// set.go implements the non-interactive `lyx config <module> --set key=value` write path:
// one layer's file is written with a single yamlengine mutation, with no editor invocation and no
// validation loop.
// A standalone or seed-only base scaffolds from the template like Edit does, so a fresh default-valued file is created and rolled back identically;
// a hub's base and every layer above it are sparse and hold only the keys set.
// A local write is checked against the local rule, and a machine write against the machine rule, before anything is written.
// An overlay module's base scaffold holds empty per-name maps, and a hub worktree write picking a layer the base does not pick is refused.

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
	if layer == LayerDefault || layer == LayerMachine {
		owner = dirs.Board
	}
	return path, owner, nil
}

// Set writes pairs into the layer's file of spec under dirs.
// It never opens an editor and never loops on validation failure.
// A standalone or seed-only base layer is scaffolded from the template when absent and written with yamlengine.SetValues, and a freshly-scaffolded file is removed on error, mirroring Edit's contract;
// a hub's base layer and every layer above the base are sparse, so only the keys set are written through yamlengine.SetSparse and nothing is scaffolded.
// A write to the local layer is checked with [ValidateLocal] first, and one to the machine layer with [ValidateMachine]; a refused one writes nothing.
// A spec with an overlay judges the write against its effective template: the one the written base document picks, or for a layer above the base the one the base file picks;
// an unknown selector value fails before anything is written, and a hub worktree write leaving the selector with a value other than the default file's is refused.
// A spec with JSONEntries writes each pair as one whole top-level entry through yamlengine.SetEntries on every layer, scaffolding an absent base layer from the template.
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
	effective, err := writeTemplate(dirs, spec, layer, path, pairs)
	if err != nil {
		return nil, err
	}
	spec.Template = effective

	if spec.JSONEntries {
		return nil, setEntries(spec, layer, path, layer == BaseLayer(dirs, spec), pairs)
	}
	if layer != BaseLayer(dirs, spec) || sparseBase(dirs, spec) {
		return setSparse(dirs, spec, layer, path, pairs)
	}

	scaffolded, err := scaffoldIfMissing(path, filepath.Dir(path), BaseScaffold(dirs, spec))
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

	result, err := yamlengine.SetValues(spec.Template, existingBytes, pairs, spec.PerNameMaps, spec.OpenMaps...)
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

// setEntries writes each pair as a whole top-level entry into the layer file at path.
// An absent base file starts from the template and any other absent file from nothing;
// a local or machine write is checked against its layer's rule first, and a refused write leaves the file as it was.
func setEntries(spec Spec, layer Layer, path string, base bool, pairs []yamlengine.KV) error {
	existing, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err) && base:
		existing = spec.Template
	case err != nil && !os.IsNotExist(err):
		return err
	}
	merged, err := yamlengine.SetEntries(existing, pairs)
	if err != nil {
		return err
	}
	if err := validateOverride(spec, layer, merged); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return os.WriteFile(path, merged, 0o644)
}

// validateOverride checks doc against the rule of layer: [ValidateMachine] for the machine layer, [ValidateLocal] for the local one, and nothing for any other.
func validateOverride(spec Spec, layer Layer, doc []byte) error {
	switch layer {
	case LayerMachine:
		return ValidateMachine(spec, doc)
	case LayerLocal:
		return ValidateLocal(spec, doc)
	}
	return nil
}

// writeTemplate returns the effective template a write to layer is judged against.
// A base-layer write is judged against the template its own document picks: the selector's value is the last pair setting it, else the one the file holds.
// A write to any layer above the base is judged against the template the base layer picks.
func writeTemplate(dirs Dirs, spec Spec, layer Layer, path string, pairs []yamlengine.KV) ([]byte, error) {
	if spec.Overlay.Selector == "" {
		return spec.Template, nil
	}
	if layer != BaseLayer(dirs, spec) {
		return TemplateUnder(dirs, spec)
	}
	for i := len(pairs) - 1; i >= 0; i-- {
		if pairs[i].Key == spec.Overlay.Selector {
			return overlayTemplate(spec, pairs[i].Value, dirs.Board != "")
		}
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return EffectiveTemplate(spec, existing, dirs.Board != "")
}

// setSparse writes pairs into the sparse layer file at path, creating it when absent.
// The merged document is checked against the layer's rule before it is written, so a refused write leaves the file as it was.
// A write to the worktree layer of a hub that would leave the overlay selector holding a value other than the default file's is refused with [selectorRefusal].
func setSparse(dirs Dirs, spec Spec, layer Layer, path string, pairs []yamlengine.KV) ([]string, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	result, err := yamlengine.SetSparse(spec.Template, existing, pairs, spec.PerNameMaps, spec.OpenMaps...)
	if err != nil {
		return nil, err
	}
	if len(result.Unknown) > 0 {
		return nil, fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))
	}
	if hubWorktreeGuardsSelector(dirs, spec, layer) {
		_, heldBefore, _ := selectorValue(spec, existing)
		if err := worktreeSelectorRefusal(dirs, spec, path, result.Merged, heldBefore); err != nil {
			return nil, err
		}
	}
	if err := validateOverride(spec, layer, result.Merged); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, result.Merged, 0o644); err != nil {
		return nil, err
	}
	return result.Preserved, nil
}
