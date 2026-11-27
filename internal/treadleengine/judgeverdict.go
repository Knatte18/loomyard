// judgeverdict.go defines JudgeVerdict, the verdict type a progress judge writes into its verdict file,
// and the strict parser that reads the file back into that type.
// The file is YAML frontmatter over unconstrained prose.
// The parser fails loud on every rule it checks, with a "treadle: "-prefixed error;
// a self-contradictory or malformed verdict file is an agent defect and must never be silently accepted.
// The parsers are package-level functions with no calling-engine name in scope;
// so their errors carry the package name.
// The fail-safe posture lives in the judge spawners in judge.go;
// they turn these errors into a warning and a fail-safe fallback, so they never reach a caller's public API.
package treadleengine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// JudgeVerdict is the progress judge's verdict.
// Its legal spellings depend on the framing the judge ran under.
type JudgeVerdict string

// The five legal JudgeVerdict spellings across both framings.
const (
	JudgeProgressing JudgeVerdict = "PROGRESSING"
	JudgeCircling    JudgeVerdict = "CIRCLING"
	JudgeContinue    JudgeVerdict = "CONTINUE"
	JudgeStop        JudgeVerdict = "STOP"
	JudgeUncertain   JudgeVerdict = "UNCERTAIN"
)

// judgeFraming selects which of the two progress-judge prompt framings a judge verdict file came from.
// It also selects which JudgeVerdict vocabulary is legal for that file.
type judgeFraming string

const (
	framingCircling  judgeFraming = "circling"
	framingMilestone judgeFraming = "milestone"
)

// judgeHeader mirrors the YAML frontmatter of a judge verdict file.
// Unknown extra keys are tolerated, since agent-written metadata in the header is harmless noise.
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

// splitFrontmatter extracts the YAML header between the opening and closing "---" delimiter lines.
// kind names the file type for error messages ("verdict" or "handoff").
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
