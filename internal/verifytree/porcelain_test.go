// porcelain_test.go covers parsePorcelainZ over literal `git status --porcelain -z` output, without spawning git.

package verifytree

import (
	"slices"
	"testing"
)

func TestParsePorcelainZ(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{name: "empty", out: "", want: nil},
		{name: "modified and untracked", out: " M a.go\x00?? b.txt\x00", want: []string{"a.go", "b.txt"}},
		{name: "unescaped special characters", out: "?? café \"x\".txt\x00", want: []string{"café \"x\".txt"}},
		{name: "arrow inside a name", out: "?? a -> b.txt\x00", want: []string{"a -> b.txt"}},
		{name: "rename skips its source record", out: "R  new.go\x00old.go\x00 M c.go\x00", want: []string{"new.go", "c.go"}},
		{name: "copy skips its source record", out: "C  copy.go\x00orig.go\x00", want: []string{"copy.go"}},
		{name: "worktree rename skips its source record", out: " R new.go\x00old.go\x00", want: []string{"new.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePorcelainZ(tt.out); !slices.Equal(got, tt.want) {
				t.Errorf("parsePorcelainZ(%q) = %q; want %q", tt.out, got, tt.want)
			}
		})
	}
}
