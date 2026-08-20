// destroy_test.go covers the gate's hermetic logic: everything destroy.go's pipeline and ownership
// resolvers do that needs no git, per the discussion's Hermetic tier split. isRegisteredLinkedWorktreeIn,
// isAnyWorktreeOf, primaryWeftBranch, and every dirtiness probe all spawn git and belong to batch 7's
// integration tier instead.
//
// Shape separation is asserted by construction, not by a test: a branchDirtiness value cannot be
// assigned to a pathRequest.dirtiness field (and vice versa) because pathRequest.dirtiness is typed
// pathDirtiness and branchRequest.dirtiness is typed branchDirtiness — the assignment simply does not
// compile, so there is no runnable test for it to fail.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestGate_CheckOrdering proves the pipeline stops at the first failing check, by submitting a
// request that would fail two checks and asserting the reported Check is the earlier one.
func TestGate_CheckOrdering(t *testing.T) {
	t.Run("ContainmentBeforeOwnership", func(t *testing.T) {
		container := t.TempDir()
		// target is outside container (fails containment) AND declares an ownership kind that would
		// also fail (ownedFabricHub against a plain directory) — the reported Check must still be
		// containment.
		outside := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    outside,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckContainment)
	})

	t.Run("OwnershipBeforeDirtiness", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		// target passes containment, fails ownership (not a hub); dirtiness declares a scope that
		// would also need probing (never reached, since ownership fails first — no git spawn here).
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckOwnership)
	})
}

// TestGate_Containment exercises refuseUncontainedPath's semantics through the gate.
func TestGate_Containment(t *testing.T) {
	tests := []struct {
		name        string
		buildTarget func(container string) string
	}{
		{
			name: "DotDot",
			buildTarget: func(container string) string {
				return filepath.Join(container, "..")
			},
		},
		{
			name: "DotDotSlashX",
			buildTarget: func(container string) string {
				sibling := filepath.Join(filepath.Dir(container), "sibling")
				if err := os.Mkdir(sibling, 0o755); err != nil && !os.IsExist(err) {
					t.Fatalf("mkdir sibling: %v", err)
				}
				return filepath.Join(container, "..", filepath.Base(sibling))
			},
		},
		{
			name: "EqualToContainer",
			buildTarget: func(container string) string {
				return container
			},
		},
		{
			// Card 12 names "." explicitly, distinct from EqualToContainer: this exercises
			// filepath.Join's Clean folding a literal "." component down to the container itself,
			// which happens to produce the same resolved path as EqualToContainer but through the
			// Clean path rather than an already-bare container string.
			name: "DotOnly",
			buildTarget: func(container string) string {
				return filepath.Join(container, ".")
			},
		},
		{
			name: "AbsoluteOutsideContainer",
			buildTarget: func(container string) string {
				outside := t.TempDir()
				return outside
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := t.TempDir()
			target := tt.buildTarget(container)
			req := pathRequest{
				what:      "test",
				container: container,
				target:    target,
				ownership: ownedFabricHub(),
				dirtiness: dirtyScopeTracked(),
			}
			err := checkPathRequest(req)
			assertRefusalCheck(t, err, CheckContainment)
		})
	}

	t.Run("WithinContainerPassesContainment", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		// Containment passes; ownership then fails (not a hub) — proves containment did not refuse.
		assertRefusalCheck(t, err, CheckOwnership)
	})

	// Card 12 names "both platform separators": refuseUncontainedPath resolves through
	// filepath.Rel, which is separator-native, so the forward slash and the backslash do not carry
	// the same escaping meaning on every OS. Each subtest below asserts the behaviour that is
	// actually correct for the separator it names, on whichever OS the test runs.
	t.Run("EscapeWithForwardSlash", func(t *testing.T) {
		container := t.TempDir()
		sibling := filepath.Join(filepath.Dir(container), "sibling-fwd")
		if err := os.Mkdir(sibling, 0o755); err != nil && !os.IsExist(err) {
			t.Fatalf("mkdir sibling: %v", err)
		}
		// "/" is a path separator on every OS Go supports (including Windows), so this must
		// escape the container everywhere.
		target := container + "/../" + filepath.Base(sibling)
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckContainment)
	})

	t.Run("EscapeWithBackslash", func(t *testing.T) {
		container := t.TempDir()
		// filepath.Join places one real "/" between container and the backslash sequence, so on a
		// platform where "\" is not a separator, this names a single, literally-backslashed child
		// of container rather than escaping it.
		target := filepath.Join(container, `..\sibling-back`)
		if runtime.GOOS == "windows" {
			// "\" is a path separator on Windows: create the real sibling directory the resolved
			// path points at, so the gate's absent-target no-op does not mask the containment check.
			sibling := filepath.Join(filepath.Dir(container), "sibling-back")
			if err := os.Mkdir(sibling, 0o755); err != nil && !os.IsExist(err) {
				t.Fatalf("mkdir sibling: %v", err)
			}
		} else {
			// On every other OS "\" is an ordinary filename character, not a separator, so the
			// literal string names a single oddly-named child directory of container.
			// Create that literal entry so the gate's absent-target no-op does not mask the check.
			if err := os.Mkdir(target, 0o755); err != nil && !os.IsExist(err) {
				t.Fatalf("mkdir literal target: %v", err)
			}
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		if runtime.GOOS == "windows" {
			// "\" is a path separator on Windows, so this escapes the container just like "/".
			assertRefusalCheck(t, err, CheckContainment)
		} else {
			// The literal entry does not actually leave the container — containment must pass,
			// and ownership fails next (not a hub), same as WithinContainerPassesContainment above.
			assertRefusalCheck(t, err, CheckOwnership)
		}
	})
}

