// verdict.go defines the review-file contract: the Verdict, Severity and Finding types.
// It also holds ParseReview, the strict parser that turns a review file's bytes into those types.
// The review-parse gate (CheckReviewFile, ReviewGate) lets a round's reviewer repair an unparseable review file.
// The review file is YAML frontmatter over unconstrained prose.
// ParseReview enforces every pinned rule fail-loud, so a malformed round can never look approved.

package burlerengine

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Verdict is the round-level judgment recorded in a review file's frontmatter.
type Verdict string

// The two legal Verdict values.
// ParseReview rejects any other spelling, including a different case, because the review file is machine-read.
const (
	VerdictApproved Verdict = "APPROVED"
	VerdictBlocking Verdict = "BLOCKING"
)

// Severity is the per-finding severity tag.
// It is one of four fixed values, and every rubric criterion must resolve into one of them.
type Severity string

// The four legal Severity values.
const (
	SeverityBlocking Severity = "BLOCKING"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityNit      Severity = "NIT"
)

// Class is the per-finding class tag.
// It says what kind of finding it is.
// It decides who decides the finding and when a review loop stops, never whether it is fixed.
type Class string

// The four legal Class values.
const (
	// ClassDesign: the design is wrong, a decision is missing or rests on a false premise.
	ClassDesign Class = "design"
	// ClassScope: a call site, file or case the work missed.
	ClassScope Class = "scope"
	// ClassDecision: a choice left open that the author must make.
	ClassDecision Class = "decision"
	// ClassConsistency: two parts of the artifact disagree, or it departs from its own conventions.
	ClassConsistency Class = "consistency"
)

// GatingClass is the one class that decides when a review loop stops, in every segment.
// A round with no finding of this class has nothing left that holds the loop open.
const GatingClass = ClassDesign

// Finding is one review-file finding.
// Its ID is unique within the file, so cross-round hydration and audit can cite it unambiguously.
// Location points at the offending content, and Summary states the problem in prose.
type Finding struct {
	ID       string   `yaml:"id"`
	Severity Severity `yaml:"severity"`
	Class    Class    `yaml:"class"`
	Location string   `yaml:"location"`
	Summary  string   `yaml:"summary"`
	// Origin is optional free-text provenance, such as `lens:<name>` or `handler`.
	// A cluster round writes it onto each consolidated finding.
	// It is never validated.
	// The parser accepts review files with and without it,
	// so a review file written without it parses the same way.
	Origin string `yaml:"origin"`
	// Fix is the exact change to make and where, so a fixer can act on the review alone.
	// The parser does not require it, so a review file without it still parses;
	// the strict parse of the current round rejects a finding whose fix is empty.
	Fix string `yaml:"fix"`
	// ParentIDs names the ids of the parent's report that a finding kept from that report covers, such as P1.
	// It is optional and never validated; the parent coverage gate entry reads it.
	ParentIDs []string `yaml:"parent_ids"`
}

// reviewHeader mirrors the review file's YAML frontmatter shape.
// The decoder tolerates unknown keys, since agent-written metadata in the header is harmless.
type reviewHeader struct {
	Verdict  string    `yaml:"verdict"`
	Findings []Finding `yaml:"findings"`
}

