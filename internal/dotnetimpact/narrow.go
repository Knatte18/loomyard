// narrow.go declares the pure core of the derivation: from scanned sources, project files and a seed, the test projects and filter terms a narrowed run takes.

package dotnetimpact

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
)

// Scope is the answer of a derivation.
type Scope struct {
	// Whole is set when the derivation cannot narrow the run, and Fallback says why.
	Whole    bool
	Fallback string
	// Projects are the test projects holding a selected class, in path order.
	Projects []Project
	// NoTestClass is set when a non-empty seed selected no test class.
	NoTestClass bool
}

// Project is one test project of a narrowed run.
type Project struct {
	// Path is the `.csproj` path relative to the tier's `dir`.
	Path string
	// Terms are the class filter terms of the project's selected classes, sorted and without repeats.
	Terms []string
}

// seed is what a derivation starts from.
type seed struct {
	// Files are the changed or targeted `.cs` paths relative to the tier's `dir`.
	Files map[string]bool
	// Types are the seeded type names.
	Types map[string]bool
}

// classRef points at one type declaration of the scanned sources.
type classRef struct {
	// index is the position among all declarations of all sources.
	index int
	// file indexes the sources.
	file int
	decl typeDecl
}

// narrow derives the Scope of seed over the scanned sources.
// projects lists every `.csproj` path the scan found, relative to the tier's `dir`.
// An empty seed is the empty scope, and a non-empty seed that selects no test class sets NoTestClass.
func narrow(sources []sourceFile, projects []string, s seed) Scope {
	if len(s.Files) == 0 && len(s.Types) == 0 {
		return Scope{}
	}
	index := newProjectIndex(projects)
	for _, file := range sortedKeys(s.Files) {
		if len(index.nearest(file)) == 0 {
			return wholeScope(fmt.Sprintf("no .csproj holds %s", file))
		}
	}

	var classes []classRef
	byName := map[string][]classRef{}
	for i, source := range sources {
		for _, decl := range source.Types {
			ref := classRef{index: len(classes), file: i, decl: decl}
			classes = append(classes, ref)
			byName[decl.Name] = append(byName[decl.Name], ref)
		}
	}

	closure := typeClosure(sources, s)
	selected := selectClasses(sources, classes, byName, closure, s)

	terms := map[string][]string{}
	for _, ref := range classes {
		if !selected[ref.index] || ref.decl.Abstract {
			continue
		}
		file := sources[ref.file].Rel
		owners := index.nearest(file)
		switch {
		case len(owners) == 0:
			return wholeScope(fmt.Sprintf("no .csproj holds %s", file))
		case len(owners) > 1:
			return wholeScope(fmt.Sprintf("%s holds more than one .csproj", index.nearestDir(file)))
		}
		terms[owners[0]] = append(terms[owners[0]], filterTerm(ref.decl))
	}
	if len(terms) == 0 {
		return Scope{NoTestClass: true}
	}
	scope := Scope{}
	for _, projectPath := range sortedKeys(terms) {
		list := terms[projectPath]
		sort.Strings(list)
		scope.Projects = append(scope.Projects, Project{Path: projectPath, Terms: slices.Compact(list)})
	}
	return scope
}

// wholeScope is the Scope of a run that cannot be narrowed.
func wholeScope(reason string) Scope {
	return Scope{Whole: true, Fallback: reason}
}

// typeClosure returns the seeded type names, the types declared in the seed files, and every type declared in a file whose identifiers name a type already in the set, until nothing changes.
func typeClosure(sources []sourceFile, s seed) map[string]bool {
	closure := map[string]bool{}
	for name := range s.Types {
		closure[name] = true
	}
	for _, source := range sources {
		if s.Files[source.Rel] {
			for _, decl := range source.Types {
				closure[decl.Name] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, source := range sources {
			if !namesAny(source, closure) {
				continue
			}
			for _, decl := range source.Types {
				if !closure[decl.Name] {
					closure[decl.Name] = true
					changed = true
				}
			}
		}
	}
	return closure
}

// namesAny reports whether the file's identifiers name any type of the set.
func namesAny(source sourceFile, names map[string]bool) bool {
	for name := range names {
		if source.Identifiers[name] {
			return true
		}
	}
	return false
}

// selectClasses returns the test classes the seed selects: a test class whose file is a seed file or names a type of the closure, and every class deriving, transitively, from a selected abstract one.
func selectClasses(sources []sourceFile, classes []classRef, byName map[string][]classRef, closure map[string]bool, s seed) []bool {
	selected := make([]bool, len(classes))
	for _, ref := range classes {
		source := sources[ref.file]
		if isTestClass(ref.decl, byName, map[string]bool{}) && (s.Files[source.Rel] || namesAny(source, closure)) {
			selected[ref.index] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, ref := range classes {
			if selected[ref.index] || ref.decl.Base == "" {
				continue
			}
			for _, base := range byName[ref.decl.Base] {
				if selected[base.index] && base.decl.Abstract {
					selected[ref.index] = true
					changed = true
					break
				}
			}
		}
	}
	return selected
}

// isTestClass reports whether decl is a class carrying a test attribute, or a non-abstract class whose base, followed through the scanned sources by name, reaches one.
// visited guards a cyclic base chain.
func isTestClass(decl typeDecl, byName map[string][]classRef, visited map[string]bool) bool {
	if decl.Kind != "class" {
		return false
	}
	if decl.TestAttributed || decl.HasTestMethod {
		return true
	}
	return !decl.Abstract && baseReachesTest(decl.Base, byName, visited)
}

// baseReachesTest reports whether a class named name, or any base of one, carries a test attribute.
func baseReachesTest(name string, byName map[string][]classRef, visited map[string]bool) bool {
	if name == "" || visited[name] {
		return false
	}
	visited[name] = true
	for _, ref := range byName[name] {
		if ref.decl.TestAttributed || ref.decl.HasTestMethod || baseReachesTest(ref.decl.Base, byName, visited) {
			return true
		}
	}
	return false
}

// filterTerm returns the `dotnet test` filter term selecting the class: `FullyQualifiedName~` and the fully qualified name, a nested class joined to its outer class by `+`.
// An abstract class has no term.
func filterTerm(t typeDecl) string {
	if t.Abstract {
		return ""
	}
	name := strings.Join(append(slices.Clone(t.Outer), t.Name), "+")
	if t.Namespace != "" {
		name = t.Namespace + "." + name
	}
	return "FullyQualifiedName~" + name
}

// projectIndex answers which `.csproj` files sit in which directory.
type projectIndex struct {
	byDir map[string][]string
}

func newProjectIndex(projects []string) projectIndex {
	index := projectIndex{byDir: map[string][]string{}}
	for _, project := range projects {
		dir := path.Dir(project)
		index.byDir[dir] = append(index.byDir[dir], project)
	}
	return index
}

// nearestDir returns the nearest ancestor directory of file, up to the tier's `dir`, that holds a `.csproj`, or "" when none does.
func (index projectIndex) nearestDir(file string) string {
	for dir := path.Dir(file); ; dir = path.Dir(dir) {
		if len(index.byDir[dir]) > 0 {
			return dir
		}
		if dir == "." || dir == "/" {
			return ""
		}
	}
}

// nearest returns the `.csproj` paths of the directory nearestDir finds.
func (index projectIndex) nearest(file string) []string {
	dir := index.nearestDir(file)
	if dir == "" {
		return nil
	}
	return index.byDir[dir]
}

// sortedKeys returns the keys of m in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
