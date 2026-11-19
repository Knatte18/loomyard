// changedfuncs_test.go tests the finder of a diff's changed functions over in-memory file sets.

package testreach_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

func TestChangedFuncs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	module := testreach.Module{Dir: "/w/mod", Rel: "mod", Path: "ex.com/m"}
	type want struct {
		pkg, file, fn string
		new           bool
		text          string
	}
	tests := []struct {
		name    string
		base    testreach.FileSet
		head    testreach.FileSet
		want    []want
		wantErr string
	}{
		{
			name: "NewFunctionAndBodyChangeAreFound",
			base: testreach.FileSet{
				"mod/a.go": []byte("package p\n\nfunc Keep() {}\n\nfunc Body() { _ = 1 }\n"),
			},
			head: testreach.FileSet{
				"mod/a.go":     []byte("package p\n\nfunc Keep() {}\n\nfunc Body() { _ = 2 }\n\nfunc Fresh() {}\n"),
				"mod/sub/s.go": []byte("package sub\n\nfunc S() {}\n"),
			},
			want: []want{
				{"ex.com/m", "a.go", "Body", false, "func Body() { _ = 2 }"},
				{"ex.com/m", "a.go", "Fresh", true, "func Fresh() {}"},
				{"ex.com/m/sub", "s.go", "S", true, "func S() {}"},
			},
		},
		{
			name: "MovedLinesWithUnchangedTextAreNotFound",
			base: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc A() {}\n")},
			head: testreach.FileSet{"mod/a.go": []byte("package p\n\nvar x = 1\n\n\nfunc A() {}\n")},
			want: nil,
		},
		{
			name: "FunctionMovedToAnotherFileOfItsPackageIsNotFound",
			base: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc A() {}\n")},
			head: testreach.FileSet{"mod/b.go": []byte("package p\n\nfunc A() {}\n")},
			want: nil,
		},
		{
			name: "DeletedFunctionIsNotFound",
			base: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc A() {}\n\nfunc B() {}\n")},
			head: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc A() {}\n")},
			want: nil,
		},
		{
			name: "TestFileFunctionIsNotFound",
			base: testreach.FileSet{},
			head: testreach.FileSet{"mod/a_test.go": []byte("package p\n\nfunc helper() {}\n")},
			want: nil,
		},
		{
			name: "BuildConstrainedVariantIsFoundInItsOwnFile",
			base: testreach.FileSet{
				"mod/x_linux.go": []byte("package p\n\nfunc Plat() int { return 1 }\n"),
				"mod/x_other.go": []byte("package p\n\nfunc Plat() int { return 2 }\n"),
			},
			head: testreach.FileSet{
				"mod/x_linux.go": []byte("package p\n\nfunc Plat() int { return 10 }\n"),
				"mod/x_other.go": []byte("package p\n\nfunc Plat() int { return 2 }\n"),
			},
			want: []want{{"ex.com/m", "x_linux.go", "Plat", false, "func Plat() int { return 10 }"}},
		},
		{
			name: "SecondInitChangedAloneIsFoundAlone",
			base: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc init() { _ = 1 }\n\nfunc init() { _ = 2 }\n")},
			head: testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc init() { _ = 1 }\n\nfunc init() { _ = 3 }\n")},
			want: []want{{"ex.com/m", "a.go", "init", false, "func init() { _ = 3 }"}},
		},
		{
			name: "MethodKeyIsTypeDotMethod",
			base: testreach.FileSet{},
			head: testreach.FileSet{"mod/a.go": []byte("package p\n\ntype T struct{}\n\nfunc (t *T) M() {}\n")},
			want: []want{{"ex.com/m", "a.go", "T.M", true, "func (t *T) M() {}"}},
		},
		{
			name: "FilesOutsideTheModuleOrUnderSkippedDirectoriesAreNotFound",
			base: testreach.FileSet{},
			head: testreach.FileSet{
				"other/a.go":         []byte("package p\n\nfunc A() {}\n"),
				"mod/testdata/a.go":  []byte("package p\n\nfunc A() {}\n"),
				"mod/.hidden/a.go":   []byte("package p\n\nfunc A() {}\n"),
				"mod/_ignored/a.go":  []byte("package p\n\nfunc A() {}\n"),
				"mod/sub/testdata/a": []byte("not go"),
			},
			want: nil,
		},
		{
			name:    "UnparsableFileIsAnErrorNamingIt",
			base:    testreach.FileSet{},
			head:    testreach.FileSet{"mod/a.go": []byte("package p\n\nfunc (")},
			wantErr: "mod/a.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := testreach.ChangedFuncs(module, tt.base, tt.head)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ChangedFuncs() error = %v; want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChangedFuncs() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ChangedFuncs() = %+v; want %d functions", got, len(tt.want))
			}
			for i, w := range tt.want {
				wantFunc := testreach.ChangedFunc{
					Key:  testcover.FuncKey{Package: w.pkg, File: w.file, Func: w.fn},
					Hash: testcover.ContentHash([]byte(w.text)),
					New:  w.new,
				}
				if got[i] != wantFunc {
					t.Errorf("ChangedFuncs()[%d] = %+v; want %+v", i, got[i], wantFunc)
				}
			}
		})
	}
}
