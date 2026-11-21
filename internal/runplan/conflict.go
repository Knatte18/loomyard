// conflict.go holds the plan re-resolution at dispatch and its refusal, the plan conflict.
// It also holds the never-tracked plan-conflict record that refusal leaves in the implementer-neutral conflict directory.
// The record has its writers, readers and removers here too.

package runplan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// ErrPlanConflict is the sentinel a *PlanConflictError unwraps to.
// It marks a dispatch refused because the plan re-resolution reported a blocking finding.
// Callers match it with errors.Is.
// Dispatching a batch on a re-resolve that failed is worse than not dispatching.
var ErrPlanConflict = errors.New("runplan: plan re-resolution at dispatch reported a blocking finding")

// The three causes a PlanConflictError carries, and the only values its Cause takes.
const (
	// ConflictCausePlanDefect is a conflict found before any batch was begun: the approved plan contradicts the tree.
	ConflictCausePlanDefect = "plan_defect"
	// ConflictCauseTreeMoved is a conflict found after a batch was begun: the tree moved under the plan.
	ConflictCauseTreeMoved = "tree_moved"
	// ConflictCausePlanOrder marks a conflict where every blocking finding is a delete a later card still references.
	ConflictCausePlanOrder = "plan_order"
)

// PlanConflictError is the refusal the dispatch re-resolution returns when it reports blocking findings.
// Batch is the batch whose dispatch was refused.
// Cause is ConflictCausePlanDefect, ConflictCauseTreeMoved or ConflictCausePlanOrder.
// Findings is every blocking finding as an error string, in the order the re-resolution reported them.
// WayForward is the implementer's way forward for Cause, which Error renders last.
type PlanConflictError struct {
	Batch      int
	Cause      string
	Findings   []string
	WayForward string
}

// Error renders the refusal with its class and cause, the findings and the way forward.
func (e *PlanConflictError) Error() string {
	return fmt.Sprintf("%s (plan_conflict: %s): %s; way forward: %s", ErrPlanConflict.Error(), e.Cause, strings.Join(e.Findings, "; "), e.WayForward)
}

// Unwrap returns ErrPlanConflict, so errors.Is identifies the refusal.
func (e *PlanConflictError) Unwrap() error { return ErrPlanConflict }

// ResolveAtDispatch re-resolves plan against the current tree, never from a cache, before batch number is dispatched.
// It returns the informational findings as advisories; the run continues past them.
// Blocking findings refuse with a *PlanConflictError whose way forward is wayForward(cause).
// An infrastructure error (errors.Is(err, planindex.ErrQuarryUnavailable)) blocks exactly like a blocking finding:
// dispatching a batch on a re-resolve that failed is worse than not dispatching.
//
// The scope is the cards still to be built:
// a card already built contradicts the tree by design;
// re-resolving it would report a working plan as a blocking defect.
// The caller records the batch after this call, so on a first dispatch this batch is still validated;
// a re-dispatch of a batch whose earlier attempt already landed its work is not refused for it.
// The forthcoming half keeps the Create targets of begun, unrecorded batches out of the status check,
// so a re-dispatched batch whose attempt landed nothing does not refuse the later cards that Use them.
// Once any batch is begun, a pending card's Delete target that is already gone is no blocking finding.
// It arrives in the advisories as delete-target-gone.
//
// The resolve pass canonicalizes handles, which rewrites the plan on disk,
// so the call re-baselines st's plan fingerprint under runDir before every refusal.
// A restamp failure never masks a resolve error: the caller is already returning for that reason.
func ResolveAtDispatch(geom Geometry, plan *planparser.Plan, batches []batcher.Batch, st *State, number int, wayForward func(cause string) string) (advisories []string, err error) {
	begun, forthcoming := DispatchScope(batches, st)
	index, err := geom.index()
	if err != nil {
		return nil, err
	}
	resolveFindings, resolveErr := index.ValidateDispatch(plan, geom.WorktreeRoot, begun, forthcoming)
	// The status and Create-inversion passes run after the resolve pass's rewrite,
	// so a rewritten plan routinely also reports a blocking finding,
	// and the rewrite also survives the pass's own hard-error paths.
	// Otherwise the state keeps the pre-rewrite fingerprint while the plan on disk carries this run's own sanctioned edit,
	// and every later dispatch refuses it as a foreign one.
	if err := RestampFingerprint(st, plan.Dir, geom.RunDir); err != nil && resolveErr == nil {
		return nil, err
	}
	if resolveErr != nil {
		return nil, resolveErr
	}
	var blocking []string
	orderOnly := true
	for _, f := range resolveFindings {
		if f.Severity == planindex.SeverityBlocking {
			blocking = append(blocking, f.Error())
			orderOnly = orderOnly && f.Check == planindex.CheckDeleteBeforeReference
		} else {
			advisories = append(advisories, f.Error())
		}
	}
	if len(blocking) == 0 {
		return advisories, nil
	}
	// begun is computed before the caller records its batch.
	// A first dispatch of batch 1 has none begun (plan_defect).
	// A re-dispatch of a recorded batch has it begun (tree_moved).
	// A set holding any other finding keeps those two causes, since moving a delete would not settle it.
	cause := ConflictCauseTreeMoved
	switch {
	case orderOnly:
		cause = ConflictCausePlanOrder
	case len(begun) == 0:
		cause = ConflictCausePlanDefect
	}
	return nil, &PlanConflictError{Batch: number, Cause: cause, Findings: blocking, WayForward: wayForward(cause)}
}

