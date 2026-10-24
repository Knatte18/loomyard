//go:build tier0

// drop_test.go pins DropKeys over the document shapes a retired key can sit in.

package yamlengine

import "testing"

// TestDropKeys pins which keys are removed with their comments, the reported lines, that a document without the keys comes back byte-identical, and the refusal of a document with no mapping root.
func TestDropKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existing      string
		wantOut       string
		wantLines     []string
		wantErr       bool
		wantUnchanged bool
	}{
		{
			name:      "two keys are removed with their comments",
			existing:  "keep: 1 # stays\n# head of old\nold_a: 2 # goes\nold_b: [x, y]\nlast: 3\n",
			wantOut:   "keep: 1 # stays\nlast: 3\n",
			wantLines: []string{"old_a: retired, removed", "old_b: retired, removed"},
		},
		{name: "a document holding none is unchanged", existing: "keep: 1\n# note\nlast: 3\n", wantUnchanged: true},
		{name: "an empty document is unchanged", existing: "", wantUnchanged: true},
		{name: "a non-mapping root errors", existing: "- old_a\n- old_b\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, lines, err := DropKeys([]byte(tt.existing), []string{"old_a", "old_b"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("DropKeys() error = %v; want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			wantOut := tt.wantOut
			if tt.wantUnchanged {
				wantOut = tt.existing
			}
			if string(out) != wantOut {
				t.Errorf("DropKeys() out = %q; want %q", out, wantOut)
			}
			if len(lines) != len(tt.wantLines) {
				t.Fatalf("DropKeys() lines = %q; want %q", lines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if lines[i] != want {
					t.Errorf("DropKeys() lines[%d] = %q; want %q", i, lines[i], want)
				}
			}

			again, againLines, err := DropKeys(out, []string{"old_a", "old_b"})
			if err != nil || string(again) != string(out) || len(againLines) != 0 {
				t.Errorf("DropKeys() on its own output = (%q, %q, %v); want it unchanged with no lines", again, againLines, err)
			}
		})
	}
}
