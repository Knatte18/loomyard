// destructiveguard_test.go guards internal/fabricengine against destructive primitive calls outside destroy.go.
// destroy.go is the one file the Fabric Destruction Chokepoint Invariant permits to perform one.
// It enforces `PATTERN-fabric-boundary`.
//
// The guard scans the module-relative packages it lists for raw-substring banned tokens.
// Its per-file scankit allowlist is keyed by module-relative, slash-separated path, and the value is the reason.
// It also enforces a minimum-scanned-files floor.
// scankit supplies the module root and the production-file walk.
//
// Two banned-token choices are easy to get wrong, so each states its reason.
//
// The bare "RemoveAll(" is banned rather than "os.RemoveAll(", because the bare form is a superset.
// It catches the qualified call, as in the allowlisted probe-clone teardown in internal/fabricengine.
// It also catches the method-call spelling `root.RemoveAll(` that destroy.go uses to remove through os.Root.
// The narrower "os.RemoveAll(" would miss that spelling.
//
// The two raw per-side handle forms of `.ResetHard(` are banned, not `.ResetHard(` itself.
// The broad form would flag correctly migrated callers.
// The gated reset is a method call on the pair handle, as in `f.ResetHard(rec, ...)` in pull.go.
// Banning the raw handles targets reaching past the gate to the underlying repo field.
// It needs no leading dot, so it matches under any receiver name.
//
// The allowlist is per-file, so a new raw removal added inside an allowlisted file goes uncaught.
// The junction.go and hook.go entries are whole-file allowlists for their audited call sites.
// They are not a blanket exemption for later removals in those files.
//
// Two exemption mechanisms answer two different questions:
// destructiveGuardAllowlist asks whether one file's one audited call site is safe.
// destructiveGuardExcludedDirs asks whether a whole subdirectory is a different package outside the invariant's subject.
// They are not interchangeable.
// A directory exclusion skips a whole subtree at the walk level;
// per-file allowlist rows cannot do that for a package scope.
//
// This file also carries TestMutationRecord_FabricengineProductionSource, the Mutation Record Invariant's guard.
// It pins two shapes by raw source inspection alone.
// Both check internal/fabricengine/destroy.go and the mutating result types' declarations:
// every executor in destroy.go declares a leading `rec *Mutations` parameter,
// and every mutating verb's result type embeds MutationRecord while the read-only verbs' result types do not.
// Its blind spots: it never inspects an executor's body for a `rec.Append`/`rec.AppendRef` call,
// so it cannot tell a correctly recording executor from one whose parameter is a dead letter,
// and it cannot tell a real recording call from one sitting inside a comment.
// Whether each body's recording call is present and correct is a review obligation, not something this guard proves.
// A new Kind added to mutation.go with no recording site anywhere is caught by nothing here either.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// destructiveGuardScanPackages lists the module-relative package subtrees this guard walks.
// The destructive primitives live in internal/fabricengine, the subtree it scans.
var destructiveGuardScanPackages = []string{
	"internal/fabricengine",
}

// destructiveGuardBannedTokens are the raw substrings a non-test .go file in the scanned packages may not contain.
// A file on destructiveGuardAllowlist is exempt.
// They name the destructive calls and the reset, replay and remote-branch primitives.
// They also name the literals that construct the gate's tokens.
// The token literals are banned because this guard, not the type system, keeps the tokens unforgeable.
var destructiveGuardBannedTokens = []string{
	"RemoveAll(",
	"os.Remove(",
	`"worktree", "remove"`,
	`"branch", "-D"`,
	"code.ResetHard(",
	"records.ResetHard(",
	"fslink.Remove(",
	"createdToken{",
	"createdBranchToken{",
	".DeleteRemoteBranch(",
	".UpdateRemoteBranchLeased(",
	".CherryPick(",
	".ResetKeep(",
}

