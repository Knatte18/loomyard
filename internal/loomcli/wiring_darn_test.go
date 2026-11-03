// wiring_darn_test.go covers the wiring a darn run adds to wire: the darn seams, the verify source and attempts, the perch's settings taken from loom.yaml's Code-Review keys, the early refusals of a malformed or absent darn.yaml, the routing per recipe, the rename of retired darn rows when a verb arms, and the Darn-Write row's status and failure-record seams.
// Every test is Tier 1: wire resolves no cwd and spawns no process over a hand-built hub.

package loomcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/stencilstore"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// wantDarnLandingCall is the landing call gateConfigWithTwoTiers renders for the `finalize` key.
const wantDarnLandingCall = "lym gate test --wait --gate finalize"

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_DarnRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	darnSeams := []string{"DarnTable", "Darn.Commit", "Darn.LatestOutcome", "Darn.CommittedRecord", "RecordWrite.ReadBody", "RecordWrite.Commit"}

	t.Run("a darn seed fills the darn seams, the verify source and the attempts", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		// A darn seed has no Plan-Review row, so wire reads no tapestry.yaml and wires a hub that holds none.
		for _, dir := range []string{loc.AnchorPath(), fabricengine.BoardDir(loc.HubPath)} {
			if err := os.Remove(configengine.ConfigFile(dir, "tapestry")); err != nil {
				t.Fatalf("Remove(tapestry.yaml in %q) = %v; want nil", dir, err)
			}
		}
		seedDarnHubConfig(t, loc.HubPath, "5")
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
		// envkit.NilSeams skips ReviewInput through nilLegal, so a direct check pins that a darn seed keeps the shared wiring's seam, which answers not found since darn has no Parent-Align report.
		if c.env.ReviewInput == nil {
			t.Fatal("ReviewInput is nil after wire() of a darn seed; want it told")
		}
		if path, _, found, err := c.env.ReviewInput(); err != nil || found {
			t.Errorf("ReviewInput() = %q, %v, %v; want not found, nil", path, found, err)
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
		baseline, err := c.env.Drift.BaselineTable("")
		if err != nil {
			t.Fatalf("c.env.Drift.BaselineTable() = _, %v; want nil", err)
		}
		if reading := baseline.Values["subject_reading"]; !strings.Contains(reading, loomengine.DiscussionDecisionRecord(loc)) || strings.Contains(reading, "lym board get") {
			t.Errorf("Drift baseline subject reading = %q; want the decision record and no board entry read", reading)
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

	t.Run("a local loom override sets a darn run's perch budget and hard cap", func(t *testing.T) {
		t.Parallel()
		loc := hubLocation(t, "pair", ".")
		seedDarnHubConfig(t, loc.HubPath, "3")
		localPath := configengine.LocalFile(loc.AnchorPath(), "loom")
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(localPath), err)
		}
		if err := os.WriteFile(localPath, []byte("code_max_bounces: 2\ncode_hard_max_bounces: 3\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
		}
		c := &loomCLI{runID: shedrun.SelfRunID, recipe: shedrun.RecipeDarn}
		if err := c.wire(loc, loc.AnchorPath()); err != nil {
			t.Fatalf("wire() = %v; want nil", err)
		}
		segment := c.env.Segments["Code-Review"]
		if segment.MaxBounces != 2 || segment.MaxBouncesKey != "code_max_bounces" || segment.HardMaxBounces != 3 || segment.HardMaxBouncesKey != "code_hard_max_bounces" || segment.ConfigFile != "loom.yaml" {
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

// TestWire_SamplerRecipe pins the wiring a sampler seed adds to wire: the Sampler-Session row's seams, the tree-scope verify seams and attempts it shares with darn, and the strict load of sampler.yaml.
// The four seams that need reed's strand table are left to wireLanding.
//
//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/landingshed#PullRequestDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#RecordWriteDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/loomshed#SamplerDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_SamplerRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	writeSamplerConfig := func(t *testing.T, hubPath, contents string) {
		t.Helper()
		path := configengine.ConfigFile(fabricengine.BoardDir(hubPath), "sampler")
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}

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
			History:         []shedengine.HistoryEntry{{Producer: loomshed.NameRecordWrite, Outcome: shedengine.Done}},
		}
		if err := state.WriteJSON(statusPath, statusLock, status); err != nil {
			t.Fatalf("state.WriteJSON() = %v; want nil", err)
		}
		if stamp, err := sampler.Stamp(); err != nil || stamp != 1 {
			t.Errorf("Sampler.Stamp() after Record-Write is Done = %d, %v; want 1, nil", stamp, err)
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

// TestArmAt_RenamesRetiredDarnRows pins that arming any verb over a darn run carries a status file still naming the writer row's former name onto Darn-Write, and leaves a tapestry run's file alone.
// A live driver holding the run lock keeps the old name, and the verb still arms.
func TestArmAt_RenamesRetiredDarnRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name        string
		recipe      string
		holdRunLock bool
		want        string
	}{
		{name: "darn", recipe: shedrun.RecipeDarn, want: loomshed.NameDarnWrite},
		{name: "tapestry", recipe: shedrun.RecipeTapestry, want: "Darn"},
		{name: "darn with a live driver", recipe: shedrun.RecipeDarn, holdRunLock: true, want: "Darn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			if err := shedrun.WriteSeed(loc, shedrun.SelfRunID, shedrun.Seed{Recipe: tt.recipe, Driver: shedrun.DriverGo}); err != nil {
				t.Fatalf("shedrun.WriteSeed() = %v; want nil", err)
			}
			statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := shedengine.Status{CurrentProducer: "Darn", State: shedengine.StateBlocked, History: []shedengine.HistoryEntry{{Producer: "Darn", Outcome: shedengine.Stuck}}}
			if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
				t.Fatalf("state.WriteJSON() = %v; want nil", err)
			}
			if tt.holdRunLock {
				runLock := shedrun.RunLock(loc, shedrun.SelfRunID)
				held, locked, err := lock.TryAcquireWriteLock(runLock)
				if err != nil || !locked {
					t.Fatalf("TryAcquireWriteLock(%q) = _, %v, %v; want held", runLock, locked, err)
				}
				t.Cleanup(func() { _ = held.Release() })
			}

			c := &loomCLI{}
			if _, err := c.armAt(loc, "status", nil); err != nil {
				t.Fatalf("armAt(status) = %v; want nil", err)
			}
			got, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if err != nil {
				t.Fatalf("read status = %v; want nil", err)
			}
			if got.CurrentProducer != tt.want || got.History[0].Producer != tt.want {
				t.Errorf("CurrentProducer, History[0].Producer = %q, %q; want both %q", got.CurrentProducer, got.History[0].Producer, tt.want)
			}
		})
	}
}

