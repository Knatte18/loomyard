// destroy_test.go covers the gate's hermetic logic: everything destroy.go's pipeline and ownership
// resolvers do that needs no git, per the discussion's Hermetic tier split. isRegisteredLinkedWorktreeIn,
// isWarpCheckout, primaryWeftBranch, and every dirtiness probe all spawn git and belong to batch 7's
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
		assertRefusalCheck(t, err, checkContainment)
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
		assertRefusalCheck(t, err, checkOwnership)
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
			assertRefusalCheck(t, err, checkContainment)
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
		assertRefusalCheck(t, err, checkOwnership)
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
			assertRefusalCheck(t, err, checkContainment)
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
		assertRefusalCheck(t, err, checkContainment)
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
		assertRefusalCheck(t, err, checkOwnership)
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
	assertRefusalCheck(t, err, checkDirtiness)
}

// TestGate_ZeroValueDeclarationsAreRefusals proves an omitted ownership or dirtiness declaration is a
// loud failure, for both request shapes.
func TestGate_ZeroValueDeclarationsAreRefusals(t *testing.T) {
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
		assertRefusalCheck(t, err, checkOwnership)
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
		assertRefusalCheck(t, err, checkDirtiness)
	})

	t.Run("BranchZeroOwnership", func(t *testing.T) {
		req := branchRequest{
			what:      "test",
			repoDir:   t.TempDir(),
			branch:    "task-weft",
			dirtiness: dirtyCheckedOutBranch(),
		}
		err := checkBranchRequest(req)
		assertRefusalCheck(t, err, checkOwnership)
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
		assertRefusalCheck(t, err, checkDirtiness)
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
	if err := removePath(req); err != nil {
		t.Errorf("removePath() on absent target = %v; want nil", err)
	}
	if _, err := os.Lstat(absent); !os.IsNotExist(err) {
		t.Errorf("removePath() on absent target unexpectedly created something at %s", absent)
	}
}

// TestGate_TokenRoundTrip proves createExclusiveDir refuses an already-existing path, and that the
// token it returns authorises removal of exactly that path and no other.
func TestGate_TokenRoundTrip(t *testing.T) {
	container := t.TempDir()
	target := filepath.Join(container, "fresh")

	tok, err := createExclusiveDir(target)
	if err != nil {
		t.Fatalf("createExclusiveDir(%s) = %v; want nil error", target, err)
	}
	if tok.path != filepath.Clean(target) || tok.worktree {
		t.Errorf("createExclusiveDir(%s) token = %+v; want path=%s worktree=false", target, tok, target)
	}

	if _, err := createExclusiveDir(target); err == nil {
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
		refusal := &destructiveRefusal{Check: checkOwnership, What: "test", Target: "x", Reason: "no"}
		if got := surfaceRefusal(refusal); got != refusal {
			t.Errorf("surfaceRefusal(refusal) = %v; want the same refusal unchanged", got)
		}
	})

	t.Run("WrappedRefusalIsSurfaced", func(t *testing.T) {
		refusal := &destructiveRefusal{Check: checkDirtiness, What: "test", Target: "x", Reason: "no"}
		wrapped := fmt.Errorf("context: %w", refusal)
		got := surfaceRefusal(wrapped)
		if got != wrapped {
			t.Errorf("surfaceRefusal(wrapped refusal) = %v; want the wrapped error unchanged", got)
		}
	})
}

// assertRefusalCheck fails the test unless err is a *destructiveRefusal carrying want as its Check.
func assertRefusalCheck(t *testing.T, err error, want destructiveCheck) {
	t.Helper()
	var refusal *destructiveRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v (%T); want a *destructiveRefusal", err, err)
	}
	if refusal.Check != want {
		t.Errorf("refusal.Check = %s; want %s", refusal.Check, want)
	}
}