// destructiveGuardAllowlist exempts files from this guard, keyed by module-relative, slash-separated path.
// Each entry gives the reason the file's audited destructive call sites are safe.
var destructiveGuardAllowlist = []scankit.Entry{
	{Key: "internal/fabricengine/destroy.go", Why: "the gate's own file — the one file the invariant permits to perform a destructive primitive"},
	{Key: "internal/fabricengine/gitexclude.go", Why: "writeFileAtomically's os.Remove(tempPath) cleans up a temp file the same function created " +
		"under a repo-wide flock, never operator content"},
	{Key: "internal/fabricengine/pendingflags.go", Why: "takePending's os.Remove(taken) deletes the pending-push flag file it renamed to a name private to the call one line earlier, " +
		"a fabric-internal marker in the records worktree's untracked lock directory, never operator content"},
	{Key: "internal/fabricengine/pushlockholder.go", Why: "holdPushLock's release os.Remove(path) deletes the holder record the same function wrote beside fabric.push.lock, " +
		"a fabric-internal marker in the records worktree's untracked lock directory, never operator content"},
	{Key: "internal/fabricengine/recordsprobe.go", Why: "the binding probe's os.RemoveAll(probeDir) removes the throwaway probe clone directory the " +
		"same function created moments earlier"},
	{Key: "internal/fabricengine/resetsubpathcheck.go", Why: "checkSubpathBeforeReset's os.RemoveAll(stageDir) removes the throwaway staging directory the " +
		"same function created moments earlier"},
	{Key: "internal/fabricengine/index.go", Why: "refreshCorrIndexAfterSwitch's os.Remove(path) deliberately deletes the correspondence-index " +
		"cache before rebuilding it, so a failed refresh misses honestly rather than answering cross-branch"},
	{Key: "internal/fabricengine/mergestate.go", Why: "deleteMergeState's os.Remove(path) deletes fabric's own merge-state record inside the " +
		"records-side gitdir, fabric-internal metadata, never operator content"},
	{Key: "internal/fabricengine/junction.go", Why: "two audited sites, both removing a directory the same call just emptied by rename and " +
		"both using os.Remove rather than RemoveAll, so the OS itself refuses the moment anything is left inside: " +
		"adoptDotLymContent's os.Remove(link) for the code-side `.lym` root, and mergeAdoptionTree's os.Remove(srcPath) for each " +
		"source subdirectory the recursive merge has just drained — whole-file allowlist for exactly these two, not a blanket exemption"},
	{Key: "internal/fabricengine/failedpush.go", Why: "ClearPushFailure's os.Remove(path) deletes fabric's own failed-push record under the hub's machine-local " +
		"scratch directory, a fabric-internal marker of a push origin refused, never operator content"},
	{Key: "internal/fabricengine/moverecord.go", Why: "RemoveMoveRecord's os.Remove deletes fabric's own move record in the records worktree's lock directory, " +
		"fabric-internal metadata, never operator content"},
	{Key: "internal/fabricengine/hook.go", Why: "chainUserHook's os.Remove(userHookPath) removes the user-hook backup that same function wrote " +
		"ten lines earlier, on its own rollback path after a failed chain write"},
	{Key: "internal/fabricengine/doc.go", Why: "the package doc's prose explains this slice's destruction rationale and must be able to name " +
		"the banned tokens; its only non-comment line is the package clause, so it can never carry a real call"},
}

// destructiveGuardMinScannedFiles is the vacuous-scan floor for the walk over the scanned packages.
// It sits below the packages' production file count and above zero,
// so it catches a misconfigured walk without tracking the packages' size as files are added or removed.
const destructiveGuardMinScannedFiles = 30

// destructiveGuardExcludedDirs are module-relative, slash-separated directories the walk skips (filepath.SkipDir).
// Each value gives the reason the directory lies outside the scanned package.
// It nests under internal/fabricengine on disk, so the path alone does not show this.
var destructiveGuardExcludedDirs = map[string]string{}

// destructiveGuardRecordingExecutors lists the destroy.go executors that must take a leading `rec *Mutations` parameter.
// Each row pairs the executor with the raw declaration-line prefix the guard matches against the source.
// repointLink has no row: it records nothing of its own, since there is no link_repointed kind,
// and it passes rec straight through to removeLink, so it needs no row and is not an exemption.
var destructiveGuardRecordingExecutors = []struct {
	name       string
	declPrefix string
}{
	{"removePath", "func removePath(rec *Mutations, "},
	{"removeGitWorktree", "func removeGitWorktree(rec *Mutations, "},
	{"removeWorktreeLeftover", "func removeWorktreeLeftover(rec *Mutations, "},
	{"removeLink", "func removeLink(rec *Mutations, "},
	{"repointLink", "func repointLink(rec *Mutations, "},
	{"deleteBranch", "func deleteBranch(rec *Mutations, "},
	{"deleteRemoteBranch", "func deleteRemoteBranch(rec *Mutations, "},
	{"updateRemoteBranch", "func updateRemoteBranch(rec *Mutations, "},
	{"createExclusiveDir", "func createExclusiveDir(rec *Mutations, "},
	{"createGitWorktree", "func createGitWorktree(rec *Mutations, "},
	{"resetHardTo", "func resetHardTo(rec *Mutations, "},
	{"dropSeedCommits", "func dropSeedCommits(rec *Mutations, "},
	{"DropLocal", "func DropLocal(rec *Mutations, "},
}

