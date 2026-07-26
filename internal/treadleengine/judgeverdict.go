// judgeverdict.go defines the two verdict-file contracts perch's ephemeral
// LLM utilities read back — JudgeVerdict (both progress-judge framings) and
// TriageVerdict (asking-triage) — plus the strict parsers that turn a
// verdict file's raw bytes into those types. Both files are YAML
// frontmatter over unconstrained prose, mirroring burlerengine.ParseReview's
// contract and error posture: every rule below is enforced fail-loud with a
// "perch: "-prefixed error, because a self-contradictory or malformed
// verdict file is an agent defect that must never be silently accepted —
// the fail-safe posture lives one layer up, in judge.go's spawners, not
// here.
package perchengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// JudgeVerdict is the progress judge's verdict, recorded in a judge verdict
// file's frontmatter. Its legal spellings depend on which judgeFraming the
// call used: the circling-check framing allows JudgeProgressing,
// JudgeCircling, JudgeUncertain; the milestone framing allows
// JudgeContinue, JudgeStop, JudgeUncertain.
type JudgeVerdict string

// The five legal JudgeVerdict spellings across both framings. ParseJudgeVerdict
// rejects any other spelling, including a different case or the wrong
// framing's vocabulary, since the verdict file is machine-read.
const (
	JudgeProgressing JudgeVerdict = "PROGRESSING"
	JudgeCircling    JudgeVerdict = "CIRCLING"
	JudgeContinue    JudgeVerdict = "CONTINUE"
	JudgeStop        JudgeVerdict = "STOP"
	JudgeUncertain   JudgeVerdict = "UNCERTAIN"
)

// TriageVerdict is the asking-triage call's verdict, recorded in a triage
// verdict file's frontmatter.
type TriageVerdict string

// The two legal TriageVerdict values. ParseTriageVerdict rejects any other
// spelling, including a different case, since the verdict file is
// machine-read.
const (
	TriageRetry  TriageVerdict = "RETRY"
	TriageGiveUp TriageVerdict = "GIVE_UP"
)

// judgeFraming selects which of the two progress-judge prompt framings a
// judge verdict file came from, and therefore which JudgeVerdict vocabulary
// is legal for it.
type judgeFraming string

const (
	framingCircling  judgeFraming = "circling"
	framingMilestone judgeFraming = "milestone"
)

// judgeHeader mirrors a judge or triage verdict file's YAML frontmatter
// shape. Unknown extra keys are tolerated (no KnownFields), matching
// burlerengine's reviewHeader: agent-written metadata in the header is
// harmless noise.
type judgeHeader struct {
	Verdict   string `yaml:"verdict"`
	Rationale string `yaml:"rationale"`
}

// ParseJudgeVerdict parses the raw bytes of a progress-judge verdict file
// into a JudgeVerdict and its rationale. The file must open with a "---"
// line and contain a closing "---" line delimiting YAML frontmatter (CRLF
// line endings are tolerated); prose after the closing delimiter, including
// the required "## Themes" section, is unconstrained and ignored. Every
// rule below is enforced fail-loud with a "perch: "-prefixed error:
//   - the frontmatter must be present, closed, and valid YAML;
//   - verdict must be exactly one of framing's vocabulary (case-sensitive) —
//     {PROGRESSING, CIRCLING, UNCERTAIN} for framingCircling, {CONTINUE,
//     STOP, UNCERTAIN} for framingMilestone;
//   - rationale must be non-empty.
func ParseJudgeVerdict(content []byte, framing judgeFraming) (JudgeVerdict, string, error) {
	header, err := splitFrontmatter(content)
	if err != nil {
		return "", "", err
	}

	var parsed judgeHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", fmt.Errorf("perch: judge verdict file frontmatter is not valid YAML: %w", err)
	}

	verdict, err := parseJudgeVerdict(parsed.Verdict, framing)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("perch: judge verdict file is missing a non-empty rationale")
	}

	return verdict, parsed.Rationale, nil
}

// ParseTriageVerdict parses the raw bytes of an asking-triage verdict file
// into a TriageVerdict and its rationale, applying the same frontmatter and
// non-empty-rationale rules as ParseJudgeVerdict, with the triage
// vocabulary: verdict must be exactly RETRY or GIVE_UP (case-sensitive).
func ParseTriageVerdict(content []byte) (TriageVerdict, string, error) {
	header, err := splitFrontmatter(content)
	if err != nil {
		return "", "", err
	}

	var parsed judgeHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", fmt.Errorf("perch: triage verdict file frontmatter is not valid YAML: %w", err)
	}

	verdict, err := parseTriageVerdict(parsed.Verdict)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("perch: triage verdict file is missing a non-empty rationale")
	}

	return verdict, parsed.Rationale, nil
}

// splitFrontmatter extracts the YAML header text between a verdict file's
// opening and closing "---" delimiter lines. This is a package-private copy
// of burlerengine's splitFrontmatter (same three fail-loud checks: opening
// "---", closing "---", non-empty header; CRLF-tolerant) rather than an
// export of burler's, since the two parsers evolve independently (batch
// decision, 03-judge-triage.md). Each line is compared with its trailing
// "\r" trimmed so CRLF content parses identically to LF content.
func splitFrontmatter(content []byte) (string, error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("perch: verdict file must open with a \"---\" frontmatter delimiter line")
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return "", fmt.Errorf("perch: verdict file frontmatter is missing its closing \"---\" delimiter line")
	}

	header := strings.Join(lines[1:closingIdx], "\n")
	if strings.TrimSpace(header) == "" {
		return "", fmt.Errorf("perch: verdict file frontmatter is empty")
	}
	return header, nil
}

// parseJudgeVerdict validates raw against framing's legal JudgeVerdict
// spellings, case-sensitively — the verdict file is machine-read, so a
// lowercase, misspelled, or wrong-framing verdict must fail loud rather than
// silently defaulting.
func parseJudgeVerdict(raw string, framing judgeFraming) (JudgeVerdict, error) {
	switch framing {
	case framingCircling:
		switch JudgeVerdict(raw) {
		case JudgeProgressing, JudgeCircling, JudgeUncertain:
			return JudgeVerdict(raw), nil
		default:
			return "", fmt.Errorf("perch: judge verdict file verdict must be exactly %q, %q, or %q, got %q", JudgeProgressing, JudgeCircling, JudgeUncertain, raw)
		}
	case framingMilestone:
		switch JudgeVerdict(raw) {
		case JudgeContinue, JudgeStop, JudgeUncertain:
			return JudgeVerdict(raw), nil
		default:
			return "", fmt.Errorf("perch: judge verdict file verdict must be exactly %q, %q, or %q, got %q", JudgeContinue, JudgeStop, JudgeUncertain, raw)
		}
	default:
		return "", fmt.Errorf("perch: unknown judge framing %q", framing)
	}
}

// parseTriageVerdict validates raw against the two legal TriageVerdict
// spellings, case-sensitively.
func parseTriageVerdict(raw string) (TriageVerdict, error) {
	switch TriageVerdict(raw) {
	case TriageRetry, TriageGiveUp:
		return TriageVerdict(raw), nil
	default:
		return "", fmt.Errorf("perch: triage verdict file verdict must be exactly %q or %q, got %q", TriageRetry, TriageGiveUp, raw)
	}
}
