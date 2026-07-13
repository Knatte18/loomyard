// validate.go implements Validate, plan-format v2's complete machine check
// set (docs/modules/plan-format.md's "Validation checks" section), run in
// this fixed order: format/approval (format-unrecognized, plan-unapproved),
// Batch Index <-> file consistency (index-file-mismatch), verify: presence
// (verify-missing), chain-end soundness (chain-end-dangling), the
// oversized-batch context/card-count cap (batch-oversized), scope
// well-formedness for both "## Scope" entries and every card's typed
// file-op paths (scope-malformed), the five move-* checks that police a
// card's Moves: field and its "## Rename mechanic" companion section
// (move-format, move-redundant, move-source-missing, move-target-collision,
// move-mechanic-missing), the per-card structural checks (card-missing-field,
// card-field-overlap), the numbering checks (card-numbering,
// card-count-mismatch), and the cross-referencing checks (path-missing,
// card-outside-scope, commit-subject-mismatch). Validate runs both as the
// standalone `lyx builder validate` verb and as builder's hard automatic
// gate inside `run` and `spawn-batch` — the fail-loud-refusal half of
// plan-format.md's contract.

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
// format-unrecognized check. v2 supersedes v1 outright — there is no
// dual-version support, and no production v1 plans exist to migrate.
const recognizedFormat = 2

