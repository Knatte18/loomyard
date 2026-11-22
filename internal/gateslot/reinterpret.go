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
// An entry that stops the whole gate config loading is removed whatever its tiers.
// It does so when the gate config's own entry check refuses it, a scope-less entry included;
// when its tiers differ from harden-live's one tier, the removal is also flagged with the way forward for harden-live.
// Any other entry that differs is left in place, as harden-live keeps its own value, and is flagged with the way forward.
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

	copies, copiesDecode := decodeGateKey(gates.Content[at+1])
	liveTiers := []int{0}
	if live := mappingValue(gates, "harden-live"); live != nil {
		liveKey, _ := decodeGateKey(live)
		liveTiers = liveKey.Tiers
	}
	setLive := fmt.Sprintf("the hardener's copies now run under gates.harden-live, "+
		"which must name one tier below tier%d: set it with `lym config gate --set 'gates.harden-live={tiers: [<live tier>], scope: diff}'` from the prime",
		BilledTier)
	matches := len(liveTiers) == 1 && liveTiers[0] < BilledTier && slices.Equal(liveTiers, copies.Tiers)
	_, loadErr := decodeGates(map[string]yaml.Node{"harden-live-copies": *gates.Content[at+1]})
	if !matches && loadErr == nil {
		return existing, nil, []string{fmt.Sprintf(
			"gates.harden-live-copies is retired and gates.harden-live names the tiers %v, but %s, "+
				"then remove the entry, which nothing reads now, with `lym config gate --unset gates.harden-live-copies` from the prime",
			liveTiers, setLive)}, nil
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
	if matches {
		return out.Bytes(), []string{fmt.Sprintf("removed gates.harden-live-copies, which names the tier harden-live already names, tier %d", liveTiers[0])}, nil, nil
	}
	copiesNamed := fmt.Sprintf("named the tiers %v", copies.Tiers)
	if !copiesDecode {
		copiesNamed = "did not decode"
	}
	return out.Bytes(),
		[]string{"removed gates.harden-live-copies, which stopped the gate config loading"},
		[]string{fmt.Sprintf("the removed gates.harden-live-copies %s and gates.harden-live names the tiers %v; %s",
			copiesNamed, liveTiers, setLive)},
		nil
}

// decodeGateKey returns the gates entry decoded and whether it decoded.
// When the entry does not decode it returns the zero GateKey, which names no tiers and no scope, and false.
func decodeGateKey(entry *yaml.Node) (GateKey, bool) {
	var key GateKey
	if err := entry.Decode(&key); err != nil {
		return GateKey{}, false
	}
	return key, true
}
