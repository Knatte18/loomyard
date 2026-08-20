// destructiveguard_test.go closes slice 12's completeness proof by machine-checking that
// internal/fabricengine's production source contains no destructive primitive call outside
// destroy.go — the one file the Fabric Destruction Chokepoint Invariant names as permitted to
// perform one.
// See CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant.
//
// This guard clones cmd/lyx/rawgitmutation_test.go's machinery wholesale: the module-relative
// scan-package list, the raw-substring banned-token slice, the per-file allowlist map keyed by
// module-relative slash-separated path with a reason as its value, the minimum-scanned-files
// floor, the exec.LookPath("go") clean skip, the go env GOMOD module-root resolution, the
// filepath.WalkDir skipping of test files, and the filepath.ToSlash normalisation before any
// comparison, which matters because Windows is the primary dev OS.
//
// Two of the eight banned tokens were corrected against a naive first guess in opposite
// directions, and the reasons are recorded here because both mistakes are easy to reintroduce.
//
// "RemoveAll(" rather than "os.RemoveAll(": the bare form is a deliberate superset. It catches the
// qualified "os.RemoveAll(" (e.g. warpprobe.go's allowlisted probe-clone teardown) AND a
// method-call spelling like destroy.go's own `root.RemoveAll(` — the os.Root-rooted removal the R3
// containment fix routes through — neither of which the narrower "os.RemoveAll(" would match.
// (An earlier binding also had a bare `var RemoveAll = os.RemoveAll` seam this token caught; that
// seam was removed once the executors began removing through os.Root, but the bare token remains the
// correct superset for the forms that survive.)
//
// "warp.ResetHard(" / "weft.ResetHard(" rather than ".ResetHard(": the broad ".ResetHard(" form
// would flag the *correctly migrated* callers, since the gated reset is reached as a method call
// on the pair handle (e.g. `f.warp.ResetHard(sha)` inside destroy.go itself, or a future
// `weft`-side caller). Banning the raw handles instead targets what is actually forbidden —
// reaching past the gate to the underlying repo field — and needs no leading dot, so it matches
// under any receiver name.
//
// The allowlist is per-file, so a *new* raw removal added inside an allowlisted file is not
// caught by this guard — the same limitation the file being cloned already has. The junction.go
// and hook.go entries in particular are whole-file allowlists for exactly one already-audited
// call each, not a blanket exemption for future removals in those files.
//
// This guard now carries two independent exemption mechanisms answering two different questions:
// destructiveGuardAllowlist ("is this one file's one audited call site safe?") and
// destructiveGuardExcludedDirs ("is this entire subdirectory a different package outside the
// invariant's subject?"). The two are never interchangeable — a directory exclusion skips an
// entire subtree at the walk level, where a growing set of per-file allowlist rows would not
// restore the guard's package-scoped intent.
//
// This file also carries TestMutationRecord_FabricengineProductionSource, the Mutation Record
// Invariant's guard (see CONSTRAINTS.md's Mutation Record Invariant). It pins two shapes by raw
// source inspection alone, both against internal/fabricengine/destroy.go and the mutating result
// types' declarations: that every one of destroy.go's eight executors declares a leading
// `rec *Mutations` parameter, and that every mutating verb's result type embeds MutationRecord
// while the read-only verbs' result types do not. Its blind spots are deliberate and
// significant: it never inspects an executor's body for a `rec.Append`/`rec.AppendRef` call, so it
// cannot tell a correctly recording executor from one whose parameter is a dead letter, and it
// cannot tell a real recording call from one sitting inside a comment. Whether each body's
// recording call is actually present and correct is a review obligation, not something this guard
// proves. A new Kind added to mutation.go with no recording site anywhere is caught by nothing
// here either — see the Mutation Record Invariant's own text for that gap.

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// destructiveGuardScanPackages are the module-relative package subtrees this guard walks: exactly
// the one package the discussion's bypass-guard decision names, internal/fabricengine.
var destructiveGuardScanPackages = []string{
	filepath.Join("internal", "fabricengine"),
}

// destructiveGuardBannedTokens are the raw substrings a non-test .go file in
// destructiveGuardScanPackages may not contain, unless the file is on destructiveGuardAllowlist.
// This is the discussion's final seven tokens plus "createdToken{", added per the overview's
// decision that the token's unforgeability is guard-enforced rather than type-enforced.
var destructiveGuardBannedTokens = []string{
	"RemoveAll(",
	"os.Remove(",
	`"worktree", "remove"`,
	`"branch", "-D"`,
	"warp.ResetHard(",
	"weft.ResetHard(",
	"fslink.Remove(",
	"createdToken{",
}

