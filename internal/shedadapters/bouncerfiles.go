// bouncerfiles.go holds the strict parsers for the Bouncer's three file contracts: verdict, ledger and focus.
// Parsers fail loud on a malformed file;
// the Bouncer's Call turns that error into a logger warning and shedengine.Stuck.

package shedadapters

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"gopkg.in/yaml.v3"
)

// bouncerVerdict is the Bouncer's own verdict, recorded in a bouncer verdict file's frontmatter.
type bouncerVerdict string

// The three legal bouncerVerdict spellings.
const (
	verdictConverged bouncerVerdict = "CONVERGED"
	verdictContinue  bouncerVerdict = "CONTINUE"
	verdictCircling  bouncerVerdict = "CIRCLING"
)

// Legacy spellings an older binary's judge wrote.
// Only parseRecordedVerdict reads them.
const (
	legacyVerdictApproved = "APPROVED"
	legacyVerdictBlocking = "BLOCKING"
)

// verdictHeader mirrors a bouncer verdict file's YAML frontmatter shape.
// Unknown extra keys are tolerated, because the decoder does not set KnownFields:
// agent-written metadata in the header is harmless noise.
type verdictHeader struct {
	Verdict   string `yaml:"verdict"`
	Rationale string `yaml:"rationale"`
}

// parseVerdict parses a bouncer verdict file into a bouncerVerdict and its rationale.
// Fails loud: the frontmatter must be present, closed, and valid YAML;
// verdict must be exactly verdictConverged, verdictContinue or verdictCircling;
// the match is case-sensitive, so a legacy word is rejected;
// rationale must be non-empty after strings.TrimSpace.
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
	case verdictConverged, verdictContinue, verdictCircling:
		// within vocabulary
	default:
		return "", "", fmt.Errorf("bouncer: verdict file verdict must be exactly %q, %q or %q, got %q", verdictConverged, verdictContinue, verdictCircling, parsed.Verdict)
	}

	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", fmt.Errorf("bouncer: verdict file is missing a non-empty rationale")
	}

	return bouncerVerdict(parsed.Verdict), parsed.Rationale, nil
}

// parseRecordedVerdict parses a verdict that was already on disk at Call entry.
// It tries parseVerdict first.
// Otherwise it reads a legacy APPROVED as verdictConverged, and a legacy BLOCKING as verdictContinue.
// The legacy result is true for either alias.
// The rest of the file must meet parseVerdict's rules: valid frontmatter and a non-empty rationale.
func parseRecordedVerdict(content []byte) (verdict bouncerVerdict, rationale string, legacy bool, err error) {
	verdict, rationale, strictErr := parseVerdict(content)
	if strictErr == nil {
		return verdict, rationale, false, nil
	}

	header, err := splitFrontmatter(content, "verdict")
	if err != nil {
		return "", "", false, err
	}
	var parsed verdictHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return "", "", false, fmt.Errorf("bouncer: verdict file frontmatter is not valid YAML: %w", err)
	}
	if strings.TrimSpace(parsed.Rationale) == "" {
		return "", "", false, strictErr
	}
	switch parsed.Verdict {
	case legacyVerdictApproved:
		return verdictConverged, parsed.Rationale, true, nil
	case legacyVerdictBlocking:
		return verdictContinue, parsed.Rationale, true, nil
	}
	return "", "", false, strictErr
}

// recordedVerdict reads and parses round's verdict file and ledger file inside runDir.
// It returns the parsed verdict and ledger, and false if either file is missing or does not parse;
// it also returns false when the ledger names another round.
// It is the predicate for whether the round has been judged.
// The parsed values come back with it, so a caller that needs them does not re-read the files.
//
// It is a free function over the run directory, not a Bouncer method, because two callers must answer it identically:
// one asks whether a round still needs a judgment, and the other asks whether the run may advance past it.
// A round about to be re-judged must not already have been written over by the producer.
// Requiring the ledger as well as the verdict keeps the two answers the same:
// a verdict whose ledger is missing or malformed reads as unjudged, just as an absent verdict does.
//
// The focus file is left out: it is an input to the next round rather than evidence about this one,
// and it can be synthesized, so including it would let a missing focus file invalidate a judgment that happened.
//
// The verdict is read through parseRecordedVerdict,
// so a run directory that an older binary judged still reads as judged.
func recordedVerdict(runDir string, round int) (bouncerVerdict, ledgerFile, bool) {
	return readJudgment(runDir, round, func(content []byte) (bouncerVerdict, error) {
		verdict, _, _, err := parseRecordedVerdict(content)
		return verdict, err
	})
}

// harvestedVerdict reads the verdict of a judge that this Call spawned or attached.
// It accepts the strict vocabulary only, so a legacy word counts as no harvest and the caller judges the round again.
func harvestedVerdict(runDir string, round int) (bouncerVerdict, ledgerFile, bool) {
	return readJudgment(runDir, round, func(content []byte) (bouncerVerdict, error) {
		verdict, _, err := parseVerdict(content)
		return verdict, err
	})
}

// readJudgment reads round's verdict through parse, then requires round's ledger to parse and to name the same round.
// It returns false on any read or parse failure, or when the ledger names another round.
func readJudgment(runDir string, round int, parse func([]byte) (bouncerVerdict, error)) (bouncerVerdict, ledgerFile, bool) {
	verdictRaw, err := os.ReadFile(verdictPath(runDir, round))
	if err != nil {
		return "", ledgerFile{}, false
	}
	verdict, err := parse(verdictRaw)
	if err != nil {
		return "", ledgerFile{}, false
	}
	ledgerRaw, err := os.ReadFile(ledgerPath(runDir, round))
	if err != nil {
		return "", ledgerFile{}, false
	}
	ledger, err := parseLedger(ledgerRaw)
	if err != nil {
		return "", ledgerFile{}, false
	}
	// The ledger's round field must match the round its filename encodes (ledgerPath(runDir, round)).
	// A mismatch means the judge wrote another round's claim into this round's file,
	// so the round reads as unjudged and the Bouncer judges it again.
	if ledger.Round != round {
		return "", ledgerFile{}, false
	}
	return verdict, ledger, true
}

