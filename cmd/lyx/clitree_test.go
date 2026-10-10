// clitree_test.go derives the CLI / Cobra Invariant's per-command obligations from one walk of newRoot(), so no hand-kept list of modules or verbs exists to drift.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// walkCommands calls fn on cmd and every descendant, skipping cobra's completion subtree.
func walkCommands(cmd *cobra.Command, fn func(*cobra.Command)) {
	if cmd.Name() == "completion" {
		return
	}
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, fn)
	}
}

// visibleChildren returns the non-hidden children of cmd, without cobra's completion.
func visibleChildren(cmd *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, child := range cmd.Commands() {
		if child.Name() == "completion" || child.Hidden {
			continue
		}
		out = append(out, child)
	}
	return out
}

// argsFor returns the argument vector that addresses cmd from the root.
func argsFor(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())[1:]
}

// bareProblems runs a bare group invocation and returns how it breaks the invariant, with its output.
func bareProblems(args []string, children []*cobra.Command) ([]string, string) {
	var out bytes.Buffer
	code := run(args, &out)
	got := out.String()
	var problems []string
	if code != 0 {
		problems = append(problems, fmt.Sprintf("exit %d; want 0 for bare group listing", code))
	}
	if strings.Contains(got, `"ok":false`) {
		problems = append(problems, "emitted an error envelope; want plain help text")
	}
	for _, child := range children {
		if !strings.Contains(got, child.Name()) {
			problems = append(problems, fmt.Sprintf("listing does not name %q", child.Name()))
		}
	}
	return problems, got
}

// bogusProblems runs an unknown-subcommand invocation and returns how it breaks the invariant, with its output.
func bogusProblems(args []string) ([]string, string) {
	var out bytes.Buffer
	code := run(args, &out)
	got := out.String()
	var problems []string
	if code != 1 {
		problems = append(problems, fmt.Sprintf("exit %d; want 1", code))
	}
	env, err := envelope.Parse(got)
	switch {
	case err != nil:
		problems = append(problems, err.Error())
	case env.OK:
		problems = append(problems, "ok = true; want an error envelope")
	case !strings.Contains(env.Error, "unknown subcommand"):
		problems = append(problems, fmt.Sprintf("error %q does not name an unknown subcommand", env.Error))
	}
	return problems, got
}

// checkInvocation fails t for every problem.
func checkInvocation(t *testing.T, args []string, problems []string, out string) {
	t.Helper()
	if len(problems) > 0 {
		t.Errorf("run(%v): %s\noutput: %s", args, strings.Join(problems, "; "), out)
	}
}

// TestCLITree_EveryCommand walks newRoot() from a cwd that is not a git repository.
// It chdirs, which is process-global state, so neither it nor its subtests run in parallel.
func TestCLITree_EveryCommand(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newRoot()
	walked := 0
	walkCommands(root, func(cmd *cobra.Command) {
		walked++
		if cmd.Short == "" {
			t.Errorf("command %q has no Short description", cmd.CommandPath())
		}

		children := visibleChildren(cmd)
		if len(children) == 0 || cmd == root {
			return
		}
		args := argsFor(cmd)

		name := strings.Join(args, " ")

		// Bare "lyx help" prints the root's help rather than its own children.
		if cmd.Name() != "help" {
			t.Run(name+"/bare", func(t *testing.T) {
				problems, out := bareProblems(args, children)
				checkInvocation(t, args, problems, out)
			})
		}

		t.Run(name+"/bogus", func(t *testing.T) {
			bogus := append(append([]string{}, args...), "bogus")
			problems, out := bogusProblems(bogus)
			checkInvocation(t, bogus, problems, out)
		})
	})
	if walked < 2 {
		t.Fatalf("walked %d commands; the tree walk is not reaching the mounted modules", walked)
	}

	t.Run("root.Long names every mounted module", func(t *testing.T) {
		children := visibleChildren(root)
		if len(children) == 0 {
			t.Fatal("newRoot() mounts no modules")
		}
		for _, child := range children {
			if !strings.Contains(root.Long, child.Name()) {
				t.Errorf("root.Long does not name mounted module %q; add it to the Available modules list in newRoot()", child.Name())
			}
		}
	})

	// The root alias for loom's bootstrap verb is named "start"; a surviving "run" child would mean the retired alias is still registered.
	t.Run("root carries no bare run child", func(t *testing.T) {
		for _, child := range root.Commands() {
			if child.Name() == "run" {
				t.Errorf("newRoot() root tree carries a bare child command named %q; the retired root alias must not survive the rename", child.Name())
			}
		}
	})
}

// maxLongBytes caps a command's Long text; a longer page belongs in the package doc.
const maxLongBytes = 2000

var (
	useArgToken  = regexp.MustCompile(`^(<[a-z0-9-]+>|\[<[a-z0-9-]+>\]|<[a-z0-9-]+>\.\.\.|\[<[a-z0-9-]+>\.\.\.\])$`)
	useFlagToken = regexp.MustCompile(`^--([a-z0-9-]+)$`)
)