// destructiveGuardAllowlist is this guard's per-file allowlist (path module-relative,
// slash-separated → reason). Every entry carries the reason its one audited destructive call
// site is safe, per the Fabric Destruction Chokepoint Invariant's requirement that every
// allowlist entry name one.
var destructiveGuardAllowlist = map[string]string{
	"internal/fabricengine/destroy.go": "the gate's own file — the one file the invariant permits to perform a destructive primitive",
	"internal/fabricengine/gitexclude.go": "writeFileAtomically's os.Remove(tempPath) cleans up a temp file the same function created " +
		"under a repo-wide flock, never operator content",
	"internal/fabricengine/warpprobe.go": "probeWeftBinding's os.RemoveAll(probeDir) removes the throwaway probe clone directory the " +
		"same function created moments earlier",
	"internal/fabricengine/index.go": "refreshCorrIndexAfterSwitch's os.Remove(path) deliberately deletes the correspondence-index " +
		"cache before rebuilding it, so a failed refresh misses honestly rather than answering cross-branch",
	"internal/fabricengine/mergestate.go": "deleteMergeState's os.Remove(path) deletes fabric's own merge-state record inside the " +
		"weft gitdir, fabric-internal metadata, never operator content",
	"internal/fabricengine/junction.go": "two audited sites, both removing a directory the same call just emptied by rename and " +
		"both using os.Remove rather than RemoveAll, so the OS itself refuses the moment anything is left inside: " +
		"adoptDotLyxContent's os.Remove(link) for the warp-side `.lyx` root, and mergeAdoptionTree's os.Remove(srcPath) for each " +
		"source subdirectory the recursive merge has just drained — whole-file allowlist for exactly these two, not a blanket exemption",
	"internal/fabricengine/hook.go": "chainUserHook's os.Remove(userHookPath) removes the user-hook backup that same function wrote " +
		"ten lines earlier, on its own rollback path after a failed chain write",
	"internal/fabricengine/doc.go": "the package doc's prose explains this slice's destruction rationale and must be able to name " +
		"the banned tokens; its only non-comment line is the package clause, so it can never carry a real call",
}

// destructiveGuardMinScannedFiles is the vacuous-scan floor for this guard's one-package walk:
// comfortably below the package's current production file count (53 at authoring time) and
// comfortably above zero. Its job is catching a misconfigured walk, not tracking the package's
// size — it is not expected to track file-count churn as the package grows or shrinks.
const destructiveGuardMinScannedFiles = 30

// destructiveGuardExcludedDirs are module-relative, slash-separated directories the walk skips
// entirely (filepath.SkipDir), keyed to the reason the directory falls outside package
// fabricengine's scope even though it nests under internal/fabricengine on disk.
var destructiveGuardExcludedDirs = map[string]string{}

// destructiveGuardRecordingExecutors is the table of every executor in
// internal/fabricengine/destroy.go the Mutation Record Invariant requires to take a leading
// `rec *Mutations` parameter, paired with the raw declaration-line prefix
// TestMutationRecord_FabricengineProductionSource greps for. repointLink is deliberately absent:
// it correctly records nothing of its own — there is no link_repointed kind — and passes rec
// straight through to removeLink, so it needs no entry in this table at all, not an exemption.
var destructiveGuardRecordingExecutors = []struct {
	name       string
	declPrefix string
}{
	{"removePath", "func removePath(rec *Mutations, "},
	{"removeGitWorktree", "func removeGitWorktree(rec *Mutations, "},
	{"removeLink", "func removeLink(rec *Mutations, "},
	{"repointLink", "func repointLink(rec *Mutations, "},
	{"deleteBranch", "func deleteBranch(rec *Mutations, "},
	{"createExclusiveDir", "func createExclusiveDir(rec *Mutations, "},
	{"createGitWorktree", "func createGitWorktree(rec *Mutations, "},
	{"resetHardTo", "func resetHardTo(rec *Mutations, "},
}

// destructiveGuardRecordingExecutorsMin is the vacuous-scan floor for
// destructiveGuardRecordingExecutors: the table declares 8 rows today, this floors well below that
// so a table that silently stopped matching (e.g. a rename that broke every declPrefix at once)
// fails loudly rather than passing on zero found declarations.
const destructiveGuardRecordingExecutorsMin = 5

// destructiveGuardMutatingResultTypes is the table of every mutating verb's result type the
// Mutation Record Invariant requires to embed MutationRecord, paired with the module-relative file
// each is declared in.
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
	{"PushResult", "internal/fabricengine/weftgit.go"},
	{"MergeResult", "internal/fabricengine/merge.go"},
	{"StageResult", "internal/fabricengine/mergestage.go"},
}

// destructiveGuardReadOnlyResultTypes is the companion table of the read-only verbs' result types
// the invariant requires to NOT embed MutationRecord — the which-verbs-record scope decision is
// machine-held here, not left to convention.
//
// It has two rows by construction rather than by omission, and which verb each row serves is not the
// natural guess: StatusResult (status.go) is the **pairs** verb and DiffResult is `diff`. The other
// two read-only verbs have no result type to pin — the `status` verb's Fabric.Status returns a bare
// []ChangeEntry and `list`'s List returns a bare []WorktreeEntry — so a reader must not read two rows
// here as a table that has drifted.
var destructiveGuardReadOnlyResultTypes = []struct {
	name string
	file string
}{
	{"StatusResult", "internal/fabricengine/status.go"},
	{"DiffResult", "internal/fabricengine/diff.go"},
}

