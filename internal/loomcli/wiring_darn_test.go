// wiring_darn_test.go covers the wiring a darn run adds to wire: the darn seams and the verify source and attempts.
// It covers the perch's settings from loom.yaml's Code-Review keys.
// It covers the early refusals of a malformed or absent darn.yaml and the routing per recipe.
// It covers the Darn-Write row's status and failure-record seams.
// It covers which recipes tell the Drift expect agent a plan directory.
// Every test that calls wire is Tier 1: wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantDarnLandingCall is the landing call the two-tier gate config renders for the `finalize` key.
const wantDarnLandingCall = "lym gate test --wait --gate finalize"

// wantDarnWriteCall is the call of the `darn-write` key, which alone carries `--stop-on-fail`.
const wantDarnWriteCall = "lym gate test --wait --gate darn-write --stop-on-fail"

//lym:seam internal/agentconfig#Config
//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RaddleCheck
//lym:seam internal/loomshed#RaddleFileDeps
//lym:seam internal/loomshed#RaddleWriteDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/loomshed#VerifyBudget
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shedrecipe#RaddleWriteEnv
//lym:seam internal/shuttleengine#Config
//lym:seam internal/weaverengine#ActiveDeps
//lym:seam internal/weaverengine#Config
//lym:seam internal/weaverengine#Geometry
//lym:seam internal/weaverengine#RunDeps
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_DarnRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	darnSeams := []string{"DarnTable", "Darn.Commit", "Darn.LatestOutcome", "Darn.CommittedRecord", "RecordWrite.ReadBody", "RecordWrite.ReadStored", "RecordWrite.Commit"}

	t.Run("a darn seed fills the darn seams, the verify source and the attempts", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		// A darn seed has no Plan-Review row, so wire reads no tapestry.yaml and wires a hub that holds none.
		for _, dir := range []string{loc.AnchorPath(), fabricengine.BoardDir(loc.HubPath)} {
			if err := os.Remove(configengine.ConfigFile(dir, "tapestry")); err != nil {
				t.Fatalf("Remove(tapestry.yaml in %q) = %v; want nil", dir, err)
			}
		}
		seedDarnHubConfig(t, loc.HubPath, "5", "verify_budget_min: 30", "verify_budget_min: 45")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		driftConfig := filepath.Join(fabricengine.BoardDir(loc.HubPath), "_lym", "config", "drift.yaml")
		if err := os.WriteFile(driftConfig, []byte(driftengine.ConfigTemplate()), 0o644); err != nil {
			t.Fatalf("WriteFile drift.yaml = %v; want nil", err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		nils := envkit.NilSeams(c.env)
		for _, seam := range darnSeams {
			for _, nilSeam := range nils {
				if nilSeam == seam {
					t.Errorf("c.env.%s is nil after wire() of a darn seed", seam)
				}
			}
		}
		// envkit.NilSeams skips ReviewInputs through nilLegal.
		// A direct check pins that a darn seed keeps the shared wiring's seam.
		// The seam answers no input, since darn has no Parent-Align report and no send-back.
		if c.env.ReviewInputs == nil {
			t.Fatal("ReviewInputs is nil after wire() of a darn seed; want it told")
		}
		if inputs, err := c.env.ReviewInputs(); err != nil || len(inputs) != 0 {
			t.Errorf("ReviewInputs() = %v, %v; want no input, nil", inputs, err)
		}
		if c.env.VerifyMergeBase == nil {
			t.Error("c.env.VerifyMergeBase is nil after wire() of a darn seed; want the merge base reader")
		}
		if c.env.DiscussionHistory == nil {
			t.Error("c.env.DiscussionHistory is nil after wire() of a darn seed; want the history seam")
		}
		if c.env.RecordEngine != "RecordWrite" {
			t.Errorf("c.env.RecordEngine = %q after wire() of a darn seed; want RecordWrite", c.env.RecordEngine)
		}
		if got, want := c.env.PatternRecordPath, loomengine.DiscussionDecisionRecord(loc); got != want {
			t.Errorf("c.env.PatternRecordPath = %q after wire() of a darn seed; want %q, so its review rows check the record's PATTERN declarations", got, want)
		}
		if c.env.VerifyTreeScope == nil {
			t.Error("c.env.VerifyTreeScope is nil after wire() of a darn seed; want the tree scope reader")
		}
		if c.env.PreflightBatcher != nil {
			t.Error("c.env.PreflightBatcher is set after wire() of a darn seed; want nil, so a broken batcher.yaml fails no darn Preflight")
		}
		if c.env.PreflightStencils == nil {
			t.Error("c.env.PreflightStencils is nil after wire() of a darn seed; want the writer's stencils check")
		} else if err := c.env.PreflightStencils(); err != nil {
			t.Errorf("c.env.PreflightStencils() = %v; want nil, since a value fills every marker of the shipped writer stencils", err)
		}
		if got, want := c.env.RecordWrite.RecordPath, loomengine.DiscussionDecisionRecord(loc); got != want {
			t.Errorf("c.env.RecordWrite.RecordPath = %q; want %q", got, want)
		}
		if got, want := c.env.RecordWrite.SupportLogPath, loomengine.DiscussionSupportLog(loc); got != want {
			t.Errorf("c.env.RecordWrite.SupportLogPath = %q; want %q", got, want)
		}
		if got, want := c.env.RecordWrite.DiscussionDir, loomengine.DiscussionDir(loc); got != want {
			t.Errorf("c.env.RecordWrite.DiscussionDir = %q; want %q", got, want)
		}
		for _, path := range []struct{ field, got, want string }{
			{"RecordPath", c.env.Darn.RecordPath, loomengine.DiscussionDecisionRecord(loc)},
			{"SupportLogPath", c.env.Darn.SupportLogPath, loomengine.DiscussionSupportLog(loc)},
			{"StopMarkerPath", c.env.Darn.StopMarkerPath, shedrun.StopMarker(loc, c.runID)},
			{"ConditionStopMarkerPath", c.env.Darn.ConditionStopMarkerPath, shedrun.ConditionStopMarker(loc, c.runID)},
		} {
			if path.got != path.want {
				t.Errorf("c.env.Darn.%s = %q; want %q", path.field, path.got, path.want)
			}
		}
		// The Drift agents read the task from the decision record, never the board entry, in a darn run.
		expect, err := c.env.Drift.ExpectTable(filepath.Join(t.TempDir(), "listing.yaml"))
		if err != nil {
			t.Fatalf("c.env.Drift.ExpectTable() = _, %v; want nil", err)
		}
		if reading := expect.Values["subject_reading"]; !strings.Contains(reading, loomengine.DiscussionDecisionRecord(loc)) || strings.Contains(reading, "lym board get") {
			t.Errorf("Drift expect subject reading = %q; want the decision record and no board entry read", reading)
		}
		if c.env.DarnVerifyAttempts != 5 {
			t.Errorf("c.env.DarnVerifyAttempts = %d; want 5", c.env.DarnVerifyAttempts)
		}
		if got, want := c.env.GateFindingsFile("Darn-Write"), shedrun.GateFindingsFile(loc, c.runID, "Darn-Write"); got != want {
			t.Errorf("c.env.GateFindingsFile(\"Darn-Write\") = %q; want %q", got, want)
		}
		if segment := c.env.Segments["Code-Review"]; segment.MaxBounces != 1 || segment.ConfigFile != "loom.yaml" {
			t.Errorf("Segments[Code-Review] = %+v; want loom.yaml's template budget 1", segment)
		}
		if _, ok := c.env.Segments["Darn-Review"]; ok {
			t.Error("a darn seed filled a Darn-Review segment; want the perch to be loom's Code-Review")
		}
		if command, err := c.env.VerifyCommand("finalize"); err != nil || command != wantDarnLandingCall {
			t.Errorf("c.env.VerifyCommand(finalize) = %q, %v; want the landing call %q, nil", command, err, wantDarnLandingCall)
		}
		for key, want := range map[string]string{"darn-write": wantDarnWriteCall, "code-burler-fix": "lym gate test --wait --gate code-burler-fix"} {
			if command, err := c.env.VerifyCommand(key); err != nil || command != want {
				t.Errorf("c.env.VerifyCommand(%s) = %q, %v; want %q, nil", key, command, err, want)
			}
		}
		if budget := c.env.VerifyBudget; budget.Limit != 45*time.Minute || budget.Notify == nil {
			t.Errorf("c.env.VerifyBudget = limit %v, Notify nil %v; want darn.yaml's 45 minutes and a Notify", budget.Limit, budget.Notify == nil)
		}
	})

	wireDarnHub := func(t *testing.T) *loomCLI {
		t.Helper()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		return c
	}

	t.Run("the wired stencils check refuses a marker of the writer stencil no value fills", func(t *testing.T) {
		t.Parallel()
		c := wireDarnHub(t)
		stencilsDir := c.runDeps.Geom.StencilsDir
		body, err := stencilstore.Read(stencilsDir, "darn-template-write")
		if err != nil {
			t.Fatalf("stencilstore.Read() = %v; want nil", err)
		}
		path := stencilstore.Path(stencilsDir, "darn-template-write")
		if err := os.WriteFile(path, append(body, []byte("\n{{.unfilled_marker}}\n")...), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}

		err = c.env.PreflightStencils()
		if err == nil {
			t.Fatal("c.env.PreflightStencils() = nil; want a refusal of the unfilled marker")
		}
		for _, want := range []string{`seat "darn"`, `"darn-template-write"`, "unfilled_marker"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("c.env.PreflightStencils() = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("the wired stencils check names the table build when the table cannot be built", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3", "writer: opus[medium]", "writer: nosuchalias[medium]")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		err := c.env.PreflightStencils()
		if err == nil {
			t.Fatal("c.env.PreflightStencils() = nil; want a refusal of the unresolvable writer model-spec")
		}
		for _, want := range []string{"build the darn writer table", "way forward"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("c.env.PreflightStencils() = %q; want it to contain %q", err.Error(), want)
			}
		}
	})

	t.Run("the wired stencils check keeps a newer stencil's own way forward and adds the config clause to any other build error", func(t *testing.T) {
		t.Parallel()
		const configClause = "fix the config or model spec it names"
		newer := &stencilstore.NewerBoardCopyError{Stencil: "darn-fragment-make-change-gated", WrittenBy: "newer-build", Running: "running-build"}
		tests := []struct {
			name             string
			err              error
			wantConfigClause bool
		}{
			{name: "a stencil a newer build wrote keeps its own way forward alone", err: fmt.Errorf("darn: DarnTable: Step 4 list: %w", newer)},
			{name: "any other build error gains the config or model-spec way forward", err: errors.New("darn: DarnTable: writer model-spec: unknown alias"), wantConfigClause: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				c := wireDarnHub(t)
				c.env.DarnTable = func(loomshed.DarnTold) (seatengine.Table, error) { return seatengine.Table{}, tt.err }

				got := c.env.PreflightStencils()

				if !errors.Is(got, tt.err) {
					t.Fatalf("c.env.PreflightStencils() = %v; want it to wrap %v", got, tt.err)
				}
				if has := strings.Contains(got.Error(), configClause); has != tt.wantConfigClause {
					t.Errorf("c.env.PreflightStencils() = %q; want the config clause %v", got.Error(), tt.wantConfigClause)
				}
				if n := strings.Count(got.Error(), "way forward:"); n != 1 {
					t.Errorf("c.env.PreflightStencils() = %q names %d ways forward; want 1", got.Error(), n)
				}
			})
		}
	})

	t.Run("with a tree listed the verify gate's told command stays the row key's whole call", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3", "tree_scope: {}", `tree_scope: {"docs": ""}`)
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if command, err := c.env.VerifyCommand("finalize"); err != nil || command != wantDarnLandingCall {
			t.Errorf("c.env.VerifyCommand(finalize) = %q, %v; want the landing call %q, nil", command, err, wantDarnLandingCall)
		}
	})

	t.Run("the writer's prompt drops its gate runs only for a pair run whose gate_fixer holds a model-spec", func(t *testing.T) {
		t.Parallel()

		const (
			gatedNoGateStep = "3. Run no `lym gate test` yourself."
			ungatedTestStep = "3. Test with `lym gate test` after the last edit of each commit"
		)
		tests := []struct {
			name      string
			pair      bool
			gateFixer string
			wantGated bool
		}{
			{name: "pair slug and a fixer", pair: true, gateFixer: "sonnet[medium]", wantGated: true},
			{name: "pair slug and an empty fixer", pair: true, gateFixer: ""},
			{name: "no pair slug and a fixer", pair: false, gateFixer: "sonnet[medium]"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				loc := hubLocation(t, "pair", ".")
				seedDarnHubConfig(t, loc.HubPath, "3", "gate_fixer: sonnet[medium]", fmt.Sprintf("gate_fixer: %q", tt.gateFixer))
				writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
				if tt.pair {
					recordPath := fabricengine.OriginRecordPath(loc)
					if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
						t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
					}
					if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
						t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
					}
				}
				c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
				if err := c.wire(loc, loc.AnchorPath()); err != nil {
					t.Fatalf("wire() = %v; want nil", err)
				}
				table, err := c.env.DarnTable(loomshed.DarnTold{})
				if err != nil {
					t.Fatalf("c.env.DarnTable() = _, %v; want nil", err)
				}
				step := table.Values["make_change"]
				wantStep, absentStep := ungatedTestStep, gatedNoGateStep
				if tt.wantGated {
					wantStep, absentStep = gatedNoGateStep, ungatedTestStep
				}
				if !strings.Contains(step, wantStep) || strings.Contains(step, absentStep) {
					t.Errorf("Step 4 list = %q; want it to hold %q and not %q", step, wantStep, absentStep)
				}
			})
		}
	})

	t.Run("the writer's measuring call follows the hub's gate config", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			gateConfig string
			// fixerOff turns darn.yaml's gate_fixer off, so the writer is ungated and names no measuring call.
			fixerOff bool
			// omitGateConfig leaves the hub with no gate.yaml.
			omitGateConfig bool
			wantCall       string
			// wantErr is a part of the error the table build fails with, empty for a table that builds.
			wantErr string
		}{
			{
				name:       "a darn-write tier that takes no {args} still builds the table of an ungated writer",
				gateConfig: strings.Replace(gateConfigWithTwoTiers(), `  tier0: {command: "go test -json {targets} {args}"`, `  tier0: {command: "go test -json {targets}"`, 1),
				fixerOff:   true,
			},
			{
				name:           "an absent gate.yaml fails the table of a gated writer",
				omitGateConfig: true,
				wantErr:        "gate config absent",
			},
			{
				name:       "a darn-write tier that takes no {args} fails the table of a gated writer",
				gateConfig: strings.Replace(gateConfigWithTwoTiers(), `  tier0: {command: "go test -json {targets} {args}"`, `  tier0: {command: "go test -json {targets}"`, 1),
				wantErr:    "gates.darn-write lists tier0, whose command holds no {args}",
			},
			{
				name:       "a darn-write tier that takes targets names the target",
				gateConfig: gateConfigWithTwoTiers(),
				wantCall:   "lym gate test --gate darn-write <its package or test project> -- <args>",
			},
			{
				name:       "a darn-write tier that takes no targets names none",
				gateConfig: strings.Replace(gateConfigWithTwoTiers(), `  tier0: {command: "go test -json {targets} {args}"`, `  tier0: {command: "dotnet test All.sln {args}"`, 1),
				wantCall:   "lym gate test --gate darn-write -- <args>",
			},
			{
				name:       "a darn-write key that lists an undeclared tier fails the table",
				gateConfig: strings.Replace(gateConfigWithTwoTiers(), "  darn-write: {tiers: [0], scope: diff,", "  darn-write: {tiers: [0, 7], scope: diff,", 1),
				wantErr:    "gates.darn-write lists tier7, which the hub does not declare",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				loc := hubLocation(t, "pair", ".")
				var replacements []string
				if tt.fixerOff {
					replacements = []string{"gate_fixer: sonnet[medium]", `gate_fixer: ""`}
				}
				seedDarnHubConfig(t, loc.HubPath, "3", replacements...)
				if !tt.omitGateConfig {
					writeHubGateConfig(t, loc.HubPath, tt.gateConfig)
				}
				recordPath := fabricengine.OriginRecordPath(loc)
				if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
				}
				if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
				}
				c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
				if err := c.wire(loc, loc.AnchorPath()); err != nil {
					t.Fatalf("wire() = %v; want nil", err)
				}

				table, err := c.env.DarnTable(loomshed.DarnTold{})

				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Errorf("c.env.DarnTable() = _, %v; want an error holding %q", err, tt.wantErr)
					}
					return
				}
				if err != nil {
					t.Fatalf("c.env.DarnTable() = _, %v; want nil", err)
				}
				step := table.Values["make_change"]
				if tt.fixerOff {
					if !strings.Contains(step, "3. Test with `lym gate test` after the last edit of each commit") {
						t.Errorf("Step 4 list = %q; want the ungated test step", step)
					}
					return
				}
				if !strings.Contains(step, "`"+tt.wantCall+"`") {
					t.Errorf("Step 4 list = %q; want it to run the measuring call `%s`", step, tt.wantCall)
				}
			})
		}
	})

	t.Run("a local loom override sets a darn run's perch budget and hard cap", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3")
		localPath := configengine.LocalFile(loc.AnchorPath(), "loom")
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(localPath), err)
		}
		if err := os.WriteFile(localPath, []byte("code: {max_bounces: 2, hard_max_bounces: 3}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		segment := c.env.Segments["Code-Review"]
		if segment.MaxBounces != 2 || segment.MaxBouncesKey != "loom.code.max_bounces" || segment.HardMaxBounces != 3 || segment.HardMaxBouncesKey != "loom.code.hard_max_bounces" || segment.ConfigFile != "loom.yaml" {
			t.Errorf("Segments[Code-Review] = %+v; want the local budget 2 and hard cap 3 under their code keys in loom.yaml", segment)
		}
	})

	t.Run("a loom seed fills the record seams and leaves the darn seams nil", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeTapestry}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.DiscussionHistory == nil || c.env.RecordEngine != "DiscussionWrite" {
			t.Errorf("a loom seed left DiscussionHistory nil = %v, RecordEngine = %q; want the history seam and DiscussionWrite", c.env.DiscussionHistory == nil, c.env.RecordEngine)
		}
		if c.env.DarnTable != nil || c.env.Darn.Commit != nil || c.env.VerifyMergeBase != nil || c.env.VerifyTreeScope != nil || c.env.DarnVerifyAttempts != 0 {
			t.Errorf("a loom seed filled a darn seam: DarnTable nil = %v, Darn.Commit nil = %v, VerifyMergeBase nil = %v, VerifyTreeScope nil = %v, DarnVerifyAttempts = %d", c.env.DarnTable == nil, c.env.Darn.Commit == nil, c.env.VerifyMergeBase == nil, c.env.VerifyTreeScope == nil, c.env.DarnVerifyAttempts)
		}
	})

	t.Run("a loom seed over a recorded parent branch gets the merge base reader in the round form", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		recordPath := fabricengine.OriginRecordPath(loc)
		if err := os.MkdirAll(filepath.Dir(recordPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(recordPath), err)
		}
		if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", recordPath, err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeTapestry}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		if c.env.VerifyMergeBase == nil || c.env.VerifyTreeScope != nil {
			t.Errorf("a loom seed over an origin record: VerifyMergeBase nil = %v, VerifyTreeScope nil = %v; want the reader set and the round form", c.env.VerifyMergeBase == nil, c.env.VerifyTreeScope == nil)
		}
	})

	t.Run("a malformed tree key names the editor way forward in place of the hub config one", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3", "tree_scope: {}", `tree_scope: {"./tools": ""}`)
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil {
			t.Fatal("wire() of a darn seed with a malformed tree key = nil; want a refusal")
		}
		for _, want := range []string{"./tools", "lym config darn's editor", "then re-run the lym command that refused"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("wire() error = %q; want it to contain %q", err.Error(), want)
			}
		}
		if strings.Contains(err.Error(), hubConfigWayForward) {
			t.Errorf("wire() error = %q; want no hub config way forward, which cannot clear the key", err.Error())
		}
	})

	t.Run("an absent darn.yaml names the hub config way forward", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil || !strings.HasSuffix(err.Error(), hubConfigWayForward) {
			t.Errorf("wire() error = %v; want it to end with %q", err, hubConfigWayForward)
		}
	})
}

