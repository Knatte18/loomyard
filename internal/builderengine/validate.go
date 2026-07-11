// validate.go implements Validate, the six plan-format v1 machine checks
// (docs/modules/plan-format.md's "Validation checks" section): format/
// approval, Batch Index <-> file consistency, verify: presence, chain-end
// soundness, the oversized-batch context/card-count cap, and scope
// well-formedness. Validate runs both as the standalone `lyx builder
// validate` verb and as builder's hard automatic gate inside `run` and
// `spawn-batch` — the fail-loud-refusal half of plan-format.md's contract.

package builderengine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// recognizedFormat is the only plan-format version ParsePlan/Validate
// currently understand; a plan declaring any other format: value fails the
// format-unrecognized check.
const recognizedFormat = 1

// ValidateCaps carries the two operator-configured cap values Validate's
// batch-oversized check (5) compares each batch's estimate against.
// builderengine itself stays config-free (config wiring is a later
// module's job) — callers resolve these from builder.yaml and pass them in.
type ValidateCaps struct {
	// ContextCapTokens is the maximum estimated context size (bytes of
	// referenced Scope+Where files, divided by 4) a non-oversized batch may
	// claim before batch-oversized fires.
	ContextCapTokens int

	// CardCap is the maximum CardCount a non-oversized batch may claim
	// before batch-oversized fires.
	CardCap int
}

// ValidationError is one finding from Validate: which check tripped
// (Check, a stable kebab-case name matching plan-format.md's check names),
// which batch it concerns (Batch, empty for a plan-level finding), and a
// human-readable Detail naming the specific problem.
type ValidationError struct {
	Check  string
	Batch  string
	Detail string
}

// Error implements the error interface so a ValidationError can be used
// anywhere a single error is expected (e.g. in a test's error-substring
// assertion), formatted as "check[/batch]: detail".
func (v ValidationError) Error() string {
	if v.Batch == "" {
		return fmt.Sprintf("%s: %s", v.Check, v.Detail)
	}
	return fmt.Sprintf("%s/%s: %s", v.Check, v.Batch, v.Detail)
}

// batchID returns the stable "NN-<slug>" identifier Validate uses to name a
// batch in a ValidationError, matching the batch-report filename stem
// plan-format.md pins (NN-<batch-slug>.yaml).
func batchID(b PlanBatch) string {
	return fmt.Sprintf("%02d-%s", b.Number, b.Slug)
}

// Validate runs every plan-format v1 machine check against plan and returns
// every finding, ordered deterministically by check number and then by
// batch number within a check. worktreeRoot is the base Validate resolves
// each batch's Scope and WhereFiles entries against for the batch-oversized
// context estimate (check 5); caps supplies that check's two cap values. A
// nil/empty return means the plan passes every check.
func Validate(plan *Plan, worktreeRoot string, caps ValidateCaps) []ValidationError {
	var findings []ValidationError

	findings = append(findings, checkFormatAndApproval(plan)...)
	findings = append(findings, checkIndexFileConsistency(plan)...)
	findings = append(findings, checkVerifyPresence(plan)...)
	findings = append(findings, checkChainEndSoundness(plan)...)
	findings = append(findings, checkBatchOversized(plan, worktreeRoot, caps)...)
	findings = append(findings, checkScopeMalformed(plan)...)

	return findings
}

// checkFormatAndApproval implements check 1: format: must be the one
// recognized version, and approved: must be true.
func checkFormatAndApproval(plan *Plan) []ValidationError {
	var findings []ValidationError

	if plan.Format != recognizedFormat {
		findings = append(findings, ValidationError{
			Check:  "format-unrecognized",
			Detail: fmt.Sprintf("format %d is not recognized; only format %d is known", plan.Format, recognizedFormat),
		})
	}
	if !plan.Approved {
		findings = append(findings, ValidationError{
			Check:  "plan-unapproved",
			Detail: "plan frontmatter approved: is not true",
		})
	}

	return findings
}

// checkIndexFileConsistency implements check 2: every index entry must
// name a batch file that exists on disk, every *.md file on disk (other
// than the overview) must be named by some index entry, and the batch
// numbers must run 1..N with no gaps or duplicates.
func checkIndexFileConsistency(plan *Plan) []ValidationError {
	var findings []ValidationError

	indexed := make(map[string]bool, len(plan.Batches))
	for _, b := range plan.Batches {
		indexed[b.File] = true
		if _, err := os.Stat(filepath.Join(plan.Dir, b.File)); err != nil {
			findings = append(findings, ValidationError{
				Check:  "index-file-mismatch",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("Batch Index names file %s, which does not exist in %s", b.File, plan.Dir),
			})
		}
	}

	// Reverse direction: every batch-shaped *.md file on disk (excluding
	// the overview itself) must be referenced by the index. An unreferenced
	// file is silently orphaned work a Planner forgot to wire in.
	entries, err := os.ReadDir(plan.Dir)
	if err == nil {
		var onDisk []string
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == overviewFileName {
				continue
			}
			if !indexed[e.Name()] {
				onDisk = append(onDisk, e.Name())
			}
		}
		sort.Strings(onDisk)
		for _, name := range onDisk {
			findings = append(findings, ValidationError{
				Check:  "index-file-mismatch",
				Detail: fmt.Sprintf("file %s exists in %s but is not referenced by the Batch Index", name, plan.Dir),
			})
		}
	}

	// Numbering must run 1..N with no gaps or duplicates; plan.Batches is
	// already in Batch Index order, so this is a straight sequence check.
	for i, b := range plan.Batches {
		want := i + 1
		if b.Number != want {
			findings = append(findings, ValidationError{
				Check:  "index-file-mismatch",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("batch numbering has a gap or duplicate: expected number %d at index position %d, got %d", want, i+1, b.Number),
			})
		}
	}

	return findings
}

