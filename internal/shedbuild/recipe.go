// recipe.go declares this package's two exported types: Recipe, the decoded recipe document, and
// Row, one producer entry within it.

package shedbuild

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Recipe is a decoded recipe document: the whole graph of producer rows plus its graph metadata.
//
// Entry and Terminals are graph metadata Build never consumes, surfaced because shedcheck.Check
// takes both as told arguments and refuses to infer either.
// AmendmentTarget is optional graph metadata of the same kind: it names the row a plan amendment routes to, empty when the recipe sets none.
// Contract is empty or `loom`, and `loom` declares the recipe a loom, which Build checks against the loom contract.
// VerifySource is empty, `plan` or `tree_scope`, and a loom recipe must set it.
type Recipe struct {
	Version         int      `yaml:"version"`
	Entry           string   `yaml:"entry"`
	AmendmentTarget string   `yaml:"amendment_target"`
	Contract        string   `yaml:"contract"`
	VerifySource    string   `yaml:"verify_source"`
	Terminals       []string `yaml:"terminals"`
	Producers       []Row    `yaml:"producers"`
}

// Row is one producer entry in a Recipe: the row's identity, its engine name, its portable
// configuration, its routing, its bounce budget, and its entry gate.
//
// Config is the decoded sub-map passed to the row's engine untouched -- never normalised,
// lowercased, or key-walked -- because each registry entry owns its own key validation.
//
// A block row sets Block, the name of the recipe it runs, and BlockGate, the name of the check that gates it, instead of an engine.
// SkipTo and BlockPath sit outside the recipe format: only expansion sets them, so a file that writes either is refused.
// SkipTo is the row a skipped block row routes to, and BlockPath is the names of the block rows enclosing the row, outermost first, joined by "/".
type Row struct {
	Name       string         `yaml:"name"`
	Engine     string         `yaml:"engine"`
	Config     map[string]any `yaml:"config"`
	OnDone     string         `yaml:"on_done"`
	OnStuck    string         `yaml:"on_stuck"`
	OnReroute  string         `yaml:"on_reroute"`
	Segment    string         `yaml:"segment"`
	MaxBounces int            `yaml:"max_bounces"`
	EntryGate  EntryChecks    `yaml:"entry_gate"`
	Block      string         `yaml:"block"`
	BlockGate  string         `yaml:"block_gate"`
	SkipTo     string         `yaml:"-"`
	BlockPath  string         `yaml:"-"`
}

// EntryChecks is a row's entry_gate value: one check written as a scalar, or a list of checks that must all hold.
// Each check is raw text until the package parses it.
type EntryChecks []string

// UnmarshalYAML decodes a scalar into a one-check list and a sequence into the list of its scalars.
func (c *EntryChecks) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var text string
		if err := node.Decode(&text); err != nil {
			return err
		}
		*c = EntryChecks{text}
		return nil
	case yaml.SequenceNode:
		var texts []string
		if err := node.Decode(&texts); err != nil {
			return err
		}
		*c = texts
		return nil
	default:
		return fmt.Errorf("line %d: entry_gate must be one check or a list of checks", node.Line)
	}
}
