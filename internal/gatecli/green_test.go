//go:build tier0

// green_test.go covers the green record: a tier's fingerprint, what an entry covers, how a call subtracts it from a tier run, how writes merge and prune, and the malformed-file read.

package gatecli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// greenTier returns a tier whose command takes targets and args.
func greenTier() gateslot.Tier {
	return gateslot.Tier{Command: "go test {targets} {args}", Dir: ".", AllTargets: "./...", Counter: "go-json", TimeoutMin: 1, Parallel: 1, Slots: 1}
}

// TestGreenRecord covers the rows the gate scenario does not reach:
// the fingerprint's ignored fields, the args an entry covers, the none scoper carrying only a whole entry, the pruning of old trees, and a malformed file reading as empty.
func TestGreenRecord(t *testing.T) {
	t.Parallel()

	t.Run("the fingerprint ignores parallel, timeout and slots and nothing else", func(t *testing.T) {
		t.Parallel()
		base := greenTier()
		same := base
		same.Parallel, same.TimeoutMin, same.Slots = 8, 90, 0
		if tierFingerprint(base) != tierFingerprint(same) {
			t.Error("tierFingerprint changed with parallel, timeout_min and slots; want them ignored")
		}
		for name, change := range map[string]func(*gateslot.Tier){
			"command":     func(tier *gateslot.Tier) { tier.Command = "go test -count=1 {targets} {args}" },
			"env":         func(tier *gateslot.Tier) { tier.Env = map[string]string{"GOFLAGS": "-trimpath"} },
			"all_targets": func(tier *gateslot.Tier) { tier.AllTargets = "./internal/..." },
			"counter":     func(tier *gateslot.Tier) { tier.Counter = "vstest" },
		} {
			changed := base
			change(&changed)
			if tierFingerprint(base) == tierFingerprint(changed) {
				t.Errorf("tierFingerprint is unchanged by a changed %s; want it to differ", name)
			}
		}
	})

	t.Run("covered answers by fingerprint, tree, module and args", func(t *testing.T) {
		t.Parallel()
		guardFilter := []string{"-run", "^(TestA|TestB)$"}
		record := greenRecord{Entries: []greenEntry{
			{Fingerprint: "f", Tree: "t", ModuleDir: ".", Whole: true},
			{Fingerprint: "f", Tree: "t", ModuleDir: ".", Args: []string{"-count=1"}, Packages: []string{"./b"}},
			{Fingerprint: "g", Tree: "t", ModuleDir: ".", Args: guardFilter, Packages: []string{"./x"}},
			{Fingerprint: "h", Tree: "t", ModuleDir: ".", Packages: []string{"./b"}},
			{Fingerprint: "h", Tree: "t", ModuleDir: ".", Packages: []string{"./a", "./b"}},
		}}
		tests := []struct {
			name         string
			fingerprint  string
			tree         string
			moduleDir    string
			args         []string
			wantWhole    bool
			wantPackages []string
		}{
			{name: "a whole entry covers an empty-args request", fingerprint: "f", tree: "t", moduleDir: ".", wantWhole: true},
			{name: "an empty-args entry covers the guard step's own -run filter", fingerprint: "f", tree: "t", moduleDir: ".", args: guardFilter, wantWhole: true},
			{name: "an empty-args entry covers no other args", fingerprint: "f", tree: "t", moduleDir: ".", args: []string{"-run", "TestA"}},
			{name: "an entry under -count=1 covers an exact repeat of those args", fingerprint: "f", tree: "t", moduleDir: ".", args: []string{"-count=1"}, wantPackages: []string{"./b"}},
			{name: "a guard-filter entry never covers an empty-args request", fingerprint: "g", tree: "t", moduleDir: "."},
			{name: "a guard-filter entry covers an exact repeat of the guard group", fingerprint: "g", tree: "t", moduleDir: ".", args: guardFilter, wantPackages: []string{"./x"}},
			{name: "package entries of one key join", fingerprint: "h", tree: "t", moduleDir: ".", wantPackages: []string{"./a", "./b"}},
			{name: "another tree covers nothing", fingerprint: "f", tree: "u", moduleDir: "."},
			{name: "another module covers nothing", fingerprint: "f", tree: "t", moduleDir: "sub"},
			{name: "another fingerprint covers nothing", fingerprint: "z", tree: "t", moduleDir: "."},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				whole, packages := record.covered(tt.fingerprint, tt.tree, tt.moduleDir, tt.args)
				if whole != tt.wantWhole || !slices.Equal(packages, tt.wantPackages) {
					t.Errorf("covered(%q, %q, %q, %q) = %v, %q; want %v, %q", tt.fingerprint, tt.tree, tt.moduleDir, tt.args, whole, packages, tt.wantWhole, tt.wantPackages)
				}
			})
		}
	})

	t.Run("the none scoper carries only a whole entry and the go scoper also subtracts packages", func(t *testing.T) {
		t.Parallel()
		tier := greenTier()
		fingerprint := tierFingerprint(tier)
		run := tierRun{number: 0, tier: tier, invocations: []invocation{{dir: ".", targets: []string{"./a", "./b"}}}}
		root := t.TempDir()
		tests := []struct {
			name        string
			entry       greenEntry
			scoperGo    bool
			wantTargets [][]string
			wantCarried []string
		}{
			{name: "a whole entry carries under the none scoper", entry: greenEntry{Whole: true}},
			{name: "a package entry carries nothing under the none scoper", entry: greenEntry{Packages: []string{"./a"}}, wantTargets: [][]string{{"./a", "./b"}}},
			{name: "a package entry runs the rest under the go scoper", entry: greenEntry{Packages: []string{"./a"}}, scoperGo: true, wantTargets: [][]string{{"./b"}}, wantCarried: []string{"./a"}},
			{name: "a package entry naming every target carries under the go scoper", entry: greenEntry{Packages: []string{"./a", "./b"}}, scoperGo: true},
			{name: "a package entry naming other packages changes nothing", entry: greenEntry{Packages: []string{"./c"}}, scoperGo: true, wantTargets: [][]string{{"./a", "./b"}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				tt.entry.Fingerprint, tt.entry.Tree, tt.entry.ModuleDir = fingerprint, "t", "."
				green := &greenRun{root: root, tree: "t", record: greenRecord{Entries: []greenEntry{tt.entry}}, scoperGo: tt.scoperGo}
				plan := green.subtract(context.Background(), run, nil)
				var gotTargets [][]string
				for _, inv := range plan.invocations {
					gotTargets = append(gotTargets, inv.targets)
				}
				if !reflect.DeepEqual(gotTargets, tt.wantTargets) || !slices.Equal(plan.carried["."], tt.wantCarried) {
					t.Errorf("subtract() = targets %q, carried %q; want %q, %q", gotTargets, plan.carried["."], tt.wantTargets, tt.wantCarried)
				}
			})
		}
	})

	t.Run("a write keeps the current tree and each fingerprint's 20 newest whole-green commits and drops the rest", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), greenFileName)
		start := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
		const written = greenWholeCommitsKept + 5
		for i := range written {
			entry := greenEntry{Fingerprint: "f", Tree: fmt.Sprintf("tree%02d", i), Commit: fmt.Sprintf("commit%02d", i), ModuleDir: ".", Whole: true}
			if err := recordGreen(path, entry, start.Add(time.Duration(i)*time.Minute)); err != nil {
				t.Fatalf("recordGreen() = %v; want nil", err)
			}
		}
		other := greenEntry{Fingerprint: "g", Tree: "tree00", Commit: "commit00", ModuleDir: ".", Packages: []string{"./x"}}
		if err := recordGreen(path, other, start.Add(time.Hour)); err != nil {
			t.Fatalf("recordGreen() = %v; want nil", err)
		}

		record := readGreen(path)
		var commits []string
		for _, entry := range record.Entries {
			if entry.Fingerprint == "f" {
				commits = append(commits, entry.Commit)
			}
		}
		if len(commits) != greenWholeCommitsKept || !slices.Contains(commits, fmt.Sprintf("commit%02d", written-1)) || slices.Contains(commits, "commit00") {
			t.Errorf("fingerprint f commits after %d writes = %q; want the newest %d", written, commits, greenWholeCommitsKept)
		}
		if len(record.Entries) != greenWholeCommitsKept+1 || record.Entries[len(record.Entries)-1].Fingerprint != "g" {
			t.Errorf("entries = %+v; want the 20 kept whole commits and the package entry of the current tree", record.Entries)
		}
	})

	t.Run("whole entries under args never count against the kept commits and are dropped with their tree", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), greenFileName)
		start := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
		base := greenEntry{Fingerprint: "f", ModuleDir: ".", Whole: true}
		first := base
		first.Tree, first.Commit = "tree-first", "commit-first"
		if err := recordGreen(path, first, start); err != nil {
			t.Fatalf("recordGreen() = %v; want nil", err)
		}
		for i := range greenWholeCommitsKept {
			filtered := base
			filtered.Tree, filtered.Commit, filtered.Args = fmt.Sprintf("tree%02d", i), fmt.Sprintf("commit%02d", i), []string{"-run", "X"}
			if err := recordGreen(path, filtered, start.Add(time.Duration(i+1)*time.Minute)); err != nil {
				t.Fatalf("recordGreen() = %v; want nil", err)
			}
		}

		var commits []string
		for _, entry := range readGreen(path).Entries {
			commits = append(commits, entry.Commit)
		}
		want := []string{"commit-first", fmt.Sprintf("commit%02d", greenWholeCommitsKept-1)}
		if !slices.Equal(commits, want) {
			t.Errorf("commits after %d filtered passes = %q; want %q", greenWholeCommitsKept, commits, want)
		}
	})

	t.Run("a package entry joins the key's packages, and a whole entry replaces them and is never replaced by packages", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), greenFileName)
		now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
		entry := greenEntry{Fingerprint: "f", Tree: "t", Commit: "c", ModuleDir: "."}
		for _, step := range []struct {
			packages []string
			whole    bool
			want     greenEntry
		}{
			{packages: []string{"./b"}, want: greenEntry{Packages: []string{"./b"}}},
			{packages: []string{"./a"}, want: greenEntry{Packages: []string{"./a", "./b"}}},
			{whole: true, want: greenEntry{Whole: true}},
			{packages: []string{"./c"}, want: greenEntry{Whole: true}},
		} {
			next := entry
			next.Packages, next.Whole = step.packages, step.whole
			if err := recordGreen(path, next, now); err != nil {
				t.Fatalf("recordGreen() = %v; want nil", err)
			}
			entries := readGreen(path).Entries
			if len(entries) != 1 || entries[0].Whole != step.want.Whole || !slices.Equal(entries[0].Packages, step.want.Packages) {
				t.Fatalf("entries after %+v = %+v; want one entry %+v", step, entries, step.want)
			}
		}
	})

	t.Run("a malformed file reads as empty and a write replaces it", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), greenFileName)
		if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		if record := readGreen(path); len(record.Entries) != 0 {
			t.Errorf("readGreen(malformed) = %+v; want an empty record", record)
		}
		entry := greenEntry{Fingerprint: "f", Tree: "t", Commit: "c", ModuleDir: ".", Whole: true}
		if err := recordGreen(path, entry, time.Now()); err != nil {
			t.Fatalf("recordGreen() over a malformed file = %v; want nil", err)
		}
		if entries := readGreen(path).Entries; len(entries) != 1 || !entries[0].Whole {
			t.Errorf("readGreen() after the write = %+v; want the one whole entry", entries)
		}
		if record := readGreen(filepath.Join(t.TempDir(), greenFileName)); len(record.Entries) != 0 {
			t.Errorf("readGreen(absent) = %+v; want an empty record", record)
		}
	})
}