// ledgerEntry is one finding-identity record in a ledger file: a Key the judge uses to recognize
// recurring findings, the Rounds it was seen in, and its Status.
// Class and Severity label the finding the judge mapped to the key.
// Both are empty on an unlabelled entry, which reads as non-gating.
// Fix is the judge's fix kind: `mechanical`, `design-wrong`, or empty when the judge wrote none.
type ledgerEntry struct {
	Key      string
	Rounds   []int
	Status   string
	Class    burlerengine.Class
	Severity burlerengine.Severity
	Fix      string
}

// ledgerFile is a parsed bouncer ledger file: the Round it pertains to, its Entries, and its prose narrative.
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

// ledgerEntryYAML mirrors one ledger entry's YAML shape before validation promotes it into a ledgerEntry.
type ledgerEntryYAML struct {
	Key      string `yaml:"key"`
	Rounds   []int  `yaml:"rounds"`
	Status   string `yaml:"status"`
	Class    string `yaml:"class"`
	Severity string `yaml:"severity"`
	Fix      string `yaml:"fix"`
}

// parseLedger parses a bouncer ledger file into a ledgerFile.
// Fails loud: the frontmatter must be present, closed, and valid YAML; round must be a positive integer;
// each entry of the ledger list (which may be empty) must have a non-empty key;
// its rounds must be a non-empty list of positive integers, and its status must be exactly "open" or "resolved".
// An entry's optional class and severity, when present, must be burlerengine vocabulary.
// An entry's optional fix is kept unvalidated: a missing or unknown value is legal.
// parseLedger compares nothing against any previous ledger;
// carry-forward is stated in the judge prompt and is not enforced here.
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
		if raw.Class != "" && !slices.Contains(classOrder, burlerengine.Class(raw.Class)) {
			return ledgerFile{}, fmt.Errorf("bouncer: ledger file entry %q has class %q; want one of %v", raw.Key, raw.Class, classOrder)
		}
		if raw.Severity != "" && !slices.Contains(severityOrder, burlerengine.Severity(raw.Severity)) {
			return ledgerFile{}, fmt.Errorf("bouncer: ledger file entry %q has severity %q; want one of %v", raw.Key, raw.Severity, severityOrder)
		}
		entries = append(entries, ledgerEntry{
			Key:      raw.Key,
			Rounds:   raw.Rounds,
			Status:   raw.Status,
			Class:    burlerengine.Class(raw.Class),
			Severity: burlerengine.Severity(raw.Severity),
			Fix:      raw.Fix,
		})
	}

	return ledgerFile{
		Round:   parsed.Round,
		Entries: entries,
		Prose:   frontmatterProse(content),
	}, nil
}

// readLedger reads and parses round's ledger file inside runDir, wrapping either failure with the round.
func readLedger(runDir string, round int) (ledgerFile, error) {
	raw, err := os.ReadFile(ledgerPath(runDir, round))
	if err != nil {
		return ledgerFile{}, fmt.Errorf("shedadapters: read ledger of round %d: %w", round, err)
	}
	ledger, err := parseLedger(raw)
	if err != nil {
		return ledgerFile{}, fmt.Errorf("shedadapters: parse ledger of round %d: %w", round, err)
	}
	return ledger, nil
}

// designWrongKeys returns the keys of the ledger's entries that count as design-wrong:
// open, at severity BLOCKING or MEDIUM, and tagged anything but `mechanical`, so a missing or unknown fix tag counts.
// A LOW, NIT or unlabelled-severity entry never counts.
func designWrongKeys(ledger ledgerFile) map[string]bool {
	keys := map[string]bool{}
	for _, entry := range ledger.Entries {
		if entry.Status != "open" || entry.Fix == "mechanical" {
			continue
		}
		if entry.Severity == burlerengine.SeverityBlocking || entry.Severity == burlerengine.SeverityMedium {
			keys[entry.Key] = true
		}
	}
	return keys
}

// focusFile is a parsed bouncer focus file.
// It holds the round the file targets, the lenses excluded from that round, its focus directives, and its prose.
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
// Fails loud: the frontmatter must be present, closed, and valid YAML, and round must be a positive int.
// exclude_lenses and focus must each be lists of strings, legally empty;
// a scalar where a list is required is a parse error, which yaml.Unmarshal into a []string field already produces.
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

// renderFocus marshals f into a bouncer focus file's bytes.
// The output is a YAML frontmatter header, wrapped between opening and closing "---" lines.
// The header carries round, exclude_lenses, and focus; f.Prose is appended below the closing delimiter when non-empty.
// Marshalling the header through yaml.Marshal rather than hand-formatting it is what keeps the file intact.
// A lens name containing a YAML metacharacter then round-trips instead of corrupting the file.
// Both list fields render as an explicit empty list rather than as null when f's corresponding slice is empty.
// renderFocus therefore always writes both lists;
// parseFocus does not require that, and accepts a file with either list key absent.
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
// Each line is compared with its trailing "\r" trimmed, so CRLF content parses identically to LF content.
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

// frontmatterProse returns the text after a file's closing "---" delimiter line, CRLF-normalized and trimmed.
// It returns "" when nothing follows that line, or when the file has no closing delimiter line.
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
