// escalation.go defines the escalation record a Bouncer leaves when it hands a round to the run's parent:
// a brief file with Go-owned frontmatter over the rendered brief, and a one-line parent notice file.
// The `circling` verbs read the cause to settle an escalation of either kind.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

// EscalationCause names why a Bouncer escalated a round to the parent.
type EscalationCause string

// The two legal EscalationCause spellings.
const (
	EscalationCircling EscalationCause = "circling"
	EscalationBudget   EscalationCause = "budget"
)

// escalationHeader mirrors an escalation file's YAML frontmatter.
type escalationHeader struct {
	Round int    `yaml:"round"`
	Cause string `yaml:"cause"`
}

// writeEscalation writes round's escalation brief file and, when notice is non-empty, its parent notice file.
// The frontmatter is written even when brief is empty, so a failed render still leaves the record the `circling` verbs read.
func writeEscalation(runDir string, round int, cause EscalationCause, brief, notice string) error {
	if cause != EscalationCircling && cause != EscalationBudget {
		return fmt.Errorf("shedadapters: escalation cause must be %q or %q, got %q", EscalationCircling, EscalationBudget, cause)
	}
	headerBytes, err := yaml.Marshal(escalationHeader{Round: round, Cause: string(cause)})
	if err != nil {
		return fmt.Errorf("bouncer: render escalation frontmatter: %w", err)
	}
	content := append(append([]byte("---\n"), headerBytes...), []byte("---\n")...)
	if brief != "" {
		content = append(content, []byte(brief)...)
	}
	path := escalationPath(runDir, round)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("shedadapters: write escalation %s: %w", path, err)
	}
	if notice == "" {
		return nil
	}
	noticePath := parentNoticePath(runDir, round)
	if err := os.WriteFile(noticePath, []byte(notice), 0o644); err != nil {
		return fmt.Errorf("shedadapters: write parent notice %s: %w", noticePath, err)
	}
	return nil
}

// readEscalation reads round's escalation record, returning the cause, the notice text, and whether a record exists.
// The notice is empty when its file is absent.
// A present record that is malformed is an error:
// the round must match the filename and the cause must be one of the two values.
func readEscalation(runDir string, round int) (EscalationCause, string, bool, error) {
	path := escalationPath(runDir, round)
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("shedadapters: read escalation %s: %w", path, err)
	}
	header, err := splitFrontmatter(raw, "escalation")
	if err != nil {
		return "", "", true, err
	}
	var parsed escalationHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", true, fmt.Errorf("bouncer: escalation frontmatter is not valid YAML: %w", err)
	}
	if parsed.Round != round {
		return "", "", true, fmt.Errorf("bouncer: escalation round %d disagrees with its filename's round %d", parsed.Round, round)
	}
	cause := EscalationCause(parsed.Cause)
	if cause != EscalationCircling && cause != EscalationBudget {
		return "", "", true, fmt.Errorf("bouncer: escalation cause must be %q or %q, got %q", EscalationCircling, EscalationBudget, parsed.Cause)
	}

	noticePath := parentNoticePath(runDir, round)
	notice, err := os.ReadFile(noticePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cause, "", true, nil
		}
		return "", "", true, fmt.Errorf("shedadapters: read parent notice %s: %w", noticePath, err)
	}
	return cause, string(notice), true, nil
}