// ParseReview parses the raw bytes of a review file into a Verdict and Findings.
// It enforces every rule below fail-loud, with a burler-prefixed error for each failure.
// A self-contradictory or malformed file must never silently pass:
//   - the frontmatter must be present, closed, and valid YAML;
//   - verdict must be exactly "APPROVED" or "BLOCKING" (case-sensitive);
//   - every finding must have a non-empty id, severity, location, summary;
//   - severity must be one of the four legal values;
//   - every finding must have a non-empty class, one of the four legal classes;
//   - finding ids must be unique within the file;
//   - a BLOCKING verdict must carry at least one BLOCKING-severity finding;
//   - an APPROVED verdict must carry zero BLOCKING-severity findings.
func ParseReview(content []byte) (Verdict, []Finding, error) {
	header, _, err := splitFrontmatter(content)
	if err != nil {
		return "", nil, err
	}

	var parsed reviewHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", nil, fmt.Errorf("burler: review file frontmatter is not valid YAML: %w%s", err, quotedSummaryHint(header))
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

// proseFindingHeading matches the heading line of a finding's prose block.
// The heading is `### [SEVERITY] <title>`, with SEVERITY one of the four legal severities.
var proseFindingHeading = regexp.MustCompile(`(?m)^### \[(BLOCKING|MEDIUM|LOW|NIT)\]`)

// ParseCurrentReview is the strict parse of the current round's review file.
// It first runs the base parse, then refuses:
//   - a finding whose fix is empty or whitespace;
//   - a top-level frontmatter key other than verdict and findings;
//   - prose finding blocks whose count per severity differs from the frontmatter's.
//
// Each refusal is a burler-prefixed error that ends with what to rewrite.
// A zero-findings result means the reviewer wrote no finding.
func ParseCurrentReview(content []byte) (Verdict, []Finding, error) {
	verdict, findings, err := ParseReview(content)
	if err != nil {
		return "", nil, err
	}
	for _, f := range findings {
		if strings.TrimSpace(f.Fix) == "" {
			return "", nil, fmt.Errorf("burler: review file finding %q is missing a non-empty fix; rewrite it with a `fix` key holding the exact change to make and where", f.ID)
		}
	}
	header, body, err := splitFrontmatter(content)
	if err != nil {
		return "", nil, err
	}
	if err := checkReviewKeys(header); err != nil {
		return "", nil, err
	}
	if err := checkProseBlocks(body, findings); err != nil {
		return "", nil, err
	}
	return verdict, findings, nil
}

// checkReviewKeys refuses a frontmatter header holding a top-level key other than verdict and findings.
// Findings under a misspelled key would otherwise decode as no findings at all.
func checkReviewKeys(header string) error {
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(header), &keys); err != nil {
		return fmt.Errorf("burler: review file frontmatter is not valid YAML: %w", err)
	}
	var stray []string
	for key := range keys {
		if key != "verdict" && key != "findings" {
			stray = append(stray, key)
		}
	}
	if len(stray) == 0 {
		return nil
	}
	sort.Strings(stray)
	return fmt.Errorf("burler: review file frontmatter has the top-level key %q; its only keys are `verdict` and `findings`, so rewrite the frontmatter with those two", stray[0])
}

// checkProseBlocks refuses a review body whose `### [SEVERITY]` block count per severity differs from the frontmatter.
func checkProseBlocks(body string, findings []Finding) error {
	severities := []Severity{SeverityBlocking, SeverityMedium, SeverityLow, SeverityNit}
	prose := make(map[Severity]int, len(severities))
	for _, match := range proseFindingHeading.FindAllStringSubmatch(body, -1) {
		prose[Severity(match[1])]++
	}
	listed := make(map[Severity]int, len(severities))
	for _, f := range findings {
		listed[f.Severity]++
	}
	for _, severity := range severities {
		if prose[severity] != listed[severity] {
			return fmt.Errorf("burler: review file has %d `### [%s]` prose block(s) but %d %s finding(s) in its frontmatter; rewrite it with exactly one prose block per finding", prose[severity], severity, listed[severity], severity)
		}
	}
	return nil
}

// reviewGateAttempts is the re-prompt budget of the review-parse gate.
const reviewGateAttempts = 3

// CheckReviewFile reads the review file at path.
// It returns the strict parse's error for the file, or nil when the file parses.
// A read failure is returned wrapped with the path.
func CheckReviewFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("burler: read review file %q: %w", path, err)
	}
	_, _, err = ParseCurrentReview(content)
	return err
}

