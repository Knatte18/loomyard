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
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// uncontainedWriteScanPackages is the module-relative package subtree this guard walks: the one package
// the write-side containment audit covers, internal/fabricengine.
var uncontainedWriteScanPackages = []string{
	filepath.Join("internal", "fabricengine"),
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
var uncontainedWriteAllowlist = map[string]string{
	"internal/fabricengine/hook.go": "InstallPostCheckoutHook/chainUserHook/writeHookFile write into the git-resolved hooks " +
		"directory (git rev-parse --git-path hooks), a git-owned path never derived from an operator slug; redirecting it " +
		"would require compromising the repo's own .git, outside the hub-symlink threat model",
	"internal/fabricengine/gitexclude.go": "mutateGitExclude's os.MkdirAll(excludeDir) creates the git-owned .git/info directory " +
		"resolved by git rev-parse --git-path info/exclude; the file replacement itself is a same-directory CreateTemp+Rename under " +
		"a repo-wide flock, never a caller-derived path",
	"internal/fabricengine/clone.go": "the hub scratch directory (<hub>/_board/.lyx) and the .lyx-anchor marker are written into " +
		"the _board worktree containedWorktreeAdd just added, not the bare hub createExclusiveDir (os.Root) minted, both in this " +
		"same CloneHub call — race-only, not statically pre-plantable",
	"internal/fabricengine/warpbinding.go": "writeWarpBinding writes .lyx-warp into the _board weft worktree fabric created via " +
		"containedWorktreeAdd; it is committed onto weft:main by the caller, and the board directory is fabric-owned, never a " +
		"caller-derived slug path",
	"internal/fabricengine/weftgit.go": "ensureWeftLockDirAt's os.MkdirAll(.weft) creates the lock directory inside the weft worktree " +
		"root fabric created via containedWorktreeAdd; race-only, not statically pre-plantable",
	"internal/fabricengine/junction.go": "seedLyxJunction's os.MkdirAll(target) materialises a junction's weft-side target inside the " +
		"weft worktree fabric created via containedWorktreeAdd, and the warp-side junction LINKS route through fslink; race-only " +
		"(add.go refuses a pre-existing worktree path), not statically pre-plantable",
	"internal/fabricengine/doc.go": "the package doc's prose names the raw write primitives when explaining the containment rationale; " +
		"its only non-comment line is the package clause, so it can never carry a real call",
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
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring destructiveguard_test.go.
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

	var scanned int
	var failures []string

	for _, pkgRel := range uncontainedWriteScanPackages {
		pkgDir := filepath.Join(moduleRoot, pkgRel)

		walkErr := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			relPath, relErr := filepath.Rel(moduleRoot, path)
			if relErr != nil {
				return relErr
			}
			// Normalize to slash-separated form before any comparison, exactly as the delete-side guard
			// does: filepath.WalkDir yields backslash paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			scanned++

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(data)

			if _, allowlisted := uncontainedWriteAllowlist[relPath]; allowlisted {
				return nil
			}

			for _, tok := range uncontainedWriteBannedTokens {
				if strings.Contains(content, tok) {
					failures = append(failures, fmt.Sprintf(
						"%s: contains raw filesystem-write primitive %q — a write to a hub-relative or caller-derived path must route through an os.Root rooted at the hub (see internal/fabricengine/launchers.go's writeLaunchers and portals.go's ensureContainedLinkParent), or add a uncontainedWriteAllowlist entry in cmd/lyx/uncontainedwrite_test.go with a reason if this is a new audited safe site",
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

	if scanned < uncontainedWriteMinScannedFiles {
		t.Fatalf("uncontained-write guard: only scanned %d production .go file(s) across %v; expected at least %d — the walk may be misconfigured", scanned, uncontainedWriteScanPackages, uncontainedWriteMinScannedFiles)
	}

	if len(failures) > 0 {
		t.Errorf("write-side containment guard violated — raw filesystem-write primitive outside the os.Root chokepoint:\n%s", strings.Join(failures, "\n"))
	}
}