// TestGate_ContainmentResolvesSymlinkedAncestors is R2's regression for the symlink-mediated
// containment bypass: a link planted at an intermediate segment of a gate target used to satisfy
// the containment check, because the check related NOMINAL paths through filepath.Rel and never
// resolved anything.
// Each subtest pins one half of the fix — the escape must be refused, and the two shapes that
// legitimately involve a link (a link AS the target, a real path under a symlinked container) must
// still pass — so a future simplification back to a bare filepath.Rel fails here rather than
// silently reopening the hole.
func TestGate_ContainmentResolvesSymlinkedAncestors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Symlink needs admin/Developer Mode on Windows; the junction path is covered by the integration tier")
	}

	t.Run("SymlinkedParentSegmentEscapesAndIsRefused", func(t *testing.T) {
		// This mirrors the shape R2's live repro used against removeLaunchers exactly, ownership
		// kind included: ownedUnderGeometryRoot is declared here deliberately rather than a kind
		// that fails anyway, because a kind that refuses on its own would leave the subtest passing
		// for the wrong reason and prove nothing about containment.
		hub := t.TempDir()
		root := filepath.Join(hub, launchersDirName)
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("mkdir geometry root: %v", err)
		}

		outside := t.TempDir()
		victim := filepath.Join(outside, "victim.txt")
		if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
			t.Fatalf("write victim: %v", err)
		}

		// <root>/slug links to a directory outside the hub, so <root>/slug/victim.txt is nominally
		// inside the geometry root and really outside the hub entirely.
		if err := os.Symlink(outside, filepath.Join(root, "slug")); err != nil {
			t.Fatalf("symlink slug: %v", err)
		}
		target := filepath.Join(root, "slug", "victim.txt")

		req := pathRequest{
			what:      "remove launcher script",
			container: root,
			target:    target,
			ownership: ownedUnderGeometryRoot(root),
			dirtiness: dirtinessNA("launcher scripts are generated artifacts, never edited content"),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckContainment)

		// Assert the executor itself refuses too, not merely the check helper, and that the file
		// outside the container survives — before the fix this call deleted it and reported success.
		rec := NewMutations(hub)
		if err := removePath(rec, req); err == nil {
			t.Fatal("removePath through a symlinked parent segment = nil; want a containment refusal")
		}
		if _, err := os.Stat(victim); err != nil {
			t.Errorf("victim outside the container was destroyed: %v", err)
		}
		if rec.Len() != 0 {
			t.Errorf("rec.Len() = %d; want 0 — a refusal records nothing", rec.Len())
		}
	})

	t.Run("LinkAsTargetItselfStillPassesContainment", func(t *testing.T) {
		container := t.TempDir()
		outside := t.TempDir()
		// The link's own final component is the target. Resolving it would relocate the target
		// outside container and refuse every junction removal, so containment must not resolve it.
		target := filepath.Join(container, "junction")
		if err := os.Symlink(outside, target); err != nil {
			t.Fatalf("symlink junction: %v", err)
		}

		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("irrelevant: this subtest reads containment's verdict off the later ownership refusal"),
		}
		// Containment passes; ownership then refuses, since a link to an empty dir is not a hub.
		assertRefusalCheck(t, checkPathRequest(req), CheckOwnership)
	})

	t.Run("SymlinkedContainerAncestorStillPassesContainment", func(t *testing.T) {
		// Both sides are resolved, so a container reached through a link (macOS's /var -> /private/var
		// is the everyday case) must not start refusing its own real children.
		realDir := t.TempDir()
		linkParent := t.TempDir()
		container := filepath.Join(linkParent, "via-link")
		if err := os.Symlink(realDir, container); err != nil {
			t.Fatalf("symlink container: %v", err)
		}
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir child: %v", err)
		}

		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("irrelevant: this subtest reads containment's verdict off the later ownership refusal"),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckOwnership)
	})

	t.Run("GeometryRootOwnershipRefusesASymlinkedRoot", func(t *testing.T) {
		// ownedUnderGeometryRoot is the one ownership kind with no independent resolved-path
		// authority to cross-check against, so a link standing where the geometry root belongs must
		// be refused by the base-name test running against the RESOLVED root.
		hub := t.TempDir()
		outside := t.TempDir()
		root := filepath.Join(hub, launchersDirName)
		if err := os.Symlink(outside, root); err != nil {
			t.Fatalf("symlink geometry root: %v", err)
		}

		ok, reason := resolvePathOwnership(ownedUnderGeometryRoot(root), filepath.Join(root, "slug"))
		if ok {
			t.Fatalf("resolvePathOwnership(ownedUnderGeometryRoot(<symlinked %s>)) = true; want false", launchersDirName)
		}
		if !strings.Contains(reason, "not a fabric geometry root") {
			t.Errorf("reason = %q; want it to name the geometry-root refusal", reason)
		}
	})
}

