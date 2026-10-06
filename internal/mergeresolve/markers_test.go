package mergeresolve

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanUnresolved covers the conflict-marker scan: each of the three marker prefixes at the start of a line is reported unresolved; a marker string mid-line passes, since the match is line-anchored rather than a substring match; a path whose file no longer exists is resolved by deletion; and a read error that is not a not-exist error (a directory read as a file) is a genuine failure, distinct from deletion.
func TestScanUnresolved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// content is written to f.txt, unless asDirectory or absent is set.
		content     string
		asDirectory bool
		absent      bool
		want        []string
		wantErr     bool
	}{
		{name: "ours marker", content: "<<<<<<< HEAD\nmine\n", want: []string{"f.txt"}},
		{name: "middle marker", content: "=======\n", want: []string{"f.txt"}},
		{name: "theirs marker", content: ">>>>>>> feature\ntheirs\n", want: []string{"f.txt"}},
		{name: "mid-line marker passes", content: "note: this line mentions <<<<<<< HEAD only in passing\nordinary content\n"},
		{name: "deleted file is resolved", absent: true},
		{name: "read error is a genuine failure", asDirectory: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			switch {
			case tt.asDirectory:
				if err := os.Mkdir(filepath.Join(dir, "f.txt"), 0o755); err != nil {
					t.Fatalf("Mkdir: %v", err)
				}
			case !tt.absent:
				if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			}

			unresolved, err := scanUnresolved(dir, []string{"f.txt"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("scanUnresolved() error = %v; want error = %v", err, tt.wantErr)
			}
			if len(unresolved) != len(tt.want) || (len(tt.want) == 1 && unresolved[0] != tt.want[0]) {
				t.Errorf("scanUnresolved() = %v; want %v", unresolved, tt.want)
			}
		})
	}
}