// TestNoDestructiveBypass_FabricengineProductionSource walks internal/fabricengine's non-test .go
// files and fails if any of them (other than a destructiveGuardAllowlist entry) contains one of
// destructiveGuardBannedTokens — the eight construction/call tokens a destructive primitive
// reached outside the gate would carry.
func TestNoDestructiveBypass_FabricengineProductionSource(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// rawgitmutation_test.go and tierpurity_test.go so this gate never blocks a minimal
	// environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	var scanned int
	var failures []string

	for _, pkgRel := range destructiveGuardScanPackages {
		pkgDir := filepath.Join(moduleRoot, pkgRel)

		walkErr := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				relDir, relErr := filepath.Rel(moduleRoot, path)
				if relErr != nil {
					return relErr
				}
				if _, excluded := destructiveGuardExcludedDirs[filepath.ToSlash(relDir)]; excluded {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			relPath, relErr := filepath.Rel(moduleRoot, path)
			if relErr != nil {
				return relErr
			}
			// Normalize to slash-separated form before any comparison, exactly as
			// tierpurity_test.go/rawgitmutation_test.go do: filepath.WalkDir yields backslash
			// paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			scanned++

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(data)

			if _, allowlisted := destructiveGuardAllowlist[relPath]; allowlisted {
				return nil
			}

			for _, tok := range destructiveGuardBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains banned destructive-bypass token %q — a destructive primitive must be reached only through internal/fabricengine/destroy.go's gate (see CONSTRAINTS.md's Fabric Destruction Chokepoint Invariant), or add a destructiveGuardAllowlist entry in cmd/lyx/destructiveguard_test.go with a reason if this is a new audited exemption",
						relPath, tok,
					))
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("failed to walk %s: %v", pkgDir, walkErr)
		}
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < destructiveGuardMinScannedFiles {
		t.Fatalf("destructive bypass guard: only scanned %d production .go file(s) across %v; expected at least %d — the walk may be misconfigured", scanned, destructiveGuardScanPackages, destructiveGuardMinScannedFiles)
	}

	if len(failures) > 0 {
		t.Errorf("Fabric Destruction Chokepoint Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// TestMutationRecord_FabricengineProductionSource is the Mutation Record Invariant's guard (see
// CONSTRAINTS.md's Mutation Record Invariant). It reuses this file's machinery wholesale — the
// exec.LookPath("go") clean skip, the go env GOMOD module-root resolution, filepath.WalkDir, the
// _test.go skip, and the filepath.ToSlash normalisation before any comparison — and asserts two
// things by raw source inspection, never by inspecting an executor's body:
//
//  1. Every executor named in destructiveGuardRecordingExecutors declares a leading
//     `rec *Mutations` parameter in internal/fabricengine/destroy.go.
//  2. Every result type named in destructiveGuardMutatingResultTypes embeds MutationRecord, and
//     every result type named in destructiveGuardReadOnlyResultTypes does not.
//
// See this file's header comment for this guard's blind spots: it pins the parameter and the
// embed by declaration inspection only, never that an executor body actually appends, nor that
// what it appends is correct.
func TestMutationRecord_FabricengineProductionSource(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring this file's
	// other guard.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	destroyPath := filepath.Join(moduleRoot, "internal", "fabricengine", "destroy.go")
	destroyData, readErr := os.ReadFile(destroyPath)
	if readErr != nil {
		t.Fatalf("failed to read %s: %v", destroyPath, readErr)
	}
	destroyContent := string(destroyData)

	var scannedExecutors int
	for _, e := range destructiveGuardRecordingExecutors {
		if !strings.Contains(destroyContent, e.declPrefix) {
			t.Errorf("internal/fabricengine/destroy.go: executor %s does not declare a leading `rec *Mutations` parameter (expected to find %q) — every executor in destroy.go must take a recorder per the Mutation Record Invariant (see CONSTRAINTS.md)", e.name, e.declPrefix)
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
			t.Errorf("%s: mutating result type %s must embed MutationRecord as its first field (see CONSTRAINTS.md's Mutation Record Invariant); found %q", rt.file, rt.name, firstField)
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
			t.Errorf("%s: read-only result type %s must NOT embed MutationRecord — the which-verbs-record scope decision is machine-held (see CONSTRAINTS.md's Mutation Record Invariant)", rt.file, rt.name)
			continue
		}
		scannedReadOnlyTypes++
	}
	if scannedReadOnlyTypes != len(destructiveGuardReadOnlyResultTypes) {
		t.Fatalf("mutation record guard: only verified %d of %d declared read-only result types; the table may have drifted", scannedReadOnlyTypes, len(destructiveGuardReadOnlyResultTypes))
	}
}

// structFirstFieldLine returns the trimmed text of the first field line inside typeName's struct
// body in content, and whether a `type <typeName> struct {` declaration was found at all.
// It skips the opening-brace line itself (the remainder of the `type ... struct {` line, always
// empty once the declaration prefix is stripped) and returns the line after it — the position an
// embedded MutationRecord must occupy as the struct's first field.
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
