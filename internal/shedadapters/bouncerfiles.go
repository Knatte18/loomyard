// bouncerfiles.go defines the three file contracts the Bouncer owns — the bouncer verdict file, the
// finding-identity ledger file, and the structured next-round focus file — plus the strict parsers
// that turn each file's raw bytes into a Go value.
// Every parser here is fail-loud: a self-contradictory or malformed file must never be mistaken for
// a valid one. The fail-safe swallow of such an error into a logger.Warn plus shedengine.Stuck lives
// one layer up, in Call, exactly mirroring the two-layer posture internal/treadleengine/judgeverdict.go
// and internal/treadleengine/handoff.go already establish across two packages.

package shedadapters

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// bouncerVerdict is the Bouncer's own verdict, recorded in a bouncer verdict file's frontmatter.
type bouncerVerdict string

// The two legal bouncerVerdict spellings.
const (
	verdictApproved bouncerVerdict = "APPROVED"
	verdictBlocking bouncerVerdict = "BLOCKING"
)

// verdictHeader mirrors a bouncer verdict file's YAML frontmatter shape.
// Unknown extra keys are tolerated (no KnownFields), matching burlerengine's reviewHeader and
// treadleengine's judgeHeader: agent-written metadata in the header is harmless noise.
type verdictHeader struct {
	Verdict   string `yaml:"verdict"`
	Rationale string `yaml:"rationale"`
}

// parseVerdict parses a bouncer verdict file into a bouncerVerdict and its rationale.
// Fails loud: the frontmatter must be present, closed, and valid YAML; verdict must be exactly
// verdictApproved or verdictBlocking (case-sensitive); rationale must be non-empty after
// strings.TrimSpace.
func parseVerdict(content []byte) (bouncerVerdict, string, error) {
	header, err := splitFrontmatter(content, "verdict")
	if err != nil {
		return "", "", err
	}

	var parsed verdictHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", fmt.Errorf("bouncer: verdict file frontmatter is not valid YAML: %w", err)
	}

	switch bouncerVerdict(parsed.Verdict) {
	case verdictApproved, verdictBlocking:
		// within vocabulary
	default:
		return "", "", fmt.Errorf("bouncer: verdict file verdict must be exactly %q or %q, got %q", verdictApproved, verdictBlocking, parsed.Verdict)
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("bouncer: verdict file is missing a non-empty rationale")
	}

	return bouncerVerdict(parsed.Verdict), parsed.Rationale, nil
}

// ledgerEntry is one finding-identity record in a ledger file: a Key the judge uses to recognize
// recurring findings, the Rounds it was seen in, and its Status.
type ledgerEntry struct {
	Key    string
	Rounds []int
	Status string
}

// ledgerFile is a parsed bouncer ledger file: the Round it pertains to, its Entries, and its prose
// narrative.
type ledgerFile struct {
	Round   int
	Entries []ledgerEntry
	Prose   string
}

// ledgerHeader mirrors a ledger file's YAML frontmatter shape for unmarshaling.
type ledgerHeader struct {
	Round  int               `yaml:"round"`
	Ledger []ledgerEntryYAML `yaml:"ledger"`
}

// ledgerEntryYAML mirrors one ledger entry's YAML shape before validation promotes it into a
// ledgerEntry.
type ledgerEntryYAML struct {
	Key    string `yaml:"key"`
	Rounds []int  `yaml:"rounds"`
	Status string `yaml:"status"`
}

// parseLedger parses a bouncer ledger file into a ledgerFile.
// Fails loud: the frontmatter must be present, closed, and valid YAML; round must be a positive
// int; the (legally empty) ledger list's entries must each have a non-empty key, a non-empty
// rounds list of positive ints, and a status of exactly "open" or "resolved".
// parseLedger compares nothing against any previous ledger — carry-forward is stated in the judge
// prompt and is deliberately not enforced here.
func parseLedger(content []byte) (ledgerFile, error) {
	header, err := splitFrontmatter(content, "ledger")
	if err != nil {
		return ledgerFile{}, err
	}

	var parsed ledgerHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return ledgerFile{}, fmt.Errorf("bouncer: ledger file frontmatter is not valid YAML: %w", err)
	}

	if parsed.Round <= 0 {
		return ledgerFile{}, fmt.Errorf("bouncer: ledger file round must be a positive integer, got %d", parsed.Round)
	}

	entries := make([]ledgerEntry, 0, len(parsed.Ledger))
	for _, raw := range parsed.Ledger {
		if strings.TrimSpace(raw.Key) == "" {
			return ledgerFile{}, fmt.Errorf("bouncer: ledger file has a ledger entry with an empty key")
		}
		if len(raw.Rounds) == 0 {
			return ledgerFile{}, fmt.Errorf("bouncer: ledger file entry %q has an empty rounds list", raw.Key)
		}
		for _, round := range raw.Rounds {
			if round <= 0 {
				return ledgerFile{}, fmt.Errorf("bouncer: ledger file entry %q contains non-positive round number %d", raw.Key, round)
			}
		}
		switch raw.Status {
		case "open", "resolved":
			// within vocabulary
		default:
			return ledgerFile{}, fmt.Errorf("bouncer: ledger file entry %q has status %q; want exactly \"open\" or \"resolved\"", raw.Key, raw.Status)
		}
		entries = append(entries, ledgerEntry{Key: raw.Key, Rounds: raw.Rounds, Status: raw.Status})
	}

	return ledgerFile{
		Round:   parsed.Round,
		Entries: entries,
		Prose:   frontmatterProse(content),
	}, nil
}

