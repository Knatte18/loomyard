// cliwire_test.go pins the shared wiring prologue's behaviour at tier 1: every case is untagged,
// spawns no process, and resolves no cwd.
//
// websterFixture and burlerFixture are test fixtures mirroring the real descriptors declared in
// webstercli and burlercli. They exist so this package's own tests never import either caller
// package -- the descriptor-carries-variance split would otherwise force cliwire's tests to depend
// on the very packages it must stay ignorant of.
//
// A divergence between a fixture here and its real descriptor is NOT caught by these tests, nor by
// the existing composition tests in webstercli/burlercli, nor by either package's
// cli_integration_test.go: after batch 2, no surviving test in either CLI package asserts a
// descriptor field's text (the retained TestWire_TargetDirRefusedInHubMode there only checks that
// the error mentions --target-dir). What catches it is the per-package
// TestWireModule_DescriptorIsVerbatim test batch 2 cards 6 and 7 add, which pins each real
// wireModule's own field values and produced refusals.

package cliwire

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// websterFixture stands in for webstercli's own wireModule descriptor, including webster's plan
// rules. Its MissingPlanRefusal reproduces webster's live message from wiring.go's wireStandalone,
// verbatim, including its two interpolated paths in their existing order -- the resolved plan
// directory first, the recourse location second.
var websterFixture = Module{
	Name:             "webster",
	StateArtifacts:   "state, locks, rendered prompts and trace logs",
	TargetRole:       "the repository it drives",
	TargetRecourse:   "Drive a target",
	HubTargetSubject: "the worktree is already the target",
	Plan: &PlanRules{
		DefaultPlanDir: func(base string) string { return filepath.Join(base, "_lyx", "plan") },
		MissingPlanRefusal: func(planDir, recourse string) string {
			return fmt.Sprintf("webster: standalone plan directory %s does not exist or contains no plan files -- there is no bootstrap and no empty-plan fallback. Place an authored plan at %s, which is what `run` requires (Master's own in-pane verbs are flagless and resolve that default); --plan-dir points the bracket and read-only verbs at a plan elsewhere, but `run` refuses it", planDir, recourse)
		},
	},
}

// burlerFixture stands in for burlercli's own wireModule descriptor. It carries no Plan, mirroring
// burler's own descriptor: burler parses no plan.
var burlerFixture = Module{
	Name:             "burler",
	StateArtifacts:   "instruction files, shuttle run directories and trace logs",
	TargetRole:       "the repository it reviews",
	TargetRecourse:   "Review a target",
	HubTargetSubject: "the anchor path is already the target",
}

// hash8AndStateDir returns both of standalonestate.Derive's values for target under the environment
// setStandaloneStateRoot has already installed. Every case that drives ResolveStandalone needs
// stateDir -- to seed the default plan directory, to assert against standalonegeom.LogsDir(stateDir)
// and standalonegeom.StencilsDir(stateDir), and to make the derived stencils path uncreatable -- so
// this one helper covers them all.
func hash8AndStateDir(t *testing.T, target string) (stateDir, hash8 string) {
	t.Helper()
	stateDir, hash8, err := standalonestate.Derive(target)
	if err != nil {
		t.Fatalf("standalonestate.Derive(%q) = %v; want nil error", target, err)
	}
	return stateDir, hash8
}

// seedGitRepositoryRoot marks dir as a git repository root by creating the ".git" entry
// RepositoryRootOf looks for, and returns dir. It writes no git objects and spawns no git.
func seedGitRepositoryRoot(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
	return dir
}

// seedPlanDir writes one minimal, non-empty ".md" file into dir, satisfying planDirHasContent so a
// standalone ResolveStandalone call reaches its own success path.
func seedPlanDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
}

// setStandaloneStateRoot redirects both XDG_STATE_HOME and LOCALAPPDATA to fresh t.TempDir() values,
// so a case that reaches standalonestate.Derive stays hermetic. Not t.Parallel() -- t.Setenv panics
// under a parallel test.
func setStandaloneStateRoot(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
}

