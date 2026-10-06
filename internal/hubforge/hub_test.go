//go:build integration

// hub_test.go proves the bare-pair template builder and the CloneAndWire-backed hub factory each do
// what their doc comments claim: the template carries the symbolic-ref gotcha's fix and a genuinely
// empty weft bare, and NewHub produces a real, fully-wired fabric hub rather than a hand-assembled
// stand-in — the whole reason NewHub calls CloneAndWire instead of CloneHub alone.
// It also proves the helpers a built hub offers — the config seeders, AddPair, AddPairWith and
// OpenFabric — and that hub teardown is safe.

package hubforge

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"gopkg.in/yaml.v3"
)

func TestBuildBareTemplate(t *testing.T) {
	t.Parallel()

	warpBare, weftBare := buildBareTemplate()

	t.Run("WarpHEADResolvesToMain", func(t *testing.T) {
		head := gitkit.Git(t, warpBare, "symbolic-ref", "HEAD")
		if head != "refs/heads/main" {
			t.Errorf("warp bare HEAD = %q; want refs/heads/main", head)
		}
	})

	t.Run("WarpCommitCarriesRootAndBackendEntries", func(t *testing.T) {
		out := gitkit.Git(t, warpBare, "ls-tree", "-r", "--name-only", "main")
		entries := strings.Split(out, "\n")
		var hasRoot, hasBackend bool
		for _, e := range entries {
			if e == "README" {
				hasRoot = true
			}
			if strings.HasPrefix(e, "backend/") {
				hasBackend = true
			}
		}
		if !hasRoot {
			t.Errorf("warp bare main tree = %v; want a root README entry", entries)
		}
		if !hasBackend {
			t.Errorf("warp bare main tree = %v; want a backend/ entry", entries)
		}
	})

	t.Run("WeftBareIsGenuinelyEmpty", func(t *testing.T) {
		out := gitkit.Git(t, weftBare, "for-each-ref")
		if out != "" {
			t.Errorf("weft bare for-each-ref = %q; want no refs at all", out)
		}
	})
}

