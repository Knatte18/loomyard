// acceptance.go reads and writes the round's acceptance record, round-<n>-acceptance.md, in a segment's run directory.
// The record is YAML frontmatter with the round and the hex SHA-256 digest of the accepted review.
// Only the review row writes it, after the review passes the strict parse.
// The review row and the fix row both read it.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// roundAcceptancePath returns the path of round n's acceptance record under runDir.
// The file is named round-<n>-acceptance.md, with n in decimal.
func roundAcceptancePath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-acceptance.md", n))
}

// acceptanceHeader mirrors an acceptance record's YAML frontmatter shape.
type acceptanceHeader struct {
	Round  int    `yaml:"round"`
	Digest string `yaml:"digest"`
}

// writeAcceptance records digest as the accepted review of round n under runDir.
func writeAcceptance(runDir string, n int, digest string) error {
	header, err := yaml.Marshal(acceptanceHeader{Round: n, Digest: digest})
	if err != nil {
		return fmt.Errorf("shedadapters: render round %d acceptance record: %w", n, err)
	}
	content := "---\n" + string(header) + "---\n"
	if err := os.WriteFile(roundAcceptancePath(runDir, n), []byte(content), 0o644); err != nil {
		return fmt.Errorf("shedadapters: write round %d acceptance record: %w", n, err)
	}
	return nil
}

// readAcceptance returns the accepted review's digest recorded for round n under runDir.
// A record that is absent, does not parse, or names another round or no digest reads as absent, with a nil error.
// Any other read error is returned.
func readAcceptance(runDir string, n int) (string, bool, error) {
	content, err := os.ReadFile(roundAcceptancePath(runDir, n))
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("shedadapters: read round %d acceptance record: %w", n, err)
	}
	header, err := splitFrontmatter(content, "acceptance")
	if err != nil {
		return "", false, nil
	}
	var parsed acceptanceHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil || parsed.Round != n || parsed.Digest == "" {
		return "", false, nil
	}
	return parsed.Digest, true, nil
}
