// testmaporder_test.go covers how `lym gate testmap` orders its jobs.

package gatecli

import (
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
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
