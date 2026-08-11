// clone_reset_guard_test.go pins --reset's hub predicate.
//
// The hub name --reset acts on is DERIVED, never typed: from the warp URL in the two-argument form,
// and from the warp URL recorded on the weft's own binding in the one-argument form, where the
// operator never sees the name of the directory being deleted.
// Before the guard, --reset ran an unconditional RemoveAll on that derived path, so any directory
// that merely happened to be called `<name>-HUB` was destroyed, user content and all.
//
// These tests drive resetHub directly rather than CloneHub, so they spawn no git and stay in the
// untagged Tier-1 tier per the Test Tier Purity Invariant.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResetHub_RefusesADirectoryThatIsNotAHub parks user content in a directory named like a hub
// and asserts resetHub refuses it and leaves every byte in place.
func TestResetHub_RefusesADirectoryThatIsNotAHub(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	hubPath := filepath.Join(parent, "warp"+HubSuffix)
	precious := filepath.Join(hubPath, "important", "data.txt")
	if err := os.MkdirAll(filepath.Dir(precious), 0o755); err != nil {
		t.Fatalf("seed non-hub directory: %v", err)
	}
	const sentinel = "NOT-A-HUB-USER-DATA"
	if err := os.WriteFile(precious, []byte(sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", precious, err)
	}

	err := resetHub(parent, hubPath)
	if err == nil {
		t.Fatalf("resetHub(%s) = nil; want a refusal for a directory that is not a fabric hub", hubPath)
	}
	if !strings.Contains(err.Error(), "not a fabric hub") {
		t.Errorf("resetHub error = %q; want it to say the target is not a fabric hub", err)
	}

	content, readErr := os.ReadFile(precious)
	if readErr != nil {
		t.Fatalf("--reset destroyed user content in a directory that is not a hub: %v", readErr)
	}
	if !strings.Contains(string(content), sentinel) {
		t.Fatalf("user content lost: %q no longer contains %q", content, sentinel)
	}
}

// TestResetHub_RemovesARealHub is the guard's counter-test: a directory carrying either structural
// mark of a hub must still be torn down, so --reset stays an idempotent re-clone rather than being
// disabled outright.
func TestResetHub_RemovesARealHub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		child string
	}{
		{name: "BoardEntry", child: BoardDirName},
		{name: "WeftSibling", child: "warp-weft"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parent := t.TempDir()
			hubPath := filepath.Join(parent, "warp"+HubSuffix)
			if err := os.MkdirAll(filepath.Join(hubPath, tt.child), 0o755); err != nil {
				t.Fatalf("seed hub: %v", err)
			}

			if err := resetHub(parent, hubPath); err != nil {
				t.Fatalf("resetHub(%s) = %v; want nil for a directory carrying %s", hubPath, err, tt.child)
			}
			if _, statErr := os.Stat(hubPath); !os.IsNotExist(statErr) {
				t.Errorf("hub still present at %s after resetHub", hubPath)
			}
		})
	}
}

// TestResetHub_AbsentPathIsANoop keeps --reset idempotent: resetting a hub that was never created
// is success, not an error.
func TestResetHub_AbsentPathIsANoop(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	hubPath := filepath.Join(parent, "never-created"+HubSuffix)
	if err := resetHub(parent, hubPath); err != nil {
		t.Fatalf("resetHub(absent) = %v; want nil", err)
	}
}