// TestNewHub runs the factory and helper checks over one hub per anchor, "." and "backend".
// Running at "backend" is the point of the anchored hub: a "."-only test passes even when a seeding or
// resolution base is wrong, because there the anchored and un-anchored weft paths coincide.
// Steps run serially in this order: the config steps come before the steps that add pairs, and the
// fabric-config seed comes last because it changes the branch prefix later pairs would inherit.
// Each anchor's scenario calls t.Parallel; no step does, because they share the one hub.
func TestNewHub(t *testing.T) {
	t.Parallel()

	for _, anchor := range []string{".", "backend"} {
		t.Run(anchor, func(t *testing.T) {
			t.Parallel()

			h := NewHub(t, anchor)

			steps := []struct {
				name string
				run  func(t *testing.T)
			}{
				{"IsARealHub", func(t *testing.T) {
					assertRealHub(t, h)

					if _, err := os.Stat(h.PrimeWorktree()); err != nil {
						t.Errorf("prime warp worktree missing at %s: %v", h.PrimeWorktree(), err)
					}
					if _, err := os.Stat(h.PrimeWeft()); err != nil {
						t.Errorf("weft sibling missing at %s: %v", h.PrimeWeft(), err)
					}

					primeCwd := filepath.Join(h.PrimeWorktree(), h.Anchor)
					l, err := lyxcwd.Resolve(primeCwd)
					if err != nil {
						t.Fatalf("lyxcwd.Resolve(%s): %v", primeCwd, err)
					}
					if l.AnchorRel != anchor {
						t.Errorf("resolved AnchorRel = %q; want %q", l.AnchorRel, anchor)
					}

					if rooted := anchor == "."; (h.WeftBase == h.PrimeWeft()) != rooted {
						t.Errorf("h.WeftBase = %s, h.PrimeWeft() = %s at the %q anchor; want them equal only at the root anchor", h.WeftBase, h.PrimeWeft(), anchor)
					}
				}},
				{"ConfigMaterializedWithoutSeeding", func(t *testing.T) {
					// A freshly built hub already carries a materialized config file for at least one
					// registered module without any seeding call. This is what licenses deleting a
					// SeedConfig call rather than retargeting it, so it is not optional colour.
					const module = "loom"
					template, ok := configreg.Template(module)
					if !ok {
						t.Fatalf("configreg.Template(%q): module not registered", module)
					}

					configPath := configengine.ConfigFile(h.Location.AnchorPath(), module)
					got, err := os.ReadFile(configPath)
					if err != nil {
						t.Fatalf("read %s: %v", configPath, err)
					}
					if len(got) == 0 {
						t.Fatalf("%s: want non-empty content, got none", configPath)
					}

					var gotDoc, wantDoc any
					if err := yaml.Unmarshal(got, &gotDoc); err != nil {
						t.Fatalf("parse %s: %v", configPath, err)
					}
					if err := yaml.Unmarshal([]byte(template()), &wantDoc); err != nil {
						t.Fatalf("parse %s's registered template: %v", module, err)
					}
					if !reflect.DeepEqual(wantDoc, gotDoc) {
						t.Errorf("%s content = %#v; want it to match the registered %s template %#v", configPath, gotDoc, module, wantDoc)
					}
				}},
				{"SeedConfigRedundantSeedDoesNotFatal", func(t *testing.T) {
					// SeedConfig returns normally when handed a seed byte-identical to what the clone
					// already committed -- a seed that stages nothing, exactly the shape its --allow-empty
					// commit exists to tolerate -- and a genuinely different seed still lands on disk,
					// proving --allow-empty did not turn the helper into a no-op.
					loomConfigPath := configengine.ConfigFile(h.WeftBase, "loom")
					original, err := os.ReadFile(loomConfigPath)
					if err != nil {
						t.Fatalf("read already-materialised loom config %s: %v", loomConfigPath, err)
					}

					SeedConfig(t, h, map[string]string{"loom": string(original)})

					afterRedundantSeed, err := os.ReadFile(loomConfigPath)
					if err != nil {
						t.Fatalf("read loom config after redundant seed %s: %v", loomConfigPath, err)
					}
					if string(afterRedundantSeed) != string(original) {
						t.Errorf("loom config after redundant seed = %q; want unchanged %q", afterRedundantSeed, original)
					}

					const different = "different: true\n"
					SeedConfig(t, h, map[string]string{"loom": different})

					afterDifferentSeed, err := os.ReadFile(loomConfigPath)
					if err != nil {
						t.Fatalf("read loom config after different seed %s: %v", loomConfigPath, err)
					}
					if string(afterDifferentSeed) != different {
						t.Errorf("loom config after different seed = %q; want %q", afterDifferentSeed, different)
					}
				}},
				{"SeedConfigVisibleFromWarpSide", func(t *testing.T) {
					// An override seeded with SeedConfig reads back through the warp-side _lyx junction.
					const module = "loom"
					const override = "hubforge-seeded-override: true\n"
					SeedConfig(t, h, map[string]string{module: override})

					configPath := configengine.ConfigFile(h.Location.AnchorPath(), module)
					got, err := os.ReadFile(configPath)
					if err != nil {
						t.Fatalf("read %s: %v", configPath, err)
					}
					if string(got) != override {
						t.Errorf("%s content = %q; want the seeded override %q", configPath, got, override)
					}
				}},
				{"AddPairWiresPortalAndLauncher", func(t *testing.T) {
					const slug = "px"
					AddPair(t, h, slug)

					portal := fabricengine.PortalLink(h.Location, slug)
					if _, err := os.Stat(portal); err != nil {
						t.Errorf("pair portal link missing at %s: %v", portal, err)
					}
					if got := h.PairPortalLink(slug); got != portal {
						t.Errorf("h.PairPortalLink(%s) = %s; want %s", slug, got, portal)
					}

					launcherDir := fabricengine.LauncherDir(h.Location, slug)
					if _, err := os.Stat(launcherDir); err != nil {
						t.Errorf("pair launcher dir missing at %s: %v", launcherDir, err)
					}
					if got := h.PairLauncherDir(slug); got != launcherDir {
						t.Errorf("h.PairLauncherDir(%s) = %s; want %s", slug, got, launcherDir)
					}
				}},
				{"AddPairWith_SkipPushKeepsWeftBranchOffTheBare", func(t *testing.T) {
					skipped := AddPairWith(t, h, "skipped", fabricengine.AddOptions{SkipPush: true})
					skippedWeft := fabricengine.WeftBranchName(skipped.Branch)
					if skipped.Pushed {
						t.Errorf("AddPairWith(SkipPush) Pushed = true; want false")
					}
					if gitkit.BranchExists(t, h.WeftBare, skippedWeft) {
						t.Errorf("branch %q is on the weft bare; want it absent under SkipPush", skippedWeft)
					}

					pushed := AddPairWith(t, h, "pushed", fabricengine.AddOptions{})
					pushedWeft := fabricengine.WeftBranchName(pushed.Branch)
					if !pushed.Pushed || !gitkit.BranchExists(t, h.WeftBare, pushedWeft) {
						t.Errorf("zero-options AddPairWith: Pushed = %v, weft branch on bare = %v; want both true", pushed.Pushed, gitkit.BranchExists(t, h.WeftBare, pushedWeft))
					}
				}},
				{"OpenFabricOpensThePrimePair", func(t *testing.T) {
					f := OpenFabric(t, h)

					gotSHA, err := f.HeadSHA()
					if err != nil {
						t.Fatalf("HeadSHA: %v", err)
					}
					if want := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); gotSHA != want {
						t.Errorf("HeadSHA = %s; want the prime warp's HEAD %s", gotSHA, want)
					}
					gotBranch, err := f.CurrentBranch()
					if err != nil {
						t.Fatalf("CurrentBranch: %v", err)
					}
					if want := gitkit.CurrentBranch(t, h.PrimeWorktree()); gotBranch != want {
						t.Errorf("CurrentBranch = %q; want %q", gotBranch, want)
					}
				}},
				{"SeedFabricConfig_CommitsAndLeavesBoardClean", func(t *testing.T) {
					// SeedFabricConfig's write is visible at h.BoardDir() and leaves the board clean — the
					// commit through fabricengine.NewBolt is what makes an uncommitted seed unsafe rather
					// than merely untidy, since h.BoardDir() is the checkout the destruction gate's
					// dirtiness check observes.
					const override = "pathspec: []\nbranch_prefix: fabric-seeded\n"
					SeedFabricConfig(t, h, override)

					fabricConfigPath := configengine.ConfigFile(h.BoardDir(), "fabric")
					got, err := os.ReadFile(fabricConfigPath)
					if err != nil {
						t.Fatalf("read %s: %v", fabricConfigPath, err)
					}
					if string(got) != override {
						t.Errorf("%s content = %q; want the seeded override %q", fabricConfigPath, got, override)
					}

					if status := gitkit.GitStatusPorcelain(t, h.BoardDir()); status != "" {
						t.Errorf("git status --porcelain at %s = %q; want empty after SeedFabricConfig commits", h.BoardDir(), status)
					}
				}},
			}
			for _, step := range steps {
				if !t.Run(step.name, step.run) {
					return
				}
			}
		})
	}
}