// ValidateCaps carries the two operator-configured cap values Validate's
// batch-oversized check (5) compares each batch's estimate against.
// builderengine itself stays config-free (config wiring is a later
// module's job) — callers resolve these from builder.yaml and pass them in.
type ValidateCaps struct {
	// ContextCapTokens is the maximum estimated context size (bytes of
	// referenced Scope + card file-op paths, divided by 4) a non-oversized
	// batch may claim before batch-oversized fires.
	ContextCapTokens int

	// CardCap is the maximum len(PlanBatch.Cards) a non-oversized batch may
	// claim before batch-oversized fires.
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

// Validate runs every plan-format v2 machine check against plan and returns
// every finding, ordered deterministically by check number and then by
// batch number within a check. worktreeRoot is the base Validate resolves
// each batch's Scope and card file-op path entries against for the
// batch-oversized context estimate (check 5); caps supplies that check's
// two cap values. A nil/empty return means the plan passes every check.
func Validate(plan *Plan, worktreeRoot string, caps ValidateCaps) []ValidationError {
	var findings []ValidationError

	findings = append(findings, checkFormatAndApproval(plan)...)
	findings = append(findings, checkIndexFileConsistency(plan)...)
	findings = append(findings, checkVerifyPresence(plan)...)
	findings = append(findings, checkChainEndSoundness(plan)...)
	findings = append(findings, checkBatchOversized(plan, worktreeRoot, caps)...)
	findings = append(findings, checkScopeMalformed(plan)...)
	findings = append(findings, checkMoveFormat(plan)...)
	findings = append(findings, checkMoveRedundant(plan)...)
	findings = append(findings, checkMoveSourceMissing(plan, worktreeRoot)...)
	findings = append(findings, checkMoveTargetCollision(plan, worktreeRoot)...)
	findings = append(findings, checkMoveMechanicMissing(plan)...)
	findings = append(findings, checkCardMissingField(plan)...)
	findings = append(findings, checkCardFieldOverlap(plan)...)
	findings = append(findings, checkCardNumbering(plan)...)
	findings = append(findings, checkCardCountMismatch(plan)...)
	findings = append(findings, checkPathMissing(plan, worktreeRoot)...)
	findings = append(findings, checkCardOutsideScope(plan)...)
	findings = append(findings, checkCommitSubjectMismatch(plan)...)

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
// greater than the declaring batch's own number — and the declaring batch
// must itself be VerifyDeferred, since chain membership is keyed on
// chain-end: alone (ChainMembers/ChainEndFor), so a batch declaring
// chain-end: next to a real verify: command silently joins the chain's
// destructive rollback set while never deferring anything.
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

		if !b.VerifyDeferred {
			findings = append(findings, ValidationError{
				Check:  "chain-end-dangling",
				Batch:  batchID(b),
				Detail: fmt.Sprintf("batch declares chain-end: %d but not verify: deferred; a chain intermediate must declare both", b.ChainEnd),
			})
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
// (bytes of its existing Scope entries plus every card's five typed
// file-op path fields and both sides of every Moves: pair, resolved
// against worktreeRoot, divided by 4) over caps.ContextCapTokens, or its
// card count over caps.CardCap, without Oversized: true, fails loudly. A
// path that does not exist on disk (a Creates: target, or a Moves:
// destination that has not landed yet) contributes zero bytes, per
// pathSizeOnDisk's existing semantics (context-estimate-inputs decision).
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
		for _, c := range b.Cards {
			for _, fields := range [][]string{c.ContextFiles, c.EditsFiles, c.CreatesFiles, c.DeletesFiles} {
				for _, p := range fields {
					totalBytes += pathSizeOnDisk(filepath.Join(worktreeRoot, p))
				}
			}
			for _, mv := range c.Moves {
				totalBytes += pathSizeOnDisk(filepath.Join(worktreeRoot, mv.Old))
				totalBytes += pathSizeOnDisk(filepath.Join(worktreeRoot, mv.New))
			}
		}
		estimateTokens := int(totalBytes / 4)

		cardCount := len(b.Cards)
		overContext := estimateTokens > caps.ContextCapTokens
		overCards := cardCount > caps.CardCap
		if overContext || overCards {
			findings = append(findings, ValidationError{
				Check: "batch-oversized",
				Batch: batchID(b),
				Detail: fmt.Sprintf(
					"estimated context %d tokens (cap %d) and card count %d (cap %d) without oversized: true",
					estimateTokens, caps.ContextCapTokens, cardCount, caps.CardCap,
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
// as long as its entries "exist or are creatable". It also runs the same
// well-formedness reason over every card's five normalized file-op field
// lists (both Moves sides counting as the fifth), citing the offending card
// in Detail — card-path well-formedness reuses this check name rather than
// minting a new one (validator-check-set decision).
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

		for _, c := range b.Cards {
			for _, fields := range [][]string{c.ContextFiles, c.EditsFiles, c.CreatesFiles, c.DeletesFiles} {
				for _, p := range fields {
					if reason := scopeEntryMalformedReason(p); reason != "" {
						findings = append(findings, ValidationError{
							Check:  "scope-malformed",
							Batch:  batchID(b),
							Detail: fmt.Sprintf("card %02d.%d path %q is malformed: %s", c.BatchPrefix, c.Number, p, reason),
						})
					}
				}
			}
			for _, mv := range c.Moves {
				for _, p := range []string{mv.Old, mv.New} {
					if reason := scopeEntryMalformedReason(p); reason != "" {
						findings = append(findings, ValidationError{
							Check:  "scope-malformed",
							Batch:  batchID(b),
							Detail: fmt.Sprintf("card %02d.%d path %q is malformed: %s", c.BatchPrefix, c.Number, p, reason),
						})
					}
				}
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

// checkMoveFormat implements move-format: every card's non-well-formed
// "Moves:" sub-bullet (retained verbatim in PlanCard.MovesRaw by the
// lenient-card-parse parser) yields one finding, quoting the raw bullet and
// naming the offending card so a Planner can find and fix it.
func checkMoveFormat(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			for _, raw := range c.MovesRaw {
				findings = append(findings, ValidationError{
					Check: "move-format",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"card %02d.%d Moves: entry %q does not match the required `src` -> `dst` grammar",
						c.BatchPrefix, c.Number, raw,
					),
				})
			}
		}
	}

	return findings
}

// checkMoveRedundant implements move-redundant: a batch-level check, mill's
// own semantics — a path that is both a Moves: endpoint (either side of any
// card's pair) and named in the same batch's Creates:/Deletes: (across all
// its cards) is a conflicting instruction: the Planner must pick one
// mechanism, not both. Findings are sorted by path within a batch.
func checkMoveRedundant(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		endpoints := make(map[string]bool)
		createsDeletes := make(map[string]bool)
		for _, c := range b.Cards {
			for _, mv := range c.Moves {
				endpoints[mv.Old] = true
				endpoints[mv.New] = true
			}
			for _, p := range c.CreatesFiles {
				createsDeletes[p] = true
			}
			for _, p := range c.DeletesFiles {
				createsDeletes[p] = true
			}
		}

		var conflicts []string
		for p := range endpoints {
			if createsDeletes[p] {
				conflicts = append(conflicts, p)
			}
		}
		sort.Strings(conflicts)

		for _, p := range conflicts {
			findings = append(findings, ValidationError{
				Check: "move-redundant",
				Batch: batchID(b),
				Detail: fmt.Sprintf(
					"%q is both a Moves: endpoint and in Creates:/Deletes: in the same batch; use Moves: or Creates:/Deletes:, not both",
					p,
				),
			})
		}
	}

	return findings
}

// createsUnion returns the union, across every batch and card in plan, of
// every CreatesFiles entry: mill's plan-wide suppression semantics —
// order-independent, so a Moves: source or target satisfied by ANY batch's
// Creates: (earlier or later in the index) is not flagged. Consulted by both
// checkMoveSourceMissing and checkMoveTargetCollision.
func createsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			for _, p := range c.CreatesFiles {
				union[p] = true
			}
		}
	}
	return union
}

// movesTargetsUnion returns the union, across every batch and card in plan,
// of every MovePair.New: the second plan-wide suppression set, letting a
// chained rename (batch A: X -> Y, batch B: Y -> Z) pass move-source-missing
// regardless of batch order.
func movesTargetsUnion(plan *Plan) map[string]bool {
	union := make(map[string]bool)
	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			for _, mv := range c.Moves {
				union[mv.New] = true
			}
		}
	}
	return union
}

