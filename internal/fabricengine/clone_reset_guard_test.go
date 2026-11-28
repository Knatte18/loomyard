// clone_reset_guard_test.go pins --reset's hub predicate, and the refusal of --discard-work without --reset.
//
// The hub name --reset acts on is derived, never typed:
// in the two-argument form from the warp URL, and in the one-argument form from the weft's recorded warp URL.
// --reset tears down only a directory carrying a structural hub mark:
// a directory merely named `<name>-LYMHUB` is left alone.
//
// These tests drive resetHub directly, or CloneHub only up to a refusal before any git.
// They therefore spawn no git and stay in tier0 per the Test Tier Purity Invariant.

package fabricengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestResetHub_RefusesADirectoryThatIsNotAHub parks user content in a directory named like a hub
// and asserts resetHub refuses it and leaves every byte in place.
//
//testtiming:keep resetHub refusing a directory named like a hub and leaving every byte of user content in place; coverage of its blocks by other tests does not show an assertion of this
func TestResetHub_RefusesADirectoryThatIsNotAHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	parent := t.TempDir()
	hubPath := filepath.Join(parent, "warp"+fabriclayout.HubSuffix)
	precious := filepath.Join(hubPath, "important", "data.txt")
	if err := os.MkdirAll(filepath.Dir(precious), 0o755); err != nil {
		t.Fatalf("seed non-hub directory: %v", err)
	}
	const sentinel = "NOT-A-HUB-USER-DATA"
	if err := os.WriteFile(precious, []byte(sentinel+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", precious, err)
	}

	err := resetHub(NewMutations(""), parent, hubPath, fabriclayout.CloneOptions{})
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

// TestResetHub_RemovesARealHub asserts that a directory carrying either structural hub mark is torn down.
// It keeps --reset an idempotent re-clone rather than disabling it outright.
//
//testtiming:keep resetHub still tearing down a directory carrying either structural hub mark, so --reset stays an idempotent re-clone; coverage of its blocks by other tests does not show an assertion of this
func TestResetHub_RemovesARealHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		child string
	}{
		{name: "BoardEntry", child: fabriclayout.BoardDirName},
		{name: "WeftSibling", child: "warp-weft"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parent := t.TempDir()
			hubPath := filepath.Join(parent, "warp"+fabriclayout.HubSuffix)
			if err := os.MkdirAll(filepath.Join(hubPath, tt.child), 0o755); err != nil {
				t.Fatalf("seed hub: %v", err)
			}

			if err := resetHub(NewMutations(""), parent, hubPath, fabriclayout.CloneOptions{}); err != nil {
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
	tiergate.Require(t, 0)
	t.Parallel()

	parent := t.TempDir()
	hubPath := filepath.Join(parent, "never-created"+fabriclayout.HubSuffix)
	if err := resetHub(NewMutations(""), parent, hubPath, fabriclayout.CloneOptions{}); err != nil {
		t.Fatalf("resetHub(absent) = %v; want nil", err)
	}
}

// TestCloneHub_DiscardWorkWithoutResetIsRefused asserts that --discard-work alone is refused.
// The refusal names --reset as the way forward and comes before anything in the destination is created.
func TestCloneHub_DiscardWorkWithoutResetIsRefused(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	parent := t.TempDir()
	_, err := CloneHub(parent, fabriclayout.CloneOptions{WeftURL: "unread-url", DiscardWork: true})
	const want = "--discard-work only widens --reset; way forward: pass --reset with it, or leave it out"
	if err == nil || err.Error() != want {
		t.Errorf("CloneHub(DiscardWork without Reset) error = %v; want %q", err, want)
	}
	entries, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatalf("read %s: %v", parent, readErr)
	}
	if len(entries) != 0 {
		t.Errorf("%s holds %d entries after the refusal; want none", parent, len(entries))
	}
}