// assertRealHub runs the real-hub assertions against h: every path is sourced
// through fabricengine's own name accessors — BoardDir, WiredNames — and lyxdirs/lyxcwd's own
// exported names, never a hardcoded string, because a hardcoded string is precisely the invented
// shape this whole task removes.
func assertRealHub(t *testing.T, h *Hub) {
	t.Helper()

	boardInfo, err := os.Stat(h.BoardDir())
	if err != nil {
		t.Fatalf("_board missing at %s: %v", h.BoardDir(), err)
	}
	if !boardInfo.IsDir() {
		t.Errorf("_board at %s: want a real directory, got mode %v", h.BoardDir(), boardInfo.Mode())
	}

	anchorMarker := filepath.Join(h.BoardDir(), lyxcwd.AnchorFileName)
	if _, err := os.Stat(anchorMarker); err != nil {
		t.Errorf("%s marker missing at %s: %v", lyxcwd.AnchorFileName, anchorMarker, err)
	}

	// The hub's scratch directory lives under _board, not at hub level, and its path is sourced from
	// fabricengine.HubScratchDir — the same constructor CloneHub materialises it through — so this
	// assertion cannot drift from the geometry it is checking.
	hubScratch := fabricengine.HubScratchDir(h.Path)
	if _, err := os.Stat(hubScratch); err != nil {
		t.Errorf("hub scratch %s missing at %s: %v", lyxdirs.DotLyxDirName, hubScratch, err)
	}
	isLink, err := fslink.IsLink(hubScratch)
	if err != nil {
		t.Errorf("fslink.IsLink(%s): %v", hubScratch, err)
	} else if isLink {
		t.Errorf("hub scratch %s at %s: want a real directory, got a link", lyxdirs.DotLyxDirName, hubScratch)
	}

	names, err := fabricengine.WiredNames(h.BoardDir())
	if err != nil {
		t.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
	}
	for _, name := range names {
		link := filepath.Join(h.PrimeWorktree(), h.Anchor, name)
		isLink, err := fslink.IsLink(link)
		if err != nil {
			t.Errorf("fslink.IsLink(%s): %v", link, err)
			continue
		}
		if !isLink {
			t.Errorf("wired junction %s: want a link, got none", link)
		}
	}

	fabricConfig := configengine.ConfigFile(h.BoardDir(), "fabric")
	if _, err := os.Stat(fabricConfig); err != nil {
		t.Errorf("repo-wide fabric.yaml missing at %s: %v", fabricConfig, err)
	}

	warpBinding := filepath.Join(h.BoardDir(), fabricengine.WarpBindingFileName)
	if _, err := os.Stat(warpBinding); err != nil {
		t.Errorf("weft:main warp-URL binding missing at %s: %v", warpBinding, err)
	}
}

