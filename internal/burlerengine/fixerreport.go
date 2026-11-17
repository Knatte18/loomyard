// fixerreport.go defines the fixer-report's frontmatter contract: one disposition per finding the fixer owns.
// It holds the sole parser of that frontmatter, the check of a report against the accepted review,
// and the pass-on-cap gate entry that lets the fixer repair a report that fails the check.

package burlerengine

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Disposition is what a fixer did with one finding.
type Disposition string

// The five legal Disposition values.
const (
	// DispositionFixed: the finding was fixed as its fix key proposed, or its fix key left the change open.
	DispositionFixed Disposition = "fixed"
	// DispositionFixedDifferently: the premise holds and the fixer resolved it with a change of its own.
	DispositionFixedDifferently Disposition = "fixed-differently"
	// DispositionDisputed: the fixer rebuts the finding on one of the two dispute grounds.
	DispositionDisputed Disposition = "disputed"
	// DispositionDeferred: the fixer could not act on the finding alone this round.
	DispositionDeferred Disposition = "deferred"
	// DispositionRuling: the fixer acted on a parent's ruling on a decision-class finding.
	DispositionRuling Disposition = "ruling"
)

// The two legal dispute grounds of a disputed finding.
const (
	disputeGroundPremiseFalse  = "premise false"
	disputeGroundEveryFixWorse = "every fix worse"
)

// fixerReportGateName labels the fixer-report check in a gate outcome.
const fixerReportGateName = "fixer-report"

// fixerReportGateAttempts is the re-prompt budget of the fixer-report gate entry.
const fixerReportGateAttempts = 3

// fixerReportWayForward opens every refusal's closing clause; each refusal then names what to add or remove.
const fixerReportWayForward = "way forward: rewrite the frontmatter of the fixer-report"

// FindingDisposition is one frontmatter entry of a fixer-report: the fixer's disposition of one review finding.
// Ground, Rebuttal and Evidence belong to a disputed entry, and Reason to a fixed-differently, deferred or ruling one.
type FindingDisposition struct {
	ID          string      `yaml:"id"`
	Disposition Disposition `yaml:"disposition"`
	Ground      string      `yaml:"ground"`
	Rebuttal    string      `yaml:"rebuttal"`
	Evidence    []string    `yaml:"evidence"`
	Reason      string      `yaml:"reason"`
}

// FixerReport is the parsed frontmatter of a fixer-report.
// Spawned is a pointer so a present `spawned` key is told apart from an absent one:
// only the engine writes it, on the report it writes itself when no fixer is spawned.
type FixerReport struct {
	Dispositions []FindingDisposition `yaml:"dispositions"`
	Spawned      *bool                `yaml:"spawned"`
}

// ParseFixerReport parses the frontmatter of a fixer-report.
// It refuses a report with no frontmatter, an unclosed one or invalid YAML.
// It also refuses a top-level key other than `dispositions` and `spawned`.
// It refuses a disposition outside the five legal values too.
// Each refusal ends with the way forward.
func ParseFixerReport(content []byte) (FixerReport, error) {
	header, err := fixerReportHeader(content)
	if err != nil {
		return FixerReport{}, err
	}
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal([]byte(header), &keys); err != nil {
		return FixerReport{}, fmt.Errorf("burler: fixer-report frontmatter is not valid YAML: %w; %s so it is valid YAML", err, fixerReportWayForward)
	}
	var stray []string
	for key := range keys {
		if key != "dispositions" && key != "spawned" {
			stray = append(stray, key)
		}
	}
	if len(stray) > 0 {
		sort.Strings(stray)
		return FixerReport{}, fmt.Errorf("burler: fixer-report frontmatter has the top-level key %q; its only keys are `dispositions` and `spawned`; "+
			"way forward: rewrite the frontmatter of the fixer-report with only the dispositions and spawned keys", stray[0])
	}
	var report FixerReport
	if err := yaml.Unmarshal([]byte(header), &report); err != nil {
		return FixerReport{}, fmt.Errorf("burler: fixer-report frontmatter is not valid YAML: %w; %s so it is valid YAML", err, fixerReportWayForward)
	}
	for _, entry := range report.Dispositions {
		switch entry.Disposition {
		case DispositionFixed, DispositionFixedDifferently, DispositionDisputed, DispositionDeferred, DispositionRuling:
		default:
			return FixerReport{}, fmt.Errorf("burler: fixer-report entry %q has the disposition %q, which is none of fixed, fixed-differently, disputed, deferred and ruling; "+
				"%s with one of those five for it", entry.ID, entry.Disposition, fixerReportWayForward)
		}
	}
	return report, nil
}

// fixerReportHeader returns the YAML text between the report's opening and closing "---" delimiter lines.
func fixerReportHeader(content []byte) (string, error) {
	lines := strings.Split(string(content), "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("burler: fixer-report must open with a \"---\" frontmatter delimiter line; %s by opening the file with one", fixerReportWayForward)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return strings.Join(lines[1:i], "\n"), nil
		}
	}
	return "", fmt.Errorf("burler: fixer-report frontmatter is missing its closing \"---\" delimiter line; %s by adding the closing line", fixerReportWayForward)
}

