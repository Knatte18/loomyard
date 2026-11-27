// reinterpret.go carries a hub's agents.yaml across the retirement of the pr-reviewer role.

package agentconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// PRReviewerRetiredReinterpretationID names the change that retired the pr-reviewer role.
const PRReviewerRetiredReinterpretationID = "agents-pr-reviewer-role-retired"

// retiredPRReviewerRole is the role lym no longer starts.
const retiredPRReviewerRole = "pr-reviewer"

// DropPRReviewerRole removes the retired pr-reviewer role from the bytes of a hub agents.yaml, which Load refuses.
// It removes the role's skill list, and every efforts entry of the role, with or without a model.
// It keeps every other key and the comments on them, and returns one rewrote line per removed key.
// A file naming the role nowhere is returned unchanged.
// The local agents.yaml of each worktree it is told is read too.
// A key of the role there is flagged with the way forward, and so is a file that cannot be read.
func DropPRReviewerRole(existing []byte, worktrees []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	rewritten, removed, err := withoutPRReviewerRole(existing)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, key := range removed {
		rewrote = append(rewrote, fmt.Sprintf("%s is removed, since lym no longer starts the %s role", key, retiredPRReviewerRole))
	}
	for _, worktree := range worktrees {
		path := configengine.LocalFile(worktree.Anchor, ModuleName)
		local, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var held []string
		if err == nil {
			_, held, err = withoutPRReviewerRole(local)
		}
		if err != nil {
			flagged = append(flagged, fmt.Sprintf("%s could not be read: %v; way forward: fix the file the error names", path, err))
			continue
		}
		for _, key := range held {
			flagged = append(flagged, fmt.Sprintf("%s sets %s, of the %s role lym no longer starts, so agents.yaml stops loading there%s", path, key, retiredPRReviewerRole, layerWayForward))
		}
	}
	return rewritten, rewrote, flagged, nil
}

// withoutPRReviewerRole returns the agents.yaml bytes without the pr-reviewer role's keys, and the keys it removed.
// A section the removal empties stays as an empty mapping, which keeps the comments on it.
// A file with no key of the role comes back as it was.
func withoutPRReviewerRole(existing []byte) (rewritten []byte, removed []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, fmt.Errorf("parse the agents config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil
	}
	top := document.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		section, entries := top.Content[i].Value, top.Content[i+1]
		if (section != skillsKey && section != effortsKey) || entries.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(entries.Content); {
			key := entries.Content[j].Value
			role, _, _ := strings.Cut(key, roleModelSeparator)
			if role != retiredPRReviewerRole {
				j += 2
				continue
			}
			entries.Content = slices.Delete(entries.Content, j, j+2)
			removed = append(removed, section+"."+key)
		}
	}
	if len(removed) == 0 {
		return existing, nil, nil
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, fmt.Errorf("write the agents config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, fmt.Errorf("write the agents config: %w", err)
	}
	return out.Bytes(), removed, nil
}
