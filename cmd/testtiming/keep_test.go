package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanKeeps(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name    string
		content string
		want    map[string]string
		wantErr string
	}{
		{
			name:    "directive directly above a test",
			content: "package p\n\n//testtiming:keep pins the error text\nfunc TestA(t *testing.T) {}\n",
			want:    map[string]string{"TestA": "pins the error text"},
		},
		{
			name:    "directive below a doc comment",
			content: "package p\n\n// TestB checks b.\n//testtiming:keep pins the ordering\nfunc TestB(t *testing.T) {}\n",
			want:    map[string]string{"TestB": "pins the ordering"},
		},
		{
			name:    "no directive",
			content: "package p\n\nfunc TestC(t *testing.T) {}\n",
			want:    map[string]string{},
		},
		{
			name:    "empty reason",
			content: "package p\n\n//testtiming:keep\nfunc TestD(t *testing.T) {}\n",
			wantErr: "x_test.go:3: //testtiming:keep on TestD has no reason",
		},
		{
			name:    "whitespace-only reason",
			content: "package p\n\n//testtiming:keep   \t\nfunc TestE(t *testing.T) {}\n",
			wantErr: "x_test.go:3: //testtiming:keep on TestE has no reason",
		},
		{
			name:    "directive separated from its test",
			content: "package p\n\n//testtiming:keep reason\n\nfunc TestF(t *testing.T) {}\n",
			wantErr: "x_test.go:3: //testtiming:keep is not directly above a top-level test function",
		},
		{
			name:    "doubled directive",
			content: "package p\n\n//testtiming:keep first\n//testtiming:keep second\nfunc TestG(t *testing.T) {}\n",
			wantErr: "x_test.go:3: //testtiming:keep is not directly above a top-level test function",
		},
		{
			name:    "directive above a non-test function",
			content: "package p\n\n//testtiming:keep reason\nfunc helper() {}\n",
			wantErr: "x_test.go:3: //testtiming:keep is not directly above a top-level test function",
		},
		{
			name:    "directive at end of file",
			content: "package p\n\nfunc TestH(t *testing.T) {}\n//testtiming:keep reason",
			wantErr: "x_test.go:4: //testtiming:keep is not directly above a top-level test function",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte(row.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := scanKeeps(dir)
			if row.wantErr != "" {
				if err == nil || !strings.HasSuffix(strings.ReplaceAll(err.Error(), dir+string(filepath.Separator), ""), row.wantErr) {
					t.Fatalf("scanKeeps error = %v, want one ending %q", err, row.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, row.want) {
				t.Fatalf("scanKeeps = %v, want %v", got, row.want)
			}
		})
	}
}