// TestNewHub_Concurrent launches N concurrent NewHub calls from one test, asserting every returned hub
// is independently a real hub and that no two share a Path, Container, WarpBare, or WeftBare — the
// structural parallel safety a sync.Once template read followed by per-call tb.TempDir() is supposed
// to guarantee.
func TestNewHub_Concurrent(t *testing.T) {
	t.Parallel()

	const n = 8

	hubs := make([]*Hub, n)
	var wg sync.WaitGroup
	for i := range hubs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hubs[i] = NewHub(t, ".")
		}(i)
	}
	wg.Wait()

	seenPaths := make(map[string]int, n*4)
	recordUnique := func(field, value string, hubIndex int) {
		if prior, ok := seenPaths[value]; ok {
			t.Errorf("hub %d and hub %d share %s = %s; want every concurrent hub to be independent", prior, hubIndex, field, value)
			return
		}
		seenPaths[value] = hubIndex
	}

	for i, h := range hubs {
		assertRealHub(t, h)
		recordUnique("Path", h.Path, i)
		recordUnique("Container", h.Container, i)
		recordUnique("WarpBare", h.WarpBare, i)
		recordUnique("WeftBare", h.WeftBare, i)
	}
}

// TestNewHub_TeardownRemovesJunctionsKeepsTargets builds a hub in a nested t.Run subtest, capturing
// each junction path and its fslink.PointsTo target before the subtest returns, then asserts — after
// the subtest returns and its teardown has run — that every junction path is gone and, the
// load-bearing half, every captured target directory still exists with its content intact.
//
// The hub's own container lives outside any t.TempDir() the subtest owns, cleaned up only at this
// test's own end, so the subtest's teardown (registerTeardown, registered exactly as NewHub registers
// it) is what is under test here, isolated from Go's own recursive directory removal.
func TestNewHub_TeardownRemovesJunctionsKeepsTargets(t *testing.T) {
	t.Parallel()

	container, err := os.MkdirTemp("", "hubforge-teardown-keeps-targets-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(container); err != nil {
			t.Logf("remove %s: %v", container, err)
		}
	})

	type junction struct {
		link, target string
	}
	var junctions []junction

	t.Run("hub", func(t *testing.T) {
		warpBare, weftBare := copyBares(t)

		res, err := fabriccli.CloneAndWire(container, fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(weftBare),
			WarpURL:   filepath.ToSlash(warpBare),
			Shortname: TestShortname,
		})
		if err != nil {
			t.Fatalf("CloneAndWire: %v", err)
		}
		registerTeardown(t, res.HubPath)

		loc, err := lyxcwd.Resolve(res.PrimeCwd)
		if err != nil {
			t.Fatalf("lyxcwd.Resolve(%s): %v", res.PrimeCwd, err)
		}
		h := &Hub{
			Path:      res.HubPath,
			Anchor:    res.Anchor,
			Location:  loc,
			Topology:  fabricengine.NewTopology(fabricengine.Config{}),
			WarpBare:  warpBare,
			WeftBare:  weftBare,
			WeftBase:  res.WeftBase,
			Container: container,
		}

		AddPair(t, h, "px")

		walkErr := filepath.WalkDir(h.Path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			isLink, err := fslink.IsLink(path)
			if err != nil {
				return err
			}
			if !isLink {
				return nil
			}
			target, err := fslink.PointsTo(path)
			if err != nil {
				return err
			}
			junctions = append(junctions, junction{link: path, target: target})
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", h.Path, walkErr)
		}
	})

	if len(junctions) == 0 {
		t.Fatalf("captured zero junctions under the hub; want at least one")
	}
	for _, j := range junctions {
		if _, err := os.Lstat(j.link); !os.IsNotExist(err) {
			t.Errorf("junction %s: want gone after teardown, lstat err = %v", j.link, err)
		}
		info, err := os.Stat(j.target)
		if err != nil {
			t.Errorf("target %s: want it to survive teardown intact: %v", j.target, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("target %s: want a directory, got mode %v", j.target, info.Mode())
		}
	}
}

