// entries_gatefixer.go implements the optional "fixer" key of a gate element.
// splitGateFixer reads and strips the key before resolveGateSpec runs.
// applyGateFixer hands the row's finished spec to Env.GateFixer.
// Only the constructors of rows that accept the key call these two functions.

package shedrecipe

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateFixerKey is the element key that names the setting holding a row's fixer model-spec.
const gateFixerKey = "fixer"

// GateFixerRequest is what a row hands Env.GateFixer to wrap its gate with a cold fixer.
type GateFixerRequest struct {
	// Row is the name of the row whose gate is wrapped.
	Row string
	// Entry is the name of the gate element that carried the fixer key, the one entry whose failure spawns a fixer.
	Entry string
	// Model is the parsed model-spec the fixer runs on.
	Model modelspec.Spec
	// Spec is the row's complete gate spec, closures included.
	Spec shuttleengine.GateSpec
	// Forked reports whether the row's run evaluates the entries' Forked closures.
	Forked bool
	// SkipForked, when non-nil, reports whether a failing forked evaluation's findings are for the writer alone, so no fixer is spawned.
	SkipForked func(findings string) bool
}

// splitGateFixer reads the optional "fixer" key of each element of cfg's "gates" list.
// It returns cfg without the key, so a row that does not read the key still refuses it as unknown.
// The key names a `<module>.<key>` setting, read through env.Setting, that holds a model-spec;
// an empty value turns the fixer off and yields a nil request, leaving the row as it is without the key.
// An absent or malformed "gates" key is returned unchanged, for the gate resolution to report.
// Every refusal names entry, the element and the way forward.
// It refuses a key that is not a `<module>.<key>` string.
// It refuses a setting that cannot be read or that holds a non-string.
// It refuses a value that the model-spec parser rejects, and a key repeated on two elements of one list.
// env.Setting is required only when an element carries the key.
func splitGateFixer(entry string, cfg Config, env Env) (Config, *GateFixerRequest, error) {
	elems, present, err := configMapList(cfg, "gates")
	if err != nil || !present {
		return cfg, nil, nil
	}
	carrier := -1
	stripped := make([]any, len(elems))
	var request *GateFixerRequest
	for i, elem := range elems {
		raw, carries := elem[gateFixerKey]
		if !carries {
			stripped[i] = map[string]any(elem)
			continue
		}
		if carrier >= 0 {
			return nil, nil, fmt.Errorf("shedrecipe: %s: config key %q element %d repeats %q, which element %d already carries; way forward: keep %q on one element of the list", entry, "gates", i, gateFixerKey, carrier, gateFixerKey)
		}
		carrier = i
		model, on, err := gateFixerModel(entry, i, raw, env)
		if err != nil {
			return nil, nil, err
		}
		if on {
			name, _ := elem["name"].(string)
			request = &GateFixerRequest{Entry: name, Model: model}
		}
		rest := make(map[string]any, len(elem))
		for key, value := range elem {
			if key != gateFixerKey {
				rest[key] = value
			}
		}
		stripped[i] = rest
	}
	if carrier < 0 {
		return cfg, nil, nil
	}
	out := make(Config, len(cfg))
	for key, value := range cfg {
		out[key] = value
	}
	out["gates"] = stripped
	return out, request, nil
}

// gateFixerModel resolves the "fixer" value of element index to its model-spec, reporting false for an empty setting value, which turns the fixer off.
func gateFixerModel(entry string, index int, raw any, env Env) (modelspec.Spec, bool, error) {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("shedrecipe: %s: config key %q element %d: config key %q %s", entry, "gates", index, gateFixerKey, fmt.Sprintf(format, args...))
	}
	reference, isString := raw.(string)
	module, key, dotted := strings.Cut(reference, ".")
	if !isString || !dotted || module == "" || key == "" {
		return modelspec.Spec{}, false, fail("must be a <module>.<key> string, got %v; way forward: name the setting that holds the fixer's model-spec, or remove %q", raw, gateFixerKey)
	}
	if err := requireSeam(entry, "Setting", env.Setting); err != nil {
		return modelspec.Spec{}, false, err
	}
	value, err := env.Setting(module, key)
	if err != nil {
		return modelspec.Spec{}, false, fail("names setting %s, which cannot be read: %v; way forward: name a key of the module's config, or remove %q", reference, err, gateFixerKey)
	}
	text, isString := value.(string)
	if !isString {
		return modelspec.Spec{}, false, fail("names setting %s, which holds %v, want a model-spec string; way forward: set it with `lym config %s`, or \"\" to turn the fixer off", reference, value, module)
	}
	if text == "" {
		return modelspec.Spec{}, false, nil
	}
	spec, err := modelspec.Parse(text)
	if err != nil {
		return modelspec.Spec{}, false, fail("names setting %s, which holds %q: %v; way forward: set the key to a model-spec with `lym config %s`, or \"\" to turn the fixer off", reference, text, err, module)
	}
	return spec, true, nil
}

// applyGateFixer returns spec unchanged for a nil request or a nil env.GateFixer, and otherwise returns what env.GateFixer makes of the request with row and spec filled.
func applyGateFixer(row string, spec shuttleengine.GateSpec, req *GateFixerRequest, env Env) (shuttleengine.GateSpec, error) {
	if req == nil || env.GateFixer == nil {
		return spec, nil
	}
	req.Row = row
	req.Spec = spec
	wrapped, err := env.GateFixer(*req)
	if err != nil {
		return nil, fmt.Errorf("shedrecipe: %s: gate fixer: %w", row, err)
	}
	return wrapped, nil
}