// pathExistsOnDisk reports whether worktreeRoot-joined path exists on disk —
// shared by move-source-missing's absence check and
// move-target-collision's existence check.
func pathExistsOnDisk(worktreeRoot, path string) bool {
	_, err := os.Stat(filepath.Join(worktreeRoot, path))
	return err == nil
}

// checkMoveSourceMissing implements move-source-missing: a Moves: source
// that neither exists on disk nor is created or relocated by another batch
// (plan-wide createsUnion/movesTargetsUnion suppression, per mill's
// semantics) is a dangling rename instruction.
func checkMoveSourceMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createsUnion(plan)
	movesTargets := movesTargetsUnion(plan)

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			for _, mv := range c.Moves {
				if pathExistsOnDisk(worktreeRoot, mv.Old) {
					continue
				}
				if creates[mv.Old] || movesTargets[mv.Old] {
					continue
				}
				findings = append(findings, ValidationError{
					Check: "move-source-missing",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"Moves: source %q does not exist on disk and is not created or relocated by another batch",
						mv.Old,
					),
				})
			}
		}
	}

	return findings
}

// checkMoveTargetCollision implements move-target-collision: three OR'd
// conditions per Moves: target, first match wins per occurrence: the target
// already exists on disk; more than one batch names it as a Moves: target;
// or it collides with a DIFFERENT batch's Creates: entry (same-batch overlap
// is move-redundant's job, mirroring mill, so it is deliberately skipped
// here).
func checkMoveTargetCollision(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	// targetBatches counts, per target path, the distinct batches that name
	// it as a Moves: target — a size over 1 is condition (2).
	targetBatches := make(map[string]map[string]bool)
	// targetCreatesBatches records, per target path, the distinct batches
	// whose Creates: field names it — used by condition (3) to detect a
	// DIFFERENT batch's Creates: collision.
	targetCreatesBatches := make(map[string]map[string]bool)
	for _, b := range plan.Batches {
		id := batchID(b)
		for _, c := range b.Cards {
			for _, mv := range c.Moves {
				if targetBatches[mv.New] == nil {
					targetBatches[mv.New] = make(map[string]bool)
				}
				targetBatches[mv.New][id] = true
			}
			for _, p := range c.CreatesFiles {
				if targetCreatesBatches[p] == nil {
					targetCreatesBatches[p] = make(map[string]bool)
				}
				targetCreatesBatches[p][id] = true
			}
		}
	}

	for _, b := range plan.Batches {
		id := batchID(b)
		for _, c := range b.Cards {
			for _, mv := range c.Moves {
				target := mv.New

				var detail string
				switch {
				case pathExistsOnDisk(worktreeRoot, target):
					detail = fmt.Sprintf("Moves: target %q already exists on disk", target)
				case len(targetBatches[target]) > 1:
					detail = fmt.Sprintf("Moves: target %q is targeted by more than one batch", target)
				default:
					for owner := range targetCreatesBatches[target] {
						if owner != id {
							detail = fmt.Sprintf("Moves: target %q collides with another batch's Creates: entry", target)
							break
						}
					}
				}

				if detail != "" {
					findings = append(findings, ValidationError{
						Check:  "move-target-collision",
						Batch:  id,
						Detail: detail,
					})
				}
			}
		}
	}

	return findings
}