// CheckFixerReport judges a fixer-written report against the findings of the accepted review.
// With decisionsRouted, a decision-class finding is the parent's and needs no entry.
// It refuses the first of these it finds, with the way forward:
// a `spawned` key; an entry for an id the review lacks or a listed twice; a finding with no entry;
// a disputed entry without a legal ground, a rebuttal or `<path>:<line>` evidence;
// a fixed-differently, deferred or ruling entry without a reason;
// and a ruling on a finding that is not decision-class or on a round that routes no decisions.
func CheckFixerReport(report FixerReport, findings []Finding, decisionsRouted bool) error {
	if report.Spawned != nil {
		return fmt.Errorf("burler: fixer-report carries a `spawned` key, which only the engine writes; %s by removing the `spawned` key", fixerReportWayForward)
	}
	classOf := make(map[string]Class, len(findings))
	for _, f := range findings {
		classOf[f.ID] = f.Class
	}
	listed := make(map[string]bool, len(report.Dispositions))
	for _, entry := range report.Dispositions {
		class, known := classOf[entry.ID]
		if !known {
			return fmt.Errorf("burler: fixer-report has an entry for %q, which the review does not list; %s by removing the entry for %q", entry.ID, fixerReportWayForward, entry.ID)
		}
		if listed[entry.ID] {
			return fmt.Errorf("burler: fixer-report lists %q twice; %s by keeping one entry for %q", entry.ID, fixerReportWayForward, entry.ID)
		}
		listed[entry.ID] = true
		if err := checkDispositionKeys(entry, class, decisionsRouted); err != nil {
			return err
		}
	}
	for _, f := range findings {
		if listed[f.ID] || (decisionsRouted && f.Class == ClassDecision) {
			continue
		}
		return fmt.Errorf("burler: fixer-report has no entry for finding %q; %s by adding an entry for %q", f.ID, fixerReportWayForward, f.ID)
	}
	return nil
}

// checkDispositionKeys refuses an entry that lacks the keys its disposition needs.
func checkDispositionKeys(entry FindingDisposition, class Class, decisionsRouted bool) error {
	switch entry.Disposition {
	case DispositionDisputed:
		if entry.Ground != disputeGroundPremiseFalse && entry.Ground != disputeGroundEveryFixWorse {
			return fmt.Errorf("burler: fixer-report entry %q is disputed with the ground %q; %s by setting `ground` to `premise false` or `every fix worse` for %q",
				entry.ID, entry.Ground, fixerReportWayForward, entry.ID)
		}
		if strings.TrimSpace(entry.Rebuttal) == "" {
			return fmt.Errorf("burler: fixer-report entry %q is disputed with no rebuttal; %s by adding a non-empty `rebuttal` to %q", entry.ID, fixerReportWayForward, entry.ID)
		}
		if len(entry.Evidence) == 0 {
			return fmt.Errorf("burler: fixer-report entry %q is disputed with no evidence; %s by adding `evidence` items of the form <path>:<line> to %q", entry.ID, fixerReportWayForward, entry.ID)
		}
		for _, item := range entry.Evidence {
			if !isPathLine(item) {
				return fmt.Errorf("burler: fixer-report entry %q has the evidence item %q, which is not of the form <path>:<line>; %s by writing every item of %q as <path>:<line>",
					entry.ID, item, fixerReportWayForward, entry.ID)
			}
		}
	case DispositionFixedDifferently, DispositionDeferred, DispositionRuling:
		if strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("burler: fixer-report entry %q is %s with no reason; %s by adding a non-empty `reason` to %q", entry.ID, entry.Disposition, fixerReportWayForward, entry.ID)
		}
		if entry.Disposition == DispositionRuling && (class != ClassDecision || !decisionsRouted) {
			return fmt.Errorf("burler: fixer-report entry %q is a ruling, which only a routed decision-class finding takes; %s by giving %q another disposition", entry.ID, fixerReportWayForward, entry.ID)
		}
	}
	return nil
}

// isPathLine reports whether item is a non-empty path, a colon and a positive decimal line number.
func isPathLine(item string) bool {
	colon := strings.LastIndex(item, ":")
	if colon <= 0 {
		return false
	}
	line := item[colon+1:]
	for _, digit := range line {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	number, err := strconv.Atoi(line)
	return err == nil && number > 0
}

// FixerReportGateEntry returns the gate entry that re-prompts a fixer to repair its report at reportPath.
// The gate reads the review at reviewPath through ParseCurrentReview and fails with the parse or check error.
// It lets the run through once its re-prompt budget is spent, so a slip in the frontmatter never fails the round.
func FixerReportGateEntry(reportPath, reviewPath string, decisionsRouted bool) shuttleengine.GateEntry {
	return shuttleengine.GateEntry{
		Name:      fixerReportGateName,
		Gate:      fixerReportGate(reportPath, reviewPath, decisionsRouted),
		Attempts:  fixerReportGateAttempts,
		PassOnCap: true,
	}
}

// fixerReportGate returns the closure of the fixer-report entry; see FixerReportGateEntry.
// A missing or refused report, or a review that no longer parses, is a failed result, never a gate error.
func fixerReportGate(reportPath, reviewPath string, decisionsRouted bool) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		if err := checkFixerReportFile(reportPath, reviewPath, decisionsRouted); err != nil {
			return shuttleengine.GateResult{Findings: fmt.Sprintf("%s\n\nRewrite the frontmatter of the fixer-report (%s) so it passes.", err, reportPath)}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}
}

// checkFixerReportFile reads the review and the report and returns the first refusal of either, or nil.
func checkFixerReportFile(reportPath, reviewPath string, decisionsRouted bool) error {
	reviewContent, err := os.ReadFile(reviewPath)
	if err != nil {
		return fmt.Errorf("burler: read review file %q: %w", reviewPath, err)
	}
	_, findings, err := ParseCurrentReview(reviewContent)
	if err != nil {
		return err
	}
	reportContent, err := os.ReadFile(reportPath)
	if err != nil {
		return fmt.Errorf("burler: read fixer-report %q: %w", reportPath, err)
	}
	report, err := ParseFixerReport(reportContent)
	if err != nil {
		return err
	}
	return CheckFixerReport(report, findings, decisionsRouted)
}