// TestGate_SlugValidation proves a derived-slug request is refused before anything is touched, and
// reported as a containment refusal.
func TestGate_SlugValidation(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"DotDot", ".."},
		{"ReservedBoardName", BoardDirName},
		{"WeftSuffixed", "task-weft"},
		{"Empty", ""},
		{"ContainsSeparator", "a/b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := t.TempDir()
			target := filepath.Join(container, "child")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatalf("mkdir target: %v", err)
			}
			req := pathRequest{
				what:      "test",
				container: container,
				target:    target,
				slug:      &slugSpec{name: tt.slug, junctionNames: nil},
				ownership: ownedFabricHub(),
				dirtiness: dirtyScopeTracked(),
			}
			err := checkPathRequest(req)
			assertRefusalCheck(t, err, CheckContainment)
		})
	}
}

// TestGate_Force proves force satisfies dirtiness and satisfies nothing else — not containment, not
// ownership. The containment case matters as much as the ownership case: "remove .." is a containment
// failure, so a force-satisfies-containment reading would bring it back behind a flag.
func TestGate_Force(t *testing.T) {
	t.Run("SatisfiesDirtiness", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFreshlyCreatedPath(createdToken{path: filepath.Clean(target), worktree: false}),
			dirtiness: dirtyScopeTracked(),
			force:     true,
		}
		// force:true means checkPathDirtiness returns without probing — no git spawn, still hermetic.
		if err := checkPathRequest(req); err != nil {
			t.Errorf("checkPathRequest() with force=true = %v; want nil", err)
		}
	})

	t.Run("DoesNotSatisfyContainment", func(t *testing.T) {
		container := t.TempDir()
		outside := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    outside,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
			force:     true,
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckContainment)
	})

	t.Run("DoesNotSatisfyOwnership", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
			force:     true,
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckOwnership)
	})
}

