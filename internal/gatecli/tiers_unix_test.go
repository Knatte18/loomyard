//go:build !windows

// tiers_unix_test.go holds the tier command rendering test, whose expected quoting is the sh form.

package gatecli

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRenderTierCommand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		command string
		filter  string
		targets []string
		terms   []string
		args    []string
		want    string
	}{
		{name: "each target and arg is quoted", command: "run {targets} -- {args}", targets: []string{"./a b", "it's"}, args: []string{"-run", "^(X|Y)$"}, want: `run './a b' 'it'\''s' -- '-run' '^(X|Y)$'`},
		{name: "parallel is filled", command: "make -j{parallel}", targets: []string{"x"}, want: "make -j3"},
		{name: "the test timeout is filled in minutes", command: "go test -timeout {test_timeout} {targets}", targets: []string{"x"}, want: "go test -timeout 25m 'x'"},
		{name: "a placeholder the command lacks is not added", command: "build", targets: []string{"x"}, args: []string{"y"}, want: "build"},
		{name: "a target holding a placeholder is not expanded again", command: "run {targets} {args}", targets: []string{"{args}"}, args: []string{"z"}, want: "run '{args}' 'z'"},
		{name: "a tier filter alone", command: "t {targets} {filter}", filter: "Category!=Slow", targets: []string{"p"}, want: "t 'p' --filter 'Category!=Slow'"},
		{name: "no filter and no terms renders nothing", command: "t {targets} {filter}", targets: []string{"p"}, want: "t 'p' "},
		{name: "terms alone", command: "t {filter}", terms: []string{"FullyQualifiedName~A", "FullyQualifiedName~B"}, want: "t --filter 'FullyQualifiedName~A|FullyQualifiedName~B'"},
		{name: "terms compose with the tier filter", command: "t {filter}", filter: "Category!=Slow", terms: []string{"FullyQualifiedName~A"}, want: "t --filter '(Category!=Slow)&(FullyQualifiedName~A)'"},
		{name: "terms past the budget render the tier filter alone", command: "t {filter}", filter: "Category!=Slow", terms: []string{strings.Repeat("A", greenCommandBudget)}, want: "t --filter 'Category!=Slow'"},
		{name: "terms given to a command without a filter placeholder filter nothing", command: "t {targets}", targets: []string{"p"}, terms: []string{"FullyQualifiedName~A"}, want: "t 'p'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := renderTierCommand(2, gateslot.Tier{Command: tc.command, Filter: tc.filter, Parallel: 3, TestTimeoutMin: 25}, tc.targets, tc.terms, tc.args)
			if err != nil {
				t.Fatalf("renderTierCommand() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("renderTierCommand() = %q; want %q", got, tc.want)
			}
		})
	}
}
