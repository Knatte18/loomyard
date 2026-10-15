// runsettings.go — the one migrate-then-validate step for a task's run settings.

package configreg

import (
	"fmt"
	"math"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// RunSettingsError is a refusal of a task's run settings.
// Module names the module refused; Cause is the reason, which for a key-level refusal names the key.
type RunSettingsError struct {
	Module string
	Cause  error
}

// Error names the module and the cause.
func (e *RunSettingsError) Error() string {
	return fmt.Sprintf("run_settings for module %q: %v", e.Module, e.Cause)
}

// Unwrap returns the cause, so a key refusal still matches [configengine.ErrInvalid].
func (e *RunSettingsError) Unwrap() error {
	return e.Cause
}

// PrepareRunSettings migrates and validates a task's run settings, decoded from JSON as module name to an object of that module's keys.
// It returns each named module's local-layer YAML document, and drops a module whose object is empty.
// A key a declared move relocated (see KeyMoves) is first rewritten into its target module's object.
// Every refusal is a [*RunSettingsError]; value semantics are left to the module's load.
// A nil or empty settings returns an empty map.
func PrepareRunSettings(settings map[string]any) (map[string][]byte, error) {
	settings = moveRunSettings(settings)
	names := make([]string, 0, len(settings))
	for name := range settings {
		names = append(names, name)
	}
	sort.Strings(names)

	documents := map[string][]byte{}
	for _, name := range names {
		module, ok := Lookup(name)
		if !ok {
			return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(fmt.Errorf("module %q is not registered", name))}
		}
		keys, ok := settings[name].(map[string]any)
		if !ok {
			return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(fmt.Errorf("the value must be an object of keys"))}
		}
		if len(keys) == 0 {
			continue
		}
		document, err := yaml.Marshal(wholeNumbersAsIntegers(keys))
		if err != nil {
			return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(err)}
		}
		if module.Migrate != nil {
			document, _, err = module.Migrate(document)
			if err != nil {
				return nil, &RunSettingsError{Module: name, Cause: err}
			}
		}
		if err := configengine.ValidateLocal(module.Spec(), document); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		if module.CheckValues != nil {
			if err := module.CheckValues(document); err != nil {
				return nil, &RunSettingsError{Module: name, Cause: err}
			}
		}
		documents[name] = document
	}
	return documents, nil
}

// wholeNumbersAsIntegers rewrites every whole-valued float64 in a JSON-decoded value to an int64, so it marshals as a YAML integer.
func wholeNumbersAsIntegers(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		converted := make(map[string]any, len(typed))
		for key, entry := range typed {
			converted[key] = wholeNumbersAsIntegers(entry)
		}
		return converted
	case []any:
		converted := make([]any, len(typed))
		for i, entry := range typed {
			converted[i] = wholeNumbersAsIntegers(entry)
		}
		return converted
	case float64:
		if typed == math.Trunc(typed) && math.Abs(typed) < 1<<53 {
			return int64(typed)
		}
	}
	return value
}
