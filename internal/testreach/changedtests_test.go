// changedtests_test.go tests the finder of a diff's changed test functions over in-memory file sets.

package testreach_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestChangedTests(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	module := testreach.Module{Dir: "/w/mod", Rel: "mod", Path: "ex.com/m"}
	tests := []struct {
		name    string
		base    testreach.FileSet
		head    testreach.FileSet
		want    []testreach.ChangedTest
		wantErr string
	}{
		{
			name: "NewTestAndChangedTestAreFoundSortedByPackageAndName",
			base: testreach.FileSet{
				"mod/sub/s_test.go": []byte("package sub\n\nfunc TestKeep(t *testing.T) {}\n\nfunc TestBody(t *testing.T) { _ = 1 }\n"),
			},
			head: testreach.FileSet{
				"mod/sub/s_test.go": []byte("package sub\n\nfunc TestKeep(t *testing.T) {}\n\nfunc TestBody(t *testing.T) { _ = 2 }\n"),
				"mod/a_test.go":     []byte("package p_test\n\nfunc TestFresh(t *testing.T) {}\n"),
			},
			want: []testreach.ChangedTest{
				{Package: "ex.com/m", File: "a_test.go", Name: "TestFresh", New: true},
				{Package: "ex.com/m/sub", File: "s_test.go", Name: "TestBody"},
			},
		},
		{
			name: "HelpersMethodsTestMainAndLowerCaseSuffixesAreNoTests",
			base: testreach.FileSet{},
			head: testreach.FileSet{"mod/a_test.go": []byte("package p\n\nfunc helper() {}\n\nfunc TestMain(m *testing.M) {}\n\n" +
				"func Testify(t *testing.T) {}\n\nfunc (s suite) TestOnSuite(t *testing.T) {}\n\nfunc Test(t *testing.T) {}\n\nfunc Test_x(t *testing.T) {}\n")},
			want: []testreach.ChangedTest{
				{Package: "ex.com/m", File: "a_test.go", Name: "Test", New: true},
				{Package: "ex.com/m", File: "a_test.go", Name: "Test_x", New: true},
			},
		},
		{
			name: "TestMovedVerbatimToAnotherFileIsNotFound",
			base: testreach.FileSet{"mod/a_test.go": []byte("package p\n\nfunc TestA(t *testing.T) {}\n")},
			head: testreach.FileSet{"mod/b_test.go": []byte("package p\n\nfunc TestA(t *testing.T) {}\n")},
		},
		{
			name: "DeletedTestIsNotFound",
			base: testreach.FileSet{"mod/a_test.go": []byte("package p\n\nfunc TestA(t *testing.T) {}\n")},
			head: testreach.FileSet{"mod/a_test.go": []byte("package p\n")},
		},
		{
			name: "ProductionFilesAndFilesOutsideTheModuleArePassedOver",
			base: testreach.FileSet{},
			head: testreach.FileSet{
				"mod/a.go":                []byte("package p\n\nfunc TestLike(t *testing.T) {}\n"),
				"other/o_test.go":         []byte("package o\n\nfunc TestO(t *testing.T) {}\n"),
				"mod/testdata/d_test.go":  []byte("package d\n\nfunc TestD(t *testing.T) {}\n"),
				"mod/_hidden/h_test.go":   []byte("package h\n\nfunc TestH(t *testing.T) {}\n"),
				"mod/.dotted/dot_test.go": []byte("package dot\n\nfunc TestDot(t *testing.T) {}\n"),
			},
		},
		{
			name:    "UnparsableTestFileIsAnErrorNamingIt",
			base:    testreach.FileSet{},
			head:    testreach.FileSet{"mod/bad_test.go": []byte("package p\n\nfunc TestA( {\n")},
			wantErr: "mod/bad_test.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := testreach.ChangedTests(module, tt.base, tt.head)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ChangedTests() error = %v; want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChangedTests() error = %v; want nil", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ChangedTests() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
