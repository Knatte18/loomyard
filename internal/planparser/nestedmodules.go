// nestedmodules.go holds NestedModules and ModuleOf, the one derivation of which directories are nested Go modules.
// A nested module is a directory below the worktree root holding a go.mod, on disk or declared by a card's Create, Delete or Rename of that file.

package planparser

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// goModFile is the file whose presence makes a directory a Go module.
const goModFile = "go.mod"

// NestedModules returns the sorted, slash-separated, worktree-relative nested-module directories below worktreeRoot, as the plan stands once card through has run.
// The set is every go.mod on disk, plus each directory a Create of a go.mod names on a card numbered at or below through, minus each one a Delete names, with a Rename pair counting as a Delete of its Old side and a Create of its New side.
// Cards apply in number order, so a later card's Create or Delete wins over an earlier one.
// The walk skips directories whose name starts with "." or "_" and directories named vendor, since the go tool never builds them; testdata is walked.
// The root's own go.mod is not a nested module, and a through of 0 counts no card.
// A Move card is not counted: its destination lives in Intent prose.
// A nil plan reads as the default language.
func NestedModules(plan *Plan, through int, worktreeRoot string) []string {
	if plan == nil {
		plan = &Plan{}
	}
	modules := map[string]bool{}
	for _, dir := range goModDirsOnDisk(worktreeRoot) {
		modules[dir] = true
	}

	cards := slices.Clone(plan.Cards)
	slices.SortStableFunc(cards, func(a, b Card) int { return a.Number - b.Number })
	for _, card := range cards {
		if card.Number > through {
			continue
		}
		for _, group := range card.TargetGroups {
			switch group.Type {
			case CardTypeCreate:
				for _, ref := range group.Refs {
					setGoModDir(plan, modules, ref, true)
				}
			case CardTypeDelete:
				for _, ref := range group.Refs {
					setGoModDir(plan, modules, ref, false)
				}
			case CardTypeRename:
				for _, pair := range group.Pairs {
					setGoModDir(plan, modules, pair.Old, false)
					setGoModDir(plan, modules, pair.New, true)
				}
			}
		}
	}

	result := make([]string, 0, len(modules))
	for dir := range modules {
		result = append(result, dir)
	}
	slices.Sort(result)
	return result
}

// ModuleOf returns the innermost entry of modules that is dir or an ancestor of dir by whole path segments, and "." when none is, which is the root module.
func ModuleOf(modules []string, dir string) string {
	dir = path.Clean(dir)
	best := "."
	for _, module := range modules {
		if module != dir && !strings.HasPrefix(dir, module+"/") {
			continue
		}
		if best == "." || len(module) > len(best) {
			best = module
		}
	}
	return best
}

// setGoModDir records ref in modules as present or absent when it names a nested go.mod; any other ref is ignored.
func setGoModDir(plan *Plan, modules map[string]bool, ref string, present bool) {
	file, ok := RefFile(plan, ref)
	if !ok || path.Base(file) != goModFile {
		return
	}
	dir := path.Dir(file)
	if dir == "." {
		return
	}
	if present {
		modules[dir] = true
		return
	}
	delete(modules, dir)
}

// goModDirsOnDisk returns every directory below root, as a slash-separated relative path, that holds a go.mod and that the go tool would walk into.
func goModDirsOnDisk(root string) []string {
	var dirs []string
	// An unreadable subtree contributes no modules, so the walk's errors are dropped.
	_ = filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if current == root {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "vendor" {
			return fs.SkipDir
		}
		if info, statErr := os.Stat(filepath.Join(current, goModFile)); statErr == nil && !info.IsDir() {
			rel, relErr := filepath.Rel(root, current)
			if relErr == nil {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return dirs
}