// destructiveGuardRecordingExecutorsMin is the vacuous-scan floor for the recording-executor table.
// It sits below the table's row count, so a table that silently stopped matching fails loudly.
// A rename that broke every declPrefix at once would otherwise pass on zero found declarations.
const destructiveGuardRecordingExecutorsMin = 5

// destructiveGuardMutatingResultTypes is the table of the mutating verbs' result types that must embed MutationRecord.
// Each row pairs the type with the module-relative file it is declared in.
var destructiveGuardMutatingResultTypes = []struct {
	name string
	file string
}{
	{"AddResult", "internal/fabricengine/add.go"},
	{"RemoveResult", "internal/fabricengine/remove.go"},
	{"CheckoutResult", "internal/fabricengine/checkout.go"},
	{"PruneResult", "internal/fabricengine/prune.go"},
	{"CleanupResult", "internal/fabricengine/cleanup.go"},
	{"UnwireVerbResult", "internal/fabricengine/unwire.go"},
	{"UnwireResult", "internal/fabricengine/junction.go"},
	{"ReconcileResult", "internal/fabricengine/reconcile.go"},
	{"CommitResult", "internal/fabricengine/commit.go"},
	{"PullResult", "internal/fabricengine/pull.go"},
	{"CloneResult", "internal/fabricengine/clone.go"},
	{"PushResult", "internal/fabricengine/recordsgit.go"},
	{"MergeResult", "internal/fabricengine/merge.go"},
	{"StageResult", "internal/fabricengine/mergestage.go"},
	{"BoltWriteResult", "internal/fabricengine/bolt.go"},
	{"DropLocalResult", "internal/fabricengine/destroy.go"},
}

// destructiveGuardReadOnlyResultTypes is the table of read-only verbs' result types that must not embed MutationRecord.
// The table is machine-checked, so which verbs record is not left to convention.
//
// A verb that returns a bare slice has no result type to pin, so it has no row by construction, not by omission.
// StatusResult belongs to the pairs verb and DiffResult to diff, so the rows do not follow the verb names.
var destructiveGuardReadOnlyResultTypes = []struct {
	name string
	file string
}{
	{"StatusResult", "internal/fabricengine/status.go"},
	{"DiffResult", "internal/fabricengine/diff.go"},
}

