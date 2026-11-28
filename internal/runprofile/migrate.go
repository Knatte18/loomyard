// migrate.go holds the reconcile hooks of profiles.yaml.
// It renames blocks held under a recipe's former name, and it lists former blocks beside a differing current one.

package runprofile

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// recipeRename names a recipe's former name and the name that replaced it.
type recipeRename struct {
	from, to string
}

// recipeRenames lists every recipe lym renamed, oldest first.
// No recipe carries a from name, so a block or defaults entry keyed by one is never read.
var recipeRenames = []recipeRename{{from: "loom", to: shedrun.RecipeTapestry}}

// migrateWayForward is the way forward of a refusal that finds a recipe held only under its former name.
const migrateWayForward = `run "lym config reconcile --layer default --apply" from the hub's prime worktree, which renames each block and defaults entry held under a recipe's former name`

// heldFormerName returns the first former name of recipe that entries holds as a key, and reports whether one exists.
func heldFormerName[V any](entries map[string]V, recipe string) (string, bool) {
	for _, rename := range recipeRenames {
		if rename.to != recipe {
			continue
		}
		if _, held := entries[rename.from]; held {
			return rename.from, true
		}
	}
	return "", false
}

// MigrateConfig renames every recipe block or entry keyed by a recipe's former name to the recipe's current name.
// The rename applies in each profile and under defaults, keeping the value.
// That lets a hub seeded before a recipe rename run that recipe on its profiles.
// An entry already holding the current name keeps it, and drops the former block only when it equals the current one;
// a differing former block stays untouched for the operator to settle by hand.
// It returns one line per rewrite.
// A line reads "<entry>.<from>: renamed to <entry>.<to>" or "<entry>.<from>: removed, <entry>.<to> already set".
// When nothing is rewritten, it returns the document unchanged and no lines.
// Comments and other keys survive, because the document is edited as yaml nodes.
// A document that does not parse, or whose root is not a mapping, returns an error marked configengine.ErrInvalid.
func MigrateConfig(existing []byte) ([]byte, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("runprofile: migrate profiles: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, configengine.MarkInvalid(errors.New("runprofile: migrate profiles: the top level is not a mapping"))
	}

	var lines []string
	for i := 0; i+1 < len(root.Content); i += 2 {
		entry, recipes := root.Content[i].Value, root.Content[i+1]
		if recipes.Kind != yaml.MappingNode {
			continue
		}
		for _, rename := range recipeRenames {
			at := mappingKeyIndex(recipes, rename.from)
			if at < 0 {
				continue
			}
			if current := mappingKeyIndex(recipes, rename.to); current >= 0 {
				var former, held any
				if err := recipes.Content[at+1].Decode(&former); err != nil {
					return nil, nil, configengine.MarkInvalid(fmt.Errorf("runprofile: migrate profiles: decode %s.%s: %w", entry, rename.from, err))
				}
				if err := recipes.Content[current+1].Decode(&held); err != nil {
					return nil, nil, configengine.MarkInvalid(fmt.Errorf("runprofile: migrate profiles: decode %s.%s: %w", entry, rename.to, err))
				}
				if reflect.DeepEqual(former, held) {
					recipes.Content = slices.Delete(recipes.Content, at, at+2)
					lines = append(lines, fmt.Sprintf("%s.%s: removed, %s.%s already set", entry, rename.from, entry, rename.to))
				}
				continue
			}
			recipes.Content[at].Value = rename.to
			lines = append(lines, fmt.Sprintf("%s.%s: renamed to %s.%s", entry, rename.from, entry, rename.to))
		}
	}
	if len(lines) == 0 {
		return existing, nil, nil
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("runprofile: migrate profiles: encode yaml: %w", err))
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, configengine.MarkInvalid(fmt.Errorf("runprofile: migrate profiles: encode yaml: %w", err))
	}
	return out.Bytes(), lines, nil
}

// mappingKeyIndex returns the index of key's key node among mapping's content, or -1 when mapping holds no such key.
func mappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// UnsettledFormerBlocks lists, sorted, each entry that holds a recipe's former name beside a differing current block.
// Each line reads "<entry>.<from>: differs from <entry>.<to>", so the operator can settle the pair by hand.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func UnsettledFormerBlocks(existing []byte) ([]string, error) {
	var hub map[string]any
	if err := yaml.Unmarshal(existing, &hub); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("runprofile: list the former recipe blocks profiles hold beside a differing current one: %w", err))
	}
	var unsettled []string
	for name, entry := range hub {
		hubRecipes, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		for _, rename := range recipeRenames {
			former, heldFormer := hubRecipes[rename.from]
			current, heldCurrent := hubRecipes[rename.to]
			if heldFormer && heldCurrent && !reflect.DeepEqual(former, current) {
				unsettled = append(unsettled, fmt.Sprintf("%s.%s: differs from %s.%s", name, rename.from, name, rename.to))
			}
		}
	}
	slices.Sort(unsettled)
	return unsettled, nil
}
