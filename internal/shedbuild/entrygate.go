// entrygate.go parses a recipe row's entry_gate checks and builds them into the shedengine.EntryGate the row's definition carries.

package shedbuild

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

const (
	// budgetModule is the operand module that names a segment's resolved soft cap instead of a config module.
	budgetModule = "budget"

	entryGateWayForward = "way forward: write each check as <operand>, <operand> == <literal> or <operand> != <literal>, an operand being module.key or budget.<Segment>"
)

// operandPattern is the shape of an entry gate operand: two dot-separated words.
var operandPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

// entryCheck is one parsed entry gate check.
type entryCheck struct {
	// text is the check as the recipe wrote it, trimmed.
	text string
	// module and key are the operand's two halves; module is budgetModule for a segment budget, and key is then the segment.
	module string
	key    string
	// comparison is "==", "!=" or "" for a bare operand.
	comparison string
	// literal is the YAML scalar a comparison compares against.
	literal any
}

// operand renders the check's operand as the recipe wrote it.
func (c entryCheck) operand() string {
	return c.module + "." + c.key
}

// parseEntryGate parses each of row's entry_gate checks.
// A check is `<operand>`, `<operand> == <literal>` or `<operand> != <literal>` with a YAML scalar literal, an operand being `module.key` or `budget.<Segment>`.
// Any other text is refused naming the row, the check and the way forward.
func parseEntryGate(row Row) ([]entryCheck, error) {
	checks := make([]entryCheck, 0, len(row.EntryGate))
	for _, text := range row.EntryGate {
		check, err := parseEntryCheck(text)
		if err != nil {
			return nil, fmt.Errorf("shedbuild: row %q: entry_gate check %q: %w; %s", row.Name, text, err, entryGateWayForward)
		}
		checks = append(checks, check)
	}
	return checks, nil
}

// parseEntryCheck parses one check's text.
func parseEntryCheck(text string) (entryCheck, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return entryCheck{}, errors.New("the check is empty")
	}

	at, comparison := -1, ""
	for _, candidate := range []string{"==", "!="} {
		if found := strings.Index(trimmed, candidate); found >= 0 && (at < 0 || found < at) {
			at, comparison = found, candidate
		}
	}
	operandText := trimmed
	var literal any
	if at >= 0 {
		operandText = strings.TrimSpace(trimmed[:at])
		var err error
		if literal, err = parseEntryLiteral(trimmed[at+len(comparison):]); err != nil {
			return entryCheck{}, err
		}
	}

	if !operandPattern.MatchString(operandText) {
		return entryCheck{}, fmt.Errorf("%q is not an operand of the form module.key or budget.<Segment>", operandText)
	}
	module, key, _ := strings.Cut(operandText, ".")
	return entryCheck{text: trimmed, module: module, key: key, comparison: comparison, literal: literal}, nil
}

// parseEntryLiteral reads the text after a comparison as one YAML scalar.
// An unquoted literal holding whitespace is refused, so a second comparison or an `&&` cannot hide in it.
func parseEntryLiteral(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("the literal after the comparison is empty")
	}
	var value any
	if err := yaml.Unmarshal([]byte(text), &value); err != nil {
		return nil, fmt.Errorf("the literal %q is not a YAML scalar: %w", text, err)
	}
	switch value.(type) {
	case map[string]any, []any:
		return nil, fmt.Errorf("the literal %q is not a YAML scalar", text)
	case string:
		if text[0] != '"' && text[0] != '\'' && strings.ContainsAny(text, " \t") {
			return nil, fmt.Errorf("the literal %q holds whitespace; quote it to compare against a string", text)
		}
	}
	return value, nil
}