// TestModule_ResolveStandaloneTarget covers target resolution's five cases, table-driven over both
// fixtures where the refusal message differs.
func TestModule_ResolveStandaloneTarget(t *testing.T) {
	t.Parallel()

	for _, fixture := range []Module{websterFixture, burlerFixture} {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			repo := seedGitRepositoryRoot(t, t.TempDir())
			if err := os.WriteFile(filepath.Join(repo, "notadir.txt"), []byte("x"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			t.Run("UnsetFlagResolvesToCwdAndIsNeverStatted", func(t *testing.T) {
				// cwd itself does not need to exist for this to succeed, which is what proves it is
				// never stat'd.
				cwd := filepath.Join(t.TempDir(), "does-not-exist")
				got, err := fixture.resolveStandaloneTarget(cwd, "")
				if err != nil {
					t.Fatalf("resolveStandaloneTarget(%q, \"\") = %v; want nil error", cwd, err)
				}
				if want := standalonestate.Normalize(cwd); got != want {
					t.Errorf("resolveStandaloneTarget(%q, \"\") = %q; want %q", cwd, got, want)
				}
			})

			t.Run("AbsoluteValueIsCleaned", func(t *testing.T) {
				messy := repo + string(filepath.Separator) + "."
				got, err := fixture.resolveStandaloneTarget("", messy)
				if err != nil {
					t.Fatalf("resolveStandaloneTarget(\"\", %q) = %v; want nil error", messy, err)
				}
				if want := standalonestate.Normalize(repo); got != want {
					t.Errorf("resolveStandaloneTarget(\"\", %q) = %q; want %q", messy, got, want)
				}
			})

			t.Run("RelativeValueResolvesAgainstToldCwd", func(t *testing.T) {
				parent := filepath.Dir(repo)
				rel, err := filepath.Rel(parent, repo)
				if err != nil {
					t.Fatalf("filepath.Rel() error = %v", err)
				}
				got, err := fixture.resolveStandaloneTarget(parent, rel)
				if err != nil {
					t.Fatalf("resolveStandaloneTarget(%q, %q) = %v; want nil error", parent, rel, err)
				}
				if want := standalonestate.Normalize(repo); got != want {
					t.Errorf("resolveStandaloneTarget(%q, %q) = %q; want %q", parent, rel, got, want)
				}
			})

			t.Run("AbsentPathIsRefused", func(t *testing.T) {
				_, err := fixture.resolveStandaloneTarget(repo, "reposs")
				if err == nil {
					t.Fatal("resolveStandaloneTarget() error = nil; want a refusal -- this silently resolves to the enclosing repository")
				}
				wantPrefix := fmt.Sprintf("%s: --target-dir reposs (resolved to %s) cannot be read: ", fixture.Name, filepath.Join(repo, "reposs"))
				wantSuffix := " -- a target that is not there is not an empty target, it silently resolves to whichever repository encloses it"
				if !strings.HasPrefix(err.Error(), wantPrefix) {
					t.Errorf("resolveStandaloneTarget() error = %q; want prefix %q", err.Error(), wantPrefix)
				}
				if !strings.HasSuffix(err.Error(), wantSuffix) {
					t.Errorf("resolveStandaloneTarget() error = %q; want suffix %q", err.Error(), wantSuffix)
				}
			})

			t.Run("FileIsRefused", func(t *testing.T) {
				_, err := fixture.resolveStandaloneTarget(repo, "notadir.txt")
				if err == nil {
					t.Fatal("resolveStandaloneTarget() error = nil; want a refusal")
				}
				want := fmt.Sprintf("%s: --target-dir notadir.txt (resolved to %s) is not a directory -- the standalone target is a repository to drive, and a file resolves to whichever repository encloses it", fixture.Name, filepath.Join(repo, "notadir.txt"))
				if err.Error() != want {
					t.Errorf("resolveStandaloneTarget() error = %q; want %q", err.Error(), want)
				}
			})
		})
	}
}

