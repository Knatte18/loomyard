// reinterpret.go carries a hub's gate.yaml across the retirement of the harden-live-copies gate key.

package gateslot

import (
	"bytes"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// RetiredCopiesReinterpretationID names the change that retired the harden-live-copies gate key.
const RetiredCopiesReinterpretationID = "gate-harden-live-copies-retired"

// RetireHardenLiveCopies carries a hub gate.yaml across the retirement of its gates.harden-live-copies entry.
// The hardener's concurrent copies now run under gates.harden-live, which must name one tier below BilledTier.
// An entry that equals that one tier is removed, since harden-live already says the same.
// An entry that differs is left in place, as harden-live keeps its own value, and is flagged with the way forward.
// A file with no such entry is returned unchanged.
// The worktree root and anchored directory arguments are unused, as the rewrite judges the file alone.
func RetireHardenLiveCopies(existing []byte, _, _ string) (rewritten []byte, rewrote, flagged []string, err error) {
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, fmt.Errorf("parse the gate config: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	gates := mappingValue(document.Content[0], "gates")
	if gates == nil || gates.Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	at := mappingKeyIndex(gates, "harden-live-copies")
	if at < 0 {
		return existing, nil, nil, nil
	}

	copiesTiers := gateKeyTiers(gates.Content[at+1])
	liveTiers := []int{0}
	if live := mappingValue(gates, "harden-live"); live != nil {
		liveTiers = gateKeyTiers(live)
	}
	if len(liveTiers) != 1 || liveTiers[0] >= BilledTier || !slices.Equal(liveTiers, copiesTiers) {
		return existing, nil, []string{fmt.Sprintf(
			"gates.harden-live-copies is retired and gates.harden-live names the tiers %v, but the hardener's copies now run under gates.harden-live, "+
				"which must name one tier below tier%d: set it with `lym config gate --set 'gates.harden-live={tiers: [<live tier>], scope: diff}'` "+
				"from the prime, then delete the gates.harden-live-copies entry from the hub's gate.yaml, which nothing reads now",
			liveTiers, BilledTier)}, nil
	}

	gates.Content = slices.Delete(gates.Content, at, at+2)
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, nil, nil, fmt.Errorf("write the gate config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, nil, nil, fmt.Errorf("write the gate config: %w", err)
	}
	return out.Bytes(), []string{fmt.Sprintf("removed gates.harden-live-copies, which names the tier harden-live already names, tier %d", liveTiers[0])}, nil, nil
}

// gateKeyTiers returns the tier numbers a gates entry names, or nil when the entry holds no readable tiers list.
func gateKeyTiers(entry *yaml.Node) []int {
	var key GateKey
	if err := entry.Decode(&key); err != nil {
		return nil
	}
	return key.Tiers
}
