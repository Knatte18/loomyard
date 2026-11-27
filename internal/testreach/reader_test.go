// reader_test.go tests the coverage map reader over map directories written under a temporary directory.

package testreach_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testreach"
)

const modulePath = "ex.com/m"

// builtEarly and builtLate are the build times the tests give maps, so one map is later than another.
var (
	builtEarly = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	builtLate  = builtEarly.Add(time.Hour)
)

// reachFixture is a module directory and two map directories, none of which exists until a test writes into it.
type reachFixture struct {
	module           testreach.Module
	shared, worktree string
}

func newReachFixture(t *testing.T) reachFixture {
	t.Helper()
	root := t.TempDir()
	return reachFixture{
		module:   testreach.Module{Dir: filepath.Join(root, "mod"), Path: modulePath},
		shared:   filepath.Join(root, "shared"),
		worktree: filepath.Join(root, "worktree"),
	}
}

// writeFile writes a file of the module at the slash path rel.
func (f reachFixture) writeFile(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.module.Dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCoverageMap(t *testing.T, dir, pkg string, m testcover.Map) {
	t.Helper()
	if err := testcover.WriteMap(dir, pkg, m); err != nil {
		t.Fatal(err)
	}
}

func hashOf(content string) string {
	return testcover.ContentHash([]byte(content))
}

// pkgFile is the content of the one Go file the fixture's packages hold.
const pkgFile = "package p\n"

func block(file, fn string) testcover.BlockKey {
	return testcover.BlockKey{File: file, Func: fn, Hash: "h"}
}

func TestReader(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	reachedF := testcover.FuncKey{Package: modulePath + "/a", File: "a.go", Func: "F"}
	// ownAndLinked writes the maps of two packages.
	// One reaches reachedF from its own blocks and the other through its linked functions.
	ownAndLinked := func(t *testing.T, f reachFixture) {
		writeCoverageMap(t, f.shared, "a", testcover.Map{
			Commit: "c1", BuiltAt: builtEarly,
			Blocks: []testcover.BlockKey{block("a.go", "F"), block("a.go", "G")},
			Tests:  []testcover.MapTest{{Name: "TestOwnWide", Blocks: []int{0, 1}}, {Name: "TestOwnNarrow", Blocks: []int{0}}},
		})
		writeCoverageMap(t, f.shared, "b", testcover.Map{
			Commit: "c1", BuiltAt: builtEarly,
			Blocks: []testcover.BlockKey{block("b.go", "H")},
			Funcs:  []testcover.FuncKey{reachedF},
			Tests:  []testcover.MapTest{{Name: "TestLinked", Blocks: []int{0}, Funcs: []int{0}}, {Name: "TestLinkedOnly", Funcs: []int{0}}},
		})
	}
	// unusableMaps writes a good map of a and the three kinds of map the reader passes over.
	unusableMaps := func(t *testing.T, f reachFixture) {
		writeCoverageMap(t, f.shared, "a", testcover.Map{
			Commit: "c1", BuiltAt: builtEarly,
			Blocks: []testcover.BlockKey{block("a.go", "F")},
			Tests:  []testcover.MapTest{{Name: "TestGood", Blocks: []int{0}}},
		})
		writeCoverageMap(t, f.shared, "failedbuild", testcover.Map{Commit: "c1", BuiltAt: builtEarly, BuildFailure: "does not compile"})
		writeCoverageMap(t, f.shared, "outsidetable", testcover.Map{
			Commit: "c1", BuiltAt: builtEarly,
			Blocks: []testcover.BlockKey{block("o.go", "F")},
			Tests:  []testcover.MapTest{{Name: "TestOutside", Blocks: []int{5}}},
		})
		junk := testcover.MapPath(f.shared, "junk")
		if err := os.MkdirAll(filepath.Dir(junk), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(junk, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// worktreeCase writes a shared map of a built at sharedBuilt and a worktree map of a built at worktreeBuilt.
	// The worktree map records the hash of its file as fileHash and has the format version.
	worktreeCase := func(version int, fileHash string, sharedBuilt, worktreeBuilt time.Time) func(*testing.T, reachFixture) {
		return func(t *testing.T, f reachFixture) {
			f.writeFile(t, "a/a.go", pkgFile)
			writeCoverageMap(t, f.shared, "a", testcover.Map{
				Commit: "shared", BuiltAt: sharedBuilt, Files: map[string]string{"a.go": hashOf(pkgFile)},
				Blocks: []testcover.BlockKey{block("a.go", "F")},
				Tests:  []testcover.MapTest{{Name: "TestShared", Blocks: []int{0}}},
			})
			writeCoverageMap(t, f.worktree, "a", testcover.Map{
				Commit: "worktree", BuiltAt: worktreeBuilt, Files: map[string]string{"a.go": fileHash},
				Format: &testcover.MapFormat{Version: version},
				Blocks: []testcover.BlockKey{block("a.go", "F")},
				Tests:  []testcover.MapTest{{Name: "TestWorktree", Blocks: []int{0}}},
			})
		}
	}
	aReach := func(test, commit string) testreach.Reach {
		return testreach.Reach{Package: modulePath + "/a", Test: test, Own: true, Funcs: 1, MapCommit: commit}
	}

	tests := []struct {
		name     string
		setup    func(t *testing.T, f reachFixture)
		fn       testcover.FuncKey
		want     []testreach.Reach
		wantMaps []testreach.MapSource
		check    func(t *testing.T, f reachFixture, r *testreach.Reader)
		skipMaps bool
	}{
		{
			name:  "OwnPackageTestsComeFirstThenFewestFunctions",
			setup: ownAndLinked,
			fn:    reachedF,
			want: []testreach.Reach{
				aReach("TestOwnNarrow", "c1"),
				{Package: modulePath + "/a", Test: "TestOwnWide", Own: true, Funcs: 2, MapCommit: "c1"},
				{Package: modulePath + "/b", Test: "TestLinkedOnly", Funcs: 1, MapCommit: "c1"},
				{Package: modulePath + "/b", Test: "TestLinked", Funcs: 2, MapCommit: "c1"},
			},
			skipMaps: true,
		},
		{
			name: "OutcomeAndOutOfProcessEntriesReachNothingAndAnUnresolvedEntryReachesItsBlocks",
			setup: func(t *testing.T, f reachFixture) {
				writeCoverageMap(t, f.shared, "c", testcover.Map{
					Commit: "c1", BuiltAt: builtEarly,
					Blocks: []testcover.BlockKey{block("c.go", "F")},
					Tests: []testcover.MapTest{
						{Name: "TestUnresolved", Spawn: testcover.SpawnVerdict{Unresolved: true}, Blocks: []int{0}},
						{Name: "TestOutOfProcess", Spawn: testcover.SpawnVerdict{OutOfProcess: true}, Blocks: []int{0}},
						{Name: "TestFailed", Outcome: testcover.OutcomeFailed, Blocks: []int{0}},
						{Name: "TestSkipped", Outcome: testcover.OutcomeSkipped},
					},
				})
				writeCoverageMap(t, f.shared, "a", testcover.Map{Commit: "c1", BuiltAt: builtEarly, Tests: []testcover.MapTest{
					{Name: "TestPlain"}, {Name: "TestEmpty", Outcome: testcover.OutcomeNoCoverage},
				}})
			},
			fn:       testcover.FuncKey{Package: modulePath + "/c", File: "c.go", Func: "F"},
			want:     []testreach.Reach{{Package: modulePath + "/c", Test: "TestUnresolved", Own: true, Funcs: 1, MapCommit: "c1"}},
			skipMaps: true,
			check: func(t *testing.T, _ reachFixture, r *testreach.Reader) {
				wantUnseen := []testreach.UnseenTest{
					{Test: "TestFailed", Outcome: testcover.OutcomeFailed},
					{Test: "TestOutOfProcess", Spawn: testcover.SpawnVerdict{OutOfProcess: true}},
					{Test: "TestSkipped", Outcome: testcover.OutcomeSkipped},
					{Test: "TestUnresolved", Spawn: testcover.SpawnVerdict{Unresolved: true}},
				}
				if diff := cmp.Diff(wantUnseen, r.UnseenTests("c")); diff != "" {
					t.Errorf("UnseenTests(c) mismatch (-want +got):\n%s", diff)
				}
				if got := r.UnseenTests("a"); len(got) != 0 {
					t.Errorf("UnseenTests(a) = %+v; want none, since a test that covered nothing runs nothing", got)
				}
			},
		},
		{
			name:  "WorktreeMapWithMatchingHashesOverridesALaterSharedMap",
			setup: worktreeCase(testcover.MapFormatVersion, hashOf(pkgFile), builtLate, builtEarly),
			fn:    reachedF,
			want:  []testreach.Reach{aReach("TestWorktree", "worktree")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "worktree", BuiltAt: builtEarly, Worktree: true},
			},
		},
		{
			name:  "WorktreeMapOfAnOlderFormatWithMatchingHashesLosesToALaterSharedMap",
			setup: worktreeCase(2, hashOf(pkgFile), builtLate, builtEarly),
			fn:    reachedF,
			want:  []testreach.Reach{aReach("TestShared", "shared")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "shared", BuiltAt: builtLate},
			},
		},
		{
			name:  "StaleWorktreeMapLosesToALaterSharedMap",
			setup: worktreeCase(testcover.MapFormatVersion, hashOf("changed"), builtLate, builtEarly),
			fn:    reachedF,
			want:  []testreach.Reach{aReach("TestShared", "shared")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "shared", BuiltAt: builtLate},
			},
		},
		{
			name:  "StaleWorktreeMapWinsWhenBuiltLater",
			setup: worktreeCase(testcover.MapFormatVersion, hashOf("changed"), builtEarly, builtLate),
			fn:    reachedF,
			want:  []testreach.Reach{aReach("TestWorktree", "worktree")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "worktree", BuiltAt: builtLate, Worktree: true},
			},
		},
		{
			name: "MissingWorktreeDirectoryAnswersFromTheSharedMaps",
			setup: func(t *testing.T, f reachFixture) {
				writeCoverageMap(t, f.shared, "a", testcover.Map{
					Commit: "c1", BuiltAt: builtEarly,
					Blocks: []testcover.BlockKey{block("a.go", "F")},
					Tests:  []testcover.MapTest{{Name: "TestShared", Blocks: []int{0}}},
				})
			},
			fn:   reachedF,
			want: []testreach.Reach{aReach("TestShared", "c1")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "c1", BuiltAt: builtEarly},
			},
		},
		{
			name:  "MissingSharedDirectoryAnswersWithNoReach",
			setup: func(*testing.T, reachFixture) {},
			fn:    reachedF,
		},
		{
			name:     "FuncsReachedByReturnsTheReachedSubsetWithTheMapsCommit",
			setup:    ownAndLinked,
			fn:       testcover.FuncKey{Package: modulePath + "/a", File: "a.go", Func: "Nobody"},
			skipMaps: true,
			check: func(t *testing.T, _ reachFixture, r *testreach.Reader) {
				funcG := testcover.FuncKey{Package: modulePath + "/a", File: "a.go", Func: "G"}
				among := []testcover.FuncKey{funcG, reachedF}
				checks := []struct {
					pkg, test string
					want      []testcover.FuncKey
					wantHeld  bool
				}{
					{"a", "TestOwnWide", among, true},
					{"a", "TestOwnNarrow", []testcover.FuncKey{reachedF}, true},
					{"b", "TestLinkedOnly", []testcover.FuncKey{reachedF}, true},
					{"a", "TestAbsent", nil, false},
					{"zz", "TestOwnWide", nil, false},
				}
				for _, c := range checks {
					got, source, held := r.FuncsReachedBy(c.pkg, c.test, among)
					if diff := cmp.Diff(c.want, got, cmpopts.EquateEmpty()); diff != "" {
						t.Errorf("FuncsReachedBy(%s, %s) mismatch (-want +got):\n%s", c.pkg, c.test, diff)
					}
					if held != c.wantHeld {
						t.Errorf("FuncsReachedBy(%s, %s) held = %v; want %v", c.pkg, c.test, held, c.wantHeld)
					}
					wantSource := testreach.MapSource{}
					if c.wantHeld {
						wantSource = testreach.MapSource{Package: c.pkg, Commit: "c1", BuiltAt: builtEarly}
					}
					if source != wantSource {
						t.Errorf("FuncsReachedBy(%s, %s) source = %+v; want %+v", c.pkg, c.test, source, wantSource)
					}
				}
			},
		},
		{
			name: "StaleLinkersNamesPackagesThatLinkedOlderFiles",
			setup: func(t *testing.T, f reachFixture) {
				f.writeFile(t, "a/a.go", pkgFile)
				for pkg, linkedHash := range map[string]string{"older": hashOf("old"), "current": hashOf(pkgFile), "unrelated": ""} {
					m := testcover.Map{Commit: "c1", BuiltAt: builtEarly}
					if linkedHash != "" {
						m.Linked = map[string]map[string]string{modulePath + "/a": {"a.go": linkedHash}}
					}
					writeCoverageMap(t, f.shared, pkg, m)
				}
			},
			fn:       reachedF,
			skipMaps: true,
			check: func(t *testing.T, _ reachFixture, r *testreach.Reader) {
				stale, err := r.StaleLinkers(modulePath + "/a")
				if err != nil {
					t.Fatalf("StaleLinkers() error = %v", err)
				}
				if want := []string{"older"}; !slices.Equal(stale, want) {
					t.Errorf("StaleLinkers() = %v; want %v", stale, want)
				}
			},
		},
		{
			name: "UnseenLinkerTestsReturnsTheUnseenTestsOfThePackagesLinkingThePackage",
			setup: func(t *testing.T, f reachFixture) {
				linked := map[string]map[string]string{modulePath + "/a": {"a.go": hashOf(pkgFile)}}
				failed := testcover.MapTest{Name: "TestFails", Outcome: testcover.OutcomeFailed}
				writeCoverageMap(t, f.shared, "linker", testcover.Map{
					Commit: "c1", BuiltAt: builtEarly, Linked: linked, Tests: []testcover.MapTest{failed, {Name: "TestPasses"}},
				})
				writeCoverageMap(t, f.shared, "seen", testcover.Map{
					Commit: "c1", BuiltAt: builtEarly, Linked: linked, Tests: []testcover.MapTest{{Name: "TestPasses"}},
				})
				writeCoverageMap(t, f.shared, "unrelated", testcover.Map{Commit: "c1", BuiltAt: builtEarly, Tests: []testcover.MapTest{failed}})
			},
			fn:       reachedF,
			skipMaps: true,
			check: func(t *testing.T, _ reachFixture, r *testreach.Reader) {
				want := map[string][]testreach.UnseenTest{"linker": {{Test: "TestFails", Outcome: testcover.OutcomeFailed}}}
				if diff := cmp.Diff(want, r.UnseenLinkerTests(modulePath+"/a")); diff != "" {
					t.Errorf("UnseenLinkerTests() mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name:  "UnusableMapsArePassedOverAndTheOthersStillAnswer",
			setup: unusableMaps,
			fn:    reachedF,
			want:  []testreach.Reach{aReach("TestGood", "c1")},
			wantMaps: []testreach.MapSource{
				{Package: "a", Commit: "c1", BuiltAt: builtEarly},
			},
			check: func(t *testing.T, f reachFixture, _ *testreach.Reader) {
				if _, found, err := testreach.ReadPackageMap(f.shared, "a"); err != nil || !found {
					t.Errorf("ReadPackageMap(a) = found %v, error %v; want a map", found, err)
				}
				for _, pkg := range []string{"failedbuild", "outsidetable", "absent"} {
					if _, found, err := testreach.ReadPackageMap(f.shared, pkg); err != nil || found {
						t.Errorf("ReadPackageMap(%s) = found %v, error %v; want not found", pkg, found, err)
					}
				}
				if _, _, err := testreach.ReadPackageMap(f.shared, "junk"); err == nil || !strings.Contains(err.Error(), "junk") {
					t.Errorf("ReadPackageMap(junk) error = %v; want one naming the package", err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReachFixture(t)
			tt.setup(t, f)
			r, err := testreach.Open(f.module, f.shared, f.worktree)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, r.TestsReaching(tt.fn)); diff != "" {
				t.Errorf("TestsReaching() mismatch (-want +got):\n%s", diff)
			}
			if !tt.skipMaps {
				if diff := cmp.Diff(tt.wantMaps, r.Maps(), cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("Maps() mismatch (-want +got):\n%s", diff)
				}
			}
			if tt.check != nil {
				tt.check(t, f, r)
			}
		})
	}
}

func TestFresh(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		ownFile    = "package a\n"
		otherFile  = "fixture\n"
		linkedFile = "package b\n"
	)
	tests := []struct {
		name   string
		mutate func(t *testing.T, f reachFixture)
		want   bool
	}{
		{"MatchingFiles", func(*testing.T, reachFixture) {}, true},
		{"ChangedOwnFile", func(t *testing.T, f reachFixture) { f.writeFile(t, "a/a.go", "package a\n\nvar x = 1\n") }, false},
		{"AddedFile", func(t *testing.T, f reachFixture) { f.writeFile(t, "a/extra.go", "package a\n") }, false},
		{"ChangedOtherFile", func(t *testing.T, f reachFixture) { f.writeFile(t, "a/testdata/d.txt", "changed\n") }, false},
		{"ChangedLinkedPackageFile", func(t *testing.T, f reachFixture) { f.writeFile(t, "b/b.go", "package b\n\nvar y = 1\n") }, false},
		{"FileOfANestedPackageIsNotThePackages", func(t *testing.T, f reachFixture) { f.writeFile(t, "a/inner/i.go", "package inner\n") }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReachFixture(t)
			f.writeFile(t, "a/a.go", ownFile)
			f.writeFile(t, "a/testdata/d.txt", otherFile)
			f.writeFile(t, "b/b.go", linkedFile)
			m := testcover.Map{
				Files:      map[string]string{"a.go": hashOf(ownFile)},
				OtherFiles: map[string]string{"testdata/d.txt": hashOf(otherFile)},
				Linked:     map[string]map[string]string{modulePath + "/b": {"b.go": hashOf(linkedFile)}},
			}
			tt.mutate(t, f)
			got, err := testreach.Fresh(f.module, "a", m)
			if err != nil {
				t.Fatalf("Fresh() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Fresh() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestReaderIndex(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	reachedF := testcover.FuncKey{Package: modulePath + "/a", File: "a.go", Func: "F"}
	baseMap := testcover.Map{
		Commit: "c1", BuiltAt: builtEarly,
		Blocks: []testcover.BlockKey{block("a.go", "F")},
		Tests:  []testcover.MapTest{{Name: "TestF", Blocks: []int{0}}, {Name: "TestG", Outcome: testcover.OutcomeTimedOut}},
	}
	// writeIndex writes the index of f's shared maps and requires that it holds the test named holds.
	writeIndex := func(t *testing.T, f reachFixture, holds string) {
		t.Helper()
		if err := testreach.WriteIndex(f.module, f.shared); err != nil {
			t.Fatalf("WriteIndex() error = %v", err)
		}
		data, err := os.ReadFile(filepath.Join(f.shared, testreach.IndexFileName))
		if err != nil || !bytes.Contains(data, []byte(holds)) {
			t.Fatalf("index file = %q, %v; want one holding %s", data, err, holds)
		}
	}
	tests := []struct {
		name string
		// indexBytes, when set, is written as the index beside the base map.
		indexBytes []byte
		// prepare, when set, leaves the shared maps and the index in the state the row names.
		prepare func(t *testing.T, f reachFixture)
	}{
		{name: "NoIndex"},
		{name: "IndexThatDoesNotParse", indexBytes: []byte("{not json")},
		{name: "ACurrentIndex", prepare: func(t *testing.T, f reachFixture) { writeIndex(t, f, "TestF") }},
		{name: "AnIndexOfAnotherVersionWithTheCurrentStamp", prepare: func(t *testing.T, f reachFixture) {
			writeIndex(t, f, "TestF")
			path := filepath.Join(f.shared, testreach.IndexFileName)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var index map[string]any
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
			// A version 1 index holds no unseen list, so a reader that took it would answer none.
			index["version"] = 1
			for _, info := range index["packages"].(map[string]any) {
				delete(info.(map[string]any), "unseen")
			}
			if data, err = json.Marshal(index); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "AnIndexWhoseStampDiffersAfterAMapWasRewritten", prepare: func(t *testing.T, f reachFixture) {
			old := baseMap
			old.Tests = []testcover.MapTest{{Name: "TestOld", Blocks: []int{0}}}
			writeCoverageMap(t, f.shared, "a", old)
			writeIndex(t, f, "TestOld")
			writeCoverageMap(t, f.shared, "a", baseMap)
			// A map rewritten within one modification-time tick keeps its stamp, so the row moves the file's time.
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(testcover.MapPath(f.shared, "a"), later, later); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "AnIndexWrittenBeforeAMapFileWasAdded", prepare: func(t *testing.T, f reachFixture) {
			writeIndex(t, f, "TestF")
			writeCoverageMap(t, f.shared, "b", testcover.Map{
				Commit: "c1", BuiltAt: builtEarly,
				Blocks: []testcover.BlockKey{block("b.go", "H")},
				Funcs:  []testcover.FuncKey{reachedF},
				Tests:  []testcover.MapTest{{Name: "TestLinked", Blocks: []int{0}, Funcs: []int{0}}},
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReachFixture(t)
			writeCoverageMap(t, f.shared, "a", baseMap)
			indexPath := filepath.Join(f.shared, testreach.IndexFileName)
			if tt.indexBytes != nil {
				if err := os.WriteFile(indexPath, tt.indexBytes, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			indexBytes, readErr := os.ReadFile(indexPath)
			// want and wantUnseen are what the maps alone answer:
			// the index is set aside while a reader answers from them, then put back.
			var wantUnseen []testreach.UnseenTest
			want := func() []testreach.Reach {
				if indexBytes != nil {
					if err := os.Remove(indexPath); err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := os.WriteFile(indexPath, indexBytes, 0o644); err != nil {
							t.Fatal(err)
						}
					}()
				}
				r, err := testreach.Open(f.module, f.shared, "")
				if err != nil {
					t.Fatalf("Open() without the index error = %v", err)
				}
				wantUnseen = r.UnseenTests("a")
				return r.TestsReaching(reachedF)
			}()
			if len(want) == 0 || want[0].Test != "TestF" || !want[0].Own {
				t.Fatalf("TestsReaching() from the maps alone = %+v; want TestF of the own package first", want)
			}
			r, err := testreach.Open(f.module, f.shared, "")
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if diff := cmp.Diff(want, r.TestsReaching(reachedF)); diff != "" {
				t.Errorf("TestsReaching() mismatch (-want +got):\n%s", diff)
			}
			if wantG := []testreach.UnseenTest{{Test: "TestG", Outcome: testcover.OutcomeTimedOut}}; !cmp.Equal(wantUnseen, wantG) {
				t.Fatalf("UnseenTests(a) from the maps alone = %+v; want %+v", wantUnseen, wantG)
			}
			if diff := cmp.Diff(wantUnseen, r.UnseenTests("a")); diff != "" {
				t.Errorf("UnseenTests(a) mismatch (-want +got):\n%s", diff)
			}
			got, readErr := os.ReadFile(indexPath)
			if indexBytes == nil {
				if !os.IsNotExist(readErr) {
					t.Errorf("Open() wrote the index file: read error = %v", readErr)
				}
				return
			}
			if readErr != nil || !bytes.Equal(got, indexBytes) {
				t.Errorf("index file = %q, %v; want it left as %q", got, readErr, indexBytes)
			}
		})
	}
}