// TestGreenRecord_NewestWholeAncestor covers which entries become a diff base: only a whole pass with empty args, under the tier's current fingerprint and module directory, on an ancestor, and the newest of them.
func TestGreenRecord_NewestWholeAncestor(t *testing.T) {
	t.Parallel()

	fingerprint := tierFingerprint(greenTier())
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	whole := func(commit string, age time.Duration) greenEntry {
		return greenEntry{Fingerprint: fingerprint, Commit: commit, ModuleDir: ".", Whole: true, Written: at.Add(age)}
	}
	with := func(entry greenEntry, change func(*greenEntry)) greenEntry {
		change(&entry)
		return entry
	}
	// everyCommit accepts every commit as an ancestor of HEAD and relates no two commits by history.
	everyCommit := func(commit, of string) bool { return of == "HEAD" }
	// linear accepts the commits as a chain, oldest first, each an ancestor of the next and of HEAD.
	linear := func(chain ...string) func(commit, of string) bool {
		return func(commit, of string) bool {
			if of == "HEAD" {
				return slices.Contains(chain, commit)
			}
			return slices.Index(chain, commit) <= slices.Index(chain, of)
		}
	}

	tests := []struct {
		name       string
		entries    []greenEntry
		isAncestor func(commit, of string) bool
		want       string
	}{
		{"the newest whole-green ancestor by write time among unrelated commits", []greenEntry{whole("old", 0), whole("new", time.Hour), whole("mid", time.Minute)}, everyCommit, "new"},
		{"a commit that is not an ancestor is skipped", []greenEntry{whole("old", 0), whole("new", time.Hour)}, func(commit, of string) bool { return of == "HEAD" && commit == "old" }, "old"},
		{"the newest by history wins over a later write of an older commit", []greenEntry{whole("new", 0), whole("old", time.Hour)}, linear("old", "new"), "new"},
		{"the newest by history wins when the older commit was written first", []greenEntry{whole("old", 0), whole("new", time.Hour)}, linear("old", "new"), "new"},
		{"a narrowed entry is never a base", []greenEntry{with(whole("narrow", time.Hour), func(e *greenEntry) { e.Whole, e.Packages = false, []string{"./a"} })}, everyCommit, ""},
		{"a whole entry under args is never a base", []greenEntry{with(whole("run", time.Hour), func(e *greenEntry) { e.Args = []string{"-run", "X"} })}, everyCommit, ""},
		{"a whole entry under another fingerprint is never a base", []greenEntry{with(whole("changed", time.Hour), func(e *greenEntry) { e.Fingerprint = "other" })}, everyCommit, ""},
		{"a whole entry in another module directory is never a base", []greenEntry{with(whole("nested", time.Hour), func(e *greenEntry) { e.ModuleDir = "nested" })}, everyCommit, ""},
		{"an empty record holds no base", nil, everyCommit, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := greenRecord{Entries: tt.entries}.newestWholeAncestor(fingerprint, ".", tt.isAncestor)
			if got != tt.want || found != (tt.want != "") {
				t.Errorf("newestWholeAncestor() = (%q, %v); want (%q, %v)", got, found, tt.want, tt.want != "")
			}
		})
	}
}
