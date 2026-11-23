// bouncerverify.go checks the open BLOCKING ledger entries at a spent budget.
// It covers only entries whose findings the round's fixer reported fixed.
// A short verifier pass checks only those entries against the fix commits;
// each entry it closes is marked resolved in the round's ledger, so the run's parent hears only of the entries left open.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/editdirective"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// bouncerVerifyRole is the base of the shuttleengine.Spec.Role every verifier pass carries.
// The spawn and its attach probe name the same role, so a logged attach stays attributable.
const bouncerVerifyRole = "bouncer-verify"

// verifyStencil is the stencilstore name of the verifier pass's prompt template.
const verifyStencil = "bouncer-template-verify"

// The two legal statuses of a verification entry.
const (
	verificationClosed = "closed"
	verificationOpen   = "open"
)

// verificationPath returns the path of the verifier's output file for round inside runDir.
func verificationPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-verification.md", round))
}

// verificationHeader mirrors a verification file's YAML frontmatter shape.
type verificationHeader struct {
	Round   int                     `yaml:"round"`
	Entries []verificationEntryYAML `yaml:"entries"`
}

// verificationEntryYAML is the verifier's ruling on one ledger key.
// Finding is the id of the review finding the key maps to.
type verificationEntryYAML struct {
	Key     string `yaml:"key"`
	Status  string `yaml:"status"`
	Finding string `yaml:"finding"`
	Reason  string `yaml:"reason"`
}

// parseVerification returns the keys a verification file for round closes, each mapped to the review finding id it names.
// It refuses a file with no frontmatter, invalid YAML, another round, an entry with no key, or a key listed twice.
// It also refuses a status other than closed or open, and a closed entry that names no finding.
func parseVerification(content []byte, round int) (map[string]string, error) {
	header, err := splitFrontmatter(content, "verification")
	if err != nil {
		return nil, err
	}
	var parsed verificationHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return nil, fmt.Errorf("bouncer: verification file frontmatter is not valid YAML: %w", err)
	}
	if parsed.Round != round {
		return nil, fmt.Errorf("bouncer: verification file names round %d; want %d", parsed.Round, round)
	}
	closed := map[string]string{}
	seen := map[string]bool{}
	for _, entry := range parsed.Entries {
		if strings.TrimSpace(entry.Key) == "" {
			return nil, errors.New("bouncer: verification file has an entry with an empty key")
		}
		if seen[entry.Key] {
			return nil, fmt.Errorf("bouncer: verification file lists key %q more than once", entry.Key)
		}
		seen[entry.Key] = true
		switch entry.Status {
		case verificationClosed:
			finding := strings.TrimSpace(entry.Finding)
			if finding == "" {
				return nil, fmt.Errorf("bouncer: verification file entry %q is closed and names no finding", entry.Key)
			}
			closed[entry.Key] = finding
		case verificationOpen:
		default:
			return nil, fmt.Errorf("bouncer: verification file entry %q has status %q; want exactly %q or %q", entry.Key, entry.Status, verificationClosed, verificationOpen)
		}
	}
	return closed, nil
}