// TestRepositoryRootOf covers the repository-root lift: a subdirectory resolves to the repository
// root, the nearest ".git" wins over the topmost, a directory with no repository above it is
// returned unchanged, and a ".git" FILE (as a linked worktree records it) counts as a repository
// root.
func TestRepositoryRootOf(t *testing.T) {
	t.Parallel()

	t.Run("SubdirectoryLiftsToRoot", func(t *testing.T) {
		t.Parallel()
		root := seedGitRepositoryRoot(t, t.TempDir())
		sub := filepath.Join(root, "src", "inner")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if got := RepositoryRootOf(sub); got != root {
			t.Errorf("RepositoryRootOf(%q) = %q; want %q", sub, got, root)
		}
	})

	t.Run("NearestRepositoryWinsOverTopmost", func(t *testing.T) {
		t.Parallel()
		outer := seedGitRepositoryRoot(t, t.TempDir())
		inner := seedGitRepositoryRoot(t, filepath.Join(outer, "nested"))
		sub := filepath.Join(inner, "src")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if got := RepositoryRootOf(sub); got != inner {
			t.Errorf("RepositoryRootOf(%q) = %q; want the nearest repository %q, not the outer one %q", sub, got, inner, outer)
		}
	})

	t.Run("NoRepositoryAboveIsReturnedUnchanged", func(t *testing.T) {
		t.Parallel()
		plain := t.TempDir()
		if got := RepositoryRootOf(plain); got != plain {
			t.Errorf("RepositoryRootOf(%q) = %q; want it unchanged", plain, got)
		}
	})

	t.Run("GitFileCountsAsARepositoryRoot", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.git/worktrees/x"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		sub := filepath.Join(dir, "src")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if got := RepositoryRootOf(sub); got != dir {
			t.Errorf("RepositoryRootOf(%q) = %q; want %q -- a .git FILE, as a linked worktree records it, still counts", sub, got, dir)
		}
	})
}

// TestModule_RefuseNestedStandaloneGeometry covers the nested-geometry refusal over both fixtures,
// asserting both wordings verbatim -- this is what pins the per-CLI noun phrases as descriptor data.
func TestModule_RefuseNestedStandaloneGeometry(t *testing.T) {
	t.Parallel()

	for _, fixture := range []Module{websterFixture, burlerFixture} {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()

			t.Run("StateDirInsideTargetIsRefusedWithTheStateHomeLever", func(t *testing.T) {
				target := t.TempDir()
				stateDir := filepath.Join(target, "state-home")
				err := fixture.refuseNestedStandaloneGeometry(target, stateDir)
				if err == nil {
					t.Fatal("refuseNestedStandaloneGeometry() error = nil; want a refusal")
				}
				want := fmt.Sprintf("%s: the derived state directory %s lies inside the standalone target %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. The state home is nested under the target -- a repository rooted at your home directory is the usual cause. Point XDG_STATE_HOME (LOCALAPPDATA on Windows) at a directory outside %s and re-run", fixture.Name, stateDir, target, fixture.StateArtifacts, fixture.TargetRole, target)
				if err.Error() != want {
					t.Errorf("refuseNestedStandaloneGeometry() error = %q; want %q", err.Error(), want)
				}
			})

			t.Run("TargetInsideStateDirIsRefusedWithTheTargetLever", func(t *testing.T) {
				stateDir := t.TempDir()
				target := filepath.Join(stateDir, "checkout")
				err := fixture.refuseNestedStandaloneGeometry(target, stateDir)
				if err == nil {
					t.Fatal("refuseNestedStandaloneGeometry() error = nil; want a refusal")
				}
				want := fmt.Sprintf("%s: the standalone target %s lies inside the derived state directory %s: standalone mode keeps its %s strictly outside %s, so the two must be disjoint. %s outside the state home, or point XDG_STATE_HOME (LOCALAPPDATA on Windows) elsewhere, and re-run", fixture.Name, target, stateDir, fixture.StateArtifacts, fixture.TargetRole, fixture.TargetRecourse)
				if err.Error() != want {
					t.Errorf("refuseNestedStandaloneGeometry() error = %q; want %q", err.Error(), want)
				}
			})

			t.Run("DisjointPathsPass", func(t *testing.T) {
				if err := fixture.refuseNestedStandaloneGeometry(t.TempDir(), t.TempDir()); err != nil {
					t.Errorf("refuseNestedStandaloneGeometry() = %v; want nil for disjoint paths", err)
				}
			})
		})
	}
}

// TestModule_RefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome is R6-15's regression
// test. The target has already been through standalonestate.Normalize with every symlink resolved,
// while the derived state directory had not, so a state home that reaches INSIDE the target only
// through a symlink used to read as disjoint -- and lyx then wrote its state tree, run locks and
// trace logs into the operator's own checkout, which is precisely what this guard exists to
// prevent. The leaf (<stateHome>/lyx/<hash8>) is deliberately absent: a first run's own leaf does
// not exist yet, which is exactly when plain EvalSymlinks gives up and falls back to Clean.
func TestModule_RefuseNestedStandaloneGeometry_SeesThroughASymlinkedStateHome(t *testing.T) {
	t.Parallel()

	target := standalonestate.Normalize(t.TempDir())
	inside := filepath.Join(target, "state-home")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", inside, err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Skipf("symlinks unavailable on this host: %v", err)
	}
	stateDir := filepath.Join(link, "lyx", "abcd1234")

	if err := websterFixture.refuseNestedStandaloneGeometry(target, stateDir); err == nil {
		t.Errorf("refuseNestedStandaloneGeometry(%q, %q) = nil; want a refusal -- the state home is nested under the target through a symlink", target, stateDir)
	}
}

