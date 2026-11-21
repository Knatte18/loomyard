// conflict_test.go covers the never-tracked Webster halt reason file in the conflict directory.
// Tier 0: a temp directory.

package runplan

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHaltReason_RoundTrip(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "conflict")
	if got, err := ReadHaltReason(dir); err != nil || got != "" {
		t.Fatalf("ReadHaltReason(absent) = %q, %v; want \"\", nil", got, err)
	}
	if err := WriteHaltReason(dir, "stuck on card 3"); err != nil {
		t.Fatalf("WriteHaltReason: %v", err)
	}
	if got, err := ReadHaltReason(dir); err != nil || got != "stuck on card 3" {
		t.Errorf("ReadHaltReason() = %q, %v; want %q, nil", got, err, "stuck on card 3")
	}
	if err := RemoveHaltReason(dir); err != nil {
		t.Fatalf("RemoveHaltReason: %v", err)
	}
	if got, err := ReadHaltReason(dir); err != nil || got != "" {
		t.Errorf("ReadHaltReason(removed) = %q, %v; want \"\", nil", got, err)
	}
	if err := RemoveHaltReason(dir); err != nil {
		t.Errorf("RemoveHaltReason(absent) = %v; want nil", err)
	}
}
