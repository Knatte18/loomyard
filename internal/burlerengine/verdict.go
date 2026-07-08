// verdict.go defines the review-file contract — Verdict, Severity, Finding —
// and ParseReview, the strict parser that turns a round's raw review-file
// bytes into those types. The review file is YAML frontmatter over
// unconstrained prose; ParseReview enforces every pinned rule fail-loud so a
// malformed round can never look approved.

package burlerengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Verdict is the round-level judgment recorded in a review file's
// frontmatter.
type Verdict string

// The two legal Verdict values. ParseReview rejects any other spelling,
// including a different case, since the review file is machine-read.
const (
	VerdictApproved Verdict = "APPROVED"
	VerdictBlocking Verdict = "BLOCKING"
)

// Severity is the per-finding severity tag, mapped onto the fixed
// four-value vocabulary a rubric's criteria must resolve into.
type Severity string

// The four legal Severity values.
const (
	SeverityBlocking Severity = "BLOCKING"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityNit      Severity = "NIT"
)

// Finding is one recorded review-file finding: a stable ID (perch's future
// cycle-detection key), a Severity from the fixed vocabulary, a Location
// pointing at the offending content, and a prose Summary.
type Finding struct {
	ID       string   `yaml:"id"`
	Severity Severity `yaml:"severity"`
	Location string   `yaml:"location"`
	Summary  string   `yaml:"summary"`
}

// reviewHeader mirrors the review file's YAML frontmatter shape. Unknown
// extra keys are tolerated (no KnownFields) per the yaml-strictness-split
// decision: agent-written metadata in the header is harmless noise, unlike
// the CLI's strict profile-YAML decode.
type reviewHeader struct {
	Verdict  string    `yaml:"verdict"`
	Findings []Finding `yaml:"findings"`
}

// ParseReview parses the raw bytes of a burler review file into a Verdict
// and its Findings. The file must open with a "---" line and contain a
// closing "---" line delimiting YAML frontmatter (CRLF line endings are
// tolerated); prose after the closing delimiter is unconstrained and
// ignored. Every rule below is enforced fail-loud with a burler-prefixed
// error, because a self-contradictory or malformed review file is a
// reviewer-agent defect that must never be silently accepted as a passing
// round:
//   - the frontmatter must be present, closed, and valid YAML;
//   - verdict must be exactly "APPROVED" or "BLOCKING" (case-sensitive);
//   - every finding must have a non-empty id, severity, location, summary;
//   - severity must be one of the four Severity constants;
//   - finding ids must be unique (perch keys cycle detection on them);
//   - a BLOCKING verdict must carry at least one BLOCKING-severity finding;
//   - an APPROVED verdict must carry zero BLOCKING-severity findings.
func ParseReview(content []byte) (Verdict, []Finding, error) {
	header, err := splitFrontmatter(content)
	if err != nil {
		return "", nil, err
	}

	var parsed reviewHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", nil, fmt.Errorf("burler: review file frontmatter is not valid YAML: %w", err)
	}

	verdict, err := parseVerdict(parsed.Verdict)
	if err != nil {
		return "", nil, err
	}

	if err := validateFindings(parsed.Findings); err != nil {
		return "", nil, err
	}

	// The two verdict/findings consistency rules are symmetric: a
	// self-contradictory file (BLOCKING with nothing blocking it, or
	// APPROVED despite a blocking finding) must never look approved, so
	// both directions are hard errors rather than a silent demotion or
	// promotion.
	blockingCount := 0
	for _, f := range parsed.Findings {
		if f.Severity == SeverityBlocking {
			blockingCount++
		}
	}
	if verdict == VerdictBlocking && blockingCount == 0 {
		return "", nil, fmt.Errorf("burler: review file verdict is BLOCKING but carries zero BLOCKING-severity findings")
	}
	if verdict == VerdictApproved && blockingCount > 0 {
		return "", nil, fmt.Errorf("burler: review file verdict is APPROVED but carries %d BLOCKING-severity finding(s) — a self-contradictory review file must never look approved", blockingCount)
	}

	return verdict, parsed.Findings, nil
}

// splitFrontmatter extracts the YAML header text between the file's
// opening and closing "---" delimiter lines. Each line is compared with its
// trailing "\r" trimmed so CRLF content parses identically to LF content.
func splitFrontmatter(content []byte) (string, error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("burler: review file must open with a \"---\" frontmatter delimiter line")
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return "", fmt.Errorf("burler: review file frontmatter is missing its closing \"---\" delimiter line")
	}

	header := strings.Join(lines[1:closingIdx], "\n")
	if strings.TrimSpace(header) == "" {
		return "", fmt.Errorf("burler: review file frontmatter is empty")
	}
	return header, nil
}

// parseVerdict validates raw against the two legal Verdict spellings,
// case-sensitively — the review file is machine-read, so a lowercase or
// misspelled verdict must fail loud rather than silently defaulting.
func parseVerdict(raw string) (Verdict, error) {
	switch Verdict(raw) {
	case VerdictApproved, VerdictBlocking:
		return Verdict(raw), nil
	default:
		return "", fmt.Errorf("burler: review file verdict must be exactly %q or %q, got %q", VerdictApproved, VerdictBlocking, raw)
	}
}

// validateFindings enforces the per-finding rules that do not depend on the
// verdict: every key non-empty, severity within vocabulary, and ids unique.
func validateFindings(findings []Finding) error {
	seenIDs := make(map[string]bool, len(findings))
	for _, f := range findings {
		if strings.TrimSpace(f.ID) == "" {
			return fmt.Errorf("burler: review file finding is missing a non-empty id")
		}
		if strings.TrimSpace(string(f.Severity)) == "" {
			return fmt.Errorf("burler: review file finding %q is missing a non-empty severity", f.ID)
		}
		if strings.TrimSpace(f.Location) == "" {
			return fmt.Errorf("burler: review file finding %q is missing a non-empty location", f.ID)
		}
		if strings.TrimSpace(f.Summary) == "" {
			return fmt.Errorf("burler: review file finding %q is missing a non-empty summary", f.ID)
		}
		switch f.Severity {
		case SeverityBlocking, SeverityMedium, SeverityLow, SeverityNit:
			// within vocabulary
		default:
			return fmt.Errorf("burler: review file finding %q has unknown severity %q; want one of %q, %q, %q, %q", f.ID, f.Severity, SeverityBlocking, SeverityMedium, SeverityLow, SeverityNit)
		}
		if seenIDs[f.ID] {
			return fmt.Errorf("burler: review file has duplicate finding id %q", f.ID)
		}
		seenIDs[f.ID] = true
	}
	return nil
}
