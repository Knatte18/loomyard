// reinterpret.go carries a hub's batcher.yaml across the retirement of its active key.

package batcher

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// RetiredActiveReinterpretationID names the change that moved the batch profile choice out of batcher.yaml.
const RetiredActiveReinterpretationID = "batcher-active-retired"

// RetireActive carries a hub batcher.yaml across the retirement of its top-level active key.
// Each implementer now names its profile in its own config, so the key is removed whatever it named,
// and the rewrite line names the value it held and the keys that choose a profile now.
// A file with no active key is returned unchanged.
// The worktrees argument is unused, as the rewrite judges the file alone.
func RetireActive(existing []byte, _ []configengine.HubWorktree) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the batcher config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	root := document.Content[0]
	at := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == retiredActiveKey {
			at = i
			break
		}
	}
	if at < 0 {
		return existing, nil, nil, nil
	}

	held := root.Content[at+1].Value
	root.Content = slices.Delete(root.Content, at, at+2)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, nil, fmt.Errorf("write the batcher config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("write the batcher config: %w", err)
	}
	return out.Bytes(), []string{fmt.Sprintf(
		"removed %s, which named %q; %s, set from the prime with `lym config <module> --layer default --set %s=<profile>`",
		keyName(retiredActiveKey), held, activeInstead(), ChoiceKey)}, nil, nil
}