// focusFile is a parsed bouncer focus file: the Round it targets, the lenses excluded from that
// round, the round's own focus directives, and its prose narrative.
type focusFile struct {
	Round         int
	ExcludeLenses []string
	Focus         []string
	Prose         string
}

// focusHeader mirrors a focus file's YAML frontmatter shape for unmarshaling.
type focusHeader struct {
	Round         int      `yaml:"round"`
	ExcludeLenses []string `yaml:"exclude_lenses"`
	Focus         []string `yaml:"focus"`
}

// parseFocus parses a bouncer focus file into a focusFile.
// Fails loud: the frontmatter must be present, closed, and valid YAML; round must be a positive
// int; exclude_lenses and focus must each be lists of strings, legally empty — a scalar where a
// list is required is a parse error, which yaml.Unmarshal into a []string field already produces.
// Both list fields yield empty, non-nil slices when absent or empty.
func parseFocus(content []byte) (focusFile, error) {
	header, err := splitFrontmatter(content, "focus")
	if err != nil {
		return focusFile{}, err
	}

	var parsed focusHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return focusFile{}, fmt.Errorf("bouncer: focus file frontmatter is not valid YAML: %w", err)
	}

	if parsed.Round <= 0 {
		return focusFile{}, fmt.Errorf("bouncer: focus file round must be a positive integer, got %d", parsed.Round)
	}

	excludeLenses := parsed.ExcludeLenses
	if excludeLenses == nil {
		excludeLenses = []string{}
	}
	focus := parsed.Focus
	if focus == nil {
		focus = []string{}
	}

	return focusFile{
		Round:         parsed.Round,
		ExcludeLenses: excludeLenses,
		Focus:         focus,
		Prose:         frontmatterProse(content),
	}, nil
}

// renderFocus marshals f into a bouncer focus file's bytes: a YAML frontmatter header carrying
// round, exclude_lenses, and focus, wrapped between an opening and a closing "---" line, with
// f.Prose appended below the closing delimiter when non-empty.
// Marshalling the header through yaml.Marshal rather than hand-formatting it is what makes a lens
// name containing a YAML metacharacter round-trip instead of corrupting the file.
// Both list fields render as an explicit empty list rather than as null when f's corresponding
// slice is empty, so the emitted file satisfies parseFocus's always-present-list shape.
func renderFocus(f focusFile) ([]byte, error) {
	excludeLenses := f.ExcludeLenses
	if excludeLenses == nil {
		excludeLenses = []string{}
	}
	focus := f.Focus
	if focus == nil {
		focus = []string{}
	}

	header := focusHeader{
		Round:         f.Round,
		ExcludeLenses: excludeLenses,
		Focus:         focus,
	}
	headerBytes, err := yaml.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("bouncer: render focus file frontmatter: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(headerBytes)
	b.WriteString("---\n")
	if f.Prose != "" {
		b.WriteString(f.Prose)
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

// writeFocus renders f via renderFocus and writes the result to path at mode 0o644.
func writeFocus(path string, f focusFile) error {
	content, err := renderFocus(f)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("bouncer: write focus file %s: %w", path, err)
	}
	return nil
}

// splitFrontmatter extracts the YAML header text between the file's opening and closing "---"
// delimiter lines. kind names the file type for error messages ("verdict", "ledger", or "focus").
// Each line is compared with its trailing "\r" trimmed so CRLF content parses identically to LF
// content.
func splitFrontmatter(content []byte, kind string) (string, error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("bouncer: %s file must open with a \"---\" frontmatter delimiter line", kind)
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 {
		return "", fmt.Errorf("bouncer: %s file frontmatter is missing its closing \"---\" delimiter line", kind)
	}

	header := strings.Join(lines[1:closingIdx], "\n")
	if strings.TrimSpace(header) == "" {
		return "", fmt.Errorf("bouncer: %s file frontmatter is empty", kind)
	}
	return header, nil
}

// frontmatterProse returns everything after a file's closing "---" frontmatter delimiter line,
// CRLF-normalized and trimmed, and "" when there is nothing after it.
// It is called only after splitFrontmatter has already validated that the opening and closing
// delimiters exist, so the closing line is guaranteed to be found here; this is a separate,
// minimal scan rather than a splitFrontmatter return value because splitFrontmatter's contract is
// header-only and prose is a distinct, kind-agnostic need.
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