const (
	// ConflictRecordName is the file in the conflict directory a dispatch writes when it refuses with a plan conflict.
	ConflictRecordName = "plan-conflict.md"
	// routedConflictName is the file holding the findings of the conflict last routed to plan review.
	routedConflictName = "plan-conflict-routed.md"
	// HaltReasonName is the file in the conflict directory holding the stuck reason of the latest stuck Webster call.
	// A plan review the parent routes the run back to raises it as a finding; Webster removes it when a call ends done.
	HaltReasonName = "webster-halt.md"
)

// ConflictDir returns the never-tracked directory under anchorRoot that holds the plan-conflict record.
// It also holds the last-routed conflict.
// It belongs to no implementer, so a reroute to plan review finds a conflict whichever implementer refused.
func ConflictDir(anchorRoot string) string {
	return filepath.Join(anchorRoot, lymdirs.DotLymDirName, lymdirs.ImplementerDirName)
}

// ConflictRecordPath returns the conflict record's path under dir.
func ConflictRecordPath(dir string) string {
	return filepath.Join(dir, ConflictRecordName)
}

// RoutedConflictPath returns the path of the last-routed conflict file under dir.
func RoutedConflictPath(dir string) string {
	return filepath.Join(dir, routedConflictName)
}

// WriteConflictRecord writes the conflict record naming conflict's batch, cause and blocking findings.
// It creates dir first, since no lock file of either implementer sits in it.
func WriteConflictRecord(dir string, conflict *PlanConflictError) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runplan: create plan-conflict directory: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Plan conflict at begin-batch %02d\ncause: %s\n\n", conflict.Batch, conflict.Cause)
	for _, finding := range conflict.Findings {
		b.WriteString("- " + finding + "\n")
	}
	if err := os.WriteFile(ConflictRecordPath(dir), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("runplan: write plan-conflict record: %w", err)
	}
	return nil
}

// ConflictRecord is the conflict record as read back from disk.
// Batch is 0 when the header names none.
type ConflictRecord struct {
	Batch    int
	Findings []string
}

// ReadConflictRecord reads the conflict record under scratchDir.
// An absent record reads as a ConflictRecord with no findings.
func ReadConflictRecord(scratchDir string) (ConflictRecord, error) {
	recordPath := ConflictRecordPath(scratchDir)
	data, err := os.ReadFile(recordPath)
	if os.IsNotExist(err) {
		return ConflictRecord{}, nil
	}
	if err != nil {
		return ConflictRecord{}, fmt.Errorf("runplan: read %s: %w", recordPath, err)
	}
	record := ConflictRecord{Findings: findingLines(string(data))}
	for _, line := range strings.Split(string(data), "\n") {
		if number, ok := strings.CutPrefix(line, "# Plan conflict at begin-batch "); ok {
			record.Batch, _ = strconv.Atoi(number)
		}
	}
	return record, nil
}

// RemoveConflictRecord removes the conflict record, and is a no-op when it is absent.
func RemoveConflictRecord(scratchDir string) error {
	return removeIfPresent(ConflictRecordPath(scratchDir))
}

