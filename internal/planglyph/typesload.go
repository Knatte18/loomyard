// typesload.go gives caller-uncovered type information: the loader seam, its real go list implementation, and the walk that turns loaded packages into resolved reference lines.
// A load that fails or times out is logged and answered by the import-path scan, never by an error.

package planglyph

import (
	"context"
	"go/ast"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
	"golang.org/x/tools/go/packages"
)

// typesLoader loads the type-checked packages of the directories dirs, worktree-relative, under root.
type typesLoader interface {
	load(root string, dirs []string) ([]*packages.Package, error)
}

// goListLoader is the typesLoader that runs go list through go/packages, offline and bounded by timeout.
type goListLoader struct {
	timeout time.Duration
}

// defaultTypesLoader is the loader the exported plan gate entry points use.
var defaultTypesLoader typesLoader = goListLoader{timeout: 3 * time.Minute}

// load runs packages.Load over dirs from root, test variants and the integration, tmux and llm build tags included.
// Imports come from compiler export data, so only the packages of dirs are type-checked from source.
// It returns an error when the load fails or outlives the loader's timeout.
func (l goListLoader) load(root string, dirs []string) ([]*packages.Package, error) {
	patterns := make([]string, len(dirs))
	for i, dir := range dirs {
		patterns[i] = "./" + dir
	}

	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	config := &packages.Config{
		Context:    ctx,
		Dir:        root,
		Env:        append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=readonly"),
		Tests:      true,
		BuildFlags: []string{"-tags=integration,tmux,llm"},
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
	}

	logger.Info("planglyph: spawning go list for caller-uncovered", "worktree_root", root, "dirs", len(dirs), "timeout", l.timeout)
	started := time.Now()
	loaded, err := packages.Load(config, patterns...)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		logger.Warn("planglyph: go list for caller-uncovered failed", "worktree_root", root, "elapsed", time.Since(started), "error", err)
		return nil, err
	}
	logger.Info("planglyph: go list for caller-uncovered finished", "worktree_root", root, "packages", len(loaded), "elapsed", time.Since(started))
	return loaded, nil
}

// loadTypedReferences loads the packages that can hold a subject's references and returns, from them, the checked files and the typed and unresolved references per subject.
// The load covers the directories of candidates plus each subject's declaring directory, less every directory inside a nested module, since it runs from the root module.
// It is skipped, with no loader called, when root holds no go.mod or no directory is left.
// A failed load is logged by the loader and leaves everything empty.
func loadTypedReferences(plan *planparser.Plan, root string, subjects []coverageSubject, candidates []string, loader typesLoader) (checked map[string]bool, typed, unresolved []map[string][]int) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return typedReferences(nil, root, subjects)
	}

	modules := planparser.NestedModules(plan, 0, root)
	wanted := make(map[string]bool)
	for _, file := range candidates {
		wanted[path.Dir(file)] = true
	}
	for _, subject := range subjects {
		wanted[subject.dir] = true
	}
	var dirs []string
	for dir := range wanted {
		if planparser.ModuleOf(modules, dir) == "." {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		return typedReferences(nil, root, subjects)
	}
	sort.Strings(dirs)

	loaded, err := loader.load(root, dirs)
	if err != nil {
		return typedReferences(nil, root, subjects)
	}
	return typedReferences(loaded, root, subjects)
}

// typedReferences walks the syntax of every loaded package that carries types info.
// checked holds the worktree-relative files so walked.
// references holds, per subject, the lines of each checked file where an identifier's used object is the subject, deduplicated across a package and its test variant.
// unresolved holds, per subject, the lines of a checked file where an identifier named like the subject has neither a used nor a defined object, so the scan must decide it.
func typedReferences(pkgs []*packages.Package, root string, subjects []coverageSubject) (checked map[string]bool, references []map[string][]int, unresolved []map[string][]int) {
	checked = make(map[string]bool)
	references = make([]map[string][]int, len(subjects))
	unresolved = make([]map[string][]int, len(subjects))
	for i := range subjects {
		references[i] = make(map[string][]int)
		unresolved[i] = make(map[string][]int)
	}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, syntax := range pkg.Syntax {
			relative, err := filepath.Rel(root, pkg.Fset.Position(syntax.Package).Filename)
			if err != nil || strings.HasPrefix(relative, "..") {
				continue
			}
			relative = filepath.ToSlash(relative)
			checked[relative] = true

			ast.Inspect(syntax, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				for i, subject := range subjects {
					if ident.Name != subject.name {
						continue
					}
					line := pkg.Fset.Position(ident.Pos()).Line
					switch obj := pkg.TypesInfo.Uses[ident]; {
					case obj != nil:
						if isSubjectObject(subject, obj) && !insideOwnSpan(subject, relative, line) {
							references[i][relative] = append(references[i][relative], line)
						}
					case pkg.TypesInfo.Defs[ident] == nil:
						unresolved[i][relative] = append(unresolved[i][relative], line)
					}
				}
				return true
			})
		}
	}

	for i := range subjects {
		for file, lines := range references[i] {
			slices.Sort(lines)
			references[i][file] = slices.Compact(lines)
		}
		for file, lines := range unresolved[i] {
			slices.Sort(lines)
			unresolved[i][file] = slices.Compact(lines)
		}
	}
	return checked, references, unresolved
}

// isSubjectObject reports whether obj is the member subject names.
// A package-level subject is an object of the subject's package scope with its name.
// A method subject is a method of that name on the subject's receiver type in the subject's package, reached through a pointer, a generic instantiation, a promotion or a method value alike; a method of an interface or of another type is not.
func isSubjectObject(subject coverageSubject, obj types.Object) bool {
	if obj.Name() != subject.name || obj.Pkg() == nil || obj.Pkg().Path() != subject.importPath {
		return false
	}
	if !subject.isMethod {
		return obj.Parent() == obj.Pkg().Scope()
	}

	method, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	signature, ok := method.Origin().Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false
	}
	receiver := signature.Recv().Type()
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = pointer.Elem()
	}
	named, ok := receiver.(*types.Named)
	return ok && named.Origin().Obj().Name() == subject.receiver
}
