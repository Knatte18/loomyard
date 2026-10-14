// guardstep.go derives the guard step every card gate ends with: the root module's `//lyx:guard` tests, found by a walk of the worktree that spawns no process.

package websterengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// guardTests is what one render of the card gates found in the root module's test files.
// The zero value renders no guard step.
type guardTests struct {
	// names maps a worktree-relative guard package directory, "." for the root, to its sorted guard test names.
	// It holds only directories with at least one.
	names map[string][]string

	// goFiles maps each directory of names to the worktree-relative, slash-separated paths of the `.go` files it directly holds.
	goFiles map[string][]string

	// scanErr is the error the guard scan returned for a test file's content, nil when it accepted every test file.
	scanErr *impactset.GuardScanError
}

// scanGuardTests finds the root module's `//lyx:guard` tests under worktree.
// A worktree with no `go.mod` at its root yields the zero value.
// The walk skips, below the root, a directory named testdata or vendor, one whose name begins with `_` or `.`, and one holding a `go.mod` with its whole subtree.
// A path the walk cannot read, or a test file that cannot be opened, is an error naming it.
// A guard scan failure sitting in a test file's content is kept in scanErr with no error returned.
func scanGuardTests(worktree string) (guardTests, error) {
	if _, err := os.Stat(filepath.Join(worktree, "go.mod")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return guardTests{}, nil
		}
		return guardTests{}, walkError(filepath.Join(worktree, "go.mod"), err)
	}

	goFiles := map[string][]string{}
	testDirs := map[string]bool{}
	walk := func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return walkError(current, err)
		}
		if entry.IsDir() {
			if current == worktree {
				return nil
			}
			return skipGuardDir(current, entry.Name())
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(worktree, current)
		if err != nil {
			return walkError(current, err)
		}
		relative = filepath.ToSlash(relative)
		dir := path.Dir(relative)
		goFiles[dir] = append(goFiles[dir], relative)
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := os.Open(current)
		if err != nil {
			return walkError(current, err)
		}
		if err := file.Close(); err != nil {
			return walkError(current, err)
		}
		testDirs[dir] = true
		return nil
	}
	if err := filepath.WalkDir(worktree, walk); err != nil {
		return guardTests{}, err
	}

	dirs := make([]string, 0, len(testDirs))
	for dir := range testDirs {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	found, err := impactset.FindGuardTests(worktree, dirs)
	if err != nil {
		var scanErr *impactset.GuardScanError
		if errors.As(err, &scanErr) {
			return guardTests{scanErr: scanErr}, nil
		}
		return guardTests{}, fmt.Errorf("webster: scan the worktree for guard tests: %w", err)
	}

	guards := guardTests{names: map[string][]string{}, goFiles: map[string][]string{}}
	for dir, names := range found {
		if len(names) == 0 {
			continue
		}
		guards.names[dir] = names
		guards.goFiles[dir] = goFiles[dir]
	}
	return guards, nil
}