// entryGateOf builds the EntryGate row's checks make, nil for a row with none.
// It refuses, naming the row and the way forward, an operand the told Env cannot answer: a `module.key` that Env.SettingKnown rejects or that meets a nil Env.SettingKnown or Env.Setting, and a segment with no Env.Segments entry.
// The returned gate reads every operand at each evaluation: a setting through Env.Setting and a segment budget as Env.Segments[<Segment>].MaxBounces.
// It stops at the first check that does not hold and returns that check's reason.
func entryGateOf(row Row, checks []entryCheck, env shedrecipe.Env) (shedengine.EntryGate, error) {
	if len(checks) == 0 {
		return nil, nil
	}
	for _, check := range checks {
		if err := check.validate(row, env); err != nil {
			return nil, err
		}
	}
	return func() (bool, string, error) {
		for _, check := range checks {
			holds, reason, err := check.evaluate(row, env)
			if err != nil || !holds {
				return false, reason, err
			}
		}
		return true, "", nil
	}, nil
}

// validate refuses a check whose operand env cannot answer.
func (c entryCheck) validate(row Row, env shedrecipe.Env) error {
	if c.module == budgetModule {
		if _, ok := env.Segments[c.key]; !ok {
			return fmt.Errorf("shedbuild: row %q: entry_gate check %q: segment %q has no Env.Segments entry; way forward: name a review segment of the recipe, or wire the segment's settings in the caller's Env.Segments", row.Name, c.text, c.key)
		}
		return nil
	}
	if env.SettingKnown == nil {
		return fmt.Errorf("shedbuild: row %q: entry_gate check %q names setting %s but Env.SettingKnown is nil; way forward: fill Env.SettingKnown in the caller that builds this recipe", row.Name, c.text, c.operand())
	}
	if env.Setting == nil {
		return fmt.Errorf("shedbuild: row %q: entry_gate check %q names setting %s but Env.Setting is nil; way forward: fill Env.Setting in the caller that builds this recipe", row.Name, c.text, c.operand())
	}
	if err := env.SettingKnown(c.module, c.key); err != nil {
		return fmt.Errorf("shedbuild: row %q: entry_gate check %q: %w", row.Name, c.text, err)
	}
	return nil
}

// evaluate reads the check's operand and reports whether the check holds.
// The reason names the check with the value read.
func (c entryCheck) evaluate(row Row, env shedrecipe.Env) (holds bool, reason string, err error) {
	var value any
	if c.module == budgetModule {
		value = env.Segments[c.key].MaxBounces
	} else if value, err = env.Setting(c.module, c.key); err != nil {
		return false, "", fmt.Errorf("shedbuild: row %q: entry_gate check %q: read setting %s: %w", row.Name, c.text, c.operand(), err)
	}

	switch {
	case c.comparison != "":
		holds = valuesEqual(value, c.literal) == (c.comparison == "==")
		reason = fmt.Sprintf("%s (%s = %v)", c.text, c.operand(), value)
	case c.module == budgetModule:
		holds = value != 0
		reason = fmt.Sprintf("%s = %v", c.operand(), value)
	default:
		enabled, isBool := value.(bool)
		if !isBool {
			return false, "", fmt.Errorf("shedbuild: row %q: entry_gate check %q: setting %s holds %v, not a bool; way forward: run `lym config %s --resolved` in the task worktree to see the layer holding the value, then fix it there: with `lym config %s` from the prime for a hub or default layer, or with `lym config %s --layer local --set %s=<bool>` in the task worktree for a value the local layer holds; then %s", row.Name, c.text, c.operand(), value, c.module, c.module, c.module, c.key, env.Resume)
		}
		holds = enabled
		reason = fmt.Sprintf("%s = %v", c.operand(), value)
	}
	return holds, reason, nil
}

// valuesEqual reports whether a setting value equals a literal, comparing any two numbers by value.
func valuesEqual(value, literal any) bool {
	if left, ok := numberOf(value); ok {
		right, ok := numberOf(literal)
		return ok && left == right
	}
	return reflect.DeepEqual(value, literal)
}

// numberOf returns v as a float64 when v is an int or a float64.
func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