// checkVerifyPresence implements check 3: every batch must carry either a
// non-empty VerifyCommand or VerifyDeferred with a non-zero ChainEnd.
func checkVerifyPresence(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		hasCommand := b.VerifyCommand != ""
		hasDeferredChain := b.VerifyDeferred && b.ChainEnd != 0
		if !hasCommand && !hasDeferredChain {
			findings = append(findings, ValidationError{
				Check:  "verify-missing",
				Batch:  batchID(b),
				Detail: "batch has neither a verify: command nor a valid verify: deferred + chain-end:",
			})
		}
	}

	return findings
}

// checkChainEndSoundness implements check 4: every ChainEnd must name an
// existing batch that is not itself VerifyDeferred and whose number is
// greater than the declaring batch's own number.
func checkChainEndSoundness(plan *Plan) []ValidationError {
	var findings []ValidationError

	byNumber := make(map[int]PlanBatch, len(plan.Batches))
	for _, b := range plan.Batches {
		byNumber[b.Number] = b
	}

	for _, b := range plan.Batches {
		if b.ChainEnd == 0 {
			continue
		}

		target, ok := byNumber[b.ChainEnd]
		switch {
		case !ok:
			findings = append(findings, ValidationError{
				Check:  "chain-end-dangling",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("chain-end: %d names a batch number that does not exist", b.ChainEnd),
			})
		case target.VerifyDeferred:
			findings = append(findings, ValidationError{
				Check:  "chain-end-dangling",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("chain-end: %d (%s) is itself verify: deferred, so it cannot be a chain's terminal verify", b.ChainEnd, batchID(target)),
			})
		case b.ChainEnd <= b.Number:
			findings = append(findings, ValidationError{
				Check:  "chain-end-dangling",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("chain-end: %d is not greater than this batch's own number %d", b.ChainEnd, b.Number),
			})
		}
	}

	return findings
}

// checkBatchOversized implements check 5: a batch's estimated context
// (bytes of its existing Scope+WhereFiles entries, resolved against
// worktreeRoot, divided by 4) over caps.ContextCapTokens, or its CardCount
// over caps.CardCap, without Oversized: true, fails loudly.
func checkBatchOversized(plan *Plan, worktreeRoot string, caps ValidateCaps) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		if b.Oversized {
			continue
		}

		var totalBytes int64
		for _, p := range b.Scope {
			totalBytes += pathSizeOnDisk(filepath.Join(worktreeRoot, p))
		}
		for _, p := range b.WhereFiles {
			totalBytes += pathSizeOnDisk(filepath.Join(worktreeRoot, p))
		}
		estimateTokens := int(totalBytes / 4)

		overContext := estimateTokens > caps.ContextCapTokens
		overCards := b.CardCount > caps.CardCap
		if overContext || overCards {
			findings = append(findings, ValidationError{
				Check: "batch-oversized",
				Batch: batchID(b),
				Detail: fmt.Sprintf(
					"estimated context %d tokens (cap %d) and card count %d (cap %d) without oversized: true",
					estimateTokens, caps.ContextCapTokens, b.CardCount, caps.CardCap,
				),
			})
		}
	}

	return findings
}

// pathSizeOnDisk returns the total byte size of path: the file's own size
// if it is a regular file, or the recursive sum of every regular file's
// size if it is a directory (Scope entries have prefix/directory
// semantics). A path that does not exist on disk contributes zero, per
// plan-format.md's "byte sizes of ... entries that exist on disk" wording
// — a not-yet-created Scope target is not an error at estimate time.
func pathSizeOnDisk(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}

	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, statErr := d.Info(); statErr == nil {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// checkScopeMalformed implements check 6: every Scope entry must be
// non-empty, relative, clean, and free of ".." escapes. Existence on disk
// is deliberately NOT required — plan-format.md's prefix list is well-formed
// as long as its entries "exist or are creatable".
func checkScopeMalformed(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, p := range b.Scope {
			if reason := scopeEntryMalformedReason(p); reason != "" {
				findings = append(findings, ValidationError{
					Check:  "scope-malformed",
					Batch:  batchID(b),
					Detail: fmt.Sprintf("scope entry %q is malformed: %s", p, reason),
				})
			}
		}
	}

	return findings
}

// scopeEntryMalformedReason reports why p is not a well-formed
// plan-format.md scope entry, or "" when p is well-formed. p is treated as
// a POSIX-style path (plan files are authored with forward slashes) so the
// check behaves the same on every platform Validate runs on.
func scopeEntryMalformedReason(p string) string {
	if p == "" {
		return "empty entry"
	}

	posix := filepath.ToSlash(p)
	if strings.HasPrefix(posix, "/") {
		return "absolute path"
	}
	for _, seg := range strings.Split(posix, "/") {
		if seg == ".." {
			return `contains a ".." escape`
		}
	}
	if cleaned := cleanPosixPath(posix); cleaned != posix {
		return fmt.Sprintf("not a clean path (cleans to %q)", cleaned)
	}

	return ""
}

// cleanPosixPath applies path.Clean's rules to a forward-slash path
// without importing the "path" package solely for this one call, keeping
// scopeEntryMalformedReason's own splitting/joining logic self-contained
// and easy to reason about alongside its ".." check above.
func cleanPosixPath(posix string) string {
	segments := strings.Split(posix, "/")
	var cleaned []string
	for _, seg := range segments {
		if seg == "" || seg == "." {
			continue
		}
		cleaned = append(cleaned, seg)
	}
	if len(cleaned) == 0 {
		return "."
	}
	return strings.Join(cleaned, "/")
}
