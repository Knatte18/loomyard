// partcheck.go holds the checks of a split plan.
// It has the parts gate, the part check with its gate and writer Accept, and the Go half of the join.
// The part check is the format check over one part's plan directory, carrying a planparser.PartScope.

package loomshed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrPartUnknown is wrapped by the error ValidatePart returns for a part name the parts file does not hold.
var ErrPartUnknown = errors.New("loomshed: no such part")

// partExportMissingCheck names the finding ValidatePart appends for an export of a part's contract that no card of the part meets.
const partExportMissingCheck = "part-export-missing"

// NewPartsGate returns the Parts-Write row's `parts` gate closure.
// It runs partsparser.Validate over the parts file under anchorPath, passing worktreeRoot, recordPath and max on.
// It maps no finding to a pass and findings to a failed gate carrying one `check[/part]: detail` line each.
// A returned error is the gate's error.
func NewPartsGate(anchorPath, worktreeRoot, recordPath string, max int) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		findings, err := partsparser.Validate(anchorPath, worktreeRoot, recordPath, max)
		if err != nil {
			return shuttleengine.GateResult{}, err
		}
		if len(findings) == 0 {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		lines := make([]string, len(findings))
		for i, finding := range findings {
			label := finding.Check
			if finding.Part != "" {
				label += "/" + finding.Part
			}
			lines[i] = label + ": " + finding.Detail
		}
		return shuttleengine.GateResult{Passed: false, Findings: strings.Join(lines, "\n")}, nil
	}
}

// readParts reads and parses the parts file under anchorPath.
// An absent or malformed file is an error that sends the reader to the parent.
// Its way forward is to re-run Parts-Write.
func readParts(anchorPath string) (*partsparser.Parts, error) {
	path := partsparser.File(anchorPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("the parts file %s is gone; a part writer or the chair reports it to the parent; way forward: the parent runs lym loom goto --to Parts-Write", path)
		}
		return nil, fmt.Errorf("read the parts file %s: %w", path, err)
	}
	parts, err := partsparser.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("the parts file %s is malformed: %w; a part writer or the chair reports it to the parent; way forward: run lym loom validate-parts to see the check, and the parent runs lym loom goto --to Parts-Write", path, err)
	}
	return parts, nil
}

// partContract returns the parts file's contract for the part named part, or an error wrapping ErrPartUnknown that names the parts the file holds.
func partContract(parts *partsparser.Parts, part string) (partsparser.Part, error) {
	contract, ok := parts.Part(part)
	if ok {
		return contract, nil
	}
	names := make([]string, len(parts.Parts))
	for i, held := range parts.Parts {
		names[i] = held.Name
	}
	return partsparser.Part{}, fmt.Errorf("%w: %q; the parts file holds %s; way forward: pass one of those names", ErrPartUnknown, part, strings.Join(names, ", "))
}

// partScope returns the part scope for the part named part.
// It holds the exports that other parts and shared spots give the part, as `plan:` handles and new file paths.
// It also holds the paths of every shared spot.
func partScope(parts *partsparser.Parts, part string) *planparser.PartScope {
	scope := &planparser.PartScope{Name: part, SpotPaths: parts.SpotPaths()}
	for _, ref := range parts.External(part) {
		if planparser.IsHandleRef(ref) {
			scope.Handles = append(scope.Handles, ref)
		} else {
			scope.Paths = append(scope.Paths, ref)
		}
	}
	return scope
}

// checkPartPlan returns the findings of one part's parsed plan.
// They are the index's format findings under the part scope;
// the context checks without the decision record;
// and one blocking part-export-missing per contract export that no card creates.
// The decision record is left out:
// pattern-changes-mismatch and pattern-line-unprescribed rest on the joined overview, which the part lacks;
// the joined plan's gate runs them.
func checkPartPlan(plan *planparser.Plan, parts *partsparser.Parts, contract partsparser.Part, worktreeRoot string, index planindex.Index, pc PlanContext) ([]planindex.Finding, error) {
	plan.Part = partScope(parts, contract.Name)
	findings, err := index.ValidateFormat(plan, worktreeRoot, nil)
	if err != nil {
		return nil, err
	}
	pc.DecisionRecordPath = ""
	contextFindings, err := planContextFindings(plan, plan.Cards, worktreeRoot, index, pc)
	if err != nil {
		return nil, err
	}
	findings = append(findings, contextFindings...)
	for _, export := range contract.Exports {
		if planparser.ExportMet(plan, export) {
			continue
		}
		findings = append(findings, planindex.Finding{
			Check:    partExportMissingCheck,
			Detail:   fmt.Sprintf("part %q's contract exports %q, and no card of the part creates it; way forward: create the export on a card of the part, declaring a `plan:` export on a **Create:** bullet with the parts file's spelling; when the part's area cannot meet it, message the chair naming the contract line", contract.Name, export),
			Severity: planindex.SeverityBlocking,
		})
	}
	return findings, nil
}

