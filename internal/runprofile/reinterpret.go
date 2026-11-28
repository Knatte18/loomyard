// reinterpret.go carries a hub's profiles.yaml across template changes the reconcile's follow alone gets wrong:
// the drop of the starting profiles' arras blocks,
// and the jacquard blocks the profiles gained beside their tapestry blocks.

package runprofile

import (
	"bytes"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// ArrasDefaultReinterpretationID names the change that stopped the starting profiles covering the arras recipe.
const ArrasDefaultReinterpretationID = "profiles-arras-default-dropped"

// JacquardBlocksReinterpretationID names the change that gave each profile holding a tapestry block a jacquard block.
const JacquardBlocksReinterpretationID = "profiles-jacquard-blocks"

// jacquardRecipe names the jacquard recipe, the run of tapestry's rows after Discussion, as a profile block key.
const jacquardRecipe = "jacquard"

// loomModule and discussionArea name the loom module and its Discussion perch's area in a recipe block.
// A jacquard block holds no Discussion area, since a jacquard run has no Discussion row.
const (
	loomModule     = "loom"
	discussionArea = "discussion"
)

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
	defaultsNode := mappingValue(document.Content[0], DefaultsEntry)
	if defaultsNode == nil || holdsBesideOwnKey(defaultsNode, shedrun.RecipeArras) || !removeMappingKey(defaultsNode, shedrun.RecipeArras) {
		line := fmt.Sprintf("%s.%s: %s names a profile with no %s block, so the file does not load, and it is written through an alias or a merge key; "+
			"way forward: remove that default by hand with `lym config profiles`", DefaultsEntry, shedrun.RecipeArras, name, shedrun.RecipeArras)
		return existing, nil, []string{line}, nil
	}
	rewritten, err = encodeDocument(&document)
	if err != nil {
		return nil, nil, nil, err
	}
	line := fmt.Sprintf("%s.%s: %s is dropped, even where another notice lists it as kept, since profile %s holds no %s block, "+
		"so such a task naming no profile now runs on the hub's own config; "+
		"way forward: to keep a default, add the block with `lym config profiles --set %s.%s='<json object>'`, then set the default again",
		DefaultsEntry, shedrun.RecipeArras, name, name, shedrun.RecipeArras, name, shedrun.RecipeArras)
	return rewritten, nil, []string{line}, nil
}

// ReinterpretJacquardBlocks gives the jacquard blocks of a hub profiles.yaml the values the hub set for tapestry runs.
// Each profile holding a tapestry block gets a jacquard block holding that block's settings but loom.discussion.
// That block replaces any jacquard block that differs.
// A profile whose tapestry block the operator tuned so keeps that tuning for jacquard runs.
// defaults.jacquard is set to the profile defaults.tapestry names when that profile then holds a jacquard block.
// It is dropped otherwise.
// It keeps comments and every other key, and returns one rewrote line per changed block or default.
// An aliased profile or defaults entry is rewritten in the mapping its alias names.
// One supplied through a merge key is flagged for a hand edit and left as it is.
// So is a jacquard block or default that would change and holds a YAML anchor.
// Replacing it could leave an alias with no anchor.
// A file whose jacquard blocks and default already match is returned unchanged, with no line.
// The worktrees argument is unused, as the rewrite judges the file alone.
func ReinterpretJacquardBlocks(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
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
	root := document.Content[0]
	holdsJacquard := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(entries)) {
		if name == DefaultsEntry {
			continue
		}
		profile, _ := entries[name].(map[string]any)
		current, held := profile[jacquardRecipe]
		holdsJacquard[name] = held
		tapestry, ok := profile[shedrun.RecipeTapestry].(map[string]any)
		if !ok {
			continue
		}
		want := jacquardSettings(tapestry)
		if reflect.DeepEqual(current, want) {
			continue
		}
		blockKey := configengine.KeyName(ModuleName, name+"."+jacquardRecipe)
		profileNode := ownMapping(root, name)
		if profileNode == nil {
			flagged = append(flagged, fmt.Sprintf("%s: profile %s holds a %s block but is supplied through a merge key, so it gained no %s block from it; "+
				"way forward: add the block by hand with `lym config profiles --set %s.%s='<json object>'`",
				blockKey, name, shedrun.RecipeTapestry, jacquardRecipe, name, jacquardRecipe))
			continue
		}
		if old := mappingValue(profileNode, jacquardRecipe); old != nil && holdsAnchor(old) {
			flagged = append(flagged, fmt.Sprintf("%s: profile %s's %s block carries a YAML anchor another entry may alias, so it is left as it is; "+
				"way forward: in the editor `lym config profiles` opens, write out the anchor and each alias of it, then set the block there",
				blockKey, name, jacquardRecipe))
			continue
		}
		var block yaml.Node
		if err := block.Encode(want); err != nil {
			return nil, nil, nil, fmt.Errorf("encode the %s block of profile %s: %w", jacquardRecipe, name, err)
		}
		setMappingValue(profileNode, jacquardRecipe, &block)
		holdsJacquard[name] = true
		rewrote = append(rewrote, fmt.Sprintf("%s: set to the profile's %s block without loom.%s, so its %s runs take the values its %s runs took; "+
			"way forward: none needed, or tune the block apart with `lym config profiles`",
			blockKey, shedrun.RecipeTapestry, discussionArea, jacquardRecipe, shedrun.RecipeTapestry))
	}

	defaultLines, defaultFlags := reinterpretJacquardDefault(ownMapping(root, DefaultsEntry), entries[DefaultsEntry], holdsJacquard)
	rewrote = append(rewrote, defaultLines...)
	flagged = append(flagged, defaultFlags...)
	if len(rewrote) == 0 {
		return existing, nil, flagged, nil
	}
	rewritten, err = encodeDocument(&document)
	if err != nil {
		return nil, nil, nil, err
	}
	return rewritten, rewrote, flagged, nil
}

