// changedtests_test.go covers ChangedTests: which head test methods count as added or changed against base.

package dotnetimpact

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestChangedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const unchanged = "namespace N; class T { [Test] void Same() { Run(); } }"
	tests := []struct {
		name string
		base map[string]string
		head map[string]string
		want []ChangedTest
	}{
		{
			name: "NewTestMethod_Returned",
			base: map[string]string{"T.cs": "namespace N; class T { }"},
			head: map[string]string{"T.cs": "namespace N; class T { [Test] void Added() { } }"},
			want: []ChangedTest{{File: "T.cs", Class: "N.T", Method: "Added"}},
		},
		{
			name: "NewFile_Returned",
			head: map[string]string{"T.cs": "namespace N; class T { [Test] void Added() { } }"},
			want: []ChangedTest{{File: "T.cs", Class: "N.T", Method: "Added"}},
		},
		{
			name: "ChangedBody_Returned",
			base: map[string]string{"T.cs": "namespace N; class T { [Test] void M() { A(); } [Test] void Same() { } }"},
			head: map[string]string{"T.cs": "namespace N; class T { [Test] void M() { B(); } [Test] void Same() { } }"},
			want: []ChangedTest{{File: "T.cs", Class: "N.T", Method: "M"}},
		},
		{
			name: "ChangedStringLiteral_Returned",
			base: map[string]string{"T.cs": "namespace N; class T { [Test] void M() { Check(\"a\"); } }"},
			head: map[string]string{"T.cs": "namespace N; class T { [Test] void M() { Check(\"b\"); } }"},
			want: []ChangedTest{{File: "T.cs", Class: "N.T", Method: "M"}},
		},
		{
			name: "UnchangedTest_NotReturned",
			base: map[string]string{"T.cs": unchanged},
			head: map[string]string{"T.cs": unchanged},
		},
		{
			name: "MovedUnchangedToAnotherFile_NotReturned",
			base: map[string]string{"Old.cs": unchanged},
			head: map[string]string{"New.cs": unchanged},
		},
		{
			name: "TestOnlyInBase_NotReturned",
			base: map[string]string{"T.cs": unchanged},
			head: map[string]string{"T.cs": "namespace N; class T { }"},
		},
		{
			name: "Overload_ComparedWithEverySameNamedBaseMethod",
			base: map[string]string{"T.cs": "class T { [Test] void M() { } }"},
			head: map[string]string{"T.cs": "class T { [Test] void M() { } [Test] void M(int x) { } }"},
			want: []ChangedTest{{File: "T.cs", Class: "T", Method: "M"}},
		},
		{
			name: "SameTextInAnotherClass_Returned",
			base: map[string]string{"A.cs": "class A { [Test] void M() { } }"},
			head: map[string]string{"B.cs": "class B { [Test] void M() { } }"},
			want: []ChangedTest{{File: "B.cs", Class: "B", Method: "M"}},
		},
		{
			name: "AbstractClass_ReturnedWithAbstract",
			head: map[string]string{"B.cs": "namespace N; abstract class Base { [Test] void M() { } }"},
			want: []ChangedTest{{File: "B.cs", Class: "N.Base", Method: "M", Abstract: true}},
		},
		{
			name: "NestedClass_JoinedWithPlus",
			head: map[string]string{"T.cs": "namespace N; class Outer { class Inner { [Test] void M() { } } }"},
			want: []ChangedTest{{File: "T.cs", Class: "N.Outer+Inner", Method: "M"}},
		},
		{
			name: "FileScopedDottedNamespace",
			head: map[string]string{"T.cs": "namespace A.B;\nclass T { [Fact] void M() { } }"},
			want: []ChangedTest{{File: "T.cs", Class: "A.B.T", Method: "M"}},
		},
		{
			name: "NonClassKind_Skipped",
			head: map[string]string{"T.cs": "struct S { [Test] void M() { } } interface I { [Test] void N(); }"},
		},
		{
			name: "SortedByFileThenSourceOrder",
			head: map[string]string{
				"b/T.cs": "class T { [Test] void Second() { } [Test] void First() { } }",
				"a/T.cs": "class U { [Test] void Only() { } }",
			},
			want: []ChangedTest{
				{File: "a/T.cs", Class: "U", Method: "Only"},
				{File: "b/T.cs", Class: "T", Method: "Second"},
				{File: "b/T.cs", Class: "T", Method: "First"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, ChangedTests(toBytes(tt.base), toBytes(tt.head))); diff != "" {
				t.Errorf("ChangedTests() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestChangedTest_FilterTermAndLabel(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	test := ChangedTest{File: "T.cs", Class: "N.Outer+Inner", Method: "M"}
	if got, want := test.FilterTerm(), "FullyQualifiedName~N.Outer+Inner.M"; got != want {
		t.Errorf("FilterTerm() = %q; want %q", got, want)
	}
	if got, want := test.Label(), "N.Outer+Inner.M"; got != want {
		t.Errorf("Label() = %q; want %q", got, want)
	}
}

// toBytes converts a map of file texts to the byte form ChangedTests takes.
func toBytes(files map[string]string) map[string][]byte {
	converted := make(map[string][]byte, len(files))
	for rel, text := range files {
		converted[rel] = []byte(text)
	}
	return converted
}
