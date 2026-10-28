//go:build tier1

// hubworkspace_integration_test.go drives RegenerateHubWorkspace against hubforge hubs:
// the folder list and its order, the skip exits, the mutation record, the lock, and the failures that write no file.
// Shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// workspaceFolder is one entry of the hub workspace file's folders array.
type workspaceFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// readWorkspaceFolders returns the folders of the hub workspace file at path.
func readWorkspaceFolders(t *testing.T, path string) []workspaceFolder {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	var ws struct {
		Folders []workspaceFolder `json:"folders"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("workspace file is not valid JSON: %v\n%s", err, data)
	}
	return ws.Folders
}

// regenerateHub runs RegenerateHubWorkspace against h and returns the file's path and the call's record.
func regenerateHub(t *testing.T, h *hubforge.Hub) (string, *fabricengine.Mutations) {
	t.Helper()
	rec := fabricengine.NewMutations(h.Path)
	path, err := fabricengine.RegenerateHubWorkspace(rec, h.Location)
	if err != nil {
		t.Fatalf("RegenerateHubWorkspace: %v", err)
	}
	if want := fabricengine.HubWorkspacePath(h.Location); path != want {
		t.Fatalf("RegenerateHubWorkspace path = %q; want %q", path, want)
	}
	return path, rec
}

// primeBranch returns the branch checked out in the hub's prime.
func primeBranch(t *testing.T, h *hubforge.Hub) string {
	t.Helper()
	return gitkit.Git(t, h.PrimeWorktree(), "branch", "--show-current")
}

// assertFolders fails the test unless the workspace file lists exactly names in order.
// Each folder must resolve to its anchor dir relative to the file's directory, with no backslash.
// The first name is the prime, whose dir is the prime's anchor; Bolt is the board; every other name is a pair slug.
func assertFolders(t *testing.T, h *hubforge.Hub, path string, names ...string) {
	t.Helper()
	folders := readWorkspaceFolders(t, path)
	var gotNames []string
	for _, f := range folders {
		gotNames = append(gotNames, f.Name)
	}
	if !slices.Equal(gotNames, names) {
		t.Fatalf("workspace folders = %v; want %v", gotNames, names)
	}
	anchor := h.Location.AnchorRel
	for i, f := range folders {
		if strings.Contains(f.Path, `\`) {
			t.Errorf("folder %q path %q contains a backslash", f.Name, f.Path)
		}
		var want string
		switch {
		case i == 0:
			want = filepath.Join(h.PrimeWorktree(), anchor)
		case f.Name == "Bolt":
			want = h.BoardDir()
		default:
			want = filepath.Join(h.PairCodeWorktree(f.Name), anchor)
		}
		if got := filepath.Join(filepath.Dir(path), filepath.FromSlash(f.Path)); got != want {
			t.Errorf("folder %q resolves to %q; want %q", f.Name, got, want)
		}
	}
}

// requireFolderNames fails the test unless the workspace file at path lists exactly names in order.
func requireFolderNames(t *testing.T, path string, names ...string) {
	t.Helper()
	var got []string
	for _, f := range readWorkspaceFolders(t, path) {
		got = append(got, f.Name)
	}
	if !slices.Equal(got, names) {
		t.Errorf("%s folders = %v; want %v", filepath.Base(path), got, names)
	}
}

// beamAddOptions are the options of a beam pair added without pushing.
var beamAddOptions = fabricengine.AddOptions{SkipPush: true, Beam: true}

// memberAddOptions returns the options of a pair forked from the pair parent, added without pushing.
func memberAddOptions(parent string, beam bool) fabricengine.AddOptions {
	return fabricengine.AddOptions{SkipPush: true, Parent: parent, Beam: beam}
}

// addRawWorktree adds slug's warp worktree with a raw git call, so no fabric verb regenerates the workspace file.
func addRawWorktree(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()
	gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "-b", "raw-"+slug, h.PairCodeWorktree(slug))
}

// removeWorkspaceFile removes the workspace file so a row that asserts a failure writes none can assert it stays absent.
func removeWorkspaceFile(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	if err := os.Remove(fabricengine.HubWorkspacePath(h.Location)); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove workspace file: %v", err)
	}
}

// assertNoWorkspaceFile fails the test when the workspace file exists.
func assertNoWorkspaceFile(t *testing.T, h *hubforge.Hub) {
	t.Helper()
	if _, err := os.Lstat(fabricengine.HubWorkspacePath(h.Location)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("workspace file exists after a failed regeneration (stat err = %v)", err)
	}
}

// recordedFileWrite reports whether rec holds a file_written entry for path.
func recordedFileWrite(rec *fabricengine.Mutations, hubPath, path string) bool {
	rel, err := filepath.Rel(hubPath, path)
	if err != nil {
		return false
	}
	for _, m := range rec.Entries() {
		if m.Kind == fabricengine.KindFileWritten && m.Target == filepath.ToSlash(rel) {
			return true
		}
	}
	return false
}

// TestRegenerateHubWorkspace covers RegenerateHubWorkspace's folder list, skip exits, mutation record, lock and failures.
// The top level is not parallel: the lock-ordering and log-capture rows touch process-global state (a test hook and the logger output),
// so they run before the parallel rows are released and no regeneration runs beside them.
func TestRegenerateHubWorkspace(t *testing.T) {
	t.Run("ListsFolderAddedWhileWaitingForTheLock", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := *h.Location
		hubScratch := fabricengine.HubScratchDir(h.Path)
		if err := os.MkdirAll(hubScratch, 0o755); err != nil {
			t.Fatalf("create scratch dir: %v", err)
		}
		held, err := lock.AcquireWriteLock(fabricengine.HubWorkspaceLockPath(h.Path))
		if err != nil {
			t.Fatalf("hold the hub workspace lock: %v", err)
		}
		released := false
		release := func() {
			if !released {
				released = true
				_ = held.Release()
			}
		}
		t.Cleanup(release)

		reached := make(chan struct{}, 1)
		fabricengine.SetHubWorkspaceBeforeLockHookForTest(t, func() { reached <- struct{}{} })
		type result struct {
			path string
			err  error
		}
		done := make(chan result, 1)
		go func() {
			path, err := fabricengine.RegenerateHubWorkspaceWithinForTest(fabricengine.NewMutations(h.Path), &l, time.Minute)
			done <- result{path, err}
		}()

		select {
		case <-reached:
		case <-time.After(time.Minute):
			t.Fatalf("the regeneration never reached the lock")
		}
		addRawWorktree(t, h, "b")
		release()

		var res result
		select {
		case res = <-done:
		case <-time.After(time.Minute):
			t.Fatalf("the regeneration never finished after the lock was released")
		}
		if res.err != nil {
			t.Fatalf("regeneration: %v", res.err)
		}
		assertFolders(t, h, res.path, primeBranch(t, h), "Bolt", "b")
	})

	t.Run("WarnsAboutRelativeSettingKeys", func(t *testing.T) {
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		settingsDir := filepath.Join(h.PrimeWorktree(), ".vscode")
		if err := os.MkdirAll(settingsDir, 0o755); err != nil {
			t.Fatalf("create .vscode: %v", err)
		}
		if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"x": "./rel"}`), 0o644); err != nil {
			t.Fatalf("write settings: %v", err)
		}
		logs := logcapture.Capture(t)

		regenerateHub(t, h)

		if got := logs.String(); !strings.Contains(got, "x") || !strings.Contains(got, "hub directory") {
			t.Errorf("log = %q; want a warning naming the key x and the hub directory", got)
		}
	})

	for _, anchor := range []string{".", "backend"} {
		t.Run("ListsPrimeBoltAndPairsSortedAt_"+anchor, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
			hubforge.AddPair(t, h, "b-task")
			hubforge.AddPair(t, h, "a-task")
			hubforge.AddPairWith(t, h, "c-skip-push", fabricengine.AddOptions{SkipPush: true})
			if _, err := h.Topology.AddLocal(h.Location, "d-local"); err != nil {
				t.Fatalf("AddLocal: %v", err)
			}

			path, _ := regenerateHub(t, h)

			assertFolders(t, h, path, primeBranch(t, h), "Bolt", "a-task", "b-task", "c-skip-push", "d-local")
		})
	}

	t.Run("ListsDetachedPrimeUnderItsDirectoryName", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		gitkit.Git(t, h.PrimeWorktree(), "switch", "--detach")

		path, _ := regenerateHub(t, h)

		assertFolders(t, h, path, filepath.Base(h.PrimeWorktree()), "Bolt")
	})

	t.Run("LeavesOutBoltAndKeepsBoardAbsentWithoutABoard", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		aside := filepath.Join(t.TempDir(), "board")
		if err := os.Rename(h.BoardDir(), aside); err != nil {
			t.Fatalf("move _board aside: %v", err)
		}
		t.Cleanup(func() { _ = os.Rename(aside, h.BoardDir()) })

		path, _ := regenerateHub(t, h)

		assertFolders(t, h, path, primeBranch(t, h))
		if _, err := os.Lstat(h.BoardDir()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("_board exists after the call (stat err = %v); want absent", err)
		}
	})

	skipRows := []struct {
		name    string
		anchor  string
		arrange func(t *testing.T, h *hubforge.Hub)
	}{
		{"PairWhoseWorktreeWasDeletedByHand", ".", func(t *testing.T, h *hubforge.Hub) {
			hubforge.AddPair(t, h, "gone")
			if err := os.RemoveAll(h.PairCodeWorktree("gone")); err != nil {
				t.Fatalf("delete the pair's worktree: %v", err)
			}
		}},
		{"PairWhoseAnchorSubdirWasDeleted", "backend", func(t *testing.T, h *hubforge.Hub) {
			hubforge.AddPair(t, h, "gone")
			if err := os.RemoveAll(filepath.Join(h.PairCodeWorktree("gone"), "backend")); err != nil {
				t.Fatalf("delete the pair's anchor subdir: %v", err)
			}
		}},
		{"WarpWorktreeOutsideTheHub", ".", func(t *testing.T, h *hubforge.Hub) {
			gitkit.Git(t, h.PrimeWorktree(), "worktree", "add", "-b", "outside", filepath.Join(t.TempDir(), "outside"))
		}},
		{"PairWhoseGitFileIsGarbage", ".", func(t *testing.T, h *hubforge.Hub) {
			hubforge.AddPair(t, h, "gone")
			if err := os.WriteFile(filepath.Join(h.PairCodeWorktree("gone"), ".git"), []byte("garbage\n"), 0o644); err != nil {
				t.Fatalf("overwrite the pair's .git file: %v", err)
			}
		}},
	}
	for _, tt := range skipRows {
		t.Run("Skips"+tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: tt.anchor})
			hubforge.AddPair(t, h, "keep")
			tt.arrange(t, h)

			path, _ := regenerateHub(t, h)

			assertFolders(t, h, path, primeBranch(t, h), "Bolt", "keep")
		})
	}

	t.Run("WritesABeamFileAndKeepsItsMembersOutOfTheHubFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		hubforge.AddPairWith(t, h, "m", memberAddOptions("b", false))

		path, _ := regenerateHub(t, h)

		prime := primeBranch(t, h)
		requireFolderNames(t, path, prime, "Bolt", "b")
		beamPath := fabricengine.BeamWorkspacePath(h.Location, "b")
		requireFolderNames(t, beamPath, "b", prime, "Bolt", "m")
		for file, want := range map[string]bool{path: false, beamPath: true} {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			var ws map[string]any
			if err := json.Unmarshal(data, &ws); err != nil {
				t.Fatalf("%s is not valid JSON: %v\n%s", file, err, data)
			}
			if _, marked := ws["lym.generated"]; marked != want {
				t.Errorf("%s carries the generated mark = %v; want %v", filepath.Base(file), marked, want)
			}
		}
	})

	t.Run("SplicesTheBeamsOwnSettingsIntoItsFileAndThePrimesWhenItHasNone", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		writeSettingsAt(t, h.PrimeWorktree(), `{"window.title": "prime"}`)
		beamPath := fabricengine.BeamWorkspacePath(h.Location, "b")

		path, _ := regenerateHub(t, h)
		for file, want := range map[string]string{path: "prime", beamPath: "prime"} {
			if got := workspaceTitle(t, file); got != want {
				t.Errorf("%s window.title = %q; want %q with no beam settings", filepath.Base(file), got, want)
			}
		}

		writeSettingsAt(t, fabricengine.WorktreePath(h.Location, "b"), `{"window.title": "beam"}`)
		regenerateHub(t, h)
		for file, want := range map[string]string{path: "prime", beamPath: "beam"} {
			if got := workspaceTitle(t, file); got != want {
				t.Errorf("%s window.title = %q; want %q", filepath.Base(file), got, want)
			}
		}

		// A settings.json that is a directory cannot be read: the beam falls back to the prime's settings.
		settingsPath := filepath.Join(fabricengine.WorktreePath(h.Location, "b"), ".vscode", "settings.json")
		if err := os.Remove(settingsPath); err != nil {
			t.Fatalf("remove the beam settings: %v", err)
		}
		if err := os.Mkdir(settingsPath, 0o755); err != nil {
			t.Fatalf("make the beam settings unreadable: %v", err)
		}
		regenerateHub(t, h)
		if got := workspaceTitle(t, beamPath); got != "prime" {
			t.Errorf("%s window.title = %q; want %q with unreadable beam settings", filepath.Base(beamPath), got, "prime")
		}
	})

	t.Run("PlacesAReanchoredBeamAtItsOwnAnchorFromAnyWorktree", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		hubforge.ReanchorPair(t, h, "b", "sub")
		beamLocation, err := lymcwd.ResolveWorktree(h.PairCodeWorktree("b"))
		if err != nil {
			t.Fatalf("resolve the beam's location: %v", err)
		}
		beamPath := fabricengine.BeamWorkspacePath(h.Location, "b")

		path, _ := regenerateHub(t, h)
		hubFile, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the hub file: %v", err)
		}
		beamFile, err := os.ReadFile(beamPath)
		if err != nil {
			t.Fatalf("read the beam file: %v", err)
		}

		prime := primeBranch(t, h)
		wantDirs := map[string]string{prime: h.PrimeWorktree(), "b": filepath.Join(h.PairCodeWorktree("b"), "sub")}
		for _, file := range []string{path, beamPath} {
			for _, f := range readWorkspaceFolders(t, file) {
				want, ok := wantDirs[f.Name]
				if !ok {
					continue
				}
				if got := filepath.Join(filepath.Dir(file), filepath.FromSlash(f.Path)); got != want {
					t.Errorf("%s folder %q resolves to %q; want %q", filepath.Base(file), f.Name, got, want)
				}
			}
		}
		requireFolderNames(t, beamPath, "b", prime, "Bolt")

		if _, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(h.Path), beamLocation); err != nil {
			t.Fatalf("RegenerateHubWorkspace from the beam: %v", err)
		}
		if got, _ := os.ReadFile(path); !slices.Equal(got, hubFile) {
			t.Errorf("hub file regenerated from the beam differs from the prime's:\n%s\nwant\n%s", got, hubFile)
		}
		if got, _ := os.ReadFile(beamPath); !slices.Equal(got, beamFile) {
			t.Errorf("beam file regenerated from the beam differs from the prime's:\n%s\nwant\n%s", got, beamFile)
		}
	})

	t.Run("ListsSubBeamsInTheirParentBeamsFileByThemselvesOnly", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		hubforge.AddPairWith(t, h, "c", memberAddOptions("b", true))
		hubforge.AddPairWith(t, h, "m", memberAddOptions("b", false))
		hubforge.AddPairWith(t, h, "n", memberAddOptions("c", false))

		path, _ := regenerateHub(t, h)

		prime := primeBranch(t, h)
		requireFolderNames(t, path, prime, "Bolt", "b")
		requireFolderNames(t, fabricengine.BeamWorkspacePath(h.Location, "b"), "b", prime, "Bolt", "c", "m")
		requireFolderNames(t, fabricengine.BeamWorkspacePath(h.Location, "c"), "c", "b", "Bolt", "n")
	})

	t.Run("ListsPairsWithAnUnusableParentRecordInTheHubFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		hubforge.AddPairWith(t, h, "orphan", memberAddOptions("b", false))
		hubforge.AddPairWith(t, h, "unrecorded", fabricengine.AddOptions{SkipPush: true})
		hubforge.AddPairWith(t, h, "emptyparent", fabricengine.AddOptions{SkipPush: true})
		hubforge.AddPairWith(t, h, "nolock", fabricengine.AddOptions{SkipPush: true})
		if err := os.RemoveAll(h.PairCodeWorktree("b")); err != nil {
			t.Fatalf("delete the parent's worktree by hand: %v", err)
		}
		if err := os.Remove(fabricengine.OriginRecordPathFor(l, "unrecorded")); err != nil {
			t.Fatalf("delete the origin record by hand: %v", err)
		}
		rec := fabricengine.NewMutations(h.Path)
		if err := fabricengine.WriteOrigin(rec, l, "emptyparent", fabricengine.Origin{ParentBranch: primeBranch(t, h)}); err != nil {
			t.Fatalf("rewrite the origin record with an empty parent worktree: %v", err)
		}
		if err := os.RemoveAll(filepath.Join(h.PairRecordsSibling("nolock"), ".weft")); err != nil {
			t.Fatalf("delete the records worktree's lock directory: %v", err)
		}

		path, _ := regenerateHub(t, h)

		requireFolderNames(t, path, primeBranch(t, h), "Bolt", "emptyparent", "nolock", "orphan", "unrecorded")
	})

	t.Run("RemovesOnlyAMarkedBeamFileOfAGoneBeam", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		l := h.Location
		hubforge.AddPairWith(t, h, "b", beamAddOptions)
		beamPath := fabricengine.BeamWorkspacePath(l, "b")
		if _, err := os.Stat(beamPath); err != nil {
			t.Fatalf("beam file missing after adding the beam: %v", err)
		}

		if _, err := h.Topology.RemoveLocal(l, "b", false); err != nil {
			t.Fatalf("RemoveLocal: %v", err)
		}
		if _, err := os.Lstat(beamPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("beam file of a removed beam: stat err = %v; want it gone", err)
		}

		if err := os.WriteFile(beamPath, []byte("{\"folders\": []}\n"), 0o644); err != nil {
			t.Fatalf("write an operator's file: %v", err)
		}
		regenerateHub(t, h)
		if _, err := os.Stat(beamPath); err != nil {
			t.Errorf("an unmarked file of a gone beam: stat err = %v; want it kept", err)
		}
	})

	t.Run("GivesABeamMadeByMarkBeamItsFileOnTheNextRegeneration", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPairWith(t, h, "p", fabricengine.AddOptions{SkipPush: true})
		beamPath := fabricengine.BeamWorkspacePath(h.Location, "p")
		if _, err := os.Stat(beamPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("beam file of a plain pair: stat err = %v; want none", err)
		}
		if _, err := fabricengine.MarkBeam(fabricengine.NewMutations(h.Path), h.Location, "p"); err != nil {
			t.Fatalf("MarkBeam: %v", err)
		}

		regenerateHub(t, h)

		requireFolderNames(t, beamPath, "p", primeBranch(t, h), "Bolt")
	})

	t.Run("RecordsAWriteOnlyWhenTheBytesChange", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		// The clone already wrote the file, so it goes first for the first call to have a write to record.
		removeWorkspaceFile(t, h)
		path, first := regenerateHub(t, h)
		if !recordedFileWrite(first, h.Path, path) {
			t.Fatalf("first call recorded no file_written for %s: %+v", path, first.Entries())
		}
		whole, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read workspace file: %v", err)
		}

		if _, rec := regenerateHub(t, h); recordedFileWrite(rec, h.Path, path) {
			t.Errorf("an unchanged hub recorded a file_written: %+v", rec.Entries())
		}

		addRawWorktree(t, h, "b")
		if _, rec := regenerateHub(t, h); !recordedFileWrite(rec, h.Path, path) {
			t.Errorf("a changed folder list recorded no file_written: %+v", rec.Entries())
		}

		settingsDir := filepath.Join(h.PrimeWorktree(), ".vscode")
		if err := os.MkdirAll(settingsDir, 0o755); err != nil {
			t.Fatalf("create .vscode: %v", err)
		}
		if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"editor.tabSize": 2}`), 0o644); err != nil {
			t.Fatalf("write settings: %v", err)
		}
		if _, rec := regenerateHub(t, h); !recordedFileWrite(rec, h.Path, path) {
			t.Errorf("a changed prime settings.json recorded no file_written: %+v", rec.Entries())
		}

		whole, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("read workspace file: %v", err)
		}
		if err := os.WriteFile(path, whole[:len(whole)/2], 0o644); err != nil {
			t.Fatalf("truncate workspace file: %v", err)
		}
		_, rec := regenerateHub(t, h)
		if !recordedFileWrite(rec, h.Path, path) {
			t.Errorf("a truncated file recorded no file_written: %+v", rec.Entries())
		}
		restored, err := os.ReadFile(path)
		if err != nil || string(restored) != string(whole) {
			t.Errorf("truncated file was not restored whole: err %v, got %q, want %q", err, restored, whole)
		}
	})

	t.Run("RecreatesTheScratchDirAndRecordsTheLockFileOnce", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		if err := os.RemoveAll(fabricengine.HubScratchDir(h.Path)); err != nil {
			t.Fatalf("remove the scratch dir: %v", err)
		}
		lockPath := fabricengine.HubWorkspaceLockPath(h.Path)

		_, first := regenerateHub(t, h)
		if !recordedFileWrite(first, h.Path, lockPath) {
			t.Errorf("first call recorded no file_written for the lock file: %+v", first.Entries())
		}
		if _, err := os.Stat(lockPath); err != nil {
			t.Errorf("lock file missing after the call: %v", err)
		}

		if _, second := regenerateHub(t, h); recordedFileWrite(second, h.Path, lockPath) {
			t.Errorf("second call recorded the lock file again: %+v", second.Entries())
		}
	})

	t.Run("FailsOnUnreadablePrimeSettingsAndWritesNoFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		removeWorkspaceFile(t, h)
		if err := os.MkdirAll(filepath.Join(h.PrimeWorktree(), ".vscode", "settings.json"), 0o755); err != nil {
			t.Fatalf("plant a directory at settings.json: %v", err)
		}

		_, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(h.Path), h.Location)

		if err == nil || !strings.Contains(err.Error(), "read prime settings") {
			t.Fatalf("error = %v; want one containing %q", err, "read prime settings")
		}
		assertNoWorkspaceFile(t, h)
	})

	t.Run("FailsWhenThePrimeCannotBeResolvedAndWritesNoFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		removeWorkspaceFile(t, h)
		l := *h.Location
		l.WorktreeName = "not-a-checkout"
		if err := os.MkdirAll(l.AnchorPath(), 0o755); err != nil {
			t.Fatalf("create the non-checkout dir: %v", err)
		}

		_, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(h.Path), &l)

		if err == nil || !strings.Contains(err.Error(), "resolve main worktree") {
			t.Fatalf("error = %v; want the prime-resolution error", err)
		}
		assertNoWorkspaceFile(t, h)
	})

	t.Run("FailsWhenTheLockStaysHeldAndWritesNoFile", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		removeWorkspaceFile(t, h)
		if err := os.MkdirAll(fabricengine.HubScratchDir(h.Path), 0o755); err != nil {
			t.Fatalf("create scratch dir: %v", err)
		}
		lockPath := fabricengine.HubWorkspaceLockPath(h.Path)
		held, err := lock.AcquireWriteLock(lockPath)
		if err != nil {
			t.Fatalf("hold the hub workspace lock: %v", err)
		}
		t.Cleanup(func() { _ = held.Release() })

		errs := make(chan error, 1)
		go func() {
			_, err := fabricengine.RegenerateHubWorkspaceWithinForTest(fabricengine.NewMutations(h.Path), h.Location, 50*time.Millisecond)
			errs <- err
		}()
		select {
		case err := <-errs:
			if err == nil || !strings.Contains(err.Error(), lockPath) {
				t.Fatalf("error = %v; want one naming the lock %s", err, lockPath)
			}
		case <-time.After(time.Minute):
			t.Fatalf("the regeneration did not give up on the held lock")
		}
		assertNoWorkspaceFile(t, h)
	})
}

// writeSettingsAt writes content as <dir>/.vscode/settings.json.
func writeSettingsAt(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".vscode"), 0o755); err != nil {
		t.Fatalf("create .vscode: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vscode", "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// workspaceTitle returns the window.title the workspace file at path carries in its settings.
func workspaceTitle(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var ws struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", path, err, data)
	}
	title, _ := ws.Settings["window.title"].(string)
	return title
}
