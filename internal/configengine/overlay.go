// overlay.go implements the template overlay: the one composer of a module's effective template, the base-layer selector that picks it and the refusals around that selector.
// Every load, write and reconcile path judges a document against the effective template, never the bare spec template.

package configengine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// Overlay describes a module's optional template overlay.
// Selector names the top-level template key whose scalar value in the base layer picks one overlay.
// Documents maps each selector value to its overlay document.
// The zero Overlay means the module has none.
type Overlay struct {
	Selector  string
	Documents map[string][]byte
}

// EffectiveTemplate returns the template a document is judged against:
// the spec's template with the overlay document picked by the selector in base laid over it.
// base is the base layer's document, nil when absent, and hub says whether the module is read inside a hub, which only picks the way forward of a refusal.
// An unknown or non-scalar selector value is an [ErrInvalid] error naming a way forward.
func EffectiveTemplate(spec Spec, base []byte, hub bool) ([]byte, error) {
	value, _, err := selectorValue(spec, base)
	if err != nil {
		return nil, MarkInvalid(fmt.Errorf("%w; way forward: %s", err, selectorWayForward(spec, hub)))
	}
	return overlayTemplate(spec, value, hub)
}

// TemplateUnder returns the effective template picked by the base layer's file under dirs, an absent file picking none.
// It is how a caller judging a file above the base obtains its template without reading a layer file itself.
func TemplateUnder(dirs Dirs, spec Spec) ([]byte, error) {
	path, applies := LayerFile(dirs, spec, BaseLayer(dirs, spec))
	if !applies {
		return spec.Template, nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	return EffectiveTemplate(spec, data, dirs.Board != "")
}

// overlayTemplate composes the effective template for a selector value.
// An empty value or a spec with no overlay returns the template unchanged;
// otherwise the value's document is merged over the template under the spec's open maps, so an overlay name under a per-name map replaces or adds one entry whole.
func overlayTemplate(spec Spec, value string, hub bool) ([]byte, error) {
	if value == "" || len(spec.Overlay.Documents) == 0 {
		return spec.Template, nil
	}
	document, ok := spec.Overlay.Documents[value]
	if !ok {
		names := make([]string, 0, len(spec.Overlay.Documents))
		for name := range spec.Overlay.Documents {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, MarkInvalid(fmt.Errorf("%s config: %s %q names no shipped layer (shipped: %s); way forward: %s", spec.Name, spec.Overlay.Selector, value, strings.Join(names, ", "), selectorWayForward(spec, hub)))
	}
	merged, err := yamlengine.Merge(spec.Template, document, spec.OpenMaps, spec.WholeEntries)
	if err != nil {
		return nil, MarkInvalid(fmt.Errorf("%s config: %s %q layer: %w", spec.Name, spec.Overlay.Selector, value, err))
	}
	return merged, nil
}

// selectorWayForward is the way forward of a refusal of a base-layer selector value.
func selectorWayForward(spec Spec, hub bool) string {
	if hub {
		return fmt.Sprintf("run \"lym config %s --layer default --set %s=<name>\" from the hub's prime worktree%s", spec.Name, spec.Overlay.Selector, boardDirWayForwardClause)
	}
	return fmt.Sprintf("run \"lym config %s --set %s=<name>\"", spec.Name, spec.Overlay.Selector)
}

// selectorValue reads the selector's top-level scalar from doc.
// held reports whether the key is present; an absent key or an empty value yields "".
// A non-scalar value is an [ErrInvalid] error naming the key, to which the caller adds the way forward.
func selectorValue(spec Spec, doc []byte) (value string, held bool, err error) {
	selector := spec.Overlay.Selector
	if selector == "" {
		return "", false, nil
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal(doc, &parsed); err != nil {
		return "", false, MarkInvalid(err)
	}
	node := nodeAt(&parsed, selector)
	if node == nil {
		return "", false, nil
	}
	if node.Kind != yaml.ScalarNode {
		return "", true, MarkInvalid(fmt.Errorf("%s config: %s must be one layer name", spec.Name, selector))
	}
	if node.Tag == "!!null" {
		return "", true, nil
	}
	return node.Value, true, nil
}

// selectorRefusal is the refusal of a worktree file of a hub holding a selector value other than the base's.
// heldBefore says the file held the selector before the write or edit being refused, so its line must be deleted first.
func selectorRefusal(dirs Dirs, spec Spec, path string, heldBefore bool) error {
	basePath, _ := LayerFile(dirs, spec, BaseLayer(dirs, spec))
	selector := spec.Overlay.Selector
	var wayForward strings.Builder
	if heldBefore {
		fmt.Fprintf(&wayForward, "delete the %s line from %s in an editor, then ", selector, path)
	}
	fmt.Fprintf(&wayForward, "set it in the base with \"lym config %s --layer default --set %s=<name>\" from the hub's prime worktree%s", spec.Name, selector, boardDirWayForwardClause)
	return MarkInvalid(fmt.Errorf("config file %s holds %s, which only the base file %s may hold; way forward: %s", path, selector, basePath, wayForward.String()))
}

// baseDocument returns the bytes of the base layer among layers, nil when it is absent.
func baseDocument(dirs Dirs, spec Spec, layers []LayerDoc) []byte {
	for _, layer := range layers {
		if layer.Layer == BaseLayer(dirs, spec) {
			return layer.Bytes
		}
	}
	return nil
}

// worktreeSelectorOther reports whether the worktree document of a hub holds the selector with a value other than the base document's.
// A non-scalar value counts as other.
func worktreeSelectorOther(spec Spec, worktree, base []byte) (other bool, err error) {
	value, held, err := selectorValue(spec, worktree)
	if err != nil {
		return true, nil
	}
	if !held {
		return false, nil
	}
	baseValue, _, err := selectorValue(spec, base)
	if err != nil {
		return false, MarkInvalid(fmt.Errorf("%w; way forward: %s", err, selectorWayForward(spec, true)))
	}
	return value != baseValue, nil
}