// TestGate_DirtinessNAEmptyReason proves dirtinessNA("") is a refusal, not a pass.
func TestGate_DirtinessNAEmptyReason(t *testing.T) {
	container := t.TempDir()
	target := filepath.Join(container, "child")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	req := pathRequest{
		what:      "test",
		container: container,
		target:    target,
		ownership: ownedFabricHub(),
		dirtiness: dirtinessNA(""),
	}
	err := checkPathRequest(req)
	assertRefusalCheck(t, err, CheckDirtiness)
}

// TestGate_ZeroValueDeclarationsAreRefusals proves an omitted ownership or dirtiness declaration is a
// loud failure, for both request shapes.
//
// The AbsentTarget subtests are R2's addition and the reason the shape check now runs ahead of the
// absent-target short-circuit: with the short-circuit first, a request declaring nothing at all
// passed the gate vacuously whenever its target happened not to exist — so "an omitted check is a
// loud failure" held only for targets that were there, which is exactly the case a new call site's
// first test is least likely to cover.
func TestGate_ZeroValueDeclarationsAreRefusals(t *testing.T) {
	t.Run("PathZeroOwnershipWithAbsentTarget", func(t *testing.T) {
		container := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    filepath.Join(container, "never-created"),
			dirtiness: dirtyScopeTracked(),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckOwnership)
	})

	t.Run("PathZeroDirtinessWithAbsentTarget", func(t *testing.T) {
		container := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    filepath.Join(container, "never-created"),
			ownership: ownedFabricHub(),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckDirtiness)
	})

	t.Run("PathEmptyNAReasonWithAbsentTarget", func(t *testing.T) {
		container := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    filepath.Join(container, "never-created"),
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA(""),
		}
		assertRefusalCheck(t, checkPathRequest(req), CheckDirtiness)
	})

	t.Run("WellFormedRequestOnAnAbsentTargetIsStillANoOpSuccess", func(t *testing.T) {
		// The counter-assertion, as load-bearing as the three above: moving the shape check ahead of
		// the absent-target rule must not turn the idempotent teardown sites (removePortal,
		// removeLaunchers, Remove's already-absent weft worktree) into hard failures.
		container := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    filepath.Join(container, "never-created"),
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("nothing is there to lose"),
		}
		if err := checkPathRequest(req); err != nil {
			t.Errorf("checkPathRequest on a well-formed request with an absent target = %v; want nil", err)
		}
	})

	t.Run("PathZeroOwnership", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			dirtiness: dirtyScopeTracked(),
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckOwnership)
	})

	t.Run("PathZeroDirtiness", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "child")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFabricHub(),
		}
		err := checkPathRequest(req)
		assertRefusalCheck(t, err, CheckDirtiness)
	})

	t.Run("BranchZeroOwnership", func(t *testing.T) {
		req := branchRequest{
			what:      "test",
			repoDir:   t.TempDir(),
			branch:    "task-weft",
			dirtiness: dirtyCheckedOutBranch(),
		}
		err := checkBranchRequest(req)
		assertRefusalCheck(t, err, CheckOwnership)
	})

	t.Run("BranchZeroDirtiness", func(t *testing.T) {
		// A hand-built Location is fine here — the zero-value dirtiness declaration refuses before
		// resolveManagedBranch ever runs, so primaryWeftBranch (which spawns git) is never reached.
		l := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "prime"}
		req := branchRequest{
			what:      "test",
			repoDir:   t.TempDir(),
			branch:    "task-weft",
			ownership: ownedManagedBranch(l, ""),
		}
		err := checkBranchRequest(req)
		assertRefusalCheck(t, err, CheckDirtiness)
	})
}