// reinterpretJacquardDefault sets or drops defaults.jacquard in defaultsNode.
// It sets it to the profile defaults.tapestry names when holdsJacquard says that profile holds a jacquard block.
// It drops it otherwise.
// holdsJacquard describes the profiles after the blocks' rewrite.
// defaults is the decoded defaults entry.
// A nil defaultsNode stands for a defaults entry supplied through a merge key.
// It returns one rewrote line for a change.
// It returns one flagged line for a change it leaves to a hand edit:
// a default supplied through a merge key or holding a YAML anchor.
func reinterpretJacquardDefault(defaultsNode *yaml.Node, defaults any, holdsJacquard map[string]bool) (rewrote, flagged []string) {
	entries, _ := defaults.(map[string]any)
	tapestryDefault, named := entries[shedrun.RecipeTapestry].(string)
	current, held := entries[jacquardRecipe]
	defaultKey := configengine.KeyName(ModuleName, DefaultsEntry+"."+jacquardRecipe)
	mergeFlag := fmt.Sprintf("%s: the default is supplied through a merge key, so it is left as it is; "+
		"way forward: set it by hand with `lym config profiles` to the profile %s names, or remove it when that profile holds no %s block",
		defaultKey, configengine.KeyName(ModuleName, DefaultsEntry+"."+shedrun.RecipeTapestry), jacquardRecipe)
	anchorFlag := fmt.Sprintf("%s: the default carries a YAML anchor another entry may alias, so it is left as it is; "+
		"way forward: in the editor `lym config profiles` opens, write out the anchor and each alias of it, then set or remove the default there",
		defaultKey)
	var own *yaml.Node
	if defaultsNode != nil {
		own = mappingValue(defaultsNode, jacquardRecipe)
	}
	anchored := own != nil && holdsAnchor(own)
	if named && holdsJacquard[tapestryDefault] {
		if current == tapestryDefault {
			return nil, nil
		}
		if defaultsNode == nil {
			return nil, []string{mergeFlag}
		}
		if anchored {
			return nil, []string{anchorFlag}
		}
		setMappingValue(defaultsNode, jacquardRecipe, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: tapestryDefault})
		return []string{fmt.Sprintf("%s: set to %s, the profile a %s task naming no profile takes; way forward: none needed",
			defaultKey, tapestryDefault, shedrun.RecipeTapestry)}, nil
	}
	if !held {
		return nil, nil
	}
	// Dropping an own key a merge key also supplies would leave the merged default in force.
	if defaultsNode == nil || holdsBesideOwnKey(defaultsNode, jacquardRecipe) {
		return nil, []string{mergeFlag}
	}
	if anchored {
		return nil, []string{anchorFlag}
	}
	removeMappingKey(defaultsNode, jacquardRecipe)
	return []string{fmt.Sprintf("%s: dropped, since %s names no profile holding a %s block, so a %s task naming no profile runs on the hub's own config; "+
		"way forward: to keep a default, set it with `lym config profiles` to a profile holding a %s block",
		defaultKey, configengine.KeyName(ModuleName, DefaultsEntry+"."+shedrun.RecipeTapestry), jacquardRecipe, jacquardRecipe, jacquardRecipe)}, nil
}

// jacquardSettings returns a jacquard block's settings for a profile whose tapestry block holds tapestry:
// every module's keys but the loom module's Discussion area, and no loom module when that area was its only key.
func jacquardSettings(tapestry map[string]any) map[string]any {
	settings := maps.Clone(tapestry)
	loom, ok := settings[loomModule].(map[string]any)
	if !ok {
		return settings
	}
	loom = maps.Clone(loom)
	delete(loom, discussionArea)
	if len(loom) == 0 {
		delete(settings, loomModule)
	} else {
		settings[loomModule] = loom
	}
	return settings
}

// encodeDocument writes document back to YAML bytes at the template's two-space indent.
func encodeDocument(document *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("write the profiles config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("write the profiles config: %w", err)
	}
	return out.Bytes(), nil
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

// holdsAnchor reports whether node or any node under it defines a YAML anchor.
func holdsAnchor(node *yaml.Node) bool {
	return node.Anchor != "" || slices.ContainsFunc(node.Content, holdsAnchor)
}

// holdsBesideOwnKey reports whether mapping still holds key once its own key is left out, as through a merge key.
// mapping must be part of a document that decodes.
func holdsBesideOwnKey(mapping *yaml.Node, key string) bool {
	rest := *mapping
	rest.Content = slices.Clone(mapping.Content)
	removeMappingKey(&rest, key)
	var entries map[string]any
	// The whole document decoded, so this part of it does.
	_ = rest.Decode(&entries)
	_, held := entries[key]
	return held
}

// ownMapping returns the mapping node root holds under key as its own key, following an alias to its anchored mapping.
// A write to an anchored mapping reaches every entry aliasing it, as each holds the same values.
// It returns nil when root holds key only through a merge key, or holds no mapping under it.
func ownMapping(root *yaml.Node, key string) *yaml.Node {
	node := mappingValue(root, key)
	if node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

// setMappingValue sets key's value in mapping to value.
// It writes in place when mapping holds key as its own key, and appends the pair otherwise.
func setMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
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
