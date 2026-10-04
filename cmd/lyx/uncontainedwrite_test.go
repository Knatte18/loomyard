// uncontainedwrite_test.go is the write-side twin of destructiveguard_test.go: it machine-checks that
// internal/fabricengine's production source contains no raw filesystem-WRITE primitive
// (os.MkdirAll/os.Mkdir/os.WriteFile/os.Create/os.OpenFile/os.Symlink/os.Link) outside an explicit
// per-file allowlist, each allowlist entry naming why that site is safe to write raw.
//
// It exists because five review rounds hardened one delete-side chain and the create-side worktree add
// while two OTHER create-side writers on the same `add` code path — writeLaunchers and createPortal,
// both writing to a hub-level structural container (_launchers, _portals) an attacker can pre-plant a
// static symlink at — sat undiscovered with zero containment protection. Nothing mechanically inventoried
// the package's raw WRITE primitives the way TestNoDestructiveBypass_FabricengineProductionSource
// inventories its removals, so a new uncontained write could always slip in unremarked. This guard makes
// every raw write a deliberate, reasoned allowlist entry, so the eighth link in that chain fails a test
// rather than being found by the next crucible round.
//
// The banned tokens are the os.-qualified spellings, deliberately NOT the bare forms the delete-side
// guard uses: the write-side containment fix routes the two exploitable writers through os.Root method
// calls (root.MkdirAll/root.WriteFile), which are the write-side chokepoint and must PASS, exactly as the
// bare delete-side "RemoveAll(" token catches destroy.go's own root.RemoveAll. Banning os.-qualified forms
// targets what is actually forbidden — a raw write to a caller-derived path that follows a planted symlink
// — while letting the rooted, contained writers through.
//
// The allowlist is per-file, so a NEW raw write added inside an allowlisted file is not caught — the same
// limitation the delete-side guard it clones has. Each entry is one already-audited class of call, not a
// blanket exemption for future writes in that file.

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// uncontainedWriteScanPackages is the module-relative package subtree this guard walks: the one package
// the write-side containment audit covers, internal/fabricengine.
var uncontainedWriteScanPackages = []string{
	"internal/fabricengine",
}

// uncontainedWriteBannedTokens are the raw substrings a non-test .go file in
// uncontainedWriteScanPackages may not contain unless the file is on uncontainedWriteAllowlist. Every
// one names a raw filesystem-write primitive that resolves a caller-derived path itself and follows a
// symlink planted on it, the create-side twin of the delete-side escape the destruction chokepoint closes.
var uncontainedWriteBannedTokens = []string{
	"os.MkdirAll(",
	"os.Mkdir(",
	"os.WriteFile(",
	"os.Create(",
	"os.OpenFile(",
	"os.Symlink(",
	"os.Link(",
}

// uncontainedWriteAllowlist is this guard's per-file allowlist (path module-relative, slash-separated →
// reason). Every entry names why its raw write is safe: it writes into a git-owned path (never derived
// from an operator slug), or into a worktree/board directory fabric just minted through a CONTAINED
// minter (createExclusiveDir/containedWorktreeAdd) in the same call, where only a post-creation same-UID
// race — the documented residual class, same as the gate's dirtiness window — could redirect it, never a
// static pre-plant. The two hub-level structural-container writers (launchers.go, portals.go) are
// deliberately ABSENT: they now route through an os.Root at the hub, so they carry no banned token at all.
var uncontainedWriteAllowlist = []scankit.Entry{
	{Key: "internal/fabricengine/hook.go", Why: "InstallPostCheckoutHook/chainUserHook/writeHookFile write into the git-resolved hooks " +
		"directory (git rev-parse --git-path hooks), a git-owned path never derived from an operator slug; redirecting it " +
		"would require compromising the repo's own .git, outside the hub-symlink threat model"},
	{Key: "internal/fabricengine/gitexclude.go", Why: "mutateGitExclude's os.MkdirAll(excludeDir) creates the git-owned .git/info directory " +
		"resolved by git rev-parse --git-path info/exclude; the file replacement itself is a same-directory CreateTemp+Rename under " +
		"a repo-wide flock, never a caller-derived path"},
	{Key: "internal/fabricengine/clone.go", Why: "the hub scratch directory (<hub>/_board/.lyx) and the .lyx-anchor marker are written into " +
		"the _board worktree containedWorktreeAdd just added, not the bare hub createExclusiveDir (os.Root) minted, both in this " +
		"same CloneHub call — race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/warpbinding.go", Why: "the code-side binding writer writes the code-side binding file into the _board records worktree fabric created via " +
		"containedWorktreeAdd; it is committed onto the records main branch by the caller, and the board directory is fabric-owned, never a " +
		"caller-derived slug path"},
	{Key: "internal/fabricengine/shortnamebinding.go", Why: "WriteShortname writes .lyx-shortname into the _board records worktree fabric created via " +
		"containedWorktreeAdd; it is committed onto the records main branch by the caller, and the board directory is fabric-owned, never a " +
		"caller-derived slug path"},
	{Key: "internal/fabricengine/weftgit.go", Why: "the lock-directory helper's os.MkdirAll creates the lock directory inside the records worktree " +
		"root fabric created via containedWorktreeAdd; race-only, not statically pre-plantable"},
	{Key: "internal/fabricengine/junction.go", Why: "seedLyxJunction's os.MkdirAll(target) materialises a junction's records-side target inside the " +
		"records worktree fabric created via containedWorktreeAdd, and the code-side junction LINKS route through fslink; race-only " +
		"(add.go refuses a pre-existing worktree path), not statically pre-plantable"},
	{Key: "internal/fabricengine/doc.go", Why: "the package doc's prose names the raw write primitives when explaining the containment rationale; " +
		"its only non-comment line is the package clause, so it can never carry a real call"},
}

// uncontainedWriteMinScannedFiles is the vacuous-scan floor for this guard's one-package walk: comfortably
// below the package's current production file count and above zero, catching a misconfigured walk rather
// than tracking file-count churn.
const uncontainedWriteMinScannedFiles = 30

// TestNoUncontainedWrite_FabricengineProductionSource walks internal/fabricengine's non-test .go files and
// fails if any of them (other than a uncontainedWriteAllowlist entry) contains one of
// uncontainedWriteBannedTokens — a raw filesystem-write primitive that must instead route through an
// os.Root rooted at the hub (the write-side containment chokepoint) or be an allowlisted, reasoned
// exemption. It is the write-side twin of TestNoDestructiveBypass_FabricengineProductionSource.
func TestNoUncontainedWrite_FabricengineProductionSource(t *testing.T) {
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
						"%s: contains raw filesystem-write primitive %q — a write to a hub-relative or caller-derived path must route through an os.Root rooted at the hub (see internal/fabricengine/launchers.go's writeLaunchers and portals.go's ensureContainedLinkParent), or add a uncontainedWriteAllowlist entry in cmd/lyx/uncontainedwrite_test.go with a reason if this is a new audited safe site",
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