// TestWire_SamplerRecipe pins the wiring a sampler seed adds to wire.
// It covers the Sampler-Session row's seams and the strict load of sampler.yaml.
// It also covers the tree-scope verify seams and attempts it shares with darn.
// The four seams that need reed's strand table are left to wireLanding.
//
//lym:seam internal/agentconfig#Config
//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RaddleCheck
//lym:seam internal/loomshed#RaddleFileDeps
//lym:seam internal/loomshed#RaddleWriteDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shedrecipe#RaddleWriteEnv
//lym:seam internal/shuttleengine#Config
//lym:seam internal/weaverengine#ActiveDeps
//lym:seam internal/weaverengine#Config
//lym:seam internal/weaverengine#Geometry
//lym:seam internal/weaverengine#RunDeps
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_SamplerRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a sampler seed fills the sampler seams, the tree scope and the attempts", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "5")
		writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
		writeSamplerConfig(t, loc.HubPath, samplerengine.ConfigTemplate())
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeSampler}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}

		sampler := c.env.Sampler
		for _, seam := range []struct {
			name string
			set  bool
		}{
			{"Spec", sampler.Spec != nil},
			{"Stamp", sampler.Stamp != nil},
			{"DirtyPaths", sampler.DirtyPaths != nil},
			{"CheckRecord", sampler.CheckRecord != nil},
			{"MailAgent", sampler.MailAgent != nil},
			{"PauseCovered", sampler.PauseCovered != nil},
			{"Now", sampler.Now != nil},
			{"Sleep", sampler.Sleep != nil},
		} {
			if !seam.set {
				t.Errorf("c.env.Sampler.%s is nil after wire() of a sampler seed", seam.name)
			}
		}
		if got, want := sampler.RecordPath, shedrun.SamplerRecord(loc, c.runID); got != want {
			t.Errorf("c.env.Sampler.RecordPath = %q; want %q", got, want)
		}
		if got, want := sampler.RecordLockPath, shedrun.SamplerRecordLock(loc, c.runID); got != want {
			t.Errorf("c.env.Sampler.RecordLockPath = %q; want %q", got, want)
		}
		if sampler.Sessions != nil || sampler.SessionLive != nil || sampler.RegisterSession != nil || sampler.Watch != nil {
			t.Errorf("wire() of a sampler seed filled a wireLanding seam: Sessions nil = %v, SessionLive nil = %v, RegisterSession nil = %v, Watch nil = %v", sampler.Sessions == nil, sampler.SessionLive == nil, sampler.RegisterSession == nil, sampler.Watch == nil)
		}
		if c.env.VerifyTreeScope == nil || c.env.VerifyMergeBase == nil {
			t.Errorf("a sampler seed: VerifyTreeScope nil = %v, VerifyMergeBase nil = %v; want both set, the tree scope darn's", c.env.VerifyTreeScope == nil, c.env.VerifyMergeBase == nil)
		}
		if c.env.DarnVerifyAttempts != 5 {
			t.Errorf("c.env.DarnVerifyAttempts = %d; want darn.yaml's 5", c.env.DarnVerifyAttempts)
		}
		if c.env.DarnTable != nil {
			t.Error("c.env.DarnTable is set after wire() of a sampler seed; want nil, since the recipe has no Darn-Write row")
		}

		if stamp, err := sampler.Stamp(); err != nil || stamp != 0 {
			t.Errorf("Sampler.Stamp() with no status file = %d, %v; want 0, nil", stamp, err)
		}
		statusPath, statusLock := shedrun.StatusFile(loc, c.runID), shedrun.StatusLock(loc, c.runID)
		if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
			t.Fatal(err)
		}
		status := shedengine.Status{
			CurrentProducer: loomshed.NameSamplerSession,
			State:           shedengine.StateRunning,
			History: []shedengine.HistoryEntry{
				{Producer: loomshed.NameRecordWrite, Outcome: shedengine.Done},
				{Producer: loomshed.NameDriftExpect, Outcome: shedengine.OutcomeSkipped},
			},
		}
		if err := state.WriteJSON(statusPath, statusLock, status); err != nil {
			t.Fatalf("state.WriteJSON() = %v; want nil", err)
		}
		if stamp, err := sampler.Stamp(); err != nil || stamp != 1 {
			t.Errorf("Sampler.Stamp() after Drift-Expect is Skipped = %d, %v; want 1, nil", stamp, err)
		}
	})

	t.Run("a malformed sampler.yaml is refused naming the key and the editor", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "5")
		writeSamplerConfig(t, loc.HubPath, strings.Replace(samplerengine.ConfigTemplate(), "agent: opus[high]", `agent: "opus[[bad"`, 1))
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeSampler}
		err := c.wire(loc, loc.AnchorPath())
		if err == nil {
			t.Fatal("wire() of a sampler seed with a malformed agent = nil; want a refusal")
		}
		for _, want := range []string{"agent", "lym config sampler", "lym loom resume"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("wire() error = %q; want it to contain %q", err.Error(), want)
			}
		}
	})
}

