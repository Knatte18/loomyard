//go:build tier0

// fixcap_test.go — tier0 unit tests for FixCap.Exceeded.

package landingshed

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestFixCap_Exceeded(t *testing.T) {
	t.Parallel()

	goMatcher := func(p string) bool { return planparser.IsTestFile("go", p) }
	csharpMatcher := func(p string) bool { return planparser.IsTestFile("csharp", p) }
	noneMatcher := func(p string) bool { return planparser.IsTestFile("none", p) }

	tests := []struct {
		name       string
		isTestFile func(string) bool
		change     fabricengine.LineChange
		want       string
	}{
		{"OverTheLineCap", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 41}, "41 lines, over the cap of 40"},
		{"AtTheLineCap", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 40}, ""},
		{"SmallSourceOnlyChange", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 3, AddedLines: []string{"x := 1"}}, ""},
		{"GoTestFile", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x_test.go"}, Lines: 3}, "test file internal/x/x_test.go"},
		{"CSharpTestFile", csharpMatcher, fabricengine.LineChange{Paths: []string{"src/FooTests.cs"}, Lines: 3}, "test file src/FooTests.cs"},
		{"CSharpSourceFileUnderGoMatcher", goMatcher, fabricengine.LineChange{Paths: []string{"src/FooTests.cs"}, Lines: 3}, ""},
		{"AnyFileUnderNoneMatcher", noneMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 1}, "test file internal/x/x.go"},
		{"TestdataFile", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/testdata/in.txt"}, Lines: 1}, "testdata file internal/x/testdata/in.txt"},
		{"GoMod", goMatcher, fabricengine.LineChange{Paths: []string{"go.mod"}, Lines: 1}, "configuration file go.mod"},
		{"GoSum", goMatcher, fabricengine.LineChange{Paths: []string{"go.sum"}, Lines: 1}, "configuration file go.sum"},
		{"GoWork", goMatcher, fabricengine.LineChange{Paths: []string{"go.work"}, Lines: 1}, "configuration file go.work"},
		{"Makefile", goMatcher, fabricengine.LineChange{Paths: []string{"tools/Makefile"}, Lines: 1}, "configuration file tools/Makefile"},
		{"Csproj", goMatcher, fabricengine.LineChange{Paths: []string{"src/App.csproj"}, Lines: 1}, "configuration file src/App.csproj"},
		{"Sln", goMatcher, fabricengine.LineChange{Paths: []string{"App.sln"}, Lines: 1}, "configuration file App.sln"},
		{"Props", goMatcher, fabricengine.LineChange{Paths: []string{"build/Common.props"}, Lines: 1}, "configuration file build/Common.props"},
		{"Targets", goMatcher, fabricengine.LineChange{Paths: []string{"build/Common.targets"}, Lines: 1}, "configuration file build/Common.targets"},
		{"DirectoryBuildProps", goMatcher, fabricengine.LineChange{Paths: []string{"Directory.Build.props"}, Lines: 1}, "configuration file Directory.Build.props"},
		{"GithubPath", goMatcher, fabricengine.LineChange{Paths: []string{".github/workflows/ci.yml"}, Lines: 1}, "configuration file .github/workflows/ci.yml"},
		{"GateYaml", goMatcher, fabricengine.LineChange{Paths: []string{"_lym/config/gate.yaml"}, Lines: 1}, "configuration file _lym/config/gate.yaml"},
		{"AddedTSkip", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 1, AddedLines: []string{"\tt.Skip(\"flaky\")"}}, `skips a test: t.Skip("flaky")`},
		{"AddedTSkipNow", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 1, AddedLines: []string{"\tt.SkipNow()"}}, "skips a test: t.SkipNow()"},
		{"AddedIgnoreAttribute", goMatcher, fabricengine.LineChange{Paths: []string{"src/Foo.cs"}, Lines: 1, AddedLines: []string{"[Ignore]"}}, "skips a test: [Ignore]"},
		{"AddedSkipAssignment", goMatcher, fabricengine.LineChange{Paths: []string{"src/Foo.cs"}, Lines: 1, AddedLines: []string{"[Fact(Skip = \"x\")]"}}, "skips a test: [Fact(Skip = \"x\")]"},
		{"AddedGoBuildConstraint", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 1, AddedLines: []string{"//go:build linux"}}, "build constraint: //go:build linux"},
		{"AddedPlusBuildConstraint", goMatcher, fabricengine.LineChange{Paths: []string{"internal/x/x.go"}, Lines: 1, AddedLines: []string{"// +build linux"}}, "build constraint: // +build linux"},
		{"AddedPreprocessorConstraint", goMatcher, fabricengine.LineChange{Paths: []string{"src/Foo.cs"}, Lines: 1, AddedLines: []string{"#if DEBUG"}}, "build constraint: #if DEBUG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FixCap{MaxLines: 40, IsTestFile: tt.isTestFile}.Exceeded(tt.change)
			if tt.want == "" {
				if got != "" {
					t.Errorf("Exceeded(%s) = %q; want within the caps", tt.name, got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("Exceeded(%s) = %q; want a reason containing %q", tt.name, got, tt.want)
			}
		})
	}
}
