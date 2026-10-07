// guards_test.go parses fixture test files written under t.TempDir.

package impactset

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeTestFile writes one fixture file into a package directory of root.
func writeTestFile(t *testing.T, root, dir, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(dir))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", full, err)
	}
	if err := os.WriteFile(filepath.Join(full, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func TestFindGuardTests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		file    string
		want    []string
		wantErr string
	}{
		{
			name: "a marked top-level test is found",
			file: "package p\n\n// TestA scans the tree.\n//\n//lyx:guard\nfunc TestA(t *testing.T) {}\n\nfunc TestUnmarked(t *testing.T) {}\n",
			want: []string{"TestA"},
		},
		{
			name: "a marker stacked with a testtiming keep line is found",
			file: "package p\n\n//lyx:guard\n//testtiming:keep pins the scan\nfunc TestA(t *testing.T) {}\n",
			want: []string{"TestA"},
		},
		{
			name: "a guard in an integration file is found",
			file: "//go:build integration\n\npackage p\n\n//lyx:guard\nfunc TestA(t *testing.T) {}\n",
			want: []string{"TestA"},
		},
		{
			name:    "a file that does not parse fails naming the file",
			file:    "package p\n\nfunc TestA(t *testing.T) {\n",
			wantErr: "impactset: parse dir/p/p_test.go",
		},
		{
			name:    "a marker with another comment line between it and the func fails with its line",
			file:    "package p\n\n//lyx:guard\n// a stray line\nfunc TestA(t *testing.T) {}\n",
			wantErr: "p_test.go:3",
		},
		{
			name:    "a marker above a func that is not a test fails with its line",
			file:    "package p\n\n//lyx:guard\nfunc helper() {}\n",
			wantErr: "p_test.go:3",
		},
		{
			name:    "a marker above a method fails with its line",
			file:    "package p\n\n//lyx:guard\nfunc (s S) TestA() {}\n",
			wantErr: "p_test.go:3",
		},
		{
			name:    "a marker floating in a function body fails with its line",
			file:    "package p\n\nfunc TestA(t *testing.T) {\n\t//lyx:guard\n}\n",
			wantErr: "p_test.go:4",
		},
		{
			name:    "a marker in a tmux file fails with its line",
			file:    "//go:build tmux\n\npackage p\n\n//lyx:guard\nfunc TestA(t *testing.T) {}\n",
			wantErr: "p_test.go:5",
		},
		{
			name:    "a marker in an llm file fails with its line",
			file:    "//go:build llm && linux\n\npackage p\n\n//lyx:guard\nfunc TestA(t *testing.T) {}\n",
			wantErr: "p_test.go:5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestFile(t, root, "dir/p", "p_test.go", tt.file)

			got, err := findGuardTests(root, []string{"dir/p"})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("findGuardTests() error = %v; want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("findGuardTests() error = %v; want nil", err)
			}
			if !slices.Equal(got["dir/p"], tt.want) {
				t.Errorf("findGuardTests()[dir/p] = %v; want %v", got["dir/p"], tt.want)
			}
		})
	}
}