// TestWire_DriftExpectPlanReading pins which recipes tell the Drift expect agent a plan directory:
// only a recipe with a Plan-Write row does, and a sampler or darn run is told it has no plan.
func TestWire_DriftExpectPlanReading(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name       string
		recipe     string
		wantHasDir bool
	}{
		{name: "sampler", recipe: shedrun.RecipeSampler},
		{name: "darn", recipe: shedrun.RecipeDarn},
		{name: "tapestry", recipe: shedrun.RecipeTapestry, wantHasDir: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			seedDarnHubConfig(t, loc.HubPath, "3")
			writeHubGateConfig(t, loc.HubPath, gateConfigWithTwoTiers())
			boardConfigDir := filepath.Join(fabricengine.BoardDir(loc.HubPath), "_lym", "config")
			if err := os.WriteFile(filepath.Join(boardConfigDir, "drift.yaml"), []byte(driftengine.ConfigTemplate()), 0o644); err != nil {
				t.Fatalf("WriteFile drift.yaml = %v; want nil", err)
			}
			if tt.recipe == shedrun.RecipeSampler {
				samplerPath := configengine.ConfigFile(fabricengine.BoardDir(loc.HubPath), "sampler")
				if err := os.WriteFile(samplerPath, []byte(samplerengine.ConfigTemplate()), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", samplerPath, err)
				}
			}
			c := &loomCLI{runID: shedrun.SelfRunID, recipe: tt.recipe}
			if err := c.wire(loc, loc.AnchorPath()); err != nil {
				t.Fatalf("wire() = %v; want nil", err)
			}

			expect, err := c.env.Drift.ExpectTable(filepath.Join(t.TempDir(), "listing.yaml"))
			if err != nil {
				t.Fatalf("c.env.Drift.ExpectTable() = _, %v; want nil", err)
			}
			reading := expect.Values["plan_reading"]
			planDir := planparser.PlanDir(loc.AnchorPath())
			if got := strings.Contains(reading, planDir); got != tt.wantHasDir {
				t.Errorf("plan_reading of a %s run = %q; want the plan directory %q present = %v", tt.name, reading, planDir, tt.wantHasDir)
			}
			if !tt.wantHasDir && !strings.HasPrefix(reading, "none:") {
				t.Errorf("plan_reading of a %s run = %q; want it to say the run has no plan", tt.name, reading)
			}
		})
	}
}

