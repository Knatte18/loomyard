// triage.go holds the verify gate's flaky-failure notes: the warning and the friction note that name the identities that failed once and passed on rerun.

package websterengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/friction"
)

// triageWarnings returns the warning naming the flaky identities, or nil when there are none.
func triageWarnings(flaky []string) []string {
	if len(flaky) == 0 {
		return nil
	}
	return []string{"integration verify: flaky failures passed or vanished on rerun: " + strings.Join(flaky, ", ")}
}

// writeTriageFrictionNote records the flaky identities as a friction note,
// since a failure that vanishes on rerun is environment trouble the hub collects from friction notes.
// It is a no-op when frictionDir is empty or flaky is empty.
func writeTriageFrictionNote(frictionDir string, flaky []string) error {
	if frictionDir == "" || len(flaky) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-verify-triage")
	if path == "" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Integration-suite triage: non-regression verify failures\n\n")
	b.WriteString("These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n")
	for _, id := range flaky {
		b.WriteString("- " + id + "\n")
	}
	b.WriteString("\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: triage: write friction note %s: %w", path, err)
	}
	return nil
}

// failureIDs returns the ids of fs in order.
func failureIDs(fs []VerifyFailure) []string {
	ids := make([]string, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.ID)
	}
	return ids
}
