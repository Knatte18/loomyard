//go:build tier0

// narrow_test.go covers the type-name closure, test class selection, project grouping and filter terms of narrow.

package dotnetimpact

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// baseCorpus is a production library with two types and a test project of three fixtures: one naming Circle, one naming Shape, and one naming neither.
var baseCorpus = map[string]string{
	"Lib/Circle.cs":            "namespace Lib; class Circle { }",
	"Lib/Shape.cs":             "namespace Lib; class Shape { Circle c; }",
	"Lib/Unused.cs":            "namespace Lib; class Unused { }",
	"Lib.Tests/CircleTests.cs": "[TestFixture] class CircleTests { [Test] void M() { new Circle(); } }",
	"Lib.Tests/ShapeTests.cs":  "namespace Lib.Tests; [TestFixture] class ShapeTests { [Test] void M() { new Shape(); } }",
	"Lib.Tests/OtherTests.cs":  "[TestFixture] class OtherTests { [Test] void M() { } }",
}

var baseProjects = []string{"Lib/Lib.csproj", "Lib.Tests/Lib.Tests.csproj"}

func TestNarrow(t *testing.T) {
	t.Parallel()
	circleProject := Scope{Projects: []Project{{
		Path:  "Lib.Tests/Lib.Tests.csproj",
		Terms: []string{"FullyQualifiedName~CircleTests", "FullyQualifiedName~Lib.Tests.ShapeTests"},
	}}}
	tests := []struct {
		name     string
		extra    map[string]string
		projects []string
		files    []string
		types    []string
		want     Scope
	}{
		{
			name:  "ChangedProductionType_SelectsTransitiveNamers",
			files: []string{"Lib/Circle.cs"},
			want:  circleProject,
		},
		{
			name:  "SeededTypeWithoutFile_SelectsSameClasses",
			types: []string{"Circle"},
			want:  circleProject,
		},
		{
			name:  "ChangedTestFile_SelectedWithoutNamingChangedType",
			files: []string{"Lib.Tests/OtherTests.cs"},
			want:  Scope{Projects: []Project{{Path: "Lib.Tests/Lib.Tests.csproj", Terms: []string{"FullyQualifiedName~OtherTests"}}}},
		},
		{
			name: "AbstractBase_SelectsDerivedFixtureWithItsOwnTerm",
			extra: map[string]string{
				"Lib.Tests/BaseTests.cs":    "abstract class BaseTests { [Test] void M() { new Circle(); } }",
				"Lib.Tests/DerivedTests.cs": "namespace Derived; class DerivedTests : Common.BaseTests<int> { }",
			},
			files: []string{"Lib/Circle.cs"},
			want: Scope{Projects: []Project{{
				Path: "Lib.Tests/Lib.Tests.csproj",
				Terms: []string{
					"FullyQualifiedName~CircleTests",
					"FullyQualifiedName~Derived.DerivedTests",
					"FullyQualifiedName~Lib.Tests.ShapeTests",
				},
			}}},
		},
		{
			name: "NestedTestClass_JoinsOuterWithPlus",
			extra: map[string]string{
				"Lib.Tests/NestedTests.cs": "namespace N; class Outer { class Middle { [TestFixture] class Inner { [Test] void M() { new Unused(); } } } }",
			},
			files: []string{"Lib/Unused.cs"},
			want:  Scope{Projects: []Project{{Path: "Lib.Tests/Lib.Tests.csproj", Terms: []string{"FullyQualifiedName~N.Outer+Middle+Inner"}}}},
		},
		{
			name: "TwoTestProjects_EachGetsItsOwn",
			extra: map[string]string{
				"A.Tests/ATests.cs": "[TestFixture] class ATests { [Test] void M() { new Circle(); } }",
				"B.Tests/BTests.cs": "[TestFixture] class BTests { [Test] void M() { new Circle(); } }",
			},
			projects: []string{"B.Tests/B.csproj", "A.Tests/A.csproj", "Lib/Lib.csproj", "Lib.Tests/Lib.Tests.csproj"},
			files:    []string{"Lib/Circle.cs"},
			want: Scope{Projects: []Project{
				{Path: "A.Tests/A.csproj", Terms: []string{"FullyQualifiedName~ATests"}},
				{Path: "B.Tests/B.csproj", Terms: []string{"FullyQualifiedName~BTests"}},
				circleProject.Projects[0],
			}},
		},
		{
			name:  "SeedFileWithoutProject_Whole",
			files: []string{"Loose/Thing.cs"},
			want:  Scope{Whole: true, Fallback: "no .csproj holds Loose/Thing.cs"},
		},
		{
			name: "SelectedClassWithoutProject_Whole",
			extra: map[string]string{
				"Orphan/OrphanTests.cs": "[TestFixture] class OrphanTests { [Test] void M() { new Circle(); } }",
			},
			files: []string{"Lib/Circle.cs"},
			want:  Scope{Whole: true, Fallback: "no .csproj holds Orphan/OrphanTests.cs"},
		},
		{
			name:     "NearestDirectoryWithTwoProjects_Whole",
			projects: []string{"Lib/Lib.csproj", "Lib.Tests/A.csproj", "Lib.Tests/B.csproj"},
			files:    []string{"Lib/Circle.cs"},
			want:     Scope{Whole: true, Fallback: "Lib.Tests holds more than one .csproj"},
		},
		{
			name:  "SeedSelectingNothing_NoTestClass",
			files: []string{"Lib/Unused.cs"},
			want:  Scope{NoTestClass: true},
		},
		{
			name: "EmptySeed_EmptyScope",
			want: Scope{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{}
			for rel, content := range baseCorpus {
				files[rel] = content
			}
			for rel, content := range tt.extra {
				files[rel] = content
			}
			var sources []sourceFile
			for rel, content := range files {
				sources = append(sources, parseSource(rel, []byte(content)))
			}
			sort.Slice(sources, func(i, j int) bool { return sources[i].Rel < sources[j].Rel })
			projects := tt.projects
			if projects == nil {
				projects = baseProjects
			}
			s := seed{Files: map[string]bool{}, Types: map[string]bool{}}
			for _, file := range tt.files {
				s.Files[file] = true
			}
			for _, name := range tt.types {
				s.Types[name] = true
			}
			if diff := cmp.Diff(tt.want, narrow(sources, projects, s)); diff != "" {
				t.Errorf("narrow() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
