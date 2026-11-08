// uncontainedwrite_test.go machine-checks internal/fabricengine's production source.
// It bans raw filesystem-WRITE primitives outside an explicit per-file allowlist.
// The banned primitives are os.MkdirAll, os.Mkdir, os.WriteFile, os.Create, os.OpenFile, os.Symlink and os.Link.
// Each allowlist entry names why that site is safe to write raw.
//
// Without this guard, a new uncontained write could slip into the package unremarked.
//
// The tokens are the os.-qualified spellings, not bare names such as `WriteFile(`.
// A bare name would also match the rooted calls, such as `root.WriteFile(`.
// Those rooted calls are the contained write path and must pass.
// The os.-qualified forms target a raw write to a caller-derived path that follows a planted symlink.
//
// The allowlist is per file, so a raw write added to an allowlisted file is not caught.
// Each entry covers one audited class of call, not a blanket exemption for future writes in that file.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// uncontainedWriteScanPackages lists the module-relative package subtrees the guard scans.
var uncontainedWriteScanPackages = []string{
	"internal/fabricengine",
}

// uncontainedWriteBannedTokens are the raw substrings a scanned non-test .go file may not contain.
// Each names a raw filesystem-write call.
// Such a call resolves a caller-derived path itself and follows a symlink planted on it.
// A file may hold one only if it is allowlisted.
var uncontainedWriteBannedTokens = []string{
	"os.MkdirAll(",
	"os.Mkdir(",
	"os.WriteFile(",
	"os.Create(",
	"os.OpenFile(",
	"os.Symlink(",
	"os.Link(",
}

// uncontainedWriteAllowlist maps each allowlisted file to the reason its raw writes are safe.
// Keys are module-relative, slash-separated paths.
// Most reasons rest on a git-owned path, never derived from an operator slug, or on a directory fabric created.
// Such a directory comes from a contained minter, `createExclusiveDir` or `containedWorktreeAdd`.
// A write into it is race-only: a same-UID race after creation could redirect it, never a static pre-plant.
// Writers that route through an os.Root carry no banned token and need no entry.
var uncontainedWriteAllowlist = []scankit.Entry{
	{Key: "internal/fabricengine/hook.go", Why: "InstallPostCheckoutHook/chainUserHook/writeHookFile write into the git-resolved hooks " +
		"directory (git rev-parse --git-path hooks), a git-owned path never derived from an operator slug; redirecting it " +
		"would require compromising the repo's own .git, outside the hub-symlink threat model"},
	{Key: "internal/fabricengine/gitexclude.go", Why: "mutateGitExclude's os.MkdirAll(excludeDir) creates the git-owned .git/info directory " +
		"resolved by git rev-parse --git-path info/exclude; the file replacement itself is a same-directory CreateTemp+Rename under " +
		"a repo-wide flock, never a caller-derived path"},
	{Key: "internal/fabricengine/clone.go", Why: "the hub scratch directory (in the Bolt checkout's .lym) is created in " +
		"the Bolt checkout worktree containedWorktreeAdd just added, not the bare hub createExclusiveDir (os.Root) minted, in this " +
		"same CloneHub call — race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/codebinding.go", Why: "the code-side binding writer writes the code-side binding file into the Bolt checkout records worktree fabric created via " +
		"containedWorktreeAdd; it is committed onto the records main branch by the caller, and the board directory is fabric-owned, never a " +
		"caller-derived slug path"},
	{Key: "internal/fabricengine/shortnamebinding.go", Why: "WriteShortname writes .lym-shortname into the Bolt checkout records worktree fabric created via " +
		"containedWorktreeAdd; it is committed onto the records main branch by the caller, and the board directory is fabric-owned, never a " +
		"caller-derived slug path"},
	{Key: "internal/fabricengine/recordsgit.go", Why: "the lock-directory helper's os.MkdirAll creates the lock directory inside the records worktree " +
		"root fabric created via containedWorktreeAdd; race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/junctionnames.go", Why: "BoltLockPath's os.MkdirAll creates the never-tracked lock directory under the Bolt checkout " +
		"root the caller passes, a directory fabric created via containedWorktreeAdd; race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/originreconcilemoves.go", Why: "tryPrimeLock's os.MkdirAll creates the hub-scoped ephemeral run-lock directory under the prime's .lym, " +
		"the same directory batten's own prime lock creates, so the try-acquire has a file to lock; the path is shedrun's, never caller-derived"},
	{Key: "internal/fabricengine/junction.go", Why: "seedLymJunction's os.MkdirAll(target) materialises a junction's records-side target inside the " +
		"records worktree fabric created via containedWorktreeAdd, and the code-side junction LINKS route through fslink; race-only " +
		"(add.go refuses a pre-existing worktree path), not statically pre-plantable"},
	{Key: "internal/fabricengine/scratchlink.go", Why: "the scratch step's os.MkdirAll creates the scratch target and its parent inside the pair's records " +
		"worktree, built from RecordsWorktreePath, and the root link itself goes through fslink, as junction.go's seedLymJunction does; " +
		"race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/doc.go", Why: "the package doc's prose names the raw write primitives when explaining the containment rationale; " +
		"its only non-comment line is the package clause, so it can never carry a real call"},
}

// uncontainedWriteMinScannedFiles is the floor on the files the guard must scan.
// It sits comfortably below the package's production file count and above zero.
// It catches a misconfigured walk without tracking file-count churn.
const uncontainedWriteMinScannedFiles = 30

// TestNoUncontainedWrite_FabricengineProductionSource scans internal/fabricengine's non-test .go files.
// It fails when a file outside the allowlist holds a banned token.
// A raw write must route through an os.Root rooted at the hub, the write-side containment chokepoint.
// Otherwise its file needs a reasoned allowlist entry.
// A narrowed gate runs it only when a file under internal/fabricengine or scankit changes; a whole gate always runs it.
//
//lym:guard
//lym:watch internal/fabricengine/*.go internal/fabricengine/*/*.go internal/testkit/scankit/*.go
func TestNoUncontainedWrite_FabricengineProductionSource(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(uncontainedWriteAllowlist)

	var scanned int
	var failures []string

	for _, pkgRel := range uncontainedWriteScanPackages {
		scanned += scankit.Walk(t, scankit.Options{Roots: []string{pkgRel}}, func(f *scankit.File) {
			if allow.Allowed(f.Rel) {
				return
			}
			content := string(f.Data)
			for _, tok := range uncontainedWriteBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains raw filesystem-write primitive %q — a write to a hub-relative or caller-derived path must route through an os.Root rooted at the hub (see internal/fabricengine/launchers.go's writeLaunchers and hubworkspace_write.go's writeHubWorkspace), or add a uncontainedWriteAllowlist entry in cmd/lym/uncontainedwrite_test.go with a reason if this is a new audited safe site",
						f.Rel, tok,
					))
				}
			}
		})
	}

	scankit.RequireFloor(t, scanned, uncontainedWriteMinScannedFiles, "uncontained-write guard")
	allow.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("write-side containment guard violated — raw filesystem-write primitive outside the os.Root chokepoint:\n%s", strings.Join(failures, "\n"))
	}
}