// TestSamePlanDirAndResolvePlanDir covers the default-vs-override recognition rules both functions
// share: a "." or trailing-separator spelling of the default is the default, not an override; a
// symlinked spelling of the default likewise (the second half of R6-15); an empty told value
// resolves to the default with overridden false; and a genuinely different told directory resolves
// to itself with overridden true.
func TestSamePlanDirAndResolvePlanDir(t *testing.T) {
	t.Parallel()

	t.Run("DotSpellingIsTheDefault", func(t *testing.T) {
		t.Parallel()
		def := t.TempDir()
		told := def + string(filepath.Separator) + "."
		if !SamePlanDir(told, def) {
			t.Errorf("SamePlanDir(%q, %q) = false; want true", told, def)
		}
		planDir, overridden := ResolvePlanDir(told, def)
		if overridden {
			t.Error("ResolvePlanDir() overridden = true; want false for a \".\" spelling of the default")
		}
		if planDir != told {
			t.Errorf("ResolvePlanDir() planDir = %q; want the told spelling %q", planDir, told)
		}
	})

	t.Run("TrailingSeparatorSpellingIsTheDefault", func(t *testing.T) {
		t.Parallel()
		def := t.TempDir()
		told := def + string(filepath.Separator)
		if !SamePlanDir(told, def) {
			t.Errorf("SamePlanDir(%q, %q) = false; want true", told, def)
		}
		if _, overridden := ResolvePlanDir(told, def); overridden {
			t.Error("ResolvePlanDir() overridden = true; want false for a trailing-separator spelling of the default")
		}
	})

	t.Run("SymlinkedSpellingIsTheDefault", func(t *testing.T) {
		t.Parallel()
		def := standalonestate.Normalize(t.TempDir())
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(def, link); err != nil {
			t.Skipf("symlinks unavailable on this host: %v", err)
		}
		if !SamePlanDir(link, def) {
			t.Errorf("SamePlanDir(%q, %q) = false; want true -- a symlinked spelling of the default", link, def)
		}
		if _, overridden := ResolvePlanDir(link, def); overridden {
			t.Error("ResolvePlanDir() overridden = true; want false for a symlinked spelling of the default")
		}
	})

	t.Run("EmptyToldResolvesToDefaultWithOverriddenFalse", func(t *testing.T) {
		t.Parallel()
		def := t.TempDir()
		planDir, overridden := ResolvePlanDir("", def)
		if planDir != def {
			t.Errorf("ResolvePlanDir(\"\", %q) planDir = %q; want %q", def, planDir, def)
		}
		if overridden {
			t.Error("ResolvePlanDir(\"\", ...) overridden = true; want false")
		}
	})

	t.Run("GenuinelyDifferentToldResolvesToItselfWithOverriddenTrue", func(t *testing.T) {
		t.Parallel()
		def := t.TempDir()
		other := t.TempDir()
		planDir, overridden := ResolvePlanDir(other, def)
		if planDir != other {
			t.Errorf("ResolvePlanDir(%q, %q) planDir = %q; want %q", other, def, planDir, other)
		}
		if !overridden {
			t.Error("ResolvePlanDir() overridden = false; want true for a genuinely different directory")
		}
	})
}