// TestRenameRetiredRows pins that renameRetiredRows carries every field of a status file naming a retired row onto its successor, for the loom table and for the darn one, and leaves other rows alone.
func TestRenameRetiredRows(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name    string
		renames map[string]string
		retired string
		want    string
	}{
		{name: "a tapestry run parked on Merge-In", renames: loomrecipe.RetiredRows(shedrun.RecipeTapestry), retired: "Merge-In", want: loomshed.NameParentAlign},
		{name: "a tapestry run parked on Loom-Preflight", renames: loomrecipe.RetiredRows(shedrun.RecipeTapestry), retired: "Loom-Preflight", want: loomshed.NamePreflight},
		{name: "a tapestry run parked on Tapestry-Preflight", renames: loomrecipe.RetiredRows(shedrun.RecipeTapestry), retired: "Tapestry-Preflight", want: loomshed.NamePreflight},
		{name: "a darn run parked on Darn", renames: loomrecipe.RetiredRows(shedrun.RecipeDarn), retired: "Darn", want: loomshed.NameDarnWrite},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
			if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
				t.Fatal(err)
			}
			seed := shedengine.Status{
				CurrentProducer: tt.retired, EnteredRow: tt.retired, PauseBefore: tt.retired, PauseAfter: tt.retired,
				State:   shedengine.StateBlocked,
				History: []shedengine.HistoryEntry{{Producer: loomshed.NameWebster, Outcome: shedengine.Done}, {Producer: tt.retired, Outcome: shedengine.Stuck}},
			}
			if err := state.WriteJSON(statusPath, statusLock, seed); err != nil {
				t.Fatalf("state.WriteJSON() = %v; want nil", err)
			}

			c := &loomCLI{runID: shedrun.SelfRunID, shedPaths: shedbuild.ShedPaths{
				StatusPath: statusPath, StatusLockPath: statusLock, LockPath: shedrun.RunLock(loc, shedrun.SelfRunID),
			}}
			if err := c.renameRetiredRows(tt.renames); err != nil {
				t.Fatalf("renameRetiredRows() = %v; want nil", err)
			}

			got, _, err := state.ReadJSON[shedengine.Status](statusPath, statusLock)
			if err != nil {
				t.Fatalf("read status = %v; want nil", err)
			}
			fields := []string{got.CurrentProducer, got.EnteredRow, got.PauseBefore, got.PauseAfter, got.History[1].Producer}
			for i, field := range fields {
				if field != tt.want {
					t.Errorf("renamed field %d = %q; want %q", i, field, tt.want)
				}
			}
			if got.History[0].Producer != loomshed.NameWebster {
				t.Errorf("History[0].Producer = %q; want %q, untouched", got.History[0].Producer, loomshed.NameWebster)
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
		{shedrun.RecipePawl, "Beam-Describe", loomshed.NameCodeBouncer},
	}
	for _, tt := range tests {
		t.Run("recipe "+tt.recipe, func(t *testing.T) {
			t.Parallel()
			c := &loomCLI{recipe: tt.recipe}
			c.cfg.ReviewMaxBounces, c.cfg.ReviewHardMaxBounces = 3, 4
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
		wantFound    bool
		want         loomshed.DarnOutcome
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
			name:         "a resumed run, its error cleared, names the kept findings",
			status:       &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: loomshed.NameDarnWrite, Outcome: shedengine.Stuck}}},
			findingsKept: true,
			wantFound:    true,
			want:         loomshed.DarnOutcome{Outcome: shedengine.Stuck},
			wantFindings: true,
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

// TestDarnDeps_LatestOutcomeNamesTheRowsFindingsFile pins that the LatestOutcome seam reads the run's status file and names the findings file Env.GateFindingsFile gives the Darn-Write row.
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

// firstNonEmptyState returns st, or the running state when st is empty, since a status file with no state does not decode.
func firstNonEmptyState(st shedengine.State) shedengine.State {
	if st == "" {
		return shedengine.StateRunning
	}
	return st
}
