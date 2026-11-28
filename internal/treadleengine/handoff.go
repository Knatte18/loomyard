// handoff.go defines the handoff file a judge call writes alongside its verdict.
// The next judge call reads the latest valid handoff and only the reviews that handoff does not cover.
// ParseHandoff fails loud on a malformed handoff file, so the error stays visible to its direct callers.
// run.go holds the fail-safe layer.
// It logs the parse failure and falls back to reading every review.
// It never turns the failure into an engine error or a STUCK state.
package treadleengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// Handoff is the state a judge call carries into the next call.
// CoversRounds lists the rounds whose reviews it has absorbed.
// Ledger records finding identities, and Prose holds the narrative.
type Handoff struct {
	CoversRounds []int
	Ledger       []LedgerEntry
	Prose        string
}

// LedgerEntry is one finding-identity record: a Key the judge uses to recognize recurring findings,
// the Rounds it was seen in, and its Status.
type LedgerEntry struct {
	Key    string
	Rounds []int
	Status string
}

// handoffHeader mirrors the YAML frontmatter of a handoff file for unmarshaling.
// Unknown keys are tolerated (no KnownFields), since agent-written metadata in the header is harmless.
type handoffHeader struct {
	CoversRounds []int                `yaml:"covers_rounds"`
	Ledger       []handoffLedgerEntry `yaml:"ledger"`
}

// handoffLedgerEntry mirrors one ledger entry's YAML shape before validation promotes it into a LedgerEntry.
type handoffLedgerEntry struct {
	Key    string `yaml:"key"`
	Rounds []int  `yaml:"rounds"`
	Status string `yaml:"status"`
}

// ParseHandoff parses a judge handoff file into a Handoff.
// The file must open and close with "---" delimiting YAML frontmatter; everything after is prose narrative.
// It fails loud on a malformed file.
// The frontmatter must be valid YAML, and covers_rounds must be a non-empty list of positive integers.
// Each ledger entry must have a non-empty key, a non-empty rounds list, and a status of "open" or "resolved".
func ParseHandoff(content []byte) (Handoff, error) {
	header, err := splitFrontmatter(content, "handoff")
	if err != nil {
		return Handoff{}, err
	}

	var parsed handoffHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return Handoff{}, fmt.Errorf("treadle: handoff file frontmatter is not valid YAML: %w", err)
	}

	if len(parsed.CoversRounds) == 0 {
		return Handoff{}, fmt.Errorf("treadle: handoff file covers_rounds must be a non-empty list of round numbers")
	}
	for _, round := range parsed.CoversRounds {
		if round <= 0 {
			return Handoff{}, fmt.Errorf("treadle: handoff file covers_rounds contains non-positive round number %d", round)
		}
	}

	ledger, err := validateLedgerEntries(parsed.Ledger)
	if err != nil {
		return Handoff{}, err
	}

	return Handoff{
		CoversRounds: parsed.CoversRounds,
		Ledger:       ledger,
		Prose:        frontmatterProse(content),
	}, nil
}

// validateLedgerEntries promotes the raw YAML ledger entries into LedgerEntry values.
// It enforces the per-entry shape rules.
// An empty input list is legal, since a first handoff carries no prior findings,
// and it returns an empty, non-nil slice.
func validateLedgerEntries(raw []handoffLedgerEntry) ([]LedgerEntry, error) {
	ledger := make([]LedgerEntry, 0, len(raw))
	for _, entry := range raw {
		if strings.TrimSpace(entry.Key) == "" {
			return nil, fmt.Errorf("treadle: handoff file has a ledger entry with an empty key")
		}
		if len(entry.Rounds) == 0 {
			return nil, fmt.Errorf("treadle: handoff file ledger entry %q has an empty rounds list", entry.Key)
		}
		for _, round := range entry.Rounds {
			if round <= 0 {
				return nil, fmt.Errorf("treadle: handoff file ledger entry %q contains non-positive round number %d", entry.Key, round)
			}
		}
		switch entry.Status {
		case "open", "resolved":
			// within vocabulary
		default:
			return nil, fmt.Errorf("treadle: handoff file ledger entry %q has status %q; want exactly \"open\" or \"resolved\"", entry.Key, entry.Status)
		}
		ledger = append(ledger, LedgerEntry{Key: entry.Key, Rounds: entry.Rounds, Status: entry.Status})
	}
	return ledger, nil
}

// frontmatterProse returns everything after a handoff file's closing "---" delimiter line.
// The result is CRLF-normalized and trimmed.
// It assumes the delimiters were already validated, so the closing line is always found.
func frontmatterProse(content []byte) string {
	lines := strings.Split(string(content), "\n")
	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 || closingIdx+1 >= len(lines) {
		return ""
	}
	prose := strings.Join(lines[closingIdx+1:], "\n")
	return strings.TrimSpace(strings.ReplaceAll(prose, "\r", ""))
}

// latestValidHandoff finds the most recent round with a readable, parseable handoff.
// Unreadable or unparseable handoffs are skipped with a warning.
// ok is false only when no valid handoff exists.
func latestValidHandoff(name string, rounds []roundRecord) (path string, h Handoff, ok bool) {
	for i := len(rounds) - 1; i >= 0; i-- {
		round := rounds[i]
		if round.HandoffPath == "" {
			continue
		}
		content, err := os.ReadFile(round.HandoffPath)
		if err != nil {
			logger.Warn(name+": recorded handoff file unreadable, falling back to an older handoff", "round", round.Round, "cause", err)
			continue
		}
		parsed, err := ParseHandoff(content)
		if err != nil {
			logger.Warn(name+": recorded handoff file unparseable, falling back to an older handoff", "round", round.Round, "cause", err)
			continue
		}
		return round.HandoffPath, parsed, true
	}
	return "", Handoff{}, false
}

// judgeReadSet builds the review-file read-set for a judge call.
// With a valid handoff, readSet holds the reviews of rounds the handoff does not cover, plus the current review.
// With no valid handoff, readSet defaults to all reviews.
// It also returns the path of the handoff it read from, or an empty string when there is none.
func judgeReadSet(name string, rounds []roundRecord, currentReviewPath string) (readSet []string, prevHandoffPath string) {
	path, handoff, ok := latestValidHandoff(name, rounds)
	if !ok {
		return collectJudgeReviews(rounds, currentReviewPath), ""
	}

	covered := make(map[int]bool, len(handoff.CoversRounds))
	for _, round := range handoff.CoversRounds {
		covered[round] = true
	}

	reviews := make([]string, 0, len(rounds)+1)
	for _, r := range rounds {
		if !covered[r.Round] {
			reviews = append(reviews, r.ReviewPath)
		}
	}
	return append(reviews, currentReviewPath), path
}