// TestGate_AbsentTargetIsNoOp proves an absent target is a no-op success, for every ownership kind,
// before any check runs.
func TestGate_AbsentTargetIsNoOp(t *testing.T) {
	container := t.TempDir()
	absent := filepath.Join(container, "does-not-exist")

	kinds := []pathOwnership{
		ownedRegisteredLinkedWorktree(container),
		ownedWarpCheckout(container),
		ownedFabricHub(),
		ownedUnderGeometryRoot(filepath.Join(container, launchersDirName)),
		ownedFreshlyCreatedPath(createdToken{path: absent, worktree: false}),
		ownedFreshlyCreatedWorktree(createdToken{path: absent, worktree: true}),
		ownedWiredJunction([]string{absent}, "/somewhere"),
		ownedDriftedWiredJunction([]string{absent}),
	}

	for i, own := range kinds {
		t.Run(fmt.Sprintf("kind-%d", i), func(t *testing.T) {
			req := pathRequest{
				what:      "test",
				container: container,
				target:    absent,
				ownership: own,
				dirtiness: dirtyScopeTracked(),
			}
			if err := checkPathRequest(req); err != nil {
				t.Errorf("checkPathRequest() on absent target = %v; want nil", err)
			}
		})
	}

	// removePath itself is a no-op too: it destroys nothing, and reports no error.
	req := pathRequest{
		what:      "test",
		container: container,
		target:    absent,
		ownership: ownedFabricHub(),
		dirtiness: dirtyScopeTracked(),
	}
	rec := NewMutations("")
	if err := removePath(rec, req); err != nil {
		t.Errorf("removePath() on absent target = %v; want nil", err)
	}
	if _, err := os.Lstat(absent); !os.IsNotExist(err) {
		t.Errorf("removePath() on absent target unexpectedly created something at %s", absent)
	}
	if got := rec.Len(); got != 0 {
		t.Errorf("removePath() on absent target recorded %d entries; want 0 (a successful no-op is not a removal)", got)
	}
}

// TestGate_TokenRoundTrip proves createExclusiveDir refuses an already-existing path, and that the
// token it returns authorises removal of exactly that path and no other.
func TestGate_TokenRoundTrip(t *testing.T) {
	container := t.TempDir()
	target := filepath.Join(container, "fresh")

	rec := NewMutations("")
	tok, err := createExclusiveDir(rec, target)
	if err != nil {
		t.Fatalf("createExclusiveDir(%s) = %v; want nil error", target, err)
	}
	if tok.path != filepath.Clean(target) || tok.worktree {
		t.Errorf("createExclusiveDir(%s) token = %+v; want path=%s worktree=false", target, tok, target)
	}

	if _, err := createExclusiveDir(rec, target); err == nil {
		t.Errorf("createExclusiveDir(%s) on an already-existing path = nil error; want EEXIST", target)
	}

	if ok, _ := resolvePathOwnership(ownedFreshlyCreatedPath(tok), target); !ok {
		t.Errorf("ownedFreshlyCreatedPath(tok) for %s = false; want true (the token authorises exactly this path)", target)
	}

	other := filepath.Join(container, "other")
	if ok, _ := resolvePathOwnership(ownedFreshlyCreatedPath(tok), other); ok {
		t.Errorf("ownedFreshlyCreatedPath(tok) for %s = true; want false (the token authorises only %s)", other, target)
	}

	// A directory token is rejected by ownedFreshlyCreatedWorktree — the worktree flag distinguishes
	// them.
	if ok, _ := resolvePathOwnership(ownedFreshlyCreatedWorktree(tok), target); ok {
		t.Errorf("ownedFreshlyCreatedWorktree(dirToken) for %s = true; want false", target)
	}
}