// TestNoDestructiveBypass_FabricengineProductionSource walks internal/fabricengine's non-test .go files.
// It fails if any of them, other than an allowlisted file, contains one of destructiveGuardBannedTokens.
// Each token marks a destructive call or token construction reached outside the gate.
//
//lym:guard
func TestNoDestructiveBypass_FabricengineProductionSource(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(destructiveGuardAllowlist)

	var scanned int
	var failures []string

	for _, pkgRel := range destructiveGuardScanPackages {
		scanned += scankit.Walk(t, scankit.Options{Roots: []string{pkgRel}}, func(f *scankit.File) {
			for excluded := range destructiveGuardExcludedDirs {
				if strings.HasPrefix(f.Rel, excluded+"/") {
					return
				}
			}
			if allow.Allowed(f.Rel) {
				return
			}
			content := string(f.Data)
			for _, tok := range destructiveGuardBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains banned destructive-bypass token %q — a destructive primitive must be reached only through internal/fabricengine/destroy.go's gate (see `PATTERN-fabric-boundary`), or add a destructiveGuardAllowlist entry in cmd/lym/destructiveguard_test.go with a reason if this is a new audited exemption",
						f.Rel, tok,
					))
				}
			}
		})
	}

	scankit.RequireFloor(t, scanned, destructiveGuardMinScannedFiles, "destructive bypass guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-fabric-boundary` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// TestMutationRecord_FabricengineProductionSource is the Mutation Record Invariant's guard.
// It asserts two things by raw source inspection:
//
//  1. Every executor in destructiveGuardRecordingExecutors declares a leading `rec *Mutations` parameter in destroy.go.
//  2. Every result type named in destructiveGuardMutatingResultTypes embeds MutationRecord,
//     and every result type named in destructiveGuardReadOnlyResultTypes does not.
//
// It inspects declarations only,
// so it does not prove that an executor appends to rec or that what it appends is correct.
//
//testtiming:keep internal/fabricengine/doc.go names this test as the Mutation Record Invariant's guard
//lym:guard
func TestMutationRecord_FabricengineProductionSource(t *testing.T) {
	tiergate.Require(t, 0)
	moduleRoot := scankit.Root(t)

	destroyPath := filepath.Join(moduleRoot, "internal", "fabricengine", "destroy.go")
	destroyData, readErr := os.ReadFile(destroyPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", destroyPath, readErr)
	}
	destroyContent := string(destroyData)

	var scannedExecutors int
	for _, e := range destructiveGuardRecordingExecutors {
		if !strings.Contains(destroyContent, e.declPrefix) {
			t.Errorf("internal/fabricengine/destroy.go: executor %s does not declare a leading `rec *Mutations` parameter (expected to find %q) — every executor in destroy.go must take a recorder per the mutation record section of fabricengine's package doc", e.name, e.declPrefix)
			continue
		}
		scannedExecutors++
	}
	if scannedExecutors < destructiveGuardRecordingExecutorsMin {
		t.Fatalf("mutation record guard: only matched %d of %d declared recording executors; expected at least %d — the table or destroy.go's declarations may have drifted", scannedExecutors, len(destructiveGuardRecordingExecutors), destructiveGuardRecordingExecutorsMin)
	}

	var scannedResultTypes int
	for _, rt := range destructiveGuardMutatingResultTypes {
		path := filepath.Join(moduleRoot, filepath.FromSlash(rt.file))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("failed to read %s for result type %s: %v", rt.file, rt.name, readErr)
			continue
		}
		content := string(data)
		firstField, found := structFirstFieldLine(content, rt.name)
		if !found {
			t.Errorf("%s: does not declare `type %s struct {` — the table entry may be stale", rt.file, rt.name)
			continue
		}
		if firstField != "MutationRecord" {
			t.Errorf("%s: mutating result type %s must embed MutationRecord as its first field (see the mutation record section of fabricengine's package doc); found %q", rt.file, rt.name, firstField)
			continue
		}
		scannedResultTypes++
	}
	if scannedResultTypes != len(destructiveGuardMutatingResultTypes) {
		t.Fatalf("mutation record guard: only verified %d of %d declared mutating result types; the table may have drifted", scannedResultTypes, len(destructiveGuardMutatingResultTypes))
	}

	var scannedReadOnlyTypes int
	for _, rt := range destructiveGuardReadOnlyResultTypes {
		path := filepath.Join(moduleRoot, filepath.FromSlash(rt.file))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("failed to read %s for result type %s: %v", rt.file, rt.name, readErr)
			continue
		}
		content := string(data)
		firstField, found := structFirstFieldLine(content, rt.name)
		if !found {
			t.Errorf("%s: does not declare `type %s struct {` — the table entry may be stale", rt.file, rt.name)
			continue
		}
		if firstField == "MutationRecord" {
			t.Errorf("%s: read-only result type %s must NOT embed MutationRecord — the which-verbs-record scope decision is machine-held (see the mutation record section of fabricengine's package doc)", rt.file, rt.name)
			continue
		}
		scannedReadOnlyTypes++
	}
	if scannedReadOnlyTypes != len(destructiveGuardReadOnlyResultTypes) {
		t.Fatalf("mutation record guard: only verified %d of %d declared read-only result types; the table may have drifted", scannedReadOnlyTypes, len(destructiveGuardReadOnlyResultTypes))
	}
}

// structFirstFieldLine returns the trimmed text of the first field line inside typeName's struct body in content.
// It reports whether a `type <typeName> struct {` declaration was found.
// An embedded MutationRecord must occupy that first field position.
func structFirstFieldLine(content, typeName string) (line string, found bool) {
	declPrefix := "type " + typeName + " struct {"
	idx := strings.Index(content, declPrefix)
	if idx < 0 {
		return "", false
	}
	rest := content[idx+len(declPrefix):]
	lines := strings.SplitN(rest, "\n", 3)
	if len(lines) < 2 {
		return "", true
	}
	return strings.TrimSpace(lines[1]), true
}
