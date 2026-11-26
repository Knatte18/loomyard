// runsettings.go holds the one validation step for run settings and run profiles, and their layering.

package configreg

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"

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

// PrepareRunSettings validates a task's run settings.
// The settings are decoded from JSON as module name to an object of that module's keys.
// It returns each named module's local-layer YAML document, and drops a module whose object is empty.
// A module that is not registered is refused.
// A top-level key the module's Spec maps to a RetiredWayForward or HubOnlyWayForward text is refused with that text.
// A top-level old name of the module's Spec.MovedKeys is refused naming both full names, with no way forward.
// Every refusal is a [*RunSettingsError]; value semantics are left to the module's load.
// Nil or empty settings return an empty map.
func PrepareRunSettings(settings map[string]any) (map[string][]byte, error) {
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
		if err := refuseRetiredKeys(module, keys); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		if err := refuseMovedKeys(module, keys); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		if err := refuseHubOnlyKeys(module, keys); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		document, err := yaml.Marshal(wholeNumbersAsIntegers(keys))
		if err != nil {
			return nil, &RunSettingsError{Module: name, Cause: configengine.MarkInvalid(err)}
		}
		if err := checkLocalDocument(module, document); err != nil {
			return nil, &RunSettingsError{Module: name, Cause: err}
		}
		documents[name] = document
	}
	return documents, nil
}

// refuseRetiredKeys refuses the first key, in name order, that the module's Spec maps to a RetiredWayForward text.
// The refusal is marked [configengine.ErrInvalid] and names the key in full.
// It ends with that text as its way forward.
func refuseRetiredKeys(module Module, keys map[string]any) error {
	wayForward := module.Spec().RetiredWayForward
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if text, retired := wayForward[key]; retired {
			return configengine.MarkInvalid(fmt.Errorf("key %q is retired; way forward: %s", configengine.KeyName(module.Name, key), text))
		}
	}
	return nil
}

// refuseMovedKeys refuses the first key, in name order, that is an old name in the module's Spec.MovedKeys.
// An old name that is also the area its new path lies under is refused only while its value is not an object.
// The refusal is marked [configengine.ErrInvalid] and names both full names.
// It carries no way forward, since each caller prints its own.
func refuseMovedKeys(module Module, keys map[string]any) error {
	moved := module.Spec().MovedKeys
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		newPath, isOld := moved[key]
		if !isOld {
			continue
		}
		if _, isArea := keys[key].(map[string]any); isArea && strings.HasPrefix(newPath, key+".") {
			continue
		}
		return configengine.MarkInvalid(fmt.Errorf("key %s was moved to %s", configengine.KeyName(module.Name, key), configengine.KeyName(module.Name, newPath)))
	}
	return nil
}

// refuseHubOnlyKeys refuses the first key, in name order, that the module's Spec maps to a HubOnlyWayForward text.
// The refusal is marked [configengine.ErrInvalid] and names the key in full.
// It ends with that text as its way forward.
func refuseHubOnlyKeys(module Module, keys map[string]any) error {
	wayForward := module.Spec().HubOnlyWayForward
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if text, hubOnly := wayForward[key]; hubOnly {
			return configengine.MarkInvalid(fmt.Errorf("key %q is set only in the hub's config; way forward: %s", configengine.KeyName(module.Name, key), text))
		}
	}
	return nil
}

// LayerRunSettings returns the local-layer documents of a task's run settings layered over the run profile profileName.
// Both maps are validated per module before the merge.
// Each module's task document is laid over its profile document key by key.
// It is merged with yamlengine.Merge, using the module's open maps.
// A task key replaces the profile's, and a profile key the task lacks stays.
// The merged document is checked again.
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

// MoveSettings moves the old keys of one module-keyed settings object in document, as each module's moved keys name them.
// prefix holds one key per level leading to the object, and an empty prefix names the document root.
// It returns the moved bytes and one line per change, naming both full names.
// Comments and every key it does not move are kept, and a document with no old key comes back unchanged.
func MoveSettings(document []byte, prefix []string) ([]byte, []string, error) {
	var lines []string
	for _, module := range Modules() {
		moves := module.Spec().MovedKeys
		if len(moves) == 0 {
			continue
		}
		under := append(slices.Clone(prefix), module.Name)
		moved, changes, err := yamlengine.MoveKeys(document, under, moves)
		if err != nil {
			return nil, nil, fmt.Errorf("move the old keys of %s: %w", module.Name, err)
		}
		document = moved
		for _, change := range changes {
			oldName, newName := configengine.KeyName(module.Name, change.Old), configengine.KeyName(module.Name, change.New)
			switch change.Outcome {
			case yamlengine.MoveDone:
				lines = append(lines, fmt.Sprintf("moved %s to %s", oldName, newName))
			case yamlengine.MoveDroppedNull:
				lines = append(lines, fmt.Sprintf("dropped %s, which was null", oldName))
			case yamlengine.MoveDroppedNewSet:
				lines = append(lines, fmt.Sprintf("dropped %s, since %s is set", oldName, newName))
			}
		}
	}
	return document, lines, nil
}

// MoveRunSettings moves the old keys of a module-keyed settings object decoded from JSON.
// It returns the moved object and one line per change, naming both full names.
// An object with no old key comes back as it was, with no line.
func MoveRunSettings(settings map[string]any) (map[string]any, []string, error) {
	document, err := yaml.Marshal(wholeNumbersAsIntegers(settings))
	if err != nil {
		return nil, nil, fmt.Errorf("marshal the run settings: %w", err)
	}
	moved, lines, err := MoveSettings(document, nil)
	if err != nil || len(lines) == 0 {
		return settings, nil, err
	}
	var object map[string]any
	if err := yaml.Unmarshal(moved, &object); err != nil {
		return nil, nil, fmt.Errorf("decode the moved run settings: %w", err)
	}
	return object, lines, nil
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

// checkProfiles loads the run profiles under dirs and prepares each recipe block of each profile as run settings.
// A refusal names the profile and the recipe.
func checkProfiles(dirs configengine.Dirs) error {
	profiles, err := runprofile.Load(dirs)
	if err != nil {
		return err
	}
	for _, name := range profiles.Names() {
		profile := profiles.ByName[name]
		for _, recipe := range slices.Sorted(maps.Keys(profile)) {
			if _, err := PrepareRunSettings(profile[recipe]); err != nil {
				return fmt.Errorf("run profile %q, recipe %q: %w", name, recipe, err)
			}
		}
	}
	return nil
}

// wholeNumbersAsIntegers rewrites every whole-valued float64 in a JSON-decoded value to an int64,
// so it marshals as a YAML integer.
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