// splitUse splits use on whitespace, keeping a brace-balanced {...} span inside one token.
// The second result is false when a brace is left open or closed without an opener.
func splitUse(use string) ([]string, bool) {
	var tokens []string
	var current strings.Builder
	depth := 0
	for _, r := range use {
		switch {
		case r == '{':
			depth++
		case r == '}':
			depth--
			if depth < 0 {
				return nil, false
			}
		case unicode.IsSpace(r) && depth == 0:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens, depth == 0
}

// audienceProblems returns how cmd's audience annotation breaks the form: an invocable command carries one value of the closed set, a hidden one is internal, and a non-invocable group carries none.
func audienceProblems(cmd *cobra.Command) []string {
	value, has := cmd.Annotations[clihelp.AudienceAnnotation]
	switch {
	case !clihelp.Invocable(cmd) && has:
		return []string{fmt.Sprintf("non-invocable group carries audience %q", value)}
	case !clihelp.Invocable(cmd):
		return nil
	case !has:
		return []string{"invocable command carries no audience"}
	case !slices.Contains(clihelp.Audiences, value):
		return []string{fmt.Sprintf("audience %q is outside %v", value, clihelp.Audiences)}
	case cmd.Hidden && value != clihelp.AudienceInternal:
		return []string{fmt.Sprintf("hidden command has audience %q; want %q", value, clihelp.AudienceInternal)}
	}
	return nil
}

// useProblems returns how cmd's Use breaks the form: its name, then argument tokens, required-flag pairs and brace-balanced payload spans.
func useProblems(cmd *cobra.Command) []string {
	tokens, balanced := splitUse(cmd.Use)
	if !balanced {
		return []string{fmt.Sprintf("Use %q has unbalanced braces", cmd.Use)}
	}
	if len(tokens) == 0 || tokens[0] != cmd.Name() {
		return []string{fmt.Sprintf("Use %q does not start with the command name %q", cmd.Use, cmd.Name())}
	}
	var problems []string
	rest := tokens[1:]
	for i := 0; i < len(rest); i++ {
		token := rest[i]
		switch {
		case strings.HasPrefix(token, "{"):
		case useArgToken.MatchString(token):
		case useFlagToken.MatchString(token):
			name := useFlagToken.FindStringSubmatch(token)[1]
			flag := cmd.Flags().Lookup(name)
			if flag == nil || len(flag.Annotations[cobra.BashCompOneRequiredFlag]) == 0 {
				problems = append(problems, fmt.Sprintf("Use names flag %s, which the command does not mark required", token))
			}
			if i+1 >= len(rest) || !useArgToken.MatchString(rest[i+1]) {
				problems = append(problems, fmt.Sprintf("Use flag %s is not followed by a <value> token", token))
			} else {
				i++
			}
		default:
			problems = append(problems, fmt.Sprintf("Use token %q is not <arg>, [<arg>], <arg>..., [<arg>...], --flag <value> or a {...} span", token))
		}
	}
	return problems
}

// shortProblems returns how cmd's Short breaks the form: one line, a lower-case first letter and no final period.
func shortProblems(cmd *cobra.Command) []string {
	short := cmd.Short
	var problems []string
	if first, _ := utf8.DecodeRuneInString(short); !unicode.IsLower(first) {
		problems = append(problems, fmt.Sprintf("Short %q does not start with a lower-case letter", short))
	}
	if strings.Contains(short, "\n") {
		problems = append(problems, "Short spans more than one line")
	}
	if strings.HasSuffix(short, ".") {
		problems = append(problems, fmt.Sprintf("Short %q ends with a period", short))
	}
	return problems
}

// longProblems returns how cmd's Long breaks the form: it stays within maxLongBytes.
func longProblems(cmd *cobra.Command) []string {
	if len(cmd.Long) > maxLongBytes {
		return []string{fmt.Sprintf("Long is %d bytes; the cap is %d, so move the long page into the package doc", len(cmd.Long), maxLongBytes)}
	}
	return nil
}

// formProblems runs the four form checks over cmd.
func formProblems(cmd *cobra.Command) []string {
	var problems []string
	for _, check := range []func(*cobra.Command) []string{audienceProblems, useProblems, shortProblems, longProblems} {
		problems = append(problems, check(cmd)...)
	}
	return problems
}

// formTestCommand builds a leaf with the given Use, Short and annotations.
func formTestCommand(use, short string, annotations map[string]string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Annotations: annotations, Run: func(*cobra.Command, []string) {}}
}

// TestCLIForm_EveryCommand pins the audience, Use, Short and Long form on every command of newRoot() with pure checks that run no invocation.
// The checks are proven first against small synthetic trees, so a negative case fails for the reason named.
func TestCLIForm_EveryCommand(t *testing.T) {
	t.Parallel()

	operator := map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator}
	groupWithOwnPersistentFlag := func() *cobra.Command {
		group := &cobra.Command{Use: "grp", Short: "a group", RunE: clihelp.GroupRunE}
		group.PersistentFlags().String("target", "", "")
		group.AddCommand(formTestCommand("leaf", "a leaf", operator))
		return group
	}
	groupWithAudience := func() *cobra.Command {
		group := &cobra.Command{Use: "grp", Short: "a group", Annotations: operator}
		group.AddCommand(formTestCommand("leaf", "a leaf", operator))
		return group
	}
	requiredFlag := func(required bool) *cobra.Command {
		cmd := formTestCommand("goto --to <producer>", "move a run", operator)
		cmd.Flags().String("to", "", "")
		if required {
			_ = cmd.MarkFlagRequired("to")
		}
		return cmd
	}

	synthetic := []struct {
		name  string
		check func(*cobra.Command) []string
		cmd   func() *cobra.Command
		// want is a substring of a reported problem, empty when the command must pass.
		want string
	}{
		{"audience: a leaf without one", audienceProblems, func() *cobra.Command { return formTestCommand("leaf", "a leaf", nil) }, "no audience"},
		{"audience: a value outside the set", audienceProblems, func() *cobra.Command {
			return formTestCommand("leaf", "a leaf", map[string]string{clihelp.AudienceAnnotation: "everyone"})
		}, "outside"},
		{"audience: a group carrying one", audienceProblems, groupWithAudience, "non-invocable"},
		{"audience: a hidden command that is not internal", audienceProblems, func() *cobra.Command {
			cmd := formTestCommand("leaf", "a leaf", operator)
			cmd.Hidden = true
			return cmd
		}, "hidden"},
		{"audience: a group with only its own persistent flags", audienceProblems, groupWithOwnPersistentFlag, ""},
		{"use: arguments, optionals, variadics and a payload span", useProblems, func() *cobra.Command {
			return formTestCommand("leaf <id> [<slug>] <path>... [<more>...] {id, tags?}", "a leaf", operator)
		}, ""},
		{"use: a nested payload span", useProblems, func() *cobra.Command { return formTestCommand("leaf {a, b: {c}}", "a leaf", operator) }, ""},
		{"use: an unbalanced span", useProblems, func() *cobra.Command { return formTestCommand("leaf {a, b", "a leaf", operator) }, "unbalanced"},
		{"use: an upper-case argument", useProblems, func() *cobra.Command { return formTestCommand("leaf <ID>", "a leaf", operator) }, "not <arg>"},
		{"use: a bare word", useProblems, func() *cobra.Command { return formTestCommand("leaf slug", "a leaf", operator) }, "not <arg>"},
		{"use: a required flag with its value", useProblems, func() *cobra.Command { return requiredFlag(true) }, ""},
		{"use: a flag cobra does not mark required", useProblems, func() *cobra.Command { return requiredFlag(false) }, "not mark required"},
		{"short: an upper-case first letter", shortProblems, func() *cobra.Command { return formTestCommand("leaf", "A leaf", operator) }, "lower-case"},
		{"short: a final period", shortProblems, func() *cobra.Command { return formTestCommand("leaf", "a leaf.", operator) }, "period"},
		{"short: two lines", shortProblems, func() *cobra.Command { return formTestCommand("leaf", "a leaf\nmore", operator) }, "more than one line"},
		{"short: a lower-case phrase", shortProblems, func() *cobra.Command { return formTestCommand("leaf", "list the open issues", operator) }, ""},
		{"long: one byte over the cap", longProblems, func() *cobra.Command {
			cmd := formTestCommand("leaf", "a leaf", operator)
			cmd.Long = strings.Repeat("x", maxLongBytes+1)
			return cmd
		}, "cap"},
		{"long: exactly the cap", longProblems, func() *cobra.Command {
			cmd := formTestCommand("leaf", "a leaf", operator)
			cmd.Long = strings.Repeat("x", maxLongBytes)
			return cmd
		}, ""},
	}
	for _, tc := range synthetic {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := tc.check(tc.cmd())
			switch {
			case tc.want == "" && len(problems) > 0:
				t.Errorf("problems = %v; want none", problems)
			case tc.want != "" && !strings.Contains(strings.Join(problems, "; "), tc.want):
				t.Errorf("problems = %v; want one containing %q", problems, tc.want)
			}
		})
	}

	walked := 0
	walkCommands(newRoot(), func(cmd *cobra.Command) {
		walked++
		if problems := formProblems(cmd); len(problems) > 0 {
			t.Errorf("%s: %s", cmd.CommandPath(), strings.Join(problems, "; "))
		}
	})
	if walked < 2 {
		t.Fatalf("walked %d commands; the tree walk is not reaching the mounted modules", walked)
	}
}