// checkMoveMechanicMissing implements move-mechanic-missing: a batch with at
// least one parsed Moves: pair (across any of its cards) but no "##
// Rename mechanic" section is missing the mechanical instruction that pins
// how the rename must be carried out. A batch whose every Moves: field is
// "none" (zero pairs) is skipped — a MovesRaw-only defect is
// move-format's finding, and requiring the section too would double-report
// the same underlying mistake.
func checkMoveMechanicMissing(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		hasPair := false
		for _, c := range b.Cards {
			if len(c.Moves) > 0 {
				hasPair = true
				break
			}
		}
		if hasPair && !b.HasRenameMechanic {
			findings = append(findings, ValidationError{
				Check:  "move-mechanic-missing",
				Batch:  batchID(b),
				Detail: `batch declares at least one Moves: pair but has no "## Rename mechanic" section`,
			})
		}
	}

	return findings
}

// cardFieldLabel pairs a card field's Has-presence bool with the bold label
// plan-format.md pins for it, in field order — checkCardMissingField's only
// data shape, kept next to the check so the field order stays visibly tied
// to the check that walks it.
type cardFieldLabel struct {
	present bool
	label   string
}

// checkCardMissingField implements card-missing-field: every card must
// carry all six of What:/Context:/Edits:/Creates:/Deletes:/Moves: — a
// missing label yields one finding per absent field, Detail naming the card
// and the missing label. Commit: and verify: are optional and never
// flagged.
func checkCardMissingField(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			fields := []cardFieldLabel{
				{c.HasWhat, "What:"},
				{c.HasContext, "Context:"},
				{c.HasEdits, "Edits:"},
				{c.HasCreates, "Creates:"},
				{c.HasDeletes, "Deletes:"},
				{c.HasMoves, "Moves:"},
			}
			for _, f := range fields {
				if f.present {
					continue
				}
				findings = append(findings, ValidationError{
					Check:  "card-missing-field",
					Batch:  batchID(b),
					Detail: fmt.Sprintf("card %02d.%d is missing its %s field", c.BatchPrefix, c.Number, f.label),
				})
			}
		}
	}

	return findings
}

// checkCardFieldOverlap implements card-field-overlap: within a single card,
// a path appearing in more than one of ContextFiles/EditsFiles/CreatesFiles/
// DeletesFiles, or as either side of a Moves: pair, is a conflicting
// instruction — one finding per duplicated path, Detail naming the card and
// every field the path appears in. Overlap is deliberately per-card only:
// the same path in one card's Creates: and another card's Edits: (in the
// same batch) is legitimate typed-field sequencing, not a defect.
func checkCardFieldOverlap(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			fieldsOf := make(map[string][]string)
			add := func(p, field string) {
				for _, seen := range fieldsOf[p] {
					if seen == field {
						return
					}
				}
				fieldsOf[p] = append(fieldsOf[p], field)
			}

			for _, p := range c.ContextFiles {
				add(p, "Context:")
			}
			for _, p := range c.EditsFiles {
				add(p, "Edits:")
			}
			for _, p := range c.CreatesFiles {
				add(p, "Creates:")
			}
			for _, p := range c.DeletesFiles {
				add(p, "Deletes:")
			}
			for _, mv := range c.Moves {
				add(mv.Old, "Moves:")
				add(mv.New, "Moves:")
			}

			var duplicated []string
			for p, fields := range fieldsOf {
				if len(fields) > 1 {
					duplicated = append(duplicated, p)
				}
			}
			sort.Strings(duplicated)

			for _, p := range duplicated {
				findings = append(findings, ValidationError{
					Check: "card-field-overlap",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"card %02d.%d path %q appears in more than one field: %s",
						c.BatchPrefix, c.Number, p, strings.Join(fieldsOf[p], ", "),
					),
				})
			}
		}
	}

	return findings
}

// checkCardNumbering implements card-numbering: per batch, (a) every card's
// heading-carried BatchPrefix must equal the batch's own Number, and (b)
// card Numbers must run 1..M sequentially in file order — a gap, a
// duplicate, or a wrong start each yield their own finding, mirroring
// checkIndexFileConsistency's sequence-check style.
func checkCardNumbering(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			if c.BatchPrefix != b.Number {
				findings = append(findings, ValidationError{
					Check: "card-numbering",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"card heading %02d.%d carries batch prefix %02d, but this batch's own number is %02d",
						c.BatchPrefix, c.Number, c.BatchPrefix, b.Number,
					),
				})
			}
		}

		for i, c := range b.Cards {
			want := i + 1
			if c.Number != want {
				findings = append(findings, ValidationError{
					Check: "card-numbering",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"card numbering has a gap, duplicate, or wrong start: expected card number %d at position %d, got %d",
						want, i+1, c.Number,
					),
				})
			}
		}
	}

	return findings
}

