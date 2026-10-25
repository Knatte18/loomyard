//go:build tier1

// status_integration_test.go runs "lyx beam status" through RunCLIIn over a hubforge hub with one local beam.

package beamcli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/beamcli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestStatus_PrintsThePrimesDobbyAndEachLocalBeams(t *testing.T) {
	t.Parallel()

	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

	const runSlug = "run-x"
	if err := shedrun.WriteSeed(h.Location, runSlug, shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
	}
	runStatus, err := json.Marshal(shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Plan"})
	if err != nil {
		t.Fatalf("Marshal(status) = %v; want nil", err)
	}
	statusFile := shedrun.StatusFile(h.Location, runSlug)
	if err := os.MkdirAll(filepath.Dir(statusFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", filepath.Dir(statusFile), err)
	}
	if err := os.WriteFile(statusFile, runStatus, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", statusFile, err)
	}

	var out bytes.Buffer
	beamcli.RunCLIIn(h.PairCodeWorktree("b"), &out, []string{"status"})

	env := envelope.Decode(t, out.String())
	if !env.OK {
		t.Fatalf("lyx beam status = %s; want ok", out.String())
	}
	var result struct {
		Dobbies []struct {
			Worktree string `json:"worktree"`
			View     struct {
				Alive bool `json:"alive"`
			} `json:"view"`
			Lines []string `json:"lines"`
		} `json:"dobbies"`
	}
	encoded, err := json.Marshal(env.Raw["result"])
	if err != nil {
		t.Fatalf("Marshal(result) = %v; want nil", err)
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("Unmarshal(result) = %v; want nil", err)
	}

	type entry struct {
		Worktree string
		Alive    bool
		Header   string
		// Runs is the run lines after the header, with their column gaps collapsed.
		Runs []string
	}
	var got []entry
	for _, dobby := range result.Dobbies {
		header := ""
		var runs []string
		if len(dobby.Lines) > 0 {
			header = dobby.Lines[0]
			for _, line := range dobby.Lines[1:] {
				runs = append(runs, strings.Join(strings.Fields(line), " "))
			}
		}
		got = append(got, entry{dobby.Worktree, dobby.View.Alive, header, runs})
	}
	want := []entry{
		{"prime", false, "dobby prime: not running; way forward: lyx beam start", []string{"run-x running Plan 0m no pass yet"}},
		{"b", false, "dobby b: not running; way forward: lyx beam start", []string{"no runs driven"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("lyx beam status dobbies mismatch (-want +got):\n%s", diff)
	}
}
