// shortcuts_test.go checks the embedded shortcut file against the real root and pins how the mounted shortcuts list in help and in the command index.

package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shortcutcli"
)

// shortcutProblems returns one problem per defect of shortcuts against root.
// A name must not equal a root command outside the shortcut group nor repeat, and an argv must be non-empty and resolve, through root.Find, to an invocable command that is not the root and not a shortcut.
// An argv that resolves to `reed attach` must be the shortcut's last, since the attach replaces the process on Unix and nothing runs after it.
func shortcutProblems(root *cobra.Command, shortcuts []shortcutcli.Shortcut) []string {
	var problems []string
	seen := map[string]bool{}
	for _, shortcut := range shortcuts {
		for _, child := range root.Commands() {
			if child.GroupID != shortcutcli.GroupID && child.Name() == shortcut.Name {
				problems = append(problems, fmt.Sprintf("shortcut %q is also a root command", shortcut.Name))
			}
		}
		if seen[shortcut.Name] {
			problems = append(problems, fmt.Sprintf("shortcut %q is declared twice", shortcut.Name))
		}
		seen[shortcut.Name] = true

		for position, argv := range shortcut.Argvs {
			if len(argv) == 0 {
				problems = append(problems, fmt.Sprintf("shortcut %q has an empty argv", shortcut.Name))
				continue
			}
			target, _, err := root.Find(argv)
			switch {
			case err != nil:
				problems = append(problems, fmt.Sprintf("shortcut %q argv %v does not resolve: %v", shortcut.Name, argv, err))
			case target == root:
				problems = append(problems, fmt.Sprintf("shortcut %q argv %v resolves to the root", shortcut.Name, argv))
			case !target.Runnable() || !clihelp.Invocable(target):
				problems = append(problems, fmt.Sprintf("shortcut %q argv %v resolves to %q, which is not an invocable command", shortcut.Name, argv, target.CommandPath()))
			case target.GroupID == shortcutcli.GroupID:
				problems = append(problems, fmt.Sprintf("shortcut %q argv %v resolves to the shortcut %q", shortcut.Name, argv, target.CommandPath()))
			}
			if err == nil && target.CommandPath() == "lyx reed attach" && position < len(shortcut.Argvs)-1 {
				problems = append(problems, fmt.Sprintf("shortcut %q runs %v after reed attach, which never returns", shortcut.Name, shortcut.Argvs[position+1:]))
			}
		}
	}
	return problems
}

func TestShortcuts_FileAndRootListing(t *testing.T) {
	t.Parallel()

	root := newRoot()
	shortcuts, err := shortcutcli.Load()
	if err != nil {
		t.Fatalf("shortcutcli.Load() = %v", err)
	}
	if problems := shortcutProblems(root, shortcuts); len(problems) > 0 {
		t.Errorf("the shortcut file has problems against the root: %s", strings.Join(problems, "; "))
	}

	var mounted, declared []string
	for _, child := range root.Commands() {
		if child.GroupID == shortcutcli.GroupID {
			mounted = append(mounted, child.Name())
		}
	}
	for _, shortcut := range shortcuts {
		declared = append(declared, shortcut.Name)
	}
	slices.Sort(mounted)
	slices.Sort(declared)
	if !slices.Equal(mounted, declared) {
		t.Errorf("root children in group %q = %v, want the file's shortcuts %v", shortcutcli.GroupID, mounted, declared)
	}

	synthetic := []struct {
		name      string
		shortcuts []shortcutcli.Shortcut
		want      string
	}{
		{"a name equal to a module", []shortcutcli.Shortcut{{Name: "reed", Argvs: [][]string{{"reed", "attach"}}}}, "is also a root command"},
		{"a name used twice", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"reed", "attach"}}}, {Name: "go-to", Argvs: [][]string{{"reed", "attach"}}}}, "is declared twice"},
		{"an empty argv", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{}}}}, "has an empty argv"},
		{"a bare group", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"reed"}}}}, "not an invocable command"},
		{"an unknown command", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"nosuch", "verb"}}}}, "does not resolve"},
		{"a flag alone", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"--verbose"}}}}, "resolves to the root"},
		{"another shortcut", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"prime"}}}}, "resolves to the shortcut"},
		{"an argv after reed attach", []shortcutcli.Shortcut{{Name: "go-to", Argvs: [][]string{{"reed", "attach"}, {"orch", "start"}}}}, "after reed attach, which never returns"},
	}
	for _, tt := range synthetic {
		t.Run(tt.name, func(t *testing.T) {
			problems := shortcutProblems(root, tt.shortcuts)
			if !slices.ContainsFunc(problems, func(problem string) bool { return strings.Contains(problem, tt.want) }) {
				t.Errorf("shortcutProblems = %q, want a problem containing %q", problems, tt.want)
			}
		})
	}

	var help strings.Builder
	if code := run([]string{"--help"}, &help); code != 0 {
		t.Fatalf("run(--help) = %d, output:\n%s", code, help.String())
	}
	heading, attach, prime, additional := strings.Index(help.String(), "Shortcuts:"), strings.Index(help.String(), "\n  attach "), strings.Index(help.String(), "\n  prime "), strings.Index(help.String(), "Additional Commands:")
	if heading < 0 || attach < heading || prime < attach || additional < prime {
		t.Errorf("lyx --help lacks the Shortcuts: heading with attach and prime before Additional Commands:\n%s", help.String())
	}

	var index strings.Builder
	if code := run([]string{"help", "index"}, &index); code != 0 {
		t.Fatalf("run(help index) = %d, output:\n%s", code, index.String())
	}
	for _, line := range []string{
		"- `prime [<args>...]`: shortcut: lyx orch start, then lyx reed attach",
		"- `attach [<args>...]`: shortcut: lyx reed attach",
		"- `pause [<args>...]`: shortcut: lyx tender pause",
		"- `resume [<args>...]`: shortcut: lyx tender resume",
		"- `p [<args>...]`: shortcut: lyx tender pause",
		"- `r [<args>...]`: shortcut: lyx tender resume",
	} {
		if !strings.Contains(index.String(), line) {
			t.Errorf("the operator index lacks the line %q:\n%s", line, index.String())
		}
	}
}