// checkCardCountMismatch implements card-count-mismatch: per batch, the
// Batch Index entry's "(C cards)" segment (PlanBatch.IndexCardCount) must
// equal the number of "### Card" headings the batch file actually parsed.
func checkCardCountMismatch(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		if b.IndexCardCount != len(b.Cards) {
			findings = append(findings, ValidationError{
				Check: "card-count-mismatch",
				Batch: batchID(b),
				Detail: fmt.Sprintf(
					"Batch Index declares %d card(s) but the batch file has %d",
					b.IndexCardCount, len(b.Cards),
				),
			})
		}
	}

	return findings
}

// checkPathMissing implements path-missing: every card path in
// ContextFiles, EditsFiles, and DeletesFiles must exist on disk under
// worktreeRoot, unless it is satisfied by some batch's Creates: or some
// Moves: pair's target (the same plan-wide createsUnion/movesTargetsUnion
// suppression sets move-source-missing consults). CreatesFiles is
// deliberately excluded — those paths are new by definition — and a
// card's own Moves: sources are move-source-missing's job, not this
// check's.
func checkPathMissing(plan *Plan, worktreeRoot string) []ValidationError {
	var findings []ValidationError

	creates := createsUnion(plan)
	movesTargets := movesTargetsUnion(plan)

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			for _, fields := range [][]string{c.ContextFiles, c.EditsFiles, c.DeletesFiles} {
				for _, p := range fields {
					if pathExistsOnDisk(worktreeRoot, p) {
						continue
					}
					if creates[p] || movesTargets[p] {
						continue
					}
					findings = append(findings, ValidationError{
						Check: "path-missing",
						Batch: batchID(b),
						Detail: fmt.Sprintf(
							"card %02d.%d path %q does not exist on disk and is not created or relocated by any batch",
							c.BatchPrefix, c.Number, p,
						),
					})
				}
			}
		}
	}

	return findings
}

// checkCardOutsideScope implements card-outside-scope: every card path in
// EditsFiles/CreatesFiles/DeletesFiles, and both endpoints of every Moves:
// pair, must be covered by one of the batch's own "## Scope" entries
// (pathCovers's boundary-aware prefix semantics, reused from digest.go via
// inScope — batch-local decision). ContextFiles is exempt: reading a file
// outside the batch's own scope is legitimate. A batch with an empty Scope
// declares nothing, so this check yields no findings for it.
func checkCardOutsideScope(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		if len(b.Scope) == 0 {
			continue
		}

		for _, c := range b.Cards {
			for _, fields := range [][]string{c.EditsFiles, c.CreatesFiles, c.DeletesFiles} {
				for _, p := range fields {
					if !inScope(p, b.Scope) {
						findings = append(findings, ValidationError{
							Check: "card-outside-scope",
							Batch: batchID(b),
							Detail: fmt.Sprintf(
								"card %02d.%d path %q is not covered by any Scope entry",
								c.BatchPrefix, c.Number, p,
							),
						})
					}
				}
			}
			for _, mv := range c.Moves {
				for _, p := range []string{mv.Old, mv.New} {
					if !inScope(p, b.Scope) {
						findings = append(findings, ValidationError{
							Check: "card-outside-scope",
							Batch: batchID(b),
							Detail: fmt.Sprintf(
								"card %02d.%d Moves: endpoint %q is not covered by any Scope entry",
								c.BatchPrefix, c.Number, p,
							),
						})
					}
				}
			}
		}
	}

	return findings
}

// checkCommitSubjectMismatch implements commit-subject-mismatch: a card's
// non-empty Commit value must start with the exact "NN.C: " prefix its own
// batch number and card number pin, per-card-commit-field's resume-trail
// discipline.
func checkCommitSubjectMismatch(plan *Plan) []ValidationError {
	var findings []ValidationError

	for _, b := range plan.Batches {
		for _, c := range b.Cards {
			if c.Commit == "" {
				continue
			}
			prefix := fmt.Sprintf("%02d.%d: ", b.Number, c.Number)
			if !strings.HasPrefix(c.Commit, prefix) {
				findings = append(findings, ValidationError{
					Check: "commit-subject-mismatch",
					Batch: batchID(b),
					Detail: fmt.Sprintf(
						"card %02d.%d Commit: %q does not start with the expected prefix %q",
						c.BatchPrefix, c.Number, c.Commit, prefix,
					),
				})
			}
		}
	}

	return findings
}
