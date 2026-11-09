package testcover_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestScanTestFuncs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const tiered = "func TestTiered(t *testing.T) {\n\ttiergate.Require(t, 1)\n}"
	const untiered = "func TestUntiered(t *testing.T) {}"
	tests := []struct {
		name    string
		files   map[string]string
		want    []testcover.TestFunc
		wantErr string
	}{
		{
			name: "TestFunctionsOfTestFiles",
			files: map[string]string{
				"a_test.go": "package p\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
					tiered + "\n\nfunc TestMain(m *testing.M) {}\n\nfunc Testlower(t *testing.T) {}\n\nfunc helper() {}\n",
				"b_test.go": "package p\n\nimport \"testing\"\n\ntype suite struct{}\n\nfunc (suite) TestMethod(t *testing.T) {}\n\n" + untiered + "\n",
				"c.go":      "package p\n\nfunc TestInAFileThatIsNoTestFile() {}\n",
			},
			want: []testcover.TestFunc{
				{Name: "TestTiered", Tier: 1, Tiered: true, Hash: testcover.ContentHash([]byte(tiered))},
				{Name: "TestUntiered", Hash: testcover.ContentHash([]byte(untiered))},
			},
		},
		{name: "UnparsableTestFile", files: map[string]string{"x_test.go": "package p\n\nfunc {"}, wantErr: "x_test.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := testcover.ScanTestFuncs(dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ScanTestFuncs() error = %v; want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ScanTestFuncs() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
