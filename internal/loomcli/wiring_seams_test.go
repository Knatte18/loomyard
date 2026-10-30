//go:build tier0

package loomcli

import (
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// intentionallyNil maps each shedrecipe.Env field path wire leaves nil to the reason.
// envkit.NilSeams already skips the seams whose nil is a documented default, so only wire's own gaps are listed.
var intentionallyNil = map[string]string{
	"Landing.BaseCommand":                 "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.VerifyHold":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.GateTiers":                   "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.OpenFabric":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.OpenParentFabric":            "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.TaskHead":                    "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.Shuttle":                     "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.EnginesRun":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.Lock.Store":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.Lock.Owner":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.Lock.PauseRequested":         "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.ReadParentTips":              "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.HoldsWork":                   "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.ChangedPaths":                "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.MergeBase":                   "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.FixerSeats":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.LineChange":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.IsTestFile":                  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.RowEntry":                    "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.TaskRemoteTip":               "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.CodeReviewOff":               "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.RerouteReviewed":             "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.ParentMode":      "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.EntryField":      "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.Origin":          "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.API":             "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.PushTask":        "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.CommitsMissing":  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.TaskHasCommits":  "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.DirtyPaths":      "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.MailAgent":       "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.PauseCovered":    "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.Now":             "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.Sleep":           "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.Sessions":        "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.SessionLive":     "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"Landing.PullRequest.RegisterSession": "Env.Landing is assembled in run.go, because its constructors open the fabric pair eagerly",
	"DarnTable":                           "darn-only seam, filled by wire for a darn seed",
	"Darn.Commit":                         "darn-only seam, filled by wire for a darn seed",
	"Darn.LatestOutcome":                  "darn-only seam, filled by wire for a darn seed",
	"CreateWorktree":                      "batten-only seam, filled by battencli",
	"ExecutableSetting":                   "no loom or darn row is a Command row; battencli alone fills it",
	"Command.TaskDir":                     "no loom or darn row is a Command row; battencli alone fills it",
	"Command.LogPath":                     "no loom or darn row is a Command row; battencli alone fills it",
	"Command.Run":                         "no loom or darn row is a Command row; battencli alone fills it",
	"InnerRun.Spawn":                      "batten-only seam, filled by battencli",
	"InnerRun.ResolveStatus":              "batten-only seam, filled by battencli",
	"InnerRun.ReadStatus":                 "batten-only seam, filled by battencli",
	"InnerRun.ReadDecision":               "batten-only seam, filled by battencli",
	"InnerRun.DriverAlive":                "batten-only seam, filled by battencli",
	"InnerRun.DriverStrand":               "batten-only seam, filled by battencli",
	"InnerRun.ChildRunLockHeld":           "batten-only seam, filled by battencli",
	"InnerRun.ReviveStrands":              "batten-only seam, filled by battencli",
	"InnerRun.ArbiterInFlight":            "batten-only seam, filled by battencli",
	"InnerRun.PauseRequested":             "batten-only seam, filled by battencli",
	"InnerRun.MarkWatched":                "batten-only seam, filled by battencli",
	"InnerRun.OrchStrandRecorded":         "batten-only seam, filled by battencli",
	"InnerRun.StopReport":                 "batten-only seam, filled by battencli",
	"InnerRun.Activity":                   "batten-only seam, filled by battencli",
	"SeedChild.ReadBoardType":             "batten-only seam, filled by battencli",
	"SeedChild.ChildDriver":               "batten-only seam, filled by battencli",
	"SeedChild.WriteSeed":                 "batten-only seam, filled by battencli",
	"SeedChild.CommitSeed":                "batten-only seam, filled by battencli",
	"SeedChild.PushSeed":                  "batten-only seam, filled by battencli",
	"SeedChild.MarkWatched":               "batten-only seam, filled by battencli",
	"SeedChild.ReadRunSettings":           "batten-only seam, filled by battencli",
	"SeedChild.PrepareRunSettings":        "batten-only seam, filled by battencli",
	"SeedChild.WriteLocalOverride":        "batten-only seam, filled by battencli",
	"SeedChild.RemoveLocalOverrides":      "batten-only seam, filled by battencli",
	"InnerRun.ParentLock":                 "batten-only seam, filled by battencli",
	"Teardown.Shutdown":                   "batten-only seam, filled by battencli",
	"Teardown.Remove":                     "batten-only seam, filled by battencli",
	"Land.ParentBranch":                   "batten-only seam, filled by battencli",
	"Land.PrimeBranch":                    "batten-only seam, filled by battencli",
	"Land.PrimeDirty":                     "batten-only seam, filled by battencli",
	"Land.Pull":                           "batten-only seam, filled by battencli",
	"Land.Landing":                        "batten-only seam, filled by battencli",
	"Land.RunDeploy":                      "batten-only seam, filled by battencli",
	"BeamDescribe.Entry":                  "beam-only seam, filled by wire for a beam seed",
	"BeamDescribe.CommitSubjects":         "beam-only seam, filled by wire for a beam seed",
	"BeamDescribe.Commit":                 "beam-only seam, filled by wire for a beam seed",
	"Pawl.SetStatus":                      "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.EntryProducer":                  "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.OwnedElsewhere":                 "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.MarkBeam":                       "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.SpawnPush":                      "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.RegenerateWorkspace":            "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.StopDobby":                      "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.StartDobby":                     "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.DobbyLive":                      "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.Recheck":                        "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.WriteSnapshot":                  "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.Notify":                         "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.Down":                           "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.Now":                            "pawl-only seam, filled by wire for a pawl seed",
	"Pawl.Sleep":                          "pawl-only seam, filled by wire for a pawl seed",
	"Land.PruneBoard":                     "batten-only seam, filled by battencli",
	"Land.LandLock.Acquire":               "batten-only seam, filled by battencli",
	"Land.PauseRequested":                 "batten-only seam, filled by battencli",
	"Land.Notices.OrchStrandRecorded":     "batten-only seam, filled by battencli",
	"Land.Notices.BattenHistoryLen":       "batten-only seam, filled by battencli",
	"PrimeLock.Acquire":                   "batten-only seam, filled by battencli",
}

// TestWire_EverySeamFilled pins that wire fills every shedrecipe.Env seam, VerifyHold among them, bar the batten-only ones.
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
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_EverySeamFilled(t *testing.T) {
	t.Parallel()

	loc := hubLocation(t, "pair", ".")

	c := &loomCLI{runID: shedrun.SelfRunID}
	if err := c.wire(loc, loc.AnchorPath()); err != nil {
		t.Fatalf("wire() = %v; want nil", err)
	}

	nils := envkit.NilSeams(c.env)
	for _, path := range nils {
		if _, ok := intentionallyNil[path]; !ok {
			t.Errorf("c.env.%s is nil after wire(); fill it or list it in intentionallyNil with a reason", path)
		}
	}
	for path := range intentionallyNil {
		if !slices.Contains(nils, path) {
			t.Errorf("intentionallyNil lists c.env.%s but wire() now fills it; remove the entry", path)
		}
	}
}
