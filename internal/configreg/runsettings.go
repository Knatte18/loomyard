// runsettings.go — the one migrate-then-validate step for a task's run settings and its run profile's, and their layering.

package configreg

import (
	"fmt"
	"math"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/runprofile"
	"github.com/Knatte18/loomyard/internal/yamlengine"
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
// A module that is not registered is refused, and one a declared move retired is refused with the way forward of removing its object from the run settings.
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
			if retiredByKeyMove(name) {
				return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(fmt.Errorf("module %q is not registered: its keys moved to another module; way forward: remove the %s object from run_settings", name, name))}
			}
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
		if err := checkLocalDocument(module, document); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		documents[name] = document
	}
	return documents, nil
}

// retiredByKeyMove reports whether a declared key move names the module as its source.
func retiredByKeyMove(name string) bool {
	for _, move := range KeyMoves() {
		if move.FromModule == name {
			return true
		}
	}
	return false
}

// LayerRunSettings returns the local-layer documents of a task's run settings layered over the settings of its run profile, named profileName.
// Both are prepared as PrepareRunSettings does, and each module's task document is laid over its profile document key by key through yamlengine.Merge with the module's open maps,
// so a task key replaces the profile's and a profile key the task lacks stays; the merged document is checked again.
// Every refusal is a [*RunSettingsError], one in the profile's settings wrapped with the profile's name.
func LayerRunSettings(profileName string, profile, settings map[string]any) (map[string][]byte, error) {
	documents, err := PrepareRunSettings(profile)
	if err != nil {
		return nil, fmt.Errorf("run profile %q: %w", profileName, err)
	}
	upper, err := PrepareRunSettings(settings)
	if err != nil {
		return nil, err
	}
	for name, document := range upper {
		lower, found := documents[name]
		if !found {
			documents[name] = document
			continue
		}
		module, _ := Lookup(name)
		spec := module.Spec()
		merged, err := yamlengine.Merge(lower, document, spec.OpenMaps, spec.WholeEntries)
		if err != nil {
			return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(err)}
		}
		if err := checkLocalDocument(module, merged); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: fmt.Errorf("layered over run profile %q: %w", profileName, err)}
		}
		documents[name] = merged
	}
	return documents, nil
}

// checkLocalDocument checks a module's local-layer document against the local rule and then the module's value checks.
func checkLocalDocument(module Module, document []byte) error {
	if err := configengine.ValidateLocal(module.Spec(), document); err != nil {
		return err
	}
	if module.CheckValues != nil {
		return module.CheckValues(document)
	}
	return nil
}

// checkProfiles loads the run profiles under dirs and checks each profile's settings as a task's run settings are checked, naming the profile on a refusal.
func checkProfiles(dirs configengine.Dirs) error {
	profiles, err := runprofile.Load(dirs)
	if err != nil {
		return err
	}
	for _, name := range profiles.Names() {
		if _, err := PrepareRunSettings(profiles[name]); err != nil {
			return fmt.Errorf("run profile %q: %w", name, err)
		}
	}
	return nil
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
