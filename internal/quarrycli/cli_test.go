// cli_test.go covers quarrycli's cobra seam: Command()'s exact child set.
// Per-verb answer coverage lives in verbs_test.go.

package quarrycli

import (
	"testing"
)

// TestCommand_ExactChildSet asserts Command()'s child set is exactly the four verbs: toc, glyphs,
// resolve, and expand -- no more, no fewer.
func TestCommand_ExactChildSet(t *testing.T) {
	cmd := Command()

	want := map[string]bool{"toc": true, "glyphs": true, "resolve": true, "expand": true}
	got := map[string]bool{}
	for _, child := range cmd.Commands() {
		got[child.Name()] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("Command() is missing child %q", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("Command() has unexpected child %q", name)
		}
	}
}
