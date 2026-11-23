// source_test.go covers the lexer and the per-file source model through parseSource.

package dotnetimpact

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestParseSource_TestMethods(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		src  string
		// want maps a type's name to the test methods it holds; a type with none is absent.
		want map[string][]testMethod
	}{
		{
			name: "BlockBody",
			src:  "class A { [Test] public void M() { Run(); } void Other() { } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] public void M() { Run(); }"}}},
		},
		{
			name: "StackedAttributeLists_StartAtFirstList",
			src:  "class A { [TestCase(1)]\n  [TestCase(2)]\n  void M(int x) { } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[TestCase(1)]\n  [TestCase(2)]\n  void M(int x) { }"}}},
		},
		{
			name: "ExpressionBody_EndsAtSemicolon",
			src:  "class A { [Test] int M() => Make(new[] { 1, 2 }.Length); int N() { return 1; } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] int M() => Make(new[] { 1, 2 }.Length);"}}},
		},
		{
			name: "GenericMethod_NameWithoutTypeParameters",
			src:  "class A { [Test] void Check<T>() where T : new() { } }",
			want: map[string][]testMethod{"A": {{Name: "Check", Text: "[Test] void Check<T>() where T : new() { }"}}},
		},
		{
			name: "BodilessMethod_EndsAtSemicolon",
			src:  "abstract class A { [Test] public abstract void M(); }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] public abstract void M();"}}},
		},
		{
			name: "PropertyAndField_NotRecorded",
			src:  "class A { [Test] int P { get; } [Test] int f; [Test] void M() { } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] void M() { }"}}},
		},
		{
			name: "NoTestAttribute_NotRecorded",
			src:  "class A { [Obsolete] void M() { } void N() { } }",
			want: map[string][]testMethod{},
		},
		{
			name: "NestedClass_MethodsLandOnTheirOwnType",
			src:  "class Outer { [Test] void A() { } class Inner { [Test] void B() { } } [Test] void C() { } }",
			want: map[string][]testMethod{
				"Outer": {{Name: "A", Text: "[Test] void A() { }"}, {Name: "C", Text: "[Test] void C() { }"}},
				"Inner": {{Name: "B", Text: "[Test] void B() { }"}},
			},
		},
		{
			name: "StringLiteralChange_ChangesText",
			src:  "class A { [Test] void M() { Assert.Equal(\"changed\", Run()); } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] void M() { Assert.Equal(\"changed\", Run()); }"}}},
		},
		{
			name: "ExpressionBody_OpeningWithABracket_EndsAtSemicolon",
			src:  "class A { [Test] void M() => (Run()).Check(); void N() { } }",
			want: map[string][]testMethod{"A": {{Name: "M", Text: "[Test] void M() => (Run()).Check();"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := map[string][]testMethod{}
			for _, decl := range parseSource("a/File.cs", []byte(tt.src)).Types {
				if len(decl.TestMethods) > 0 {
					got[decl.Name] = decl.TestMethods
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("parseSource TestMethods mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseSource(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		src  string
		// present lists identifiers that must be in the set, and absent lists those that must not be.
		present, absent []string
		want            []typeDecl
	}{
		{
			name:    "Comments_DropNames",
			src:     "// Hidden1\n/* Hidden2\n class Hidden3 */\nclass Real { }",
			present: []string{"Real"},
			absent:  []string{"Hidden1", "Hidden2", "Hidden3"},
			want:    []typeDecl{{Name: "Real", Kind: "class"}},
		},
		{
			name:    "Strings_DropLiteralText",
			src:     "class C { string a = \"Hidden1\"; string b = @\"Hidden2 \"\"Hidden3\"\" \"; string c = \"\"\"Hidden4 \"\" Hidden5\"\"\"; string d = \"\\\"Hidden6\"; char e = 'x'; char f = '\\''; }",
			present: []string{"C", "a", "b", "c", "d"},
			absent:  []string{"Hidden1", "Hidden2", "Hidden3", "Hidden4", "Hidden5", "Hidden6"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name:    "VerbatimOpeningWithEscapedQuote_DoesNotSwallowCode",
			src:     "class C { string a = @\"\"\"\"; string b = @\"\"\"Hidden1 \"\"x\"\" \"; Visible1 c; }",
			present: []string{"C", "a", "b", "Visible1", "c"},
			absent:  []string{"Hidden1", "x"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name:    "RecordAsIdentifier_DeclaresNoType",
			src:     "class C { void M() { foreach (var record in rs) { } if (record is Foo) { } if (record is { X: 1 }) { } } }",
			present: []string{"record", "rs", "Foo"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name:    "Interpolated_KeepHoles",
			src:     "class C { string a = $\"Hidden1 {Formatter.Format(v)} {{Hidden2}}\"; string b = $@\"Hidden3 {Verbatim.Call(\"Hidden4\")}\"; string c = @$\"{Swapped}\"; string d = $\"{(flag ? \"Hidden5\" : Fallback.Text)}\"; }",
			present: []string{"Formatter", "Format", "v", "Verbatim", "Call", "Swapped", "Fallback", "Text"},
			absent:  []string{"Hidden1", "Hidden2", "Hidden3", "Hidden4", "Hidden5"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name:    "InterpolatedRaw_KeepHoles",
			src:     "class C { string a = $\"\"\"Hidden1 {Hidden2} {RawOne.Go}\"\"\"; string b = $$\"\"\"Hidden3 {Hidden4} {{RawTwo.Go}} {{{RawThree}}}\"\"\"; }",
			present: []string{"RawOne", "RawTwo", "RawThree", "Go"},
			absent:  []string{"Hidden1", "Hidden3", "Hidden4"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name:    "Preprocessor_DropsLine",
			src:     "#if HiddenSymbol\nclass C { }\n#endif",
			present: []string{"C"},
			absent:  []string{"HiddenSymbol", "endif"},
			want:    []typeDecl{{Name: "C", Kind: "class"}},
		},
		{
			name: "Namespaces_BlockAndNested",
			src:  "namespace A.B { namespace C { class X { } } class Y { } }",
			want: []typeDecl{
				{Name: "X", Namespace: "A.B.C", Kind: "class"},
				{Name: "Y", Namespace: "A.B", Kind: "class"},
			},
		},
		{
			name: "Namespaces_FileScoped",
			src:  "namespace Ns.Inner;\nclass X { }\nstruct Y { }",
			want: []typeDecl{
				{Name: "X", Namespace: "Ns.Inner", Kind: "class"},
				{Name: "Y", Namespace: "Ns.Inner", Kind: "struct"},
			},
		},
		{
			name: "Nested_OuterChain",
			src:  "namespace N { class Outer { class Middle { class Inner { } } class Sibling { } } }",
			want: []typeDecl{
				{Name: "Outer", Namespace: "N", Kind: "class"},
				{Name: "Middle", Namespace: "N", Outer: []string{"Outer"}, Kind: "class"},
				{Name: "Inner", Namespace: "N", Outer: []string{"Outer", "Middle"}, Kind: "class"},
				{Name: "Sibling", Namespace: "N", Outer: []string{"Outer"}, Kind: "class"},
			},
		},
		{
			name: "Generic_NameWithoutParameters",
			src:  "class Box<T> where T : class, new() { }",
			want: []typeDecl{{Name: "Box", Kind: "class"}},
		},
		{
			name: "Abstract_GenericQualifiedBase",
			src:  "public abstract partial class Shape : Geometry.Core.Base<int>, IFoo { }",
			want: []typeDecl{{Name: "Shape", Kind: "class", Abstract: true, Base: "Base"}},
		},
		{
			name: "Kinds_RecordStructInterfaceEnum",
			src:  "record R(int X) : Parent(X); record struct S(int Y); record class C { } interface I : IBase { } enum E : byte { A, B }",
			want: []typeDecl{
				{Name: "R", Kind: "record", Base: "Parent"},
				{Name: "S", Kind: "record struct"},
				{Name: "C", Kind: "record"},
				{Name: "I", Kind: "interface", Base: "IBase"},
				{Name: "E", Kind: "enum"},
			},
		},
		{
			name: "Attributes_ClassSpellings",
			src: "[TestFixture] class A { }\n" +
				"[TestClassAttribute] class B { }\n" +
				"[NUnit.Framework.TestFixtureAttribute] class C { }\n" +
				"[TestFixture(typeof(int), Category = \"x\")] class D { }\n" +
				"[Category(\"x\"), TestClass] class E { }\n" +
				"[Category(\"x\")]\n[TestFixture]\nclass F { }\n" +
				"[Category(\"x\")] class G { }",
			want: []typeDecl{
				{Name: "A", Kind: "class", TestAttributed: true},
				{Name: "B", Kind: "class", TestAttributed: true},
				{Name: "C", Kind: "class", TestAttributed: true},
				{Name: "D", Kind: "class", TestAttributed: true},
				{Name: "E", Kind: "class", TestAttributed: true},
				{Name: "F", Kind: "class", TestAttributed: true},
				{Name: "G", Kind: "class"},
			},
		},
		{
			name: "Attributes_MethodSpellings",
			src: "class A { [Test] void M() { } }\n" +
				"class B { [TestMethod] public void M() { } }\n" +
				"class C { [NUnit.Framework.TestAttribute] void M() { } }\n" +
				"class D { [Category(\"x\"), Theory] void M() { } }\n" +
				"class E { [TestCase(1, 2)] void M() { } }\n" +
				"class F { [Fact(Skip = \"x\")] void M() { } }\n" +
				"class G { [Obsolete] void M() { } }",
			want: []typeDecl{
				{Name: "A", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[Test] void M() { }"}}},
				{Name: "B", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[TestMethod] public void M() { }"}}},
				{Name: "C", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[NUnit.Framework.TestAttribute] void M() { }"}}},
				{Name: "D", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[Category(\"x\"), Theory] void M() { }"}}},
				{Name: "E", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[TestCase(1, 2)] void M() { }"}}},
				{Name: "F", Kind: "class", HasTestMethod: true, TestMethods: []testMethod{{Name: "M", Text: "[Fact(Skip = \"x\")] void M() { }"}}},
				{Name: "G", Kind: "class"},
			},
		},
		{
			name: "Attributes_AssemblyTargetSetsNothing",
			src:  "[assembly: TestFixture]\n[assembly: Test, Parallelizable]\nclass A { }",
			want: []typeDecl{{Name: "A", Kind: "class"}},
		},
		{
			name: "Attributes_IndexerIsNotAList",
			src:  "class A { void M() { var x = grid[0][Test]; } }",
			want: []typeDecl{{Name: "A", Kind: "class"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSource("a/File.cs", []byte(tt.src))
			if got.Rel != "a/File.cs" {
				t.Errorf("parseSource Rel = %q; want %q", got.Rel, "a/File.cs")
			}
			for _, name := range tt.present {
				if !got.Identifiers[name] {
					t.Errorf("parseSource Identifiers lacks %q", name)
				}
			}
			for _, name := range tt.absent {
				if got.Identifiers[name] {
					t.Errorf("parseSource Identifiers holds %q", name)
				}
			}
			if diff := cmp.Diff(tt.want, got.Types); diff != "" {
				t.Errorf("parseSource Types mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
