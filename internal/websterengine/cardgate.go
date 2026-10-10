// cardgate.go derives each card's gate steps, which begin-batch and recover-batch render into the implementer prompt's card_gates marker.
// Go derives the command and the fork runs it;
// Go never runs it, so a fork that skips it is caught by the plan-level verify gate.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// gateScope is the Go package arguments a set of cards touches, grouped by module.
type gateScope struct {
	// rootDirs are the root module's package arguments.
	rootDirs []string

	// modules are the nested modules in first-seen order, each with its module-relative package arguments.
	modules []moduleScope
}

// moduleScope is one nested module's module-relative package arguments.
type moduleScope struct {
	module string
	dirs   []string
}

// gateScopeOf returns the package arguments cards' targets live in, each once, root module first and each nested module in first-seen order.
// A directory drops out when every target in it is a Delete, or when it neither names Go source nor holds a .go file in worktree, so a stencil or doc folder never reaches `go test`.
// A directory belongs to a nested module as the plan stands once the last of cards has run.
func gateScopeOf(plan *planparser.Plan, cards []planparser.Card, worktree string) gateScope {
	var scope gateScope
	if len(cards) == 0 {
		return scope
	}
	modules := planparser.NestedModules(plan, cards[len(cards)-1].Number, worktree)
	for _, card := range cards {
		for _, td := range planparser.CardTargetDirs(plan, card) {
			if td.DeleteOnly || !(td.NamesGo || holdsGoFile(filepath.Join(worktree, filepath.FromSlash(td.Dir)))) {
				continue
			}
			module := planparser.ModuleOf(modules, td.Dir)
			if module == "." {
				scope.rootDirs = appendUnique(scope.rootDirs, packageArg(td.Dir))
				continue
			}
			index := slices.IndexFunc(scope.modules, func(m moduleScope) bool { return m.module == module })
			if index < 0 {
				scope.modules = append(scope.modules, moduleScope{module: module})
				index = len(scope.modules) - 1
			}
			scope.modules[index].dirs = appendUnique(scope.modules[index].dirs, packageArg(moduleRelative(module, td.Dir)))
		}
	}
	return scope
}

// appendUnique appends value to values unless it is already there.
func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

// steps renders one `lyx gate test` step per module, the root module's first, each carrying `--tags tags` when tags is not empty.
// A scope with no directory renders no step.
func (s gateScope) steps(tags string) []string {
	flags := ""
	if tags != "" {
		flags = " --tags " + tags
	}
	var steps []string
	if len(s.rootDirs) > 0 {
		steps = append(steps, "lyx gate test"+flags+" "+strings.Join(s.rootDirs, " "))
	}
	for _, m := range s.modules {
		steps = append(steps, "lyx gate test -C "+m.module+flags+" "+strings.Join(m.dirs, " "))
	}
	return steps
}

// cardGateCommand returns the gate for card as its ordered steps: `lyx gate test` over the card's own packages, one step per module, then the comment line-break lint.
// A card with no surviving package directory runs only the lint.
func cardGateCommand(plan *planparser.Plan, card planparser.Card, worktree string) []string {
	return append(gateScopeOf(plan, []planparser.Card{card}, worktree).steps(""), "lyx loom lint-comments")
}

// moduleRelative returns dir relative to module, "." for the module directory itself.
func moduleRelative(module, dir string) string {
	if dir == module {
		return "."
	}
	return strings.TrimPrefix(dir, module+"/")
}

// packageArg spells a directory as a `go test` package argument.
func packageArg(dir string) string {
	if dir == "." {
		return "."
	}
	return "./" + dir
}

// holdsGoFile reports whether dir directly holds a .go file.
func holdsGoFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// renderCardGates renders each card in declared order as its pointer bullet with one indented sub-bullet per gate step.
// Each step is its own backticked command, never chained, so a fork runs each as its own Bash call.
// Pointers are spelled as renderCardPointers spells them.
func renderCardGates(plan *planparser.Plan, cards []planparser.Card, planDirDisplay, worktree string) string {
	var lines []string
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("- `%s/%s`:", planDirDisplay, filepath.Base(c.SourcePath)))
		for _, step := range cardGateCommand(plan, c, worktree) {
			lines = append(lines, fmt.Sprintf("  - `%s`", step))
		}
	}
	return strings.Join(lines, "\n")
}