// ValidatePart runs the part check for `lym loom validate-plan --part`, the part plan gate and a writer's Accept.
// It reads the part's contract from the parts file under anchorPath.
// It parses the part's plan directory with a part scope.
// It returns the findings of three checks:
// the format check over worktreeRoot;
// the context checks without the decision record;
// and a blocking part-export-missing finding for each contract export that no card creates.
// An absent or malformed parts file is an error, and so is a part the file does not hold, which wraps ErrPartUnknown.
// A part directory that does not parse as a plan is an error too.
// Each error names its way forward.
func ValidatePart(anchorPath, worktreeRoot, part string, index planindex.Index, pc PlanContext) ([]planindex.Finding, error) {
	parts, err := readParts(anchorPath)
	if err != nil {
		return nil, err
	}
	contract, err := partContract(parts, part)
	if err != nil {
		return nil, err
	}
	dir := partsparser.PartDir(anchorPath, part)
	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		return nil, fmt.Errorf("part %q: its plan in %s does not parse: %w; way forward: fix the part's plan files", part, dir, err)
	}
	return checkPartPlan(plan, parts, contract, worktreeRoot, index, pc)
}

// NewPartPlanGate returns the `plan-part` gate closure of a part writer's seat.
// The gate checks the part's plan directory and reports the findings as its result.
// A part directory that cannot be parsed is a finding the writer can fix.
// An index error stays a returned error.
func NewPartPlanGate(anchorPath, worktreeRoot, scratchDir, part string, index planindex.Index, pc PlanContext) shuttleengine.Gate {
	return planGate("Plan-Part-Gate", partsparser.PartDir(anchorPath, part), scratchDir, func(plan *planparser.Plan) ([]planindex.Finding, error) {
		parts, err := readParts(anchorPath)
		if err != nil {
			return nil, err
		}
		contract, err := partContract(parts, part)
		if err != nil {
			return nil, err
		}
		return checkPartPlan(plan, parts, contract, worktreeRoot, index, pc)
	})
}

// NewPartAccept returns the Accept hook of a part writer's seat.
// It runs ValidatePart and returns nil when no blocking finding remains, else an error carrying the formatted findings.
// The result it is handed is never read, so a resumed seat's zero result decides nothing.
func NewPartAccept(anchorPath, worktreeRoot, part string, index planindex.Index, pc PlanContext) func(shuttleengine.Result) error {
	return func(shuttleengine.Result) error {
		findings, err := ValidatePart(anchorPath, worktreeRoot, part, index, pc)
		if err != nil {
			return err
		}
		if hasBlockingFinding(findings) {
			return fmt.Errorf("part %q has blocking findings: %s", part, formatPlanFindings(findings))
		}
		return nil
	}
}

// JoinParts is the Go half of `lym loom join-parts`:
// it re-checks every part and joins the parts' plans into the plan under anchorPath.
// It refuses an absent or malformed parts file as ValidatePart does.
// It refuses a part whose overview is absent.
// It refuses the first part in file order with a blocking finding, carrying the findings.
// Each refusal names the part to fix and the way forward.
// The chair has that part's writer fix it, or fixes it itself when the writer no longer answers;
// then it runs `lym loom join-parts` again.
// Otherwise it returns the result or the refusal of planparser.JoinParts, which receives the parts in file order.
func JoinParts(anchorPath, worktreeRoot string, index planindex.Index, pc PlanContext) (*planparser.JoinResult, error) {
	parts, err := readParts(anchorPath)
	if err != nil {
		return nil, err
	}
	for _, part := range parts.Parts {
		overview := partsparser.PartOverview(anchorPath, part.Name)
		if _, err := os.Stat(overview); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("part %q has no plan overview at %s; way forward: have part %q's writer write its plan, or write it yourself when the writer no longer answers, then run lym loom join-parts again", part.Name, overview, part.Name)
			}
			return nil, fmt.Errorf("read part %q's overview %s: %w", part.Name, overview, err)
		}
	}
	joinParts := make([]planparser.JoinPart, len(parts.Parts))
	for i, part := range parts.Parts {
		findings, err := ValidatePart(anchorPath, worktreeRoot, part.Name, index, pc)
		if err != nil {
			return nil, err
		}
		if hasBlockingFinding(findings) {
			return nil, fmt.Errorf("part %q fails its check: %s; way forward: have part %q's writer fix it, or fix it yourself when the writer no longer answers, then run lym loom join-parts again", part.Name, formatPlanFindings(findings), part.Name)
		}
		joinParts[i] = planparser.JoinPart{Name: part.Name, Dir: partsparser.PartDir(anchorPath, part.Name), Exports: part.Exports}
	}
	return planparser.JoinParts(planparser.PlanDir(anchorPath), parts.Title, joinParts)
}
