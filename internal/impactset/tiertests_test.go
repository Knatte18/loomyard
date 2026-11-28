// tiertests_test.go covers HoldsTierTest over a package directory written for each case.

package impactset_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHoldsTierTest(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const header = "package p\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n"
	tier1Test := header + "func TestA(t *testing.T) {\n\ttiergate.Require(t, 1)\n}\n"
	tier2Test := header + "func TestB(t *testing.T) {\n\ttiergate.Require(t, 2)\n}\n"

	tests := []struct {
		name string
		// files are the directory's files, keyed by name; nil leaves the directory absent.
		files map[string]string
		tiers []int
		want  bool
	}{
		{name: "a test requiring a listed tier", files: map[string]string{"a_test.go": tier1Test}, tiers: []int{1}, want: true},
		{name: "a test requiring another tier", files: map[string]string{"a_test.go": tier1Test}, tiers: []int{0, 2}},
		{name: "a test requiring no tier counts for every tier", files: map[string]string{"a_test.go": "package p\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n"}, tiers: []int{2}, want: true},
		{name: "a benchmark requiring a listed tier", files: map[string]string{"a_test.go": header + "func BenchmarkA(b *testing.B) {\n\ttiergate.Require(b, 1)\n}\n"}, tiers: []int{1}, want: true},
		{name: "a fuzz target requiring a listed tier", files: map[string]string{"a_test.go": header + "func FuzzA(f *testing.F) {\n\ttiergate.Require(f, 1)\n}\n"}, tiers: []int{1}, want: true},
		{name: "an example alone counts for every tier", files: map[string]string{"a_test.go": "package p\n\nfunc ExampleA() {}\n"}, tiers: []int{2}, want: true},
		{name: "a second file's test of a listed tier is enough", files: map[string]string{"a_test.go": tier1Test, "b_test.go": tier2Test}, tiers: []int{2}, want: true},
		{name: "a TestMain alone holds no test", files: map[string]string{"main_test.go": "package p\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n"}, tiers: []int{0}},
		{name: "a helper whose name only starts with Test holds no test", files: map[string]string{"a_test.go": "package p\n\nfunc Testable() {}\n"}, tiers: []int{0}},
		{name: "a method named like a test holds no test", files: map[string]string{"a_test.go": "package p\n\ntype s struct{}\n\nfunc (s) TestA() {}\n"}, tiers: []int{0}},
		{name: "a file that is no test file is not read", files: map[string]string{"a.go": "package p\n\nfunc TestA() {}\n"}, tiers: []int{0}},
		{name: "a test file that does not parse counts as holding one", files: map[string]string{"a_test.go": "package p\n\nfunc {\n"}, tiers: []int{0}, want: true},
		{name: "a directory that cannot be read counts as holding one", tiers: []int{0}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "p")
			if tt.files != nil {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := impactset.HoldsTierTest(dir, tt.tiers); got != tt.want {
				t.Errorf("HoldsTierTest(%v) = %v; want %v", tt.tiers, got, tt.want)
			}
		})
	}
}