// TestGate_LinkKinds covers ownedWiredJunction and ownedDriftedWiredJunction directly.
func TestGate_LinkKinds(t *testing.T) {
	dir := t.TempDir()

	t.Run("WiredJunctionRefusesRealDirectory", func(t *testing.T) {
		realDir := filepath.Join(dir, "real")
		if err := os.Mkdir(realDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		ok, _ := resolvePathOwnership(ownedWiredJunction([]string{realDir}, "/anywhere"), realDir)
		if ok {
			t.Errorf("ownedWiredJunction on a real directory = true; want false")
		}
	})

	t.Run("WiredJunctionRefusesLinkOutsideWiredSet", func(t *testing.T) {
		target := filepath.Join(dir, "link-target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(dir, "unwired-link")
		if err := fslink.CreateDirLink(link, target); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		// wiredLinks does not name link.
		ok, _ := resolvePathOwnership(ownedWiredJunction([]string{filepath.Join(dir, "other")}, target), link)
		if ok {
			t.Errorf("ownedWiredJunction on a link outside the wired set = true; want false")
		}
	})

	t.Run("WiredJunctionRefusesLinkResolvingElsewhere", func(t *testing.T) {
		// R1's case: an operator's own symlink sits where fabric did not wire one.
		realTarget := filepath.Join(dir, "real-target")
		if err := os.Mkdir(realTarget, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		operatorTarget := filepath.Join(dir, "operator-target")
		if err := os.Mkdir(operatorTarget, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(dir, "wired-but-drifted-link")
		if err := fslink.CreateDirLink(link, operatorTarget); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		ok, _ := resolvePathOwnership(ownedWiredJunction([]string{link}, realTarget), link)
		if ok {
			t.Errorf("ownedWiredJunction on a link resolving elsewhere than expectedTarget = true; want false")
		}
	})

	t.Run("WiredJunctionAcceptsCorrectlyResolvingLink", func(t *testing.T) {
		target := filepath.Join(dir, "correct-target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(dir, "correct-link")
		if err := fslink.CreateDirLink(link, target); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		ok, reason := resolvePathOwnership(ownedWiredJunction([]string{link}, target), link)
		if !ok {
			t.Errorf("ownedWiredJunction on a correctly-resolving wired link = false (%s); want true", reason)
		}
	})

	t.Run("DriftedWiredJunctionAcceptsDanglingLink", func(t *testing.T) {
		link := filepath.Join(dir, "dangling-link")
		if err := fslink.CreateDirLink(link, filepath.Join(dir, "no-such-target")); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		ok, reason := resolvePathOwnership(ownedDriftedWiredJunction([]string{link}), link)
		if !ok {
			t.Errorf("ownedDriftedWiredJunction on a dangling wired link = false (%s); want true", reason)
		}
	})

	t.Run("DriftedWiredJunctionAcceptsMisPointedLink", func(t *testing.T) {
		wrongTarget := filepath.Join(dir, "wrong-target")
		if err := os.Mkdir(wrongTarget, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		link := filepath.Join(dir, "mispointed-link")
		if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		ok, reason := resolvePathOwnership(ownedDriftedWiredJunction([]string{link}), link)
		if !ok {
			t.Errorf("ownedDriftedWiredJunction on a mis-pointed wired link = false (%s); want true", reason)
		}
	})

	t.Run("DriftedWiredJunctionRefusesRealDirectory", func(t *testing.T) {
		realDir := filepath.Join(dir, "drifted-real-dir")
		if err := os.Mkdir(realDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		ok, _ := resolvePathOwnership(ownedDriftedWiredJunction([]string{realDir}), realDir)
		if ok {
			t.Errorf("ownedDriftedWiredJunction on a real directory = true; want false")
		}
	})
}

// TestGate_BestEffortPolicy proves surfaceRefusal's split: an operational failure is not matched, a
// *destructiveRefusal always is.
func TestGate_BestEffortPolicy(t *testing.T) {
	t.Run("OperationalFailureIsDiscarded", func(t *testing.T) {
		operational := fmt.Errorf("git exited nonzero")
		if got := surfaceRefusal(operational); got != nil {
			t.Errorf("surfaceRefusal(operational failure) = %v; want nil", got)
		}
	})

	t.Run("RefusalIsSurfaced", func(t *testing.T) {
		refusal := &destructiveRefusal{Check: CheckOwnership, What: "test", Target: "x", Reason: "no"}
		if got := surfaceRefusal(refusal); got != refusal {
			t.Errorf("surfaceRefusal(refusal) = %v; want the same refusal unchanged", got)
		}
	})

	t.Run("WrappedRefusalIsSurfaced", func(t *testing.T) {
		refusal := &destructiveRefusal{Check: CheckDirtiness, What: "test", Target: "x", Reason: "no"}
		wrapped := fmt.Errorf("context: %w", refusal)
		got := surfaceRefusal(wrapped)
		if got != wrapped {
			t.Errorf("surfaceRefusal(wrapped refusal) = %v; want the wrapped error unchanged", got)
		}
	})
}

// TestGate_RecordOnlyOnObservedEffect covers the record-only-on-observed-effect rule using only
// filesystem primitives already reachable from an untagged test in this package: removePath,
// removeLink, and createExclusiveDir. The git-spawning executors (removeGitWorktree, deleteBranch)
// are deliberately NOT covered here — an untagged test in this package may not spawn git per the
// Test Tier Purity Invariant — and their nonzero-exit-with-nil-error rule is asserted through
// this package's own live-state harness (the livestate_-prefixed package fabricengine_test files)
// tagged matrix in batch 7 instead.
func TestGate_RecordOnlyOnObservedEffect(t *testing.T) {
	t.Run("RemovePath_AbsentTargetRecordsNothing", func(t *testing.T) {
		container := t.TempDir()
		absent := filepath.Join(container, "does-not-exist")
		req := pathRequest{
			what:      "test",
			container: container,
			target:    absent,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		rec := NewMutations("")
		if err := removePath(rec, req); err != nil {
			t.Fatalf("removePath() on absent target = %v; want nil", err)
		}
		if got := rec.Len(); got != 0 {
			t.Errorf("removePath() on absent target recorded %d entries; want 0", got)
		}
	})

	t.Run("RemovePath_DirectoryRecordsRecursive", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "dir")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFreshlyCreatedPath(createdToken{path: filepath.Clean(target), worktree: false}),
			dirtiness: dirtinessNA("test"),
		}
		rec := NewMutations("")
		if err := removePath(rec, req); err != nil {
			t.Fatalf("removePath() on directory = %v; want nil", err)
		}
		entries := rec.Entries()
		if len(entries) != 1 || entries[0].Kind != KindPathRemoved || entries[0].Detail != "recursive" {
			t.Errorf("removePath() on directory recorded %+v; want exactly one path_removed with detail recursive", entries)
		}
	})

	t.Run("RemovePath_FileRecordsSingle", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "file")
		if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: container,
			target:    target,
			ownership: ownedFreshlyCreatedPath(createdToken{path: filepath.Clean(target), worktree: false}),
			dirtiness: dirtinessNA("test"),
		}
		rec := NewMutations("")
		if err := removePath(rec, req); err != nil {
			t.Fatalf("removePath() on file = %v; want nil", err)
		}
		entries := rec.Entries()
		if len(entries) != 1 || entries[0].Kind != KindPathRemoved || entries[0].Detail != "single" {
			t.Errorf("removePath() on file recorded %+v; want exactly one path_removed with detail single", entries)
		}
	})

	t.Run("RemovePath_RefusedRequestRecordsNothing", func(t *testing.T) {
		container := t.TempDir()
		outside := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    outside,
			ownership: ownedFabricHub(),
			dirtiness: dirtyScopeTracked(),
		}
		rec := NewMutations("")
		if err := removePath(rec, req); err == nil {
			t.Fatalf("removePath() on a containment-refused request = nil; want a refusal")
		}
		if got := rec.Len(); got != 0 {
			t.Errorf("removePath() on a refused request recorded %d entries; want 0", got)
		}
	})

	t.Run("RemoveLink_RefusedRequestRecordsNothing", func(t *testing.T) {
		container := t.TempDir()
		outside := t.TempDir()
		req := pathRequest{
			what:      "test",
			container: container,
			target:    outside,
			ownership: ownedFabricHub(),
			dirtiness: dirtinessNA("test"),
		}
		rec := NewMutations("")
		if err := removeLink(rec, req); err == nil {
			t.Fatalf("removeLink() on a containment-refused request = nil; want a refusal")
		}
		if got := rec.Len(); got != 0 {
			t.Errorf("removeLink() on a refused request recorded %d entries; want 0", got)
		}
	})

	t.Run("RemoveLink_AbsentTargetRecordsNothing", func(t *testing.T) {
		container := t.TempDir()
		absent := filepath.Join(container, "does-not-exist-link")
		req := pathRequest{
			what:      "test",
			container: container,
			target:    absent,
			ownership: ownedDriftedWiredJunction([]string{absent}),
			dirtiness: dirtinessNA("test"),
		}
		rec := NewMutations("")
		if err := removeLink(rec, req); err != nil {
			t.Fatalf("removeLink() on absent target = %v; want nil", err)
		}
		if got := rec.Len(); got != 0 {
			t.Errorf("removeLink() on absent target recorded %d entries; want 0 (fslink.Remove's own idempotence must not become a recorded removal)", got)
		}
	})

	t.Run("RemoveLink_PresentLinkRecordsOne", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		link := filepath.Join(dir, "link")
		if err := fslink.CreateDirLink(link, target); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		req := pathRequest{
			what:      "test",
			container: dir,
			target:    link,
			ownership: ownedWiredJunction([]string{link}, target),
			dirtiness: dirtinessNA("test"),
		}
		rec := NewMutations("")
		if err := removeLink(rec, req); err != nil {
			t.Fatalf("removeLink() on present link = %v; want nil", err)
		}
		entries := rec.Entries()
		if len(entries) != 1 || entries[0].Kind != KindLinkRemoved {
			t.Errorf("removeLink() on present link recorded %+v; want exactly one link_removed", entries)
		}
	})

	t.Run("CreateExclusiveDir_SuccessRecordsOne", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "fresh")
		rec := NewMutations("")
		if _, err := createExclusiveDir(rec, target); err != nil {
			t.Fatalf("createExclusiveDir() = %v; want nil", err)
		}
		entries := rec.Entries()
		if len(entries) != 1 || entries[0].Kind != KindDirCreated {
			t.Errorf("createExclusiveDir() recorded %+v; want exactly one dir_created", entries)
		}
	})

	t.Run("CreateExclusiveDir_EEXISTRecordsNothing", func(t *testing.T) {
		container := t.TempDir()
		target := filepath.Join(container, "already-there")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		rec := NewMutations("")
		if _, err := createExclusiveDir(rec, target); err == nil {
			t.Fatalf("createExclusiveDir() on an already-existing path = nil; want EEXIST")
		}
		if got := rec.Len(); got != 0 {
			t.Errorf("createExclusiveDir() on EEXIST recorded %d entries; want 0", got)
		}
	})

	// A nil recorder must never panic: every not-yet-threaded call site degrades to recording
	// nothing rather than crashing.
	t.Run("NilRecorderDoesNotPanic", func(t *testing.T) {
		container := t.TempDir()

		removeTarget := filepath.Join(container, "nil-rec-file")
		if err := os.WriteFile(removeTarget, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		removeReq := pathRequest{
			what:      "test",
			container: container,
			target:    removeTarget,
			ownership: ownedFreshlyCreatedPath(createdToken{path: filepath.Clean(removeTarget), worktree: false}),
			dirtiness: dirtinessNA("test"),
		}
		if err := removePath(nil, removeReq); err != nil {
			t.Errorf("removePath(nil, ...) = %v; want nil", err)
		}

		linkTarget := filepath.Join(container, "nil-rec-link-target")
		if err := os.Mkdir(linkTarget, 0o755); err != nil {
			t.Fatalf("mkdir link target: %v", err)
		}
		link := filepath.Join(container, "nil-rec-link")
		if err := fslink.CreateDirLink(link, linkTarget); err != nil {
			t.Fatalf("CreateDirLink: %v", err)
		}
		linkReq := pathRequest{
			what:      "test",
			container: container,
			target:    link,
			ownership: ownedWiredJunction([]string{link}, linkTarget),
			dirtiness: dirtinessNA("test"),
		}
		if err := removeLink(nil, linkReq); err != nil {
			t.Errorf("removeLink(nil, ...) = %v; want nil", err)
		}

		dirTarget := filepath.Join(container, "nil-rec-dir")
		if _, err := createExclusiveDir(nil, dirTarget); err != nil {
			t.Errorf("createExclusiveDir(nil, ...) = %v; want nil", err)
		}
	})
}

// assertRefusalCheck fails the test unless err is a *destructiveRefusal carrying want as its Check.
func assertRefusalCheck(t *testing.T, err error, want Check) {
	t.Helper()
	var refusal *destructiveRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v (%T); want a *destructiveRefusal", err, err)
	}
	if refusal.Check != want {
		t.Errorf("refusal.Check = %s; want %s", refusal.Check, want)
	}
}