// ReviewGate returns a gate that passes when the review file at reviewPath parses.
// A missing or unparseable review file is a failed result, never a gate error:
// its findings carry the parse error verbatim, with any quoting hint, and a closing line names the file to rewrite.
func ReviewGate(reviewPath string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		err := CheckReviewFile(reviewPath)
		if err == nil {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return shuttleengine.GateResult{
			Findings: fmt.Sprintf("%s\n\nRewrite the review file (%s) so it parses.", err.Error(), reviewPath),
		}, nil
	}
}

// ReviewGateEntry returns a gate entry that re-prompts the reviewer to repair an unparseable review file at reviewPath.
// It lets the run through once its re-prompt budget is spent,
// so the strict parse after the gate still fails the round.
func ReviewGateEntry(reviewPath string) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{Name: "review", Gate: ReviewGate(reviewPath), Attempts: reviewGateAttempts, PassOnCap: true}
}

// malformedQuotedSummary matches a `summary:` line whose value opens with a double-quoted fragment.
// The value then carries more, unquoted content on the same line: `summary: "capital" is misspelled as "captial"`.
// That is invalid YAML, since a quoted scalar must end the value.
var malformedQuotedSummary = regexp.MustCompile(`(?m)^\s*summary:\s*"[^"]*"\s*\S`)

// quotedSummaryHint returns a diagnostic suffix for a header with a quoted summary fragment and trailing prose.
// The suffix points the reviewer or the operator at the fix instead of a bare YAML parser error.
func quotedSummaryHint(header string) string {
	if malformedQuotedSummary.MatchString(header) {
		return " (a summary containing a \" character must be ONE double-quoted string covering the whole value, not a quoted fragment followed by trailing prose)"
	}
	return ""
}

// splitFrontmatter extracts the YAML header text between the file's opening and closing "---" delimiter lines.
// It returns the body below the closing delimiter.
// Each line is compared with its trailing "\r" trimmed so CRLF content parses identically to LF content.
func splitFrontmatter(content []byte) (header, body string, err error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", "", fmt.Errorf("burler: review file must open with a \"---\" frontmatter delimiter line")
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return "", "", fmt.Errorf("burler: review file frontmatter is missing its closing \"---\" delimiter line")
	}

	header = strings.Join(lines[1:closingIdx], "\n")
	if strings.TrimSpace(header) == "" {
		return "", "", fmt.Errorf("burler: review file frontmatter is empty")
	}
	return header, strings.Join(lines[closingIdx+1:], "\n"), nil
}

// parseVerdict validates raw against the two legal verdict values, case-sensitively.
// The review file is machine-read, so a lowercase or misspelled verdict must fail loud rather than default silently.
func parseVerdict(raw string) (Verdict, error) {
	switch Verdict(raw) {
	case VerdictApproved, VerdictBlocking:
		return Verdict(raw), nil
	default:
		return "", fmt.Errorf("burler: review file verdict must be exactly %q or %q, got %q", VerdictApproved, VerdictBlocking, raw)
	}
}

// validateFindings enforces the per-finding rules that do not depend on the verdict:
// every finding has a non-empty id, severity, location, summary and class;
// severity and class are in their vocabularies; and ids are unique.
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
		default:
			return fmt.Errorf("burler: review file finding %q has unknown severity %q; want one of %q, %q, %q, %q", f.ID, f.Severity, SeverityBlocking, SeverityMedium, SeverityLow, SeverityNit)
		}
		if strings.TrimSpace(string(f.Class)) == "" {
			return fmt.Errorf("burler: review file finding %q is missing a non-empty class; want one of %q, %q, %q, %q", f.ID, ClassDesign, ClassScope, ClassDecision, ClassConsistency)
		}
		switch f.Class {
		case ClassDesign, ClassScope, ClassDecision, ClassConsistency:
		default:
			return fmt.Errorf("burler: review file finding %q has unknown class %q; want one of %q, %q, %q, %q", f.ID, f.Class, ClassDesign, ClassScope, ClassDecision, ClassConsistency)
		}
		if seenIDs[f.ID] {
			return fmt.Errorf("burler: review file has duplicate finding id %q", f.ID)
		}
		seenIDs[f.ID] = true
	}
	return nil
}
