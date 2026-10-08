// cardgate.go derives each card's gate command, which begin-batch and recover-batch render into the implementer prompt's card_gates marker.
// Go derives the command and the fork runs it;
// Go never runs it, so a fork that skips it is caught by the plan-level verify gate.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// cardGateCommand returns the gate command for card: build and test everything, then the integration-tagged tests of each package directory the card's targets live in, then the comment line-break lint.
// A directory drops out when every target in it is a Delete, or when it neither names Go source nor holds a .go file in worktree, so a stencil or doc folder never reaches `go test`.
// A directory inside a nested module, as the plan stands once card has run, is tested by `go -C <module> test` with a module-relative argument, one step per module in first-seen order after the root-module step.
// An integration step is omitted when no directory survives for it; the lint step is always last.
func cardGateCommand(plan *planparser.Plan, card planparser.Card, worktree string) string {
	command := "go build ./... && go test ./..."
	modules := planparser.NestedModules(plan, card.Number, worktree)
	var rootDirs, moduleOrder []string
	moduleDirs := map[string][]string{}
	for _, td := range planparser.CardTargetDirs(plan, card) {
		if td.DeleteOnly || !(td.NamesGo || holdsGoFile(filepath.Join(worktree, filepath.FromSlash(td.Dir)))) {
			continue
		}
		module := planparser.ModuleOf(modules, td.Dir)
		if module == "." {
			rootDirs = append(rootDirs, packageArg(td.Dir))
			continue
		}
		if _, seen := moduleDirs[module]; !seen {
			moduleOrder = append(moduleOrder, module)
		}
		moduleDirs[module] = append(moduleDirs[module], packageArg(moduleRelative(module, td.Dir)))
	}
	if len(rootDirs) > 0 {
		command += " && go test -tags integration " + strings.Join(rootDirs, " ")
	}
	for _, module := range moduleOrder {
		command += " && go -C " + module + " test -tags integration " + strings.Join(moduleDirs[module], " ")
	}
	return command + " && lyx loom lint-comments"
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

// renderCardGates renders one line per card in declared order, pairing the card pointer with its gate command.
// Pointers are spelled as renderCardPointers spells them.
func renderCardGates(plan *planparser.Plan, cards []planparser.Card, planDirDisplay, worktree string) string {
	lines := make([]string, 0, len(cards))
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("- `%s/%s`: `%s`", planDirDisplay, filepath.Base(c.SourcePath), cardGateCommand(plan, c, worktree)))
	}
	return strings.Join(lines, "\n")
}