func TestRecipeRouting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		recipe  string
		wantRow string
		absent  string
	}{
		{shedrun.RecipeTapestry, loomshed.NameParentAlign, loomshed.NameDarnWrite},
		{"", loomshed.NameParentAlign, loomshed.NameDarnWrite},
		{shedrun.RecipeDarn, loomshed.NameDarnWrite, loomshed.NameParentAlign},
		{shedrun.RecipePawl, loomshed.NameFinalize, loomshed.NameCodeBouncer},
	}
	for _, tt := range tests {
		t.Run("recipe "+tt.recipe, func(t *testing.T) {
			t.Parallel()
			c := &loomCLI{recipe: tt.recipe}
			c.cfg.Review.MaxBounces, c.cfg.Review.HardMaxBounces = 3, 4
			routing, err := c.recipeRouting()
			if err != nil {
				t.Fatalf("recipeRouting() = %v; want nil", err)
			}
			rows := map[string]bool{}
			for _, p := range routing.Producers {
				rows[p.Name] = true
			}
			if !rows[tt.wantRow] || rows[tt.absent] {
				t.Errorf("recipeRouting() rows = %v; want %q and not %q", rows, tt.wantRow, tt.absent)
			}
		})
	}
}

func TestLatestDarnOutcome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		status *shedengine.Status
		// findingsKept writes the row's gate findings file before the read.
		findingsKept bool
		// findingsWritten, when set, is the RFC 3339 time the kept findings file was last written.
		findingsWritten string
		wantFound       bool
		want            loomshed.DarnOutcome
		// wantFindings expects the outcome to name the kept findings file.
		wantFindings bool
		// invalidPath gives the findings path a NUL byte, so stat fails with an invalid-argument error on every platform.
		invalidPath bool
		wantErr     bool
	}{
		{name: "absent status file"},
		{name: "no Darn-Write entry", status: &shedengine.Status{History: []shedengine.HistoryEntry{{Producer: loomshed.NamePreflight, Outcome: shedengine.Done}}}},
		{
			name:      "latest Darn-Write entry of a run that moved on, with no findings kept",
			status:    &shedengine.Status{CurrentProducer: loomshed.NameFinalize, State: shedengine.StateRunning, Error: "stale", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarnWrite, Outcome: shedengine.Done}}},
			wantFound: true,
			want:      loomshed.DarnOutcome{Outcome: shedengine.Done},
		},
		{
			name:         "a Done latest entry names no findings file an earlier halt kept",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameFinalize, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarnWrite, Outcome: shedengine.Done}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Done},
		},
		{
			name:         "a resumed run, its error cleared, names the kept findings",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings: true,
		},
		{
			name: "a later Stuck whose own gate wrote no findings names no file an earlier halt kept",
			status: &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{
				{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck, StartedAt: "2026-11-20T10:00:00Z"},
				{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Done, StartedAt: "2026-11-20T11:00:00Z"},
				{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck, StartedAt: "2026-11-20T12:00:00Z"},
			}},
			findingsKept:    true,
			findingsWritten: "2026-11-20T10:30:00Z",
			wantFound:       true,
			want:            loomshed.DarnOutcome{Outcome: shedengine.Stuck},
		},
		{
			name: "a Stuck whose own gate wrote the findings names the file",
			status: &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{
				{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck, StartedAt: "2026-11-20T12:00:00Z"},
			}},
			findingsKept:    true,
			findingsWritten: "2026-11-20T12:30:00Z",
			wantFound:       true,
			want:            loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings:    true,
		},
		{
			name:         "a Darn-Write reroute entry is skipped for the last real entry",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateBlocked, Error: "verify gate failed", History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}, {Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeReroute}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings: true,
		},
		{
			name:   "a lone Darn-Write reroute entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameParentAlign, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeReroute}}},
		},
		{
			name:        "a findings path that cannot be statted is returned as the error",
			status:      &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}},
			invalidPath: true,
			wantErr:     true,
		},
		{
			name:   "a lone Darn-Write goto entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeGoto}}},
		},
		{
			name:   "a lone Darn-Write skipped entry reports not found",
			status: &shedengine.Status{CurrentProducer: loomshed.NameFinalize, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.OutcomeSkipped}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			statusPath, lockPath := filepath.Join(dir, "status.json"), filepath.Join(dir, "status.json.lock")
			if tt.status != nil {
				tt.status.State = firstNonEmptyState(tt.status.State)
				if err := state.WriteJSON(statusPath, lockPath, *tt.status); err != nil {
					t.Fatalf("WriteJSON() = %v; want nil", err)
				}
			}
			findingsPath := filepath.Join(dir, "Darn-Write.findings.md")
			if tt.findingsKept {
				if err := os.WriteFile(findingsPath, []byte("verify failed"), 0o644); err != nil {
					t.Fatalf("write the findings file: %v", err)
				}
			}
			if tt.findingsWritten != "" {
				written, err := time.Parse(time.RFC3339, tt.findingsWritten)
				if err != nil {
					t.Fatalf("parse findingsWritten: %v", err)
				}
				if err := os.Chtimes(findingsPath, written, written); err != nil {
					t.Fatalf("Chtimes(%q) = %v; want nil", findingsPath, err)
				}
			}
			if tt.invalidPath {
				findingsPath = filepath.Join(dir, "Darn-Write\x00.findings.md")
			}
			want := tt.want
			if tt.wantFindings {
				want.FindingsPath = findingsPath
			}
			got, found, err := latestDarnOutcome(statusPath, lockPath, findingsPath)
			if tt.wantErr {
				if err == nil || found || got != (loomshed.DarnOutcome{}) {
					t.Errorf("latestDarnOutcome() = %+v, %v, %v; want the stat error and not found", got, found, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("latestDarnOutcome() error = %v; want nil", err)
			}
			if found != tt.wantFound || got != want {
				t.Errorf("latestDarnOutcome() = %+v, %v; want %+v, %v", got, found, want, tt.wantFound)
			}
		})
	}
}

