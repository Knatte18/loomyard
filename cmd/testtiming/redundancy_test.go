package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestJudgePackage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	covered := func(file string, line int) testcover.ProfileBlock {
		return testcover.ProfileBlock{File: file, StartLine: line, StartCol: 1, EndLine: line, EndCol: 2, Statements: 1, Count: 1}
	}
	run := func(name string, elapsed float64, blocks ...testcover.ProfileBlock) measuredRun {
		return measuredRun{result: testcover.RunResult{Name: name, Elapsed: elapsed, Action: testcover.ActionPass, Profile: blocks}}
	}
	// TestNarrow's package blocks are a subset of TestWide's, and its one block in m/q is its own.
	// TestFixtureOnly covers blocks of m/q alone, so it covers nothing of its package.
	runs := []measuredRun{
		run("TestNarrow", 2, covered("m/p/x.go", 1), covered("m/q/y.go", 1)),
		run("TestWide", 1, covered("m/p/x.go", 1), covered("m/p/x.go", 2), covered("m/q/y.go", 2)),
		run("TestFixtureOnly", 1, covered("m/q/y.go", 3)),
	}
	got := judgePackage("m/p", runs, map[string]string{"TestWide": "pins the order"})
	want := []verdict{
		{name: "TestNarrow", candidate: true, removable: true, covering: []string{"TestWide"}},
		{name: "TestWide", keep: "pins the order"},
		{name: "TestFixtureOnly", reason: testcover.ReasonNoBlocks},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("judgePackage =\n%+v\nwant\n%+v", got, want)
	}

	section := renderPackage(pkgReport{pkg: "p", tests: 3, verdicts: got})
	if wantRow := "| `TestNarrow` | `TestWide` | yes | no |\n"; !strings.Contains(section, wantRow) {
		t.Errorf("section =\n%s\nwant the row %q marking the candidate not contained module-wide", section, wantRow)
	}

	// TestSmall's only block is also TestBig's, so it is contained in the package and in the module.
	// Its keep reason leaves the classification as it is.
	containedRuns := []measuredRun{
		run("TestSmall", 2, covered("m/p/x.go", 1)),
		run("TestBig", 1, covered("m/p/x.go", 1), covered("m/p/x.go", 2), covered("m/q/y.go", 1)),
	}
	gotContained := judgePackage("m/p", containedRuns, map[string]string{"TestSmall": "pins the error text"})
	wantContained := []verdict{
		{name: "TestSmall", candidate: true, removable: true, covering: []string{"TestBig"}, moduleContained: true, keep: "pins the error text"},
		{name: "TestBig"},
	}
	if !reflect.DeepEqual(gotContained, wantContained) {
		t.Fatalf("judgePackage =\n%+v\nwant\n%+v", gotContained, wantContained)
	}
}

func TestRenderPackage(t *testing.T) {
	tiergate.Require(t, 0)
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
					{name: "TestA", candidate: true, removable: true, covering: []string{"TestB", "TestD"}, moduleContained: true},
					{name: "TestB"},
					{name: "TestC", reason: testcover.ReasonSkipped},
				},
			},
			want: "## internal/x\n\n" +
				"3 tests, wall 0.50s, serial 0.75s.\n\n" +
				"| Test | Covering tests | Removable | Contained module-wide |\n|---|---|---|---|\n" +
				"| `TestA` | `TestB`, `TestD` | yes | yes |\n\n" +
				"No coverage:\n\n" +
				"- `TestC`: skipped\n",
		},
		{
			name: "kept tests are listed under Kept and a kept candidate stays in the covering pool",
			in: pkgReport{
				pkg: "internal/k", tests: 4, wall: 0.5, serial: 0.75,
				verdicts: []verdict{
					{name: "TestKeptCandidate", candidate: true, covering: []string{"TestBase"}, keep: "pins the error text"},
					{name: "TestOther", candidate: true, removable: true, covering: []string{"TestKeptCandidate"}},
					{name: "TestBase"},
					{name: "TestKeptPlain", keep: "pins the ordering"},
				},
			},
			want: "## internal/k\n\n" +
				"4 tests, wall 0.50s, serial 0.75s.\n\n" +
				"| Test | Covering tests | Removable | Contained module-wide |\n|---|---|---|---|\n" +
				"| `TestOther` | `TestKeptCandidate` | yes | no |\n\n" +
				"Kept:\n\n" +
				"- `TestKeptCandidate`: pins the error text\n" +
				"- `TestKeptPlain`: pins the ordering\n\n" +
				"No test lacks coverage.\n",
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