// TestNewHub_TeardownSurvivesCorruptHub covers the two hostile shapes the live-state matrix plants: a
// junction repointed at an unrelated directory, and a warp worktree directory removed by hand.
// Teardown must complete without failing the test in either case — registerTeardown reports every
// error via tb.Logf and never tb.Fatalf/tb.Errorf, so a test that already passed must stay passed.
func TestNewHub_TeardownSurvivesCorruptHub(t *testing.T) {
	t.Parallel()

	t.Run("RepointedJunction", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")

		names, err := fabricengine.WiredNames(h.BoardDir())
		if err != nil {
			t.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
		}
		if len(names) == 0 {
			t.Fatalf("WiredNames(%s): want at least one wired name", h.BoardDir())
		}
		link := filepath.Join(h.PrimeWorktree(), h.Anchor, names[0])
		if err := fslink.Remove(link); err != nil {
			t.Fatalf("fslink.Remove(%s): %v", link, err)
		}
		unrelated := t.TempDir()
		if err := fslink.CreateDirLink(link, unrelated); err != nil {
			t.Fatalf("fslink.CreateDirLink(%s, %s): %v", link, unrelated, err)
		}
		// Teardown runs automatically when this subtest returns; a hostile repointed link must not
		// fail the test that already passed above.
	})

	t.Run("RemovedWarpWorktree", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")

		if err := os.RemoveAll(h.PrimeWorktree()); err != nil {
			t.Fatalf("remove %s: %v", h.PrimeWorktree(), err)
		}
		// Teardown runs automatically when this subtest returns; a hand-removed warp worktree must
		// not fail the test that already passed above.
	})
}