// TestResolveStandalone_PlanRules covers plan-rule resolution driven through ResolveStandalone
// itself: a missing plan directory, an empty one, and one holding no "*.md" files all produce the
// same refusal naming the DEFAULT location even when --plan-dir moved the plan off it, and a nil
// Plan field (the burlerFixture case) means the prologue performs no plan check at all.
func TestResolveStandalone_PlanRules(t *testing.T) {
	t.Run("MissingEmptyAndNoMarkdownAllProduceTheSameRefusal", func(t *testing.T) {
		tests := []struct {
			name string
			seed func(t *testing.T, dir string)
		}{
			{"MissingDirectory", func(t *testing.T, dir string) {}},
			{"EmptyDirectory", func(t *testing.T, dir string) {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}},
			{"NoMarkdownFiles", func(t *testing.T, dir string) {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
				if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				setStandaloneStateRoot(t)
				t.Cleanup(func() { logger.SetDurableSinkDir("") })
				target := t.TempDir()
				stateDir, _ := hash8AndStateDir(t, target)
				defaultPlanDir := filepath.Join(stateDir, "_lyx", "plan")
				tt.seed(t, defaultPlanDir)

				_, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
				if err == nil {
					t.Fatal("ResolveStandalone() error = nil; want the missing-plan refusal")
				}
				want := fmt.Sprintf("webster: standalone plan directory %s does not exist or contains no plan files -- there is no bootstrap and no empty-plan fallback. Place an authored plan at %s, which is what `run` requires (Master's own in-pane verbs are flagless and resolve that default); --plan-dir points the bracket and read-only verbs at a plan elsewhere, but `run` refuses it", defaultPlanDir, defaultPlanDir)
				if err.Error() != want {
					t.Errorf("ResolveStandalone() error = %q; want %q", err.Error(), want)
				}
			})
		}
	})

	t.Run("RefusalNamesTheDefaultLocationEvenWhenPlanDirMovedThePlan", func(t *testing.T) {
		setStandaloneStateRoot(t)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		target := t.TempDir()
		stateDir, _ := hash8AndStateDir(t, target)
		defaultPlanDir := filepath.Join(stateDir, "_lyx", "plan")
		override := t.TempDir()
		// override is deliberately never seeded: the refusal fires against the RESOLVED planDir
		// (the override), but its recourse must still NAME the default location, which is what
		// `run` requires.

		_, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target, PlanDirFlag: override})
		if err == nil {
			t.Fatal("ResolveStandalone() error = nil; want the missing-plan refusal")
		}
		if !strings.Contains(err.Error(), defaultPlanDir) {
			t.Errorf("ResolveStandalone() error = %q; want it to name the default plan directory %q that `run` requires", err.Error(), defaultPlanDir)
		}
		if !strings.Contains(err.Error(), override) {
			t.Errorf("ResolveStandalone() error = %q; want it to also name the moved plan directory %q that was checked for content", err.Error(), override)
		}
	})

	t.Run("NilPlanSkipsTheCheckEntirely", func(t *testing.T) {
		setStandaloneStateRoot(t)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		target := t.TempDir()

		result, err := burlerFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
		if err != nil {
			t.Fatalf("ResolveStandalone() = %v; want nil -- burlerFixture carries no Plan", err)
		}
		if result.PlanDir != "" || result.DefaultPlanDir != "" || result.PlanDirOverridden {
			t.Errorf("ResolveStandalone() result = %+v; want PlanDir, DefaultPlanDir and PlanDirOverridden at their zero values", result)
		}
	})
}

// TestResolveStandalone_SinkRedirectOrdering pins the prologue's ordering obligation: on success the
// durable sink is pointed at standalonegeom.LogsDir(stateDir) with the worktree root at the target,
// and on a prologue that fails at the nested-geometry refusal, the sentinel sink set before the call
// is still the one that receives the record -- proving the redirect sits after the guard.
func TestResolveStandalone_SinkRedirectOrdering(t *testing.T) {
	t.Run("SuccessRedirectsToStandaloneLogsDir", func(t *testing.T) {
		setStandaloneStateRoot(t)
		target := t.TempDir()
		stateDir, _ := hash8AndStateDir(t, target)
		seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

		sentinelDir := t.TempDir()
		logger.SetDurableSinkDir(sentinelDir)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })

		result, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
		if err != nil {
			t.Fatalf("ResolveStandalone() = %v; want nil", err)
		}

		sentinelFiles, err := filepath.Glob(filepath.Join(sentinelDir, "trace-*.log"))
		if err != nil {
			t.Fatalf("glob %s: %v", sentinelDir, err)
		}
		if len(sentinelFiles) != 0 {
			t.Errorf("sentinel dir %s gained %v; want it empty -- some statement above the sink redirect logged, arming the sink before the redirect could bind it", sentinelDir, sentinelFiles)
		}

		logger.Info("cliwire_test: arm the sink")

		wantDir := standalonegeom.LogsDir(result.StateDir)
		matches, err := filepath.Glob(filepath.Join(wantDir, "trace-*.log"))
		if err != nil {
			t.Fatalf("glob %s: %v", wantDir, err)
		}
		if len(matches) == 0 {
			t.Errorf("no trace-*.log file under %s; want ResolveStandalone to have redirected the durable sink there", wantDir)
		}
	})

	t.Run("NestedGeometryRefusalLeavesTheSentinelSinkUntouched", func(t *testing.T) {
		target := t.TempDir()
		t.Setenv("XDG_STATE_HOME", filepath.Join(target, ".local", "state"))
		t.Setenv("LOCALAPPDATA", filepath.Join(target, "AppData", "Local"))

		sentinelDir := t.TempDir()
		logger.SetDurableSinkDir(sentinelDir)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })

		_, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
		if err == nil {
			t.Fatal("ResolveStandalone() error = nil; want the nested-geometry refusal")
		}

		logger.Info("cliwire_test: arm the sink")
		matches, err := filepath.Glob(filepath.Join(sentinelDir, "trace-*.log"))
		if err != nil {
			t.Fatalf("glob %s: %v", sentinelDir, err)
		}
		if len(matches) == 0 {
			t.Error("no trace-*.log file under the sentinel dir; want the nesting refusal to fire before ResolveStandalone redirects the durable sink into the target")
		}
	})
}