// TestDarnDeps_LatestOutcomeNamesTheRowsFindingsFile pins that the LatestOutcome seam reads the run's status file.
// It names the findings file Env.GateFindingsFile gives the Darn-Write row.
func TestDarnDeps_LatestOutcomeNamesTheRowsFindingsFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	loc := hubLocation(t, "pair", ".")
	statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
		t.Fatal(err)
	}
	seed := shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}}
	if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
		t.Fatalf("state.WriteJSON() = %v; want nil", err)
	}
	findingsPath := shedrun.GateFindingsFile(loc, shedrun.SelfRunID, loomshed.NameDarnWrite)
	if err := os.MkdirAll(filepath.Dir(findingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findingsPath, []byte("verify failed"), 0o644); err != nil {
		t.Fatalf("write the findings file: %v", err)
	}

	got, found, err := darnDeps(loc, shedrun.SelfRunID, shedrun.RecipeDarn).LatestOutcome()
	want := loomshed.DarnOutcome{Outcome: shedengine.Stuck, FindingsPath: findingsPath}
	if err != nil || !found || got != want {
		t.Errorf("LatestOutcome() = %+v, %v, %v; want %+v, true, nil", got, found, err, want)
	}
}

// firstNonEmptyState returns st, or the running state when st is empty.
// A status file with no state does not decode, which is why an empty st reads as running.
func firstNonEmptyState(st shedengine.State) shedengine.State {
	if st == "" {
		return shedengine.StateRunning
	}
	return st
}
