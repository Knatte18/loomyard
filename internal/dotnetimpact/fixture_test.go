// fixture_test.go holds the C# tree and expectations the derivation tests of every tier share.
// It serves every tier's tests, since the package's one test binary holds them all.

package dotnetimpact

import "strings"

// treeDir is the tier directory the shared tree sits under.
const treeDir = "cs"

// csTree is a small C# tree.
// It holds a library, and a test project with one fixture naming the library's type and one naming nothing.
// It also holds files outside the tier directory.
func csTree() map[string]string {
	return map[string]string{
		"cs/Lib/Lib.csproj":             "<Project />",
		"cs/Lib/Circle.cs":              "namespace Lib; public class Circle { }",
		"cs/Lib.Tests/Lib.Tests.csproj": "<Project />",
		"cs/Lib.Tests/CircleTests.cs":   "namespace Lib.Tests; [TestFixture] public class CircleTests { [Test] public void M() { new Circle(); } }",
		"cs/Lib.Tests/OtherTests.cs":    "namespace Lib.Tests; [TestFixture] public class OtherTests { [Test] public void M() { } }",
		"cs/appsettings.json":           "{}",
		"docs/readme.md":                "docs",
		"notes.txt":                     "class Notes { }",
	}
}

// circleScope is the Scope of a change to Circle in csTree.
func circleScope() Scope {
	return Scope{Projects: []Project{{
		Path:  "Lib.Tests/Lib.Tests.csproj",
		Terms: []string{"FullyQualifiedName~Lib.Tests.CircleTests"},
	}}}
}

// isMarkdown is the inert predicate of the tests: a Markdown file is inert.
func isMarkdown(rel string) bool {
	return strings.HasSuffix(rel, ".md")
}
