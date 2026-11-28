// reinterpret.go carries a hub's profiles.yaml across the template's drop of the starting profiles' arras blocks.

package runprofile

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// ArrasDefaultReinterpretationID names the change that stopped the starting profiles covering the arras recipe.
const ArrasDefaultReinterpretationID = "profiles-arras-default-dropped"

// ReinterpretArrasDefault repairs the bytes of a hub profiles.yaml that its arras default keeps from loading.
// It drops defaults.arras when the profile it names holds no arras block.
// It keeps comments and every other key.
// The drop does not keep what the default meant, so it is reported as one flagged line with a way forward.
// A default written through an alias or a merge key is flagged for a hand edit, and the file is left as it is.
// A file with no such default is returned unchanged, with no line.
// The worktrees argument is unused, as the rewrite judges the file alone.
func ReinterpretArrasDefault(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the profiles config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil, nil
	}
	// The values are judged decoded, so a block reached through an alias or a merge key counts.
	var entries map[string]any
	if err := document.Decode(&entries); err != nil {
		return nil, nil, nil, fmt.Errorf("decode the profiles config: %w", err)
	}
	defaults, _ := entries[DefaultsEntry].(map[string]any)
	name, named := defaults[shedrun.RecipeArras].(string)
	if !named {
		return existing, nil, nil, nil
	}
	profile, _ := entries[name].(map[string]any)
	if _, covered := profile[shedrun.RecipeArras]; covered {
		return existing, nil, nil, nil
	}
	if !removeMappingKey(mappingValue(document.Content[0], DefaultsEntry), shedrun.RecipeArras) {
		line := fmt.Sprintf("%s.%s: %s names a profile with no %s block, so the file does not load, and it is written through an alias or a merge key; "+
			"way forward: remove that default by hand with `lym config profiles`", DefaultsEntry, shedrun.RecipeArras, name, shedrun.RecipeArras)
		return existing, nil, []string{line}, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, nil, fmt.Errorf("write the profiles config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("write the profiles config: %w", err)
	}
	line := fmt.Sprintf("%s.%s: %s is dropped, even where another notice lists it as kept, since profile %s holds no %s block, "+
		"so such a task naming no profile now runs on the hub's own config; "+
		"way forward: to keep a default, add the block with `lym config profiles --set %s.%s='<json object>'`, then set the default again",
		DefaultsEntry, shedrun.RecipeArras, name, name, shedrun.RecipeArras, name, shedrun.RecipeArras)
	return out.Bytes(), nil, []string{line}, nil
}

// mappingValue returns the value node mapping holds under key, nil when it holds none.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeMappingKey removes key and its value from mapping, and reports whether mapping held it as its own key.
// A nil mapping holds none.
func removeMappingKey(mapping *yaml.Node, key string) bool {
	if mapping == nil {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = slices.Delete(mapping.Content, i, i+2)
			return true
		}
	}
	return false
}
