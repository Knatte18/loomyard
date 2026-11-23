// changedtests.go names the test methods a diff adds or changes by comparing the base and head text of `.cs` files.

package dotnetimpact

import (
	"slices"
	"strings"
)

// ChangedTest is one test method that a diff adds or changes.
type ChangedTest struct {
	// File is the worktree-relative slash path of the method's source file.
	File string
	// Class is the fully qualified name of the method's class; a nested class is joined to its outer class by `+`.
	Class string
	// Method is the method's name.
	Method string
	// Abstract reports that the class is abstract, so no filter term names the test by this class.
	Abstract bool
}

// FilterTerm returns the `dotnet test` filter term that selects the test: `FullyQualifiedName~` and Class.Method.
func (t ChangedTest) FilterTerm() string {
	return "FullyQualifiedName~" + t.Label()
}

// Label returns the test as Class.Method.
func (t ChangedTest) Label() string {
	return t.Class + "." + t.Method
}

// methodIdentity is a test method's class, name and source text.
type methodIdentity struct {
	class, method, text string
}

// ChangedTests returns the test methods that head adds or changes against base, sorted by file and then source order.
// base and head map worktree-relative slash paths of `.cs` files to their text.
// A file a side lacks is absent from that side.
// A head test method is changed when no base test method of the same class and name, in any file, has the same text.
// So a test moved to another file unchanged is not returned.
// An overload is compared with every same-named base method.
// A test method only base holds is not returned.
func ChangedTests(base, head map[string][]byte) []ChangedTest {
	known := map[methodIdentity]bool{}
	for _, rel := range sortedKeys(base) {
		for _, decl := range parseSource(rel, base[rel]).Types {
			for _, method := range decl.TestMethods {
				known[methodIdentity{qualifiedName(decl), method.Name, method.Text}] = true
			}
		}
	}

	var changed []ChangedTest
	for _, rel := range sortedKeys(head) {
		for _, decl := range parseSource(rel, head[rel]).Types {
			if decl.Kind != "class" {
				continue
			}
			class := qualifiedName(decl)
			for _, method := range decl.TestMethods {
				if !known[methodIdentity{class, method.Name, method.Text}] {
					changed = append(changed, ChangedTest{File: rel, Class: class, Method: method.Name, Abstract: decl.Abstract})
				}
			}
		}
	}
	return changed
}

// qualifiedName returns the type's fully qualified name: its namespace, then its outer types and itself joined by `+`.
func qualifiedName(t typeDecl) string {
	name := strings.Join(append(slices.Clone(t.Outer), t.Name), "+")
	if t.Namespace != "" {
		name = t.Namespace + "." + name
	}
	return name
}