// step returns the guard step of cards[index], or "" when it renders none, and the step's length.
// The step is `lyx gate test --tags integration <guard packages> -- -run '^(<guard test names>)$'`, the packages in sorted directory order and the names sorted, each once.
// A guard directory drops out for cards[index] when the cards from cards[0] through cards[index] have emptied it,
// so the step never names a directory gone by that card's gate.
// The step is "" when the scan failed or no directory is left.
func (g guardTests) step(plan *planparser.Plan, cards []planparser.Card, index int) (step string, length int) {
	if g.scanErr != nil {
		return "", 0
	}
	dirs := make([]string, 0, len(g.names))
	for dir := range g.names {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)

	var packages, names []string
	for _, dir := range dirs {
		if !g.holdsFilesAfter(plan, cards[:index+1], dir) {
			continue
		}
		packages = append(packages, packageArg(dir))
		for _, name := range g.names[dir] {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	if len(packages) == 0 {
		return "", 0
	}
	slices.Sort(names)
	step = "lyx gate test --tags integration " + strings.Join(packages, " ") + " -- -run '^(" + strings.Join(names, "|") + ")$'"
	return step, len(step)
}

// holdsFilesAfter reports whether guard directory dir still holds a .go file once cards have run in order.
// Each card first removes every file one of its covers covers, then adds each .go file its Create targets and Rename new sides name in dir.
func (g guardTests) holdsFilesAfter(plan *planparser.Plan, cards []planparser.Card, dir string) bool {
	files := map[string]bool{}
	for _, file := range g.goFiles[dir] {
		files[file] = true
	}
	for _, card := range cards {
		for file := range files {
			if slices.ContainsFunc(planparser.CardCovers(card), func(cover string) bool { return planparser.RefCovers(plan, cover, file) }) {
				delete(files, file)
			}
		}
		for _, ref := range cardCreatedRefs(card) {
			if file, ok := planparser.RefFile(plan, ref); ok && path.Ext(file) == ".go" && path.Dir(file) == dir {
				files[file] = true
			}
		}
	}
	return len(files) > 0
}

// cardCreatedRefs returns the refs card brings into existence: its Create targets and the new side of each Rename pair.
func cardCreatedRefs(card planparser.Card) []string {
	var refs []string
	for _, group := range card.TargetGroups {
		switch group.Type {
		case planparser.CardTypeCreate:
			refs = append(refs, group.Refs...)
		case planparser.CardTypeRename:
			for _, pair := range group.Pairs {
				refs = append(refs, pair.New)
			}
		}
	}
	return refs
}

// guardNotice returns the prompt text that replaces the guard step where it could not be rendered, read from stencilsDir, and "" when nothing needs replacing.
// A failed scan renders its error, followed by the fix line when the failing file lies in a package one of cards targets, naming the first such card as planDirDisplay spells its pointer.
// A tooLong above 0, the largest length any card's step had, renders the over-long line.
func guardNotice(plan *planparser.Plan, cards []planparser.Card, planDirDisplay, stencilsDir string, guards guardTests, tooLong int) (string, error) {
	var paragraphs []string
	if guards.scanErr != nil {
		text, err := fillFragment(stencilsDir, "webster-fragment-guard-scan-error", map[string]string{"guard_error": guards.scanErr.Error()})
		if err != nil {
			return "", err
		}
		paragraphs = append(paragraphs, text)

		fileDir := path.Dir(guards.scanErr.File)
		for _, card := range cards {
			if !slices.ContainsFunc(planparser.CardTargetDirs(plan, card), func(target planparser.TargetDir) bool { return target.Dir == fileDir }) {
				continue
			}
			text, err := fillFragment(stencilsDir, "webster-fragment-guard-fix", map[string]string{"guard_file": guards.scanErr.File, "card_path": cardPointerPath(card, planDirDisplay)})
			if err != nil {
				return "", err
			}
			paragraphs = append(paragraphs, text)
			break
		}
	}
	if tooLong > 0 {
		text, err := fillFragment(stencilsDir, "webster-fragment-guard-too-long", map[string]string{"guard_length": strconv.Itoa(tooLong), "guard_limit": strconv.Itoa(impactset.MaxCommandLength)})
		if err != nil {
			return "", err
		}
		paragraphs = append(paragraphs, text)
	}
	return strings.Join(paragraphs, "\n\n"), nil
}

// fillFragment reads the fragment stencil name from stencilsDir, fills its markers with values and returns the text without its banner or trailing newlines.
func fillFragment(stencilsDir, name string, values map[string]string) (string, error) {
	raw, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		return "", err
	}
	filled, err := stencil.Fill(raw, values)
	if err != nil {
		return "", fmt.Errorf("fill fragment %s: %w", name, err)
	}
	return strings.TrimRight(string(filled), "\r\n"), nil
}

// skipGuardDir returns filepath.SkipDir for a directory below the root that the walk leaves out, and nil for one it enters.
func skipGuardDir(current, name string) error {
	if name == "testdata" || name == "vendor" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
		return filepath.SkipDir
	} else if !errors.Is(err, fs.ErrNotExist) {
		return walkError(current, err)
	}
	return nil
}

// walkError wraps a failure to read target while walking the worktree for guard tests.
func walkError(target string, err error) error {
	return fmt.Errorf("webster: walk the worktree for guard tests: %s: %w", target, err)
}
