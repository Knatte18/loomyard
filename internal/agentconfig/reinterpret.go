// reinterpret.go carries a hub's agents.yaml across the retirement of the treadle-judge and treadle-targeting roles.

package agentconfig

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// RetiredRolesReinterpretation names the retirement of the treadle-judge and treadle-targeting roles.
// Load refuses a key naming a role lym does not start, so a hub file that still names one stops loading.
const RetiredRolesReinterpretation = "agents-treadle-roles-retired"

// retiredRoles are the roles lym no longer starts and an older agents.yaml may still name.
var retiredRoles = []string{"treadle-judge", "treadle-targeting"}

// DropRetiredRoles is the reinterpretation of a hub's agents.yaml for RetiredRolesReinterpretation.
// It removes each retired role's key from skills.
// It also removes every efforts key whose role part names a retired role.
// The bare role and the `<role>/<model>` form are removed alike.
// Every other key and comment is carried through, re-encoded with two-space indent.
// It returns one rewrote line per removed key, in document order, and never a flagged line.
// A file naming no retired role comes back unchanged, so a second pass is a no-op.
// The rewrite takes no worktree root or anchored directory, since the keys name no path.
// A document that does not parse, or whose top level is not a mapping, returns an error marked configengine.ErrInvalid.
func DropRetiredRoles(existing []byte, _, _ string) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("drop retired roles: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return existing, nil, nil, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("drop retired roles: the top level of agents.yaml is not a mapping"))
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		section, entries := root.Content[i].Value, root.Content[i+1]
		if (section != skillsKey && section != effortsKey) || entries.Kind != yaml.MappingNode {
			continue
		}
		kept := make([]*yaml.Node, 0, len(entries.Content))
		for j := 0; j+1 < len(entries.Content); j += 2 {
			key := entries.Content[j].Value
			role, _, _ := strings.Cut(key, roleModelSeparator)
			if slices.Contains(retiredRoles, role) {
				rewrote = append(rewrote, fmt.Sprintf("%s.%s: retired role, removed", section, key))
				continue
			}
			kept = append(kept, entries.Content[j], entries.Content[j+1])
		}
		entries.Content = kept
	}
	if len(rewrote) == 0 {
		return existing, nil, nil, nil
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, nil, fmt.Errorf("drop retired roles: encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("drop retired roles: encode yaml: %w", err)
	}
	return out.Bytes(), rewrote, nil, nil
}
