// payload_test.go couples each board payload command's `{…}` Use token to the payloadKeys its parser validates against.
// It inspects the built cobra tree only and runs no command.

package boardcli

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestPayloadUse_MatchesDeclaredKeys asserts that every payload command's Use token spells exactly the keys its parser declares, markers and nested objects included.
func TestPayloadUse_MatchesDeclaredKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// path finds the command under Command(); use, when set, replaces its Use for a synthetic row.
		path      []string
		use       string
		declared  payloadKeys
		wantMatch bool
	}{
		{name: "get", path: []string{"get"}, declared: lookupPayload, wantMatch: true},
		{name: "remove", path: []string{"remove"}, declared: lookupPayload, wantMatch: true},
		{name: "promote", path: []string{"promote"}, declared: lookupPayload, wantMatch: true},
		{name: "set-status", path: []string{"set-status"}, declared: setStatusPayload, wantMatch: true},
		{name: "set-deps", path: []string{"set-deps"}, declared: setDepsPayload, wantMatch: true},
		{name: "upsert", path: []string{"upsert"}, declared: upsertPayload, wantMatch: true},
		{name: "upsert-batch", path: []string{"upsert-batch"}, declared: upsertBatchPayload, wantMatch: true},
		{name: "merge", path: []string{"merge"}, declared: mergePayload, wantMatch: true},
		{name: "intake import", path: []string{"intake", "import"}, declared: intakeImportPayload, wantMatch: true},
		{name: "intake close", path: []string{"intake", "close"}, declared: intakeClosePayload, wantMatch: true},
		{name: "a required key marked optional is a mismatch", use: "set-status {slug|id, status?}", declared: setStatusPayload, wantMatch: false},
	}

	root := Command()
	covered := map[string]bool{}
	for _, tt := range tests {
		use := tt.use
		if use == "" {
			cmd, _, err := root.Find(tt.path)
			if err != nil {
				t.Fatalf("Find(%v) error: %v", tt.path, err)
			}
			use = cmd.Use
			covered[cmd.CommandPath()] = true
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parsePayloadToken(use)
			if err != nil {
				t.Fatalf("parsePayloadToken(%q) error: %v", use, err)
			}
			diffs := diffPayloadKeys("", parsed, tt.declared)
			if tt.wantMatch && len(diffs) > 0 {
				t.Errorf("Use %q disagrees with its declared keys:\n%s", use, strings.Join(diffs, "\n"))
			}
			if !tt.wantMatch && len(diffs) == 0 {
				t.Errorf("Use %q matches its declared keys; want a mismatch", use)
			}
		})
	}

	t.Run("every payload command has a row", func(t *testing.T) {
		t.Parallel()
		var walk func(cmd *cobra.Command)
		walk = func(cmd *cobra.Command) {
			if strings.Contains(cmd.Use, "{") && !covered[cmd.CommandPath()] {
				t.Errorf("%s spells a payload in Use %q but has no row in this test", cmd.CommandPath(), cmd.Use)
			}
			for _, child := range cmd.Commands() {
				walk(child)
			}
		}
		walk(Command())
	})
}

// parsePayloadToken parses the brace-balanced `{…}` token of a Use string into the keys it spells.
func parsePayloadToken(use string) (payloadKeys, error) {
	start := strings.Index(use, "{")
	if start < 0 {
		return payloadKeys{}, fmt.Errorf("no {…} token")
	}
	return parsePayloadObject(use[start:])
}

// parsePayloadObject parses one `{…}` object, whose elements split on commas outside braces and brackets.
// `key?` is optional, `a|b` an exclusive pair, `key: {…}` a nested object, and `<task>` or `[<task>]` the upsert declaration.
func parsePayloadObject(object string) (payloadKeys, error) {
	if !strings.HasPrefix(object, "{") || !strings.HasSuffix(object, "}") {
		return payloadKeys{}, fmt.Errorf("%q is not one {…} object", object)
	}
	var keys payloadKeys
	for _, element := range splitTopLevel(object[1 : len(object)-1]) {
		name, value, hasValue := strings.Cut(element, ":")
		name = strings.TrimSpace(name)
		optional := strings.HasSuffix(name, "?")
		name = strings.TrimSuffix(name, "?")
		switch {
		case hasValue:
			nested, err := parsePayloadValue(strings.TrimSpace(value))
			if err != nil {
				return payloadKeys{}, fmt.Errorf("%s: %w", name, err)
			}
			if keys.nested == nil {
				keys.nested = map[string]payloadKeys{}
			}
			keys.nested[name] = nested
		case strings.Contains(name, "|"):
			first, second, _ := strings.Cut(name, "|")
			keys.exclusive = append(keys.exclusive, [2]string{first, second})
			continue
		}
		if optional {
			keys.optional = append(keys.optional, name)
		} else {
			keys.required = append(keys.required, name)
		}
	}
	return keys, nil
}

// parsePayloadValue parses a nested key's value: a `{…}` object, or `<task>` or `[<task>]` for the upsert declaration.
func parsePayloadValue(value string) (payloadKeys, error) {
	switch value {
	case "<task>", "[<task>]":
		return upsertPayload, nil
	}
	return parsePayloadObject(value)
}

// splitTopLevel splits s on the commas that sit outside every brace and bracket, trimming each element.
func splitTopLevel(s string) []string {
	var elements []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				elements = append(elements, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(elements, strings.TrimSpace(s[start:]))
}

// diffPayloadKeys lists every way parsed and declared disagree, each prefixed with the nested key path at.
func diffPayloadKeys(at string, parsed, declared payloadKeys) []string {
	var diffs []string
	compare := func(what string, got, want []string) {
		got, want = slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
		if !slices.Equal(got, want) {
			diffs = append(diffs, fmt.Sprintf("%s%s: Use spells %v, declared %v", at, what, got, want))
		}
	}
	compare("required", parsed.required, declared.required)
	compare("optional", parsed.optional, declared.optional)
	compare("exclusive", exclusivePairNames(parsed.exclusive), exclusivePairNames(declared.exclusive))

	var nestedNames []string
	for name := range parsed.nested {
		nestedNames = append(nestedNames, name)
	}
	var declaredNames []string
	for name := range declared.nested {
		declaredNames = append(declaredNames, name)
	}
	compare("nested", nestedNames, declaredNames)
	for name, nested := range parsed.nested {
		if want, ok := declared.nested[name]; ok {
			diffs = append(diffs, diffPayloadKeys(at+name+".", nested, want)...)
		}
	}
	return diffs
}

// exclusivePairNames renders each exclusive pair as `a|b` with its two keys sorted, so pair order does not matter.
func exclusivePairNames(pairs [][2]string) []string {
	names := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		sorted := []string{pair[0], pair[1]}
		slices.Sort(sorted)
		names = append(names, strings.Join(sorted, "|"))
	}
	return names
}
