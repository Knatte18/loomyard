// parse.go implements Parse, the strict grammar checker for model-spec strings.
// It recognizes exactly four shapes (alias, alias[bracket], engine:model, engine:model[bracket])
// and rejects everything else with a loud error naming the offending token or character — see
// contracts/specs/llm-model-spec.md for the pinned grammar this file implements against.

package modelspec

import (
	"fmt"
	"strings"
	"unicode"
)

// isIdentChar reports whether r is valid in an alias, param key, or engine name:
// lowercase letters, digits, and dash only (case-sensitive by design).
func isIdentChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}

// isModelIDChar reports whether r is valid in an escape-form model id or param value:
// isIdentChar plus dot and underscore.
func isModelIDChar(r rune) bool {
	return isIdentChar(r) || r == '.' || r == '_'
}

// validateCharset walks s and returns an error naming the first invalid rune, or nil.
func validateCharset(s, kind string, allowed func(rune) bool) error {
	for i, r := range s {
		if !allowed(r) {
			return fmt.Errorf("modelspec: invalid character %q at position %d in %s %q", r, i, kind, s)
		}
	}
	return nil
}

// Parse checks s against the strict model-spec grammar and returns the parsed Spec.
// It recognizes four shapes: alias, alias[k=v,...], engine:model-id, and engine:model-id[k=v,...].
// On success, Params is nil when s had no bracket, or the parsed map otherwise.
func Parse(s string) (Spec, error) {
	if s == "" {
		return Spec{}, fmt.Errorf("modelspec: empty spec string")
	}

	// Whitespace is rejected anywhere in the string, before any structural
	// parsing — a stray space almost always means the operator meant two
	// tokens, and silently trimming it would hide that mistake.
	for i, r := range s {
		if unicode.IsSpace(r) {
			return Spec{}, fmt.Errorf("modelspec: whitespace character %q at position %d in spec %q", r, i, s)
		}
	}

	// Split off the optional bracket part first. Finding '[' by its first
	// occurrence and then requiring the remainder to end in ']' rejects both
	// "nothing after the bracket" (extra trailing text) and an unterminated
	// bracket in one check.
	body := s
	var bracketInner string
	hasBracket := false
	if idx := strings.IndexByte(s, '['); idx >= 0 {
		hasBracket = true
		body = s[:idx]
		rest := s[idx:]
		if !strings.HasSuffix(rest, "]") {
			return Spec{}, fmt.Errorf("modelspec: bracket part in %q must end with ']' and have nothing after it", s)
		}
		bracketInner = rest[1 : len(rest)-1]
		if bracketInner == "" {
			return Spec{}, fmt.Errorf("modelspec: empty bracket %q in %q — omit the bracket entirely if there are no params", rest, s)
		}
	}

	spec := Spec{}

	// Escape form is detected by the presence of ':' in body; anything else
	// is alias form. Exactly one colon is allowed either way.
	colonIdx := strings.IndexByte(body, ':')
	if colonIdx == -1 {
		if body == "" {
			return Spec{}, fmt.Errorf("modelspec: empty alias in spec %q", s)
		}
		if err := validateCharset(body, "alias", isIdentChar); err != nil {
			return Spec{}, err
		}
		spec.Alias = body
	} else {
		if strings.Count(body, ":") != 1 {
			return Spec{}, fmt.Errorf("modelspec: exactly one ':' is allowed in escape form, found %d in %q", strings.Count(body, ":"), s)
		}
		engine := body[:colonIdx]
		model := body[colonIdx+1:]
		if engine == "" {
			return Spec{}, fmt.Errorf("modelspec: empty engine before ':' in spec %q", s)
		}
		if model == "" {
			return Spec{}, fmt.Errorf("modelspec: empty model-id after ':' in spec %q", s)
		}
		if err := validateCharset(engine, "engine", isIdentChar); err != nil {
			return Spec{}, err
		}
		if err := validateCharset(model, "model-id", isModelIDChar); err != nil {
			return Spec{}, err
		}
		if !knownEngines[engine] {
			return Spec{}, fmt.Errorf("modelspec: unknown engine %q in spec %q", engine, s)
		}
		spec.Engine = engine
		spec.Model = model
	}

	if hasBracket {
		params, err := parseBracket(bracketInner, s)
		if err != nil {
			return Spec{}, err
		}
		spec.Params = params
	}

	return spec, nil
}

// parseBracket parses the comma-separated key=value list inside a spec's bracket.
// Every rejection is its own named error per the fail-loud grammar contract.
func parseBracket(inner, fullSpec string) (map[string]string, error) {
	params := make(map[string]string)
	for _, pair := range strings.Split(inner, ",") {
		eq := strings.IndexByte(pair, '=')
		if eq == -1 {
			return nil, fmt.Errorf("modelspec: param %q in spec %q has no '=' separator", pair, fullSpec)
		}
		key := pair[:eq]
		value := pair[eq+1:]

		if key == "" {
			return nil, fmt.Errorf("modelspec: empty param key in %q in spec %q", pair, fullSpec)
		}
		if value == "" {
			return nil, fmt.Errorf("modelspec: empty value for param key %q in spec %q", key, fullSpec)
		}
		if err := validateCharset(key, "param key", isIdentChar); err != nil {
			return nil, err
		}
		if err := validateCharset(value, "param value", isModelIDChar); err != nil {
			return nil, err
		}
		if _, dup := params[key]; dup {
			return nil, fmt.Errorf("modelspec: duplicate param key %q in spec %q", key, fullSpec)
		}
		if !knownParams[key] {
			return nil, fmt.Errorf("modelspec: unknown param key %q in spec %q", key, fullSpec)
		}
		params[key] = value
	}
	return params, nil
}