// RemoveRoutedConflict removes the last-routed conflict, and is a no-op when it is absent.
func RemoveRoutedConflict(scratchDir string) error {
	return removeIfPresent(RoutedConflictPath(scratchDir))
}

// HaltReasonPath returns the Webster halt reason's path under dir.
func HaltReasonPath(dir string) string {
	return filepath.Join(dir, HaltReasonName)
}

// WriteHaltReason writes reason, newline-terminated, as the Webster halt reason under dir.
// It creates dir first, since no lock file sits in it.
func WriteHaltReason(dir, reason string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runplan: create halt-reason directory: %w", err)
	}
	if err := os.WriteFile(HaltReasonPath(dir), []byte(reason+"\n"), 0o644); err != nil {
		return fmt.Errorf("runplan: write halt reason: %w", err)
	}
	return nil
}

// ReadHaltReason returns the trimmed Webster halt reason under dir, or "" when there is none.
func ReadHaltReason(dir string) (string, error) {
	data, err := os.ReadFile(HaltReasonPath(dir))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("runplan: read %s: %w", HaltReasonPath(dir), err)
	}
	return strings.TrimSpace(string(data)), nil
}

// RemoveHaltReason removes the Webster halt reason under dir, and is a no-op when it is absent.
func RemoveHaltReason(dir string) error {
	return removeIfPresent(HaltReasonPath(dir))
}

// removeIfPresent removes the file at filePath and returns nil when there is none.
func removeIfPresent(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("runplan: remove %s: %w", filePath, err)
	}
	return nil
}

// ConflictRoute is the outcome of claiming the conflict record for a reroute to plan review.
// Found is whether a record naming at least one finding exists.
// New is whether its findings differ from the last-routed ones,
// so a reroute to plan review has not yet been asked for them.
// Findings is the record's finding lines, set when Found.
type ConflictRoute struct {
	Found    bool
	New      bool
	Findings []string
}

// ClaimConflictRoute decides whether the conflict record asks for a new reroute to plan review.
// An absent record, or one naming no finding (a torn write), claims nothing.
// A record whose findings, compared as a set, differ from the routed file's is new:
// its findings replace the routed file's before the claim returns.
// A record whose findings equal the routed ones is a recurrence and writes nothing.
// A read or write error is returned.
func ClaimConflictRoute(scratchDir string) (ConflictRoute, error) {
	recordFindings, err := readFindingLines(ConflictRecordPath(scratchDir))
	if err != nil {
		return ConflictRoute{}, err
	}
	if len(recordFindings) == 0 {
		return ConflictRoute{}, nil
	}
	routedFindings, err := readFindingLines(RoutedConflictPath(scratchDir))
	if err != nil {
		return ConflictRoute{}, err
	}
	route := ConflictRoute{Found: true, Findings: recordFindings}
	if SameFindingSet(recordFindings, routedFindings) {
		return route, nil
	}
	var routed strings.Builder
	for _, finding := range recordFindings {
		routed.WriteString("- " + finding + "\n")
	}
	if err := os.WriteFile(RoutedConflictPath(scratchDir), []byte(routed.String()), 0o644); err != nil {
		return ConflictRoute{}, fmt.Errorf("runplan: write routed plan-conflict file: %w", err)
	}
	route.New = true
	return route, nil
}

// readFindingLines returns the text after "- " of every finding line in the file at filePath.
// It returns nil when the file is absent.
func readFindingLines(filePath string) ([]string, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runplan: read %s: %w", filePath, err)
	}
	return findingLines(string(data)), nil
}

// findingLines returns the text after "- " of every finding line in a conflict record's or routed file's text.
func findingLines(text string) []string {
	var findings []string
	for _, line := range strings.Split(text, "\n") {
		if finding, ok := strings.CutPrefix(line, "- "); ok {
			findings = append(findings, finding)
		}
	}
	return findings
}

// SameFindingSet reports whether a and b hold the same conflict findings, order ignored.
func SameFindingSet(a, b []string) bool {
	sortedA, sortedB := slices.Clone(a), slices.Clone(b)
	slices.Sort(sortedA)
	slices.Sort(sortedB)
	return slices.Equal(sortedA, sortedB)
}
