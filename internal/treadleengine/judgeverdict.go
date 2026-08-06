// judgeverdict.go defines the two verdict-file contracts treadle's ephemeral LLM utilities read
// back — JudgeVerdict (both progress-judge framings) and TriageVerdict (asking-triage) — plus the
// strict parsers that turn a verdict file's raw bytes into those types.
// Both files are YAML frontmatter over unconstrained prose, mirroring burlerengine.ParseReview's
// contract and error posture: every rule below is enforced fail-loud with a "treadle: "-prefixed
// error (these parsers are package-level pure functions with no calling-engine name in scope,
// unlike the rest of this package's diagnostics — see the pinned parser-prefix resolution in the
// treadle-extraction batch notes), because a self-contradictory or malformed verdict file is an
// agent defect that must never be silently accepted — the fail-safe posture lives one layer up, in
// judge.go's spawners, which swallow these errors into a Warn + fail-safe fallback and never let
// them surface through a caller's public API.
package treadleengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// JudgeVerdict is the progress judge's verdict.
// Its legal spellings depend on which judgeFraming was used.
type JudgeVerdict string

// The five legal JudgeVerdict spellings across both framings.
const (
	JudgeProgressing JudgeVerdict = "PROGRESSING"
	JudgeCircling    JudgeVerdict = "CIRCLING"
	JudgeContinue    JudgeVerdict = "CONTINUE"
	JudgeStop        JudgeVerdict = "STOP"
	JudgeUncertain   JudgeVerdict = "UNCERTAIN"
)

// TriageVerdict is the asking-triage call's verdict, recorded in a triage verdict file's
// frontmatter.
type TriageVerdict string

// The two legal TriageVerdict values.
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

// ParseJudgeVerdict parses a progress-judge verdict file into a JudgeVerdict and its rationale.
// The file must open and close with "---" delimiting YAML frontmatter;
// prose after is unconstrained.
// Fails loud: frontmatter must be valid YAML, verdict must match framing's vocabulary, and
// rationale must be non-empty.
func ParseJudgeVerdict(content []byte, framing judgeFraming) (JudgeVerdict, string, error) {
	header, err := splitFrontmatter(content, "verdict")
	if err != nil {
		return "", "", err
	}

	var parsed judgeHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", fmt.Errorf("treadle: judge verdict file frontmatter is not valid YAML: %w", err)
	}

	verdict, err := parseJudgeVerdict(parsed.Verdict, framing)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("treadle: judge verdict file is missing a non-empty rationale")
	}

	return verdict, parsed.Rationale, nil
}

// ParseTriageVerdict parses an asking-triage verdict file into a TriageVerdict and its rationale,
// applying the same frontmatter and non-empty-rationale rules.
func ParseTriageVerdict(content []byte) (TriageVerdict, string, error) {
	header, err := splitFrontmatter(content, "verdict")
	if err != nil {
		return "", "", err
	}

	var parsed judgeHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", fmt.Errorf("treadle: triage verdict file frontmatter is not valid YAML: %w", err)
	}

	verdict, err := parseTriageVerdict(parsed.Verdict)
	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("treadle: triage verdict file is missing a non-empty rationale")
	}

	return verdict, parsed.Rationale, nil
}

// splitFrontmatter extracts the YAML header between opening and closing "---"
// delimiter lines. kind names the file type for error messages ("verdict" or
// "handoff").
func splitFrontmatter(content []byte, kind string) (string, error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("treadle: %s file must open with a \"---\" frontmatter delimiter line", kind)
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return "", fmt.Errorf("treadle: %s file frontmatter is missing its closing \"---\" delimiter line", kind)
	}

	header := strings.Join(lines[1:closingIdx], "\n")
	if strings.TrimSpace(header) == "" {
		return "", fmt.Errorf("treadle: %s file frontmatter is empty", kind)
	}
	return header, nil
}

// parseJudgeVerdict validates raw against framing's legal spellings, case-sensitively.
func parseJudgeVerdict(raw string, framing judgeFraming) (JudgeVerdict, error) {
	switch framing {
	case framingCircling:
		switch JudgeVerdict(raw) {
		case JudgeProgressing, JudgeCircling, JudgeUncertain:
			return JudgeVerdict(raw), nil
		default:
			return "", fmt.Errorf("treadle: judge verdict file verdict must be exactly %q, %q, or %q, got %q", JudgeProgressing, JudgeCircling, JudgeUncertain, raw)
		}
	case framingMilestone:
		switch JudgeVerdict(raw) {
		case JudgeContinue, JudgeStop, JudgeUncertain:
			return JudgeVerdict(raw), nil
		default:
			return "", fmt.Errorf("treadle: judge verdict file verdict must be exactly %q, %q, or %q, got %q", JudgeContinue, JudgeStop, JudgeUncertain, raw)
		}
	default:
		return "", fmt.Errorf("treadle: unknown judge framing %q", framing)
	}
}

// parseTriageVerdict validates raw against the two legal spellings, case-sensitively.
func parseTriageVerdict(raw string) (TriageVerdict, error) {
	switch TriageVerdict(raw) {
	case TriageRetry, TriageGiveUp:
		return TriageVerdict(raw), nil
	default:
		return "", fmt.Errorf("treadle: triage verdict file verdict must be exactly %q or %q, got %q", TriageRetry, TriageGiveUp, raw)
	}
}
