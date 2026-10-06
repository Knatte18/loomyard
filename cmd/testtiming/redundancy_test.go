package main

import (
	"reflect"
	"strings"
	"testing"
)

func blockSet(names ...string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

func passRun(name string, elapsed float64, blocks ...string) testRun {
	return testRun{name: name, elapsed: elapsed, action: "pass", blocks: blockSet(blocks...)}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	skipped := passRun("Skipped", 0, "b1")
	skipped.action = "skip"
	spawning := passRun("OutOfProcess", 1, "b1")
	spawning.spawn = spawnVerdict{outOfProcess: true}
	unresolved := passRun("Unresolved", 1, "b1")
	unresolved.spawn = spawnVerdict{unresolved: true}

	rows := []struct {
		name string
		runs []testRun
		want []verdict
	}{
		{
			name: "subset of another test is removable, the superset is not a candidate",
			runs: []testRun{passRun("A", 1, "b1", "b2"), passRun("B", 1, "b1", "b2", "b3")},
			want: []verdict{
				{name: "A", candidate: true, removable: true, covering: []string{"B"}},
				{name: "B"},
			},
		},
		{
			name: "mutual pair: the slowest is removed and the other kept",
			runs: []testRun{passRun("C", 2, "b1"), passRun("D", 1, "b1"), passRun("E", 1, "b9")},
			want: []verdict{
				{name: "C", candidate: true, removable: true, covering: []string{"D"}},
				{name: "D", candidate: true, covering: []string{"C"}},
				{name: "E"},
			},
		},
		{
			name: "the removable set keeps the superset, whose covering tests come by overlap and skip one that adds nothing",
			runs: []testRun{
				passRun("P", 1, "b1", "b2", "b3"), passRun("Q", 1, "b4"),
				passRun("R", 1, "b1"), passRun("T", 1, "b1", "b2", "b3", "b4"),
			},
			want: []verdict{
				{name: "P", candidate: true, removable: true, covering: []string{"T"}},
				{name: "Q", candidate: true, removable: true, covering: []string{"T"}},
				{name: "R", candidate: true, removable: true, covering: []string{"T"}},
				{name: "T", candidate: true, covering: []string{"P", "Q"}},
			},
		},
		{
			name: "no blocks, skipped, out-of-process and unresolved tests are never candidates",
			runs: []testRun{
				passRun("Empty", 1), skipped, spawning, unresolved, passRun("Base", 1, "b1", "b2"),
			},
			want: []verdict{
				{name: "Empty", reason: reasonNoBlocks},
				{name: "Skipped", reason: reasonSkipped},
				{name: "OutOfProcess", reason: reasonOutOfProcess},
				{name: "Unresolved", reason: reasonUnresolved},
				{name: "Base"},
			},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			got := classify(row.runs)
			if !reflect.DeepEqual(got, row.want) {
				t.Fatalf("classify = %+v\nwant     %+v", got, row.want)
			}
		})
	}
}

func TestRunAloneArgs(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		test string
		want []string
	}{
		{
			name: "plain name",
			test: "TestPlain",
			want: []string{"-test.run=^TestPlain$", "-test.coverprofile=/tmp/cover.out", "-test.v=true"},
		},
		{
			name: "regexp metacharacters are quoted",
			test: "Test.Dot(a+b)",
			want: []string{`-test.run=^Test\.Dot\(a\+b\)$`, "-test.coverprofile=/tmp/cover.out", "-test.v=true"},
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if got := runAloneArgs(row.test, "/tmp/cover.out"); !reflect.DeepEqual(got, row.want) {
				t.Fatalf("runAloneArgs = %q, want %q", got, row.want)
			}
		})
	}
}

func TestParseProfile(t *testing.T) {
	t.Parallel()

	profile := "mode: set\n" +
		"a/x.go:1.1,2.2 1 1\n" +
		"a/x.go:3.1,4.2 2 0\n" +
		"b/y.go:5.1,6.2 1 3\n"
	got, err := parseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	want := blockSet("a/x.go:1.1,2.2 1", "b/y.go:5.1,6.2 1")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}

	if _, err := parseProfile(strings.NewReader("mode: set\nno-count-here\n")); err == nil {
		t.Error("a profile line without a count must be an error")
	}
}

func TestRenderPackage(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name string
		in   pkgReport
		want string
	}{
		{
			name: "candidates table and no-coverage list",
			in: pkgReport{
				pkg: "internal/x", tests: 3, wall: 0.5, serial: 0.75,
				verdicts: []verdict{
					{name: "TestA", candidate: true, removable: true, covering: []string{"TestB", "TestD"}},
					{name: "TestB"},
					{name: "TestC", reason: reasonSkipped},
				},
			},
			want: "## internal/x\n\n" +
				"3 tests, wall 0.50s, serial 0.75s.\n\n" +
				"| Test | Covering tests | Removable |\n|---|---|---|\n" +
				"| `TestA` | `TestB`, `TestD` | yes |\n\n" +
				"No coverage:\n\n" +
				"- `TestC`: skipped\n",
		},
		{
			name: "nothing to report",
			in:   pkgReport{pkg: "internal/y", tests: 1, wall: 0.01, serial: 0.01, verdicts: []verdict{{name: "TestOnly"}}},
			want: "## internal/y\n\n1 tests, wall 0.01s, serial 0.01s.\n\nNo candidates.\n\nNo test lacks coverage.\n",
		},
		{
			name: "error",
			in:   pkgReport{pkg: "internal/z", err: "coverage binary: build\nfailed"},
			want: "## internal/z\n\nError: coverage binary: build failed\n",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if got := renderPackage(row.in); got != row.want {
				t.Fatalf("renderPackage =\n%q\nwant\n%q", got, row.want)
			}
		})
	}
}