// TestResolveStandalone_Stencils covers the stencils resolve-and-seed step: the derived default is
// seeded on disk, an explicitly-told stencils directory is returned as given and gains no entries,
// and a seed failure on the derived default is a hard error naming both the module and the
// directory.
func TestResolveStandalone_Stencils(t *testing.T) {
	t.Run("DerivedDefaultIsSeeded", func(t *testing.T) {
		setStandaloneStateRoot(t)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		target := t.TempDir()
		stateDir, _ := hash8AndStateDir(t, target)
		seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

		result, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
		if err != nil {
			t.Fatalf("ResolveStandalone() = %v; want nil", err)
		}
		wantDir := standalonegeom.StencilsDir(stateDir)
		if result.StencilsDir != wantDir {
			t.Errorf("ResolveStandalone() StencilsDir = %q; want %q", result.StencilsDir, wantDir)
		}
		entries, err := os.ReadDir(wantDir)
		if err != nil {
			t.Fatalf("ReadDir(%s) error = %v", wantDir, err)
		}
		if len(entries) == 0 {
			t.Error("derived stencils directory is empty; want it seeded")
		}
	})

	t.Run("ExplicitlyToldStencilsDirIsReturnedAsGivenAndGainsNoEntries", func(t *testing.T) {
		setStandaloneStateRoot(t)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		target := t.TempDir()
		stateDir, _ := hash8AndStateDir(t, target)
		seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

		told := t.TempDir()
		result, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target, StencilsDirFlag: told})
		if err != nil {
			t.Fatalf("ResolveStandalone() = %v; want nil", err)
		}
		if result.StencilsDir != told {
			t.Errorf("ResolveStandalone() StencilsDir = %q; want the told directory %q, unchanged", result.StencilsDir, told)
		}
		entries, err := os.ReadDir(told)
		if err != nil {
			t.Fatalf("ReadDir(%s) error = %v", told, err)
		}
		if len(entries) != 0 {
			t.Errorf("told stencils directory gained entries %v; want it left untouched -- an explicit override is read, never written", entries)
		}
	})

	t.Run("SeedFailureOnTheDerivedDefaultIsAHardErrorNamingModuleAndDirectory", func(t *testing.T) {
		setStandaloneStateRoot(t)
		t.Cleanup(func() { logger.SetDurableSinkDir("") })
		target := t.TempDir()
		stateDir, _ := hash8AndStateDir(t, target)

		// Make the derived stencils directory uncreatable: write a regular file at its own parent,
		// <stateDir>/_lyx, so stencilstore.Reconcile's own MkdirAll fails. The plan directory is
		// deliberately never seeded: ResolveStandalone's stencils step runs before its plan step and
		// this case never reaches the latter.
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", stateDir, err)
		}
		lyxDir := filepath.Join(stateDir, "_lyx")
		if err := os.WriteFile(lyxDir, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", lyxDir, err)
		}

		_, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
		if err == nil {
			t.Fatal("ResolveStandalone() error = nil; want the stencils seed failure")
		}
		wantDir := standalonegeom.StencilsDir(stateDir)
		if !strings.Contains(err.Error(), "webster") {
			t.Errorf("ResolveStandalone() error = %q; want it to name the module %q", err.Error(), "webster")
		}
		if !strings.Contains(err.Error(), wantDir) {
			t.Errorf("ResolveStandalone() error = %q; want it to name the stencils directory %q", err.Error(), wantDir)
		}
	})
}
