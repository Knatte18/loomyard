// autonomy_integration_test.go drives the run's autonomy over one real hub with a beam pair and a task pair:
// the keys it forces in loom's and drift's resolved config and in the run's setting lookup.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeConfigFile writes content to path, creating its directory.
func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}

// TestRunAutonomy_ForcesTheRunsKeysOverARealHub reads the run's autonomy from the task's beam.
// The task sets nothing itself.
// The layers hold the values autonomy forces away from.
// With the beam's autonomy true, every reader of a forced key reads the forced value.
// With it false, every reader reads its layered value.
// The rows rewrite the one hub's beam file, so they run serially.
func TestRunAutonomy_ForcesTheRunsKeysOverARealHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	const (
		beamPair = "ra-beam"
		task     = "ra-task"
	)
	hubforge.AddPairWith(t, h, beamPair, fabricengine.AddOptions{SkipPush: true})
	beamLocation, err := lymcwd.Resolve(fabricengine.WorktreePath(l, beamPair))
	if err != nil {
		t.Fatalf("lymcwd.Resolve(beam pair): %v", err)
	}
	if _, err := h.Topology.Add(beamLocation, task, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Add(%q) from the beam pair: %v", task, err)
	}
	board := `{"version":1,"entries":[` +
		`{"id":0,"slug":"` + beamPair + `","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
		`{"id":1,"slug":"` + task + `","title":"Task","kind":"task","labels":["bug"],"project":"` + beamPair + `"}]}`
	writeConfigFile(t, filepath.Join(h.BoardDir(), "board.json"), board)

	taskAnchor := fabricengine.WorktreePath(l, task)
	taskLocation, err := lymcwd.Resolve(taskAnchor)
	if err != nil {
		t.Fatalf("lymcwd.Resolve(task pair): %v", err)
	}
	writeConfigFile(t, configengine.ConfigFile(h.BoardDir(), "drift"), "judge_ask_parent: false\njudge_interactive: true\n")
	writeConfigFile(t, configengine.LocalFile(taskAnchor, "loom"), "pr: {review_ask_parent: false}\ndiscussion: {interactive: true}\nraddle: {auto_approve: false}\n")

	tests := []struct {
		name     string
		beamFile string
		// forced says each key reads its forced value; unset, each reads its layered value.
		forced bool
	}{
		{name: "the beam's autonomy true forces the task's keys", beamFile: "autonomy: true\n", forced: true},
		{name: "the beam's autonomy false reads the layered values", beamFile: "autonomy: false\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfigFile(t, configengine.ConfigFile(fabricengine.WorktreePath(l, beamPair), "beam"), tt.beamFile)

			wanted, err := pullRequestDeps(taskLocation, "parent", false, nil).ParentReviewWanted()
			if err != nil || wanted != tt.forced {
				t.Errorf("ParentReviewWanted() = %v, %v; want %v", wanted, err, tt.forced)
			}

			drift, err := driftDeps(taskLocation, "", "", driftTables{}).Config()
			if err != nil || drift.JudgeAskParent != tt.forced || drift.JudgeInteractive == tt.forced || drift.Autonomous != tt.forced {
				t.Errorf("driftDeps().Config() = %+v, %v; want judge_ask_parent %v, judge_interactive %v and Autonomous %v", drift, err, tt.forced, !tt.forced, tt.forced)
			}

			c := &loomCLI{runID: shedrun.SelfRunID}
			if err := c.wire(taskLocation, taskLocation.AnchorPath()); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}
			stencilkit.SeedInto(t, c.runDeps.Geom.StencilsDir)
			table, err := c.env.DiscussionTable()
			if err != nil {
				t.Fatalf("DiscussionTable() = %v; want nil", err)
			}
			prompts, err := seatengine.Prompts(seatengine.Geometry{
				WorktreeRoot:  taskLocation.WorktreePath(),
				AnchorPath:    taskLocation.AnchorPath(),
				StencilsDir:   c.runDeps.Geom.StencilsDir,
				DefaultEngine: "claude",
				ParentName:    "hub:orch",
				Shortname:     "hub",
				Slug:          task,
			}, table)
			if err != nil {
				t.Fatalf("seatengine.Prompts() = %v; want nil", err)
			}
			const autonomousText = "This session is running autonomously"
			if table.Interactive == tt.forced || strings.Contains(prompts[""], autonomousText) != tt.forced {
				t.Errorf("lead seat interactive %v, prompt names the autonomous mode %v; want interactive %v and autonomous mode %v", table.Interactive, strings.Contains(prompts[""], autonomousText), !tt.forced, tt.forced)
			}
			approve, err := c.env.Setting("loom", "raddle.auto_approve")
			if err != nil || approve != tt.forced {
				t.Errorf("Setting(loom, raddle.auto_approve) = %v, %v; want %v", approve, err, tt.forced)
			}
		})
	}
}