// closeVerifiedBlocking checks keys, round's open BLOCKING ledger keys, against the fix commits.
// It returns the keys still open.
// It checks only a round with no escalation record.
// The round's fixer must have reported a BLOCKING finding fixed or fixed differently.
// Its fix must have changed the review target when the round's snapshot record says.
// A fixer report the fixer wrote exists only after its gate passed, since a failed gate archives the round.
// A verification already on disk is used as it stands; otherwise one verifier pass writes it.
// A key is closed only when the verification closes it and names a finding the fixer reported fixed.
// Each closed key is marked resolved in the ledger.
// note names why nothing was checked when a step failed, and is "" otherwise;
// a failure leaves every key open, so the parent is asked.
func (b *Bouncer) closeVerifiedBlocking(round int, keys []string) (open []string, note string) {
	if _, _, escalated, err := readEscalation(b.cfg.RunDir, round); err != nil || escalated {
		return keys, ""
	}
	fixed := b.fixedBlockingFindings(round)
	if len(fixed) == 0 || b.fixLeftTargetUnchanged(round) {
		return keys, ""
	}
	closed, err := b.verification(round, keys, fixed)
	if err != nil {
		logger.Warn("shedadapters: bouncer could not verify the fixed BLOCKING findings; the parent is asked", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return keys, fmt.Sprintf("the check of the findings the fixer reported fixed failed: %v", err)
	}
	fixedIDs := make(map[string]bool, len(fixed))
	for _, finding := range fixed {
		fixedIDs[finding.ID] = true
	}
	confirmed := map[string]bool{}
	for _, key := range keys {
		if findingID, ok := closed[key]; ok && fixedIDs[findingID] {
			confirmed[key] = true
		} else {
			open = append(open, key)
		}
	}
	if len(confirmed) == 0 {
		return keys, ""
	}
	if err := markLedgerResolved(b.cfg.RunDir, round, confirmed); err != nil {
		logger.Warn("shedadapters: bouncer could not close the verified BLOCKING entries in the ledger; the parent is asked", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "cause", err)
		return keys, fmt.Sprintf("the verified findings could not be closed in the ledger: %v", err)
	}
	logger.Info("shedadapters: bouncer closed BLOCKING entries the verifier confirmed fixed", "producer", b.cfg.Name, "engine", bouncerEngineLabel, "round", round, "open", open)
	return open, ""
}

// fixedBlockingFindings returns the BLOCKING findings of round's review.
// Only those its fixer reported fixed or fixed differently count.
// A missing or unparseable review or fixer report, and a report the engine wrote for a fixer it never spawned, give none.
func (b *Bouncer) fixedBlockingFindings(round int) []burlerengine.Finding {
	content, err := os.ReadFile(roundFixerReportPath(b.cfg.RunDir, round))
	if err != nil {
		return nil
	}
	report, err := burlerengine.ParseFixerReport(content)
	if err != nil || report.Spawned != nil {
		return nil
	}
	findings, err := b.reviewFindings(round)
	if err != nil {
		return nil
	}
	disposition := make(map[string]burlerengine.Disposition, len(report.Dispositions))
	for _, entry := range report.Dispositions {
		disposition[entry.ID] = entry.Disposition
	}
	var fixed []burlerengine.Finding
	for _, finding := range findings {
		switch disposition[finding.ID] {
		case burlerengine.DispositionFixed, burlerengine.DispositionFixedDifferently:
			if finding.Severity == burlerengine.SeverityBlocking {
				fixed = append(fixed, finding)
			}
		}
	}
	return fixed
}

// fixLeftTargetUnchanged reports whether round's snapshot record holds equal target ids from before and after the fix.
// A record that is missing, unreadable or lacks either id does not say, and reports false.
func (b *Bouncer) fixLeftTargetUnchanged(round int) bool {
	record, found, err := readRoundSnapshot(b.cfg.RunDir, round)
	return err == nil && found && record.TargetID != "" && record.TargetID == record.PostFixTargetID
}

// verification returns the keys round's verification closes, each mapped to its finding id.
// It runs one verifier pass when no readable verification exists.
// keys and fixed fill the pass's prompt.
func (b *Bouncer) verification(round int, keys []string, fixed []burlerengine.Finding) (map[string]string, error) {
	path := verificationPath(b.cfg.RunDir, round)
	if content, err := os.ReadFile(path); err == nil {
		if closed, err := parseVerification(content, round); err == nil {
			return closed, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := b.runVerifier(round, keys, fixed); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("the verifier wrote no verification: %w", err)
	}
	closed, err := parseVerification(content, round)
	if err != nil {
		return nil, fmt.Errorf("the verifier's file %s: %w", path, err)
	}
	return closed, nil
}

// runVerifier runs one verifier pass for round on the judge's model, which writes round's verification file.
// It attaches to a live verifier when one holds that file; otherwise it archives a stale file and spawns one.
// A render, probe, archive or run failure is an error.
func (b *Bouncer) runVerifier(round int, keys []string, fixed []burlerengine.Finding) error {
	engine := b.cfg.Shuttle.EngineName(b.cfg.Models.Judge)
	prompt, err := b.RenderVerifyPrompt(engine, round, keys, fixed)
	if err != nil {
		return err
	}
	spec := shuttleengine.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		OutputFiles:  []string{verificationPath(b.cfg.RunDir, round)},
		ModelChoice:  b.cfg.Models.Judge,
		Role:         b.role(bouncerVerifyRole),
		Segment:      b.colorSegment(),
		Round:        strconv.Itoa(round),
		ConfigRole:   agentconfig.RoleBouncerJudge,
	}
	result, attached, err := b.cfg.Shuttle.Attach(spec)
	if err != nil {
		return fmt.Errorf("probe for a live verifier: %w", err)
	}
	if !attached {
		if err := archiveStaleOutputs(spec.OutputFiles, b.cfg.Now); err != nil {
			return fmt.Errorf("archive a stale verification: %w", err)
		}
		result, err = b.cfg.Shuttle.Run(spec)
		if err != nil {
			return fmt.Errorf("run the verifier: %w", err)
		}
	}
	b.recordPass(round, false, result)
	return nil
}

// RenderVerifyPrompt fills the verifier pass's prompt for round for the named engine.
// keys are the open BLOCKING ledger keys it rules on, and fixed the BLOCKING findings the fixer reported fixed.
// The edit directive is the engine's own copy.
// It fails when the template or a directive cannot be read or filled.
func (b *Bouncer) RenderVerifyPrompt(engine string, round int, keys []string, fixed []burlerengine.Finding) (string, error) {
	template, err := stencilstore.Read(b.cfg.StencilsDir, verifyStencil)
	if err != nil {
		return "", fmt.Errorf("read the verify template: %w", err)
	}
	parentDirective, err := parentdirective.Directive(b.cfg.StencilsDir, b.cfg.ParentName, false)
	if err != nil {
		return "", fmt.Errorf("read the parent directive: %w", err)
	}
	editDirective, err := editdirective.Directive(b.cfg.StencilsDir, engine)
	if err != nil {
		return "", fmt.Errorf("read the edit directive: %w", err)
	}
	keyLines := make([]string, len(keys))
	for i, key := range keys {
		keyLines[i] = fmt.Sprintf("- `%s`", key)
	}
	findingLines := make([]string, len(fixed))
	for i, finding := range fixed {
		findingLines[i] = findingLine(finding)
	}
	prompt, err := stencil.Fill(template, map[string]string{
		"round":                    strconv.Itoa(round),
		"review_path":              filepath.Join(b.cfg.RunDir, b.cfg.ReportName(round)),
		"fixer_report_path":        roundFixerReportPath(b.cfg.RunDir, round),
		"ledger_path":              ledgerPath(b.cfg.RunDir, round),
		"ledger_entries":           strings.Join(keyLines, "\n"),
		"fixed_findings":           strings.Join(findingLines, "\n"),
		"verification_path":        verificationPath(b.cfg.RunDir, round),
		"worktree":                 b.cfg.WorktreeRoot,
		parentdirective.MarkerName: parentDirective,
		editdirective.MarkerName:   editDirective,
	})
	if err != nil {
		return "", fmt.Errorf("fill the verify template: %w", err)
	}
	return string(prompt), nil
}

// markLedgerResolved rewrites round's ledger with each open entry whose key closed holds marked resolved.
// Every other entry, the round and the prose stay as they were.
// The rewrite goes to a temporary sibling that is renamed over the ledger, so a failed write leaves the ledger intact.
func markLedgerResolved(runDir string, round int, closed map[string]bool) error {
	ledger, err := readLedger(runDir, round)
	if err != nil {
		return err
	}
	for i, entry := range ledger.Entries {
		if entry.Status == "open" && closed[entry.Key] {
			ledger.Entries[i].Status = "resolved"
		}
	}
	content, err := renderLedger(ledger)
	if err != nil {
		return err
	}
	path := ledgerPath(runDir, round)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		return fmt.Errorf("write ledger of round %d: %w", round, err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace ledger of round %d: %w", round, err)
	}
	return nil
}
