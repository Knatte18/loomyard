// testmaporder_test.go covers how `lym gate testmap` orders its jobs and splits a job's tests into rounds.

package gatecli

import (
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOrderJobs_TakesTheCostliestPackagesFirstThenImportPathOrder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const module = "example.com/ordered"
	pool := &gateslot.Pool{Dir: t.TempDir()}
	// Two records of one call, each a tier of its own so their file names differ.
	records := []gateslot.RunRecord{
		{Tier: 0, Launches: []gateslot.PackageLaunch{{Package: module + "/slow", ElapsedSeconds: 2}, {Package: module + "/quick", ElapsedSeconds: 1}}},
		{Tier: 1, Launches: []gateslot.PackageLaunch{{Package: module + "/slow", ElapsedSeconds: 3}, {Package: "example.com/other/zeta", ElapsedSeconds: 9}}},
	}
	for _, record := range records {
		if _, err := gateslot.WriteRunRecord(gateslot.RunLogDir(pool.Dir), record, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	run := &testMapRun{pool: pool, modulePath: module}
	var jobs []testMapJob
	for _, pkg := range []string{"zeta", "alpha", "quick", "slow"} {
		jobs = append(jobs, testMapJob{pkg: pkg})
	}
	var got []string
	for _, job := range run.orderJobs(jobs) {
		got = append(got, job.pkg)
	}
	// zeta's launches name another module's package, so they count nothing for this one.
	if want := []string{"slow", "quick", "alpha", "zeta"}; !slices.Equal(got, want) {
		t.Errorf("orderJobs() = %v; want %v", got, want)
	}
}

func TestNextRound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	serial := testcover.TestFunc{Name: "TestSerial"}
	first := testcover.TestFunc{Name: "TestFirst", Parallel: true}
	second := testcover.TestFunc{Name: "TestSecond", Parallel: true}
	third := testcover.TestFunc{Name: "TestThird", Parallel: true}
	names := func(funcs []testcover.TestFunc) []string {
		var named []string
		for _, fn := range funcs {
			named = append(named, fn.Name)
		}
		return named
	}
	tests := []struct {
		name      string
		queue     []testcover.TestFunc
		parallel  int
		wantRound []string
		wantRest  []string
	}{
		{"ASerialTestRunsAlone", []testcover.TestFunc{serial, first}, 4, []string{"TestSerial"}, []string{"TestFirst"}},
		{
			"ParallelTestsRunSideBySideUpToTheParallel", []testcover.TestFunc{first, serial, second, third}, 2,
			[]string{"TestFirst", "TestSecond"}, []string{"TestSerial", "TestThird"},
		},
		{"AParallelOfOneRunsEachAlone", []testcover.TestFunc{first, second}, 1, []string{"TestFirst"}, []string{"TestSecond"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			round, rest := nextRound(tt.queue, tt.parallel)
			if !slices.Equal(names(round), tt.wantRound) || !slices.Equal(names(rest), tt.wantRest) {
				t.Errorf("nextRound(%v, %d) = %v, %v; want %v, %v", names(tt.queue), tt.parallel, names(round), names(rest), tt.wantRound, tt.wantRest)
			}
		})
	}
}
