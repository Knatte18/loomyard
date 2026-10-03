// cardgate.go derives each card's gate command, which begin-batch and recover-batch render into the implementer prompt's card_gates marker.
// Go derives the command and the fork runs it; Go never runs it, so a fork that skips it is caught by the plan-level verify gate.

package websterengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// cardGateCommand returns the gate command for card: build and test everything, then the integration-tagged tests of each package directory the card's targets live in.
// A directory drops out when every target in it is a Delete, or when it neither names Go source nor holds a .go file in worktree, so a stencil or doc folder never reaches `go test`.
// The integration step is omitted when no directory survives.
func cardGateCommand(plan *planparser.Plan, card planparser.Card, worktree string) string {
	command := "go build ./... && go test ./..."
	var dirs []string
	for _, td := range planparser.CardTargetDirs(plan, card) {
		if td.DeleteOnly || !(td.NamesGo || holdsGoFile(filepath.Join(worktree, filepath.FromSlash(td.Dir)))) {
			continue
		}
		dirs = append(dirs, packageArg(td.Dir))
	}
	if len(dirs) == 0 {
		return command
	}
	return command + " && go test -tags integration " + strings.Join(dirs, " ")
}

// packageArg spells a worktree-relative directory as a `go test` package argument.
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

// renderCardGates renders one line per card in declared order, pairing the card pointer with its gate command; pointers are spelled as renderCardPointers spells them.
func renderCardGates(plan *planparser.Plan, cards []planparser.Card, planDirDisplay, worktree string) string {
	lines := make([]string, 0, len(cards))
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("- `%s/%s`: `%s`", planDirDisplay, filepath.Base(c.SourcePath), cardGateCommand(plan, c, worktree)))
	}
	return strings.Join(lines, "\n")
}
