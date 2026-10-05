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

// expiredShellWarning returns the warning for one background shell the Master wait counted a turn end past.
func expiredShellWarning(label string) string {
	return fmt.Sprintf("turn end counted after background shell `%s` ran past `background_shell_wait_min`; the shell may still be running in the session", label)
}

// writeBackgroundShellFrictionNote records the shells Master's wait counted a turn end past,
// so reflection has the evidence a hang otherwise leaves only in events.jsonl.
// waitMin is the bound in minutes; a non-positive value names the key alone.
// It is a no-op when frictionDir is empty or labels is empty.
func writeBackgroundShellFrictionNote(frictionDir string, labels []string, waitMin int) error {
	if frictionDir == "" || len(labels) == 0 {
		return nil
	}
	friction.EnsureDir(frictionDir)
	path := friction.NotePath(frictionDir, "webster-background-shell")
	if path == "" {
		return nil
	}

	bound := "`background_shell_wait_min`"
	if waitMin > 0 {
		bound = fmt.Sprintf("`background_shell_wait_min` (%d minutes)", waitMin)
	}
	var b strings.Builder
	b.WriteString("Master's wait counted a turn end past a background shell\n\n")
	fmt.Fprintf(&b, "These background shells ran past %s while Master's turn end was being judged, and may still be running in the session:\n\n", bound)
	for _, l := range labels {
		b.WriteString("- " + l + "\n")
	}
	b.WriteString("\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("webster: background shells: write friction note %s: %w", path, err)
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
