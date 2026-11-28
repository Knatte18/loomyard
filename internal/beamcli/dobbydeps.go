// dobbydeps.go fills the seams of dobbyengine.Run over a hub.
// It covers the runs and tasks it reads, the pass it spawns, and the notices it sends.
// It also covers the board tidy it sends, a beam's description rewrite, and the teardown of the leftover pairs it starts.
// It also covers the hibernate request the prime's dobby carries, and the registration of the dobby process itself.

package beamcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
)

const (
	// dobbyProgram is the program name dobby signs its notices with, scoped to its beam's slug.
	dobbyProgram = orchcli.DobbyProgram
	// runNoticeTopicPrefix leads the creel topic of a run notice, followed by the run's slug.
	runNoticeTopicPrefix = "dobby-run-notice/"
	// gateHoldTopicPrefix leads the creel topic of a gate-hold notice, followed by the run's slug.
	gateHoldTopicPrefix = "dobby-gate-hold/"
	// defaultNoticeProbe is the probe interval when the batten recipe's Run-Shed row names none.
	defaultNoticeProbe = 30 * time.Second
	// defaultNoticeQuiet is the run notices' quiet window when the batten recipe's Run-Shed row names none.
	defaultNoticeQuiet = 30 * time.Minute
	// tidyModelSpec is the model spec the board-tidy agent runs on.
	tidyModelSpec = "sonnet[medium]"
	// describeGateName labels the gate over the describer agent's draft.
	describeGateName = "Beam-Describe-Gate"
	// describeGateAttempts is how many times the gate re-prompts the describer agent over a draft with a finding.
	describeGateAttempts = 3
	// passErrorTailBytes bounds how much of a pass's output dobby keeps to read its error text from.
	passErrorTailBytes = 16 << 10
	// passErrorTextRunes bounds the error text of a pass that printed no envelope.
	passErrorTextRunes = 500
)

// runNoticeTags returns the creel tags of dobby's notice about the run slug.
// The topic lets a newer notice about the run replace an unread one, and the run lets the run's teardown drop it.
func runNoticeTags(slug string) creelengine.PostTags {
	return creelengine.PostTags{Topic: runNoticeTopicPrefix + slug, Run: slug}
}

// gateHoldTags returns the creel tags of dobby's gate-hold notice about the run slug.
// The topic is its own, so the notice and a notice about the run's child never replace each other,
// and the run lets the run's teardown drop it.
func gateHoldTags(slug string) creelengine.PostTags {
	return creelengine.PostTags{Topic: gateHoldTopicPrefix + slug, Run: slug}
}

// dobbyGateHolders returns a read of the live gate slot holders of prime's hub, each with its tier's timeout.
// The timeout is zero for a holder with no tier or with a tier the gate config does not declare.
// A gate config that does not load and a holder record that does not read each fail the call.
func dobbyGateHolders(prime *lymcwd.Location) func() ([]dobbyengine.GateHolder, error) {
	return func() ([]dobbyengine.GateHolder, error) {
		cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(prime))
		if err != nil {
			return nil, fmt.Errorf("load the gate config: %w", err)
		}
		held, err := hubgeom.GateSlots(prime).Holders()
		if err != nil {
			return nil, fmt.Errorf("read the gate slot holders: %w", err)
		}
		holders := make([]dobbyengine.GateHolder, len(held))
		for i, holder := range held {
			holders[i] = dobbyengine.GateHolder{
				Slot: holder.Slot, Worktree: holder.Worktree, Site: holder.Site, Tier: holder.Tier, Marker: holder.Marker,
				PID: holder.PID, Started: holder.Started,
				TierTimeout: time.Duration(cfg.Tiers[holder.Tier].TimeoutMin) * time.Minute,
			}
		}
		return holders, nil
	}
}

// requirePosted turns a notice that no orch strand was recorded to receive into an error, so dobby tries it again.
func requirePosted(posted bool, err error) error {
	if err != nil {
		return err
	}
	if !posted {
		return errors.New("no orch strand is recorded to receive the notice")
	}
	return nil
}

// runAutonomyStamps stamps a dobby's mails about a run with the run's beam.autonomy level, read as each mail is built.
// A run whose value is refused is stamped false, and the refusal goes to the dobby's orch through post once per episode,
// so a mistyped level does not pass unseen.
// The episode ends at the run's next read that succeeds.
// It is safe for concurrent use.
type runAutonomyStamps struct {
	worktree *lymcwd.Location
	post     func(line string) error
	mu       sync.Mutex
	// told holds, for each run with an open episode, the refusal mail that episode has sent or is sending.
	told map[string]*string
}

// stamp returns line stamped with the autonomy level of the run slug, telling the orch of a refused value first.
func (s *runAutonomyStamps) stamp(slug, line string) string {
	level, refusal, read := hubgeom.AutonomyWithRefusal(s.worktree, slug)
	s.tellRefusal(slug, refusal, read)
	return orchcli.StampAutonomy(line, level)
}

// tellRefusal posts the run slug's refusal unless the open episode already has.
// A successful read ends the episode.
// A failed read with no refusal leaves the episode as it is.
// A post that fails leaves the episode untold, so the run's next refused read posts again.
// The post runs unlocked, so a slow one holds up no other mail.
// A mail of the same run stamped while the post runs may go out ahead of it.
func (s *runAutonomyStamps) tellRefusal(slug string, refusal error, read bool) {
	s.mu.Lock()
	if read {
		delete(s.told, slug)
	}
	if refusal == nil || s.told[slug] != nil {
		s.mu.Unlock()
		return
	}
	line := fmt.Sprintf("the %s of run %s is refused, so its notices carry autonomy: %s until it is fixed: %v", autonomyName(), slug, dobbyengine.AutonomyOff, refusal)
	if s.told == nil {
		s.told = map[string]*string{}
	}
	s.told[slug] = &line
	s.mu.Unlock()

	if err := s.post(line); err != nil {
		s.mu.Lock()
		// A successful read during the post may have ended this episode, and a later refusal opened another.
		if s.told[slug] == &line {
			delete(s.told, slug)
		}
		s.mu.Unlock()
		logger.Warn("beamcli: a refused "+autonomyName()+" value was not posted to the orch", "run", slug, "error", err)
	}
}

// registerDobby records this process as the dobby of geom in the run-process registry under runProcDir.
// The owner is the beam's slug, so the pair teardown of a landed beam ends its dobby;
// the prime's dobby has the hub owner instead.
func registerDobby(runProcDir string, geom dobbyengine.Geometry) error {
	return runproc.Register(runProcDir, subjectOwner(geom), runproc.SelfRecord(runproc.RoleDobby))
}

// subjectOwner returns the owner the processes of geom's subject register under:
// the beam's slug, or runproc.HubOwner for the prime's dobby.
func subjectOwner(geom dobbyengine.Geometry) string {
	if geom.Slug == "" {
		return runproc.HubOwner
	}
	return geom.Slug
}

// boltHoldGrace is how long the running turns of a run given a bolt-hold entry get to finish before the Esc.
const boltHoldGrace = 10 * time.Second

// syncBoltHoldEntries brings the bolt-hold entries of the runs in scope in line with the hold the local Bolt records.
// Machine and standing come from the Bolt's holds, and a run holding its parent lock gets no entry.
// A hub whose Bolt records no hold and whose pause record holds no bolt-hold entry has nothing to settle,
// so the tender deps, which wire every pair of the hub, are built only when there is.
func syncBoltHoldEntries(prime *lymcwd.Location, recordPath, boardDir string, scope, held []string) error {
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return err
	}
	locks := &parentlock.Store{Dir: parentlock.Dir(fabriclayout.BoltDir(prime.HubPath))}
	req := tenderengine.BoltHoldRequest{Scope: scope, Hold: held, Grace: boltHoldGrace, HoldsParentLock: locks.HeldBy}
	if len(owners.Holds) > 0 {
		req.Standing, req.Machine = true, owners.Holds[0].Machine
	}
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return err
	}
	if !req.Standing && !slices.ContainsFunc(record.Live, func(entry pauserecord.Entry) bool { return entry.Caller == pauserecord.BoltHoldCaller }) {
		return nil
	}
	deps, _, err := tendercli.HubDeps(prime, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	_, err = tenderengine.SyncBoltHold(deps, req)
	return err
}

// pruneEpisodes keeps one failure episode of the push ledger's pruning, so a failure on every tick warns once.
var pruneEpisodes logger.EpisodeSet

// settleEpisodes keeps one failure episode per refused parent target, so a failure on every tick warns once.
var settleEpisodes logger.EpisodeSet

// settleRefusedParent settles the refused parent target t of the hub of prime in this process, pushing nothing.
func settleRefusedParent(prime *lymcwd.Location, t runpusher.Target) error {
	deps := runpusher.NewDeps(t, fabricengine.PushTiming{})
	deps.Parent.HandOver = func(run, owner string) error { return pairteardown.HandOverOwnedElsewhere(prime, run, owner) }
	if t.Pair {
		deps.Parent.Refresh = func() error { return pairteardown.RefreshOwners(prime, t.Slug) }
	}
	return runpusher.SettleRefused(context.Background(), t, deps)
}

// parentTargets returns each pair target whose ledger entry holds landings, dropped ones or owed hand-overs.
func parentTargets(hubPath string) ([]runpusher.Target, error) {
	entries, err := pushledger.List(hubPath)
	if err != nil {
		return nil, err
	}
	var targets []runpusher.Target
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[key]
		parentWork := len(entry.Landings) > 0 || len(entry.Dropped) > 0 || len(entry.HandOvers) > 0
		if key == pushledger.LandingKey || entry.Worktree == "" || !parentWork {
			continue
		}
		location, err := lymcwd.NamedWorktree(hubPath, filepath.Base(entry.Worktree))
		if err != nil {
			return nil, err
		}
		targets = append(targets, runpusher.Target{Location: location, Slug: entry.Slug, Pair: true})
	}
	return targets, nil
}

// settleIfRefused settles target when its push is refused, and reports whether it is.
// A failure is logged once per episode.
func settleIfRefused(target runpusher.Target, refused func(runpusher.Target) (bool, error), settle func(runpusher.Target) error) bool {
	key := runpusher.LockPath(target)
	stands, err := refused(target)
	if err == nil && stands {
		err = settle(target)
	}
	if err != nil {
		settleEpisodes.Fail(key, "beamcli: a refused parent target was not settled", "target", key, "error", err)
		return stands
	}
	settleEpisodes.Recover(key, "beamcli: the parent target is read again", "target", key)
	return stands
}

// restartHubPushers restarts the pushers of the hub's own targets through start.
// Those targets are the prime's records, the board's branch, the landing target and each beam pair with parent work.
// A parent target whose push refused reads, the landing target or a beam pair's, is handed to settle instead.
// It then drops the push ledger entries of gone worktrees.
// A failure is logged once per episode and stops nothing.
func restartHubPushers(prime *lymcwd.Location, start func(runpusher.Target) (bool, error), refused func(runpusher.Target) (bool, error), settle func(runpusher.Target) error) {
	targets := []struct {
		key    string
		target runpusher.Target
	}{
		{prime.WorktreePath(), runpusher.Target{Location: prime}},
		{prime.WorktreePath() + " board", runpusher.BoardTarget(prime)},
		{prime.WorktreePath() + " landing", runpusher.LandingTarget(prime)},
	}
	for _, hubTarget := range targets {
		if hubTarget.target.Kind == runpusher.KindLanding && settleIfRefused(hubTarget.target, refused, settle) {
			continue
		}
		runpusher.RestartPusher(func() (bool, error) { return start(hubTarget.target) }, hubTarget.key)
	}
	pairs, err := parentTargets(prime.HubPath)
	if err != nil {
		settleEpisodes.Fail(prime.HubPath, "beamcli: the parent targets were not listed", "hub", prime.HubPath, "error", err)
	}
	for _, target := range pairs {
		if !settleIfRefused(target, refused, settle) {
			runpusher.RestartPusher(func() (bool, error) { return start(target) }, runpusher.LockPath(target))
		}
	}
	if err := runpusher.PruneLedger(prime.HubPath, time.Now()); err != nil {
		pruneEpisodes.Fail(prime.HubPath, "beamcli: the push ledger was not pruned", "hub", prime.HubPath, "error", err)
		return
	}
	pruneEpisodes.Recover(prime.HubPath, "beamcli: the push ledger prunes again", "hub", prime.HubPath)
}

// hubDobbyDeps fills every seam of dobbyengine.Run over the hub of prime, for the dobby of geom.
// The dobby starts the pushers of the worktrees it drives that are ahead of origin.
func hubDobbyDeps(prime *lymcwd.Location, geom dobbyengine.Geometry) (dobbyengine.Deps, error) {
	return hubDobbyDepsStarting(prime, geom, runpusher.StartIfAhead)
}

// hubDobbyDepsStarting fills every seam of dobbyengine.Run over the hub of prime, for the dobby of geom.
// The dobby's ticks and run reads start the pushers of the worktrees that are ahead of origin through startPusher.
// Its log lines are appended to the file the dobby tells.
// The dobby's own working directory is the hub root, so every path comes from prime and geom.
// Only the pass spawn starts its process in the prime's anchor.
func hubDobbyDepsStarting(prime *lymcwd.Location, geom dobbyengine.Geometry, startPusher func(runpusher.Target) (bool, error)) (dobbyengine.Deps, error) {
	worktree, err := lymcwd.ResolveWorktree(geom.WorktreeRoot)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	probe, err := runShedDuration("notice_probe_s", time.Second, defaultNoticeProbe)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	quiet, err := runShedDuration("notice_quiet_min", time.Minute, defaultNoticeQuiet)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	dirs := fabricengine.ConfigDirs(worktree)
	boardDir := geom.BoardDir
	parents := battencli.NewDobbyParents(prime)
	owners := func() (boardengine.Owners, error) { return boardengine.ReadOwners(boardDir) }
	slugOwner := func(slug string) (string, error) {
		read, err := owners()
		if err != nil {
			return "", err
		}
		return read.Effective(slug, parents.Of)
	}
	settleFronts, err := fronttask.HubSettleDeps(prime, geom.Slug)
	if err != nil {
		return dobbyengine.Deps{}, err
	}
	hibernateRequestPath := ""
	if geom.Slug == "" {
		hibernateRequestPath = hibernatecli.RequestPath(prime)
	}
	catchUp := battencli.HubCatchUp(prime, func() ([]string, error) {
		return battencli.DobbyPairSlugs(prime, geom, parents)
	})
	underHold := orchcli.UnderOwnHold(prime)
	autonomy := &runAutonomyStamps{worktree: worktree, post: func(line string) error {
		return requirePosted(orchcli.NotifyFromScoped(worktree, geom.Slug, dobbyProgram, line))
	}}
	runNotices := battencli.DobbyRunNotices(prime, pause.RecordPath, quiet)
	runNotices.Post = func(_ context.Context, slug, line string) error {
		stamped := autonomy.stamp(slug, line)
		return requirePosted(orchcli.NotifyRunFrom(worktree, geom.Slug, dobbyProgram, runNoticeTags(slug), stamped))
	}
	deps := dobbyengine.Deps{
		Now:   time.Now,
		Awake: pauserecord.MonotonicAwake(),
		Sleep: sleepContext,
		Watch: func() (<-chan struct{}, func(), error) {
			return openDobbyWatch(dobbyWatchDirs(prime, pause.RecordPath, boardDir, hibernateRequestPath))
		},
		CheckBinary:   selfexec.New().CheckWith,
		ProbeInterval: probe,
		Owner: func() (string, bool, error) {
			read, err := owners()
			if err != nil {
				return "", false, err
			}
			here, err := machinename.ReadIdentity()
			if err != nil {
				return "", false, err
			}
			owner := read.Prime
			if geom.Slug != "" {
				if owner, err = read.Effective(geom.Slug, parents.Of); err != nil {
					return "", false, err
				}
			}
			return owner, boardengine.OwnedElsewhere(owner, read.Registry, here), nil
		},
		UnderAdopt:   func() bool { return underHold(geom.Slug) },
		PullBolt:     func() error { return pullBolt(geom.HubRoot, boardDir) },
		ArchiveOwner: func() error { return archiveOwnerTips(prime, geom.WorktreeName) },
		CatchUp:      catchUp,
		SyncBoltHold: func(_ context.Context, scope, held []string) error {
			return syncBoltHoldEntries(prime, pause.RecordPath, boardDir, scope, held)
		},
		EndSubject: func() error { return pairteardown.EndSubjectProcesses(prime, subjectOwner(geom), true) },
		Runs: func() ([]dobbyengine.RunView, error) {
			return battencli.DobbyRuns(prime, geom, parents, pause.RecordPath, startPusher)
		},
		ClearDoneRun: func(slug string) (bool, error) { return battencli.ClearDoneRun(prime, slug) },
		SpawnPass: func(slug string, continuePass, noReopen bool) (<-chan dobbyengine.PassExit, error) {
			return spawnPass(geom, slug, continuePass, noReopen)
		},
		TearDownLeftovers: func(pairs []dobbyengine.LeftoverPair) <-chan map[string]error {
			return tearDownLeftovers(prime, pairs, fabricengine.RunStartReconcileOptions())
		},
		Notice: func(_ context.Context, line string) error {
			return requirePosted(orchcli.NotifyFromScoped(worktree, geom.Slug, dobbyProgram, line))
		},
		ReleaseParentLock: func(_ context.Context, slug string) error {
			return battencli.ReleaseRunParentLock(prime, slug)
		},
		SlugOwner: slugOwner,
		AdoptSubject: func(slug string) (string, error) {
			read, err := owners()
			if err != nil {
				return "", err
			}
			return read.AdoptSubject(slug, parents.Of)
		},
		Tasks:      func() ([]dobbyengine.TaskView, error) { return battencli.DobbyTasks(prime, boardDir, parents) },
		LoadConfig: func() (dobbyengine.Config, error) { return dobbyengine.LoadConfig(dirs) },
		TaskLimit:  proc.ReadTaskLimit,
		StatusExists: func(slug string) (bool, error) {
			return fileExists(shedrun.StatusFile(prime, slug))
		},
		PauseCovers: func(slug string) (bool, error) {
			coverage, err := pauserecord.ReadCoverage(pause.RecordPath, slug, time.Now())
			return coverage.Covered, err
		},
		RunNotices:    runNotices,
		WaitReminders: waitReminderDeps(prime, worktree, geom.Slug, pause.RecordPath, dirs),
		LandNotice: func(slug string) (bool, error) {
			chain, err := hubgeom.TaskSettingChain(prime, slug)
			if err != nil {
				return false, err
			}
			return dobbyengine.LookupLandNotice(chain)
		},
		RunWatch: dobbyengine.RunWatchDeps{
			Level: func(slug string) (string, error) {
				chain, err := hubgeom.TaskSettingChain(prime, slug)
				if err != nil {
					return "", err
				}
				return dobbyengine.LookupRunWatch(chain)
			},
			// Untagged, so no watch line replaces another unread one and a run's teardown drops none.
			Post: func(_ context.Context, slug, line string) (bool, error) {
				stamped := autonomy.stamp(slug, line)
				return orchcli.NotifyRunFrom(worktree, geom.Slug, dobbyProgram, creelengine.PostTags{}, stamped)
			},
		},
		GateHolds: dobbyengine.GateHoldDeps{
			Holders: dobbyGateHolders(prime),
			TaskWorktree: func(slug string) (string, error) {
				pair, err := pairLocation(prime, slug)
				if err != nil {
					return "", err
				}
				return pair.WorktreePath(), nil
			},
			Tree:       proc.TreeSample,
			MarkedPIDs: proc.PIDsWithEnv,
			Post: func(_ context.Context, slug, line string) error {
				stamped := autonomy.stamp(slug, line)
				return requirePosted(orchcli.NotifyRunFrom(worktree, geom.Slug, dobbyProgram, gateHoldTags(slug), stamped))
			},
		},
		SettleFronts: func() error { return fronttask.Settle(settleFronts) },
		Landings: machineLandings(func() (string, []string, error) {
			here, err := pairteardown.ThisMachine(prime.HubPath)
			return here.Current, here.Former, err
		}, func(machines []string) ([]string, error) {
			return battencli.DobbyLandings(prime, geom, parents, machines)
		}),
		Log: func(path, line string, rotate bool) {
			if err := dobbyengine.AppendLog(path, line, rotate); err != nil {
				logger.Debug("beamcli: a dobby log line was not written", "path", path, "error", err)
			}
		},
		StencilsDir: fabriclayout.StencilsDir(prime.HubPath),
		NearestOrch: func() (string, error) {
			parent, err := orchcli.NoticeDestination(worktree)
			return parent.Name, err
		},
	}
	if geom.Slug == "" {
		deps.Orphans = func() ([]dobbyengine.OrphanView, error) { return battencli.DobbyOrphans(prime, boardDir) }
		deps.OrphanRecord = fabriclayout.OrphanFirstSightPath(prime.HubPath)
		carrier, err := hibernatecli.NewDobbyCarrier(prime)
		if err != nil {
			return dobbyengine.Deps{}, err
		}
		var relayEpisodes logger.EpisodeSet
		deps.Tick = func(context.Context, dobbyengine.TickInput) (bool, error) {
			restartHubPushers(prime, startPusher, runpusher.TargetRefused, func(t runpusher.Target) error { return settleRefusedParent(prime, t) })
			return false, errors.Join(carrier.Tick(), ensureSlackRelay(prime, &relayEpisodes))
		}
		deps.CheckBinary = holdReexec(carrier.HoldsReexec, deps.CheckBinary)
		deps.PushSummary = hubPushSummary(prime)
	}
	if geom.Slug != "" {
		// The removed worktree's config is gone by the time this stops a strand, so the runner is built over the prime.
		deps.PrimeTidyShuttle = &runnerAgentShuttle{worktree: prime}
		deps.CanLand = func() (dobbyengine.LandReading, error) { return beamLandReading(prime, geom.Slug) }
		deps.Land = func() error {
			_, err := land(hubLandDeps(prime), geom.Slug)
			return err
		}
		deps.BeamReady = func() (bool, error) { return hubLandDeps(prime).Ready(geom.Slug) }
		deps.PullRequestRecorded = func() (bool, error) {
			record, found, err := pawlengine.ReadLanding(loomengine.BeamLandingPath(worktree), loomengine.BeamLandingLockPath(worktree), geom.Slug)
			return found && record.PullRequest != nil, err
		}
		if describe, err := beamDescribeDeps(prime, worktree, geom.Slug, dirs); err != nil {
			logger.Warn("beamcli: the describer agent's model or timeout does not resolve; the beam's description rewrite is off", "beam", geom.Slug, "error", err)
		} else {
			deps.BeamDescribe = describe
		}
	}
	if choice, err := resolveModel(dirs, tidyModelSpec); err != nil {
		logger.Warn("beamcli: the board-tidy model does not resolve; the board tidy is off", "spec", tidyModelSpec, "error", err)
	} else {
		deps.TidyModel = choice
		deps.TidyShuttle = &runnerAgentShuttle{worktree: worktree}
	}
	if geom.Slug == "" {
		deps.UpkeepCall = func() (string, error) {
			cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(prime))
			if err != nil {
				return "", fmt.Errorf("load the gate config: %w", err)
			}
			// Only the go scoper finds the marked tests, so under any other the call would always be refused.
			if cfg.Scoper != "go" {
				return "", nil
			}
			return cfg.AgentGateCall(gateslot.UpkeepGateKey)
		}
	}
	return deps, nil
}

// hubPushSummary fills the reads of the push summary over the hub of prime.
func hubPushSummary(prime *lymcwd.Location) dobbyengine.PushSummaryDeps {
	var timingWarnings, unreadableWaits, unreadableBeams logger.EpisodeSet
	return dobbyengine.PushSummaryDeps{
		Ledger: func() (map[pushledger.Key]pushledger.Entry, error) { return pushledger.List(prime.HubPath) },
		Refused: func(key pushledger.Key, entry pushledger.Entry) (string, bool, error) {
			return refusedPushWayForward(prime, key, entry)
		},
		Waits: func() ([]dobbyengine.RunView, error) { return hubDriveWaits(prime, &unreadableWaits) },
		BeamCatchUpWaits: func() (map[string]dobbyengine.CatchUpWait, error) {
			return hubBeamCatchUpWaits(prime, &unreadableBeams)
		},
		Threshold: func() (time.Duration, error) {
			config, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(prime))
			if err != nil {
				return 0, err
			}
			timing, warnings := config.PushTiming()
			for _, warning := range warnings {
				timingWarnings.Fail(warning, "beamcli: "+warning)
			}
			return timing.NoticeAfter, nil
		},
	}
}

// refusedPushWayForward reports whether a refused push stands on the target of key, and returns its way forward.
// A pair whose worktree is gone reads as not refused.
func refusedPushWayForward(prime *lymcwd.Location, key pushledger.Key, entry pushledger.Entry) (string, bool, error) {
	var target runpusher.Target
	switch key {
	case pushledger.BoardKey:
		target = runpusher.BoardTarget(prime)
	case pushledger.LandingKey:
		target = runpusher.LandingTarget(prime)
	default:
		if entry.Worktree == "" {
			return "", false, nil
		}
		if present, err := fileExists(entry.Worktree); err != nil || !present {
			return "", false, err
		}
		name := filepath.Base(entry.Worktree)
		location, err := lymcwd.NamedWorktree(prime.HubPath, name)
		if err != nil {
			return "", false, err
		}
		target = runpusher.Target{Location: location, Slug: entry.Slug, Pair: name != prime.WorktreeName}
	}
	refused, err := runpusher.TargetRefused(target)
	if err != nil || !refused {
		return "", false, err
	}
	records, err := fabricengine.ListFailedPushes(prime.HubPath)
	if err != nil {
		return "", false, err
	}
	for _, record := range records {
		if !fabricengine.IsPushOwed(&fabricengine.PushFailedError{Record: record}) && recordNamesTarget(record, target) {
			return fabricengine.FailedPushWayForward(record), true, nil
		}
	}
	return "read the failed-push records with `lym tender status`", true, nil
}

// recordNamesTarget reports whether the failed-push record is of a branch the target pushes.
func recordNamesTarget(record fabriclayout.FailedPush, target runpusher.Target) bool {
	primeCode := record.Repo == fabriclayout.RepoCode && record.Prime
	switch target.Kind {
	case runpusher.KindBoard:
		return record.Repo == fabriclayout.RepoBolt
	case runpusher.KindLanding:
		return primeCode
	default:
		worktree := target.Location.WorktreePath()
		return !primeCode && (record.RepoPath == worktree || record.CodeWorktree == worktree ||
			record.RepoPath == fabricengine.RecordsWorktree(target.Location))
	}
}

// hubDriveWaits lists each run of the hub of prime whose driver waits on the network under its held lock.
// It reads each pair's own run and each batten run in the prime.
// A wait record that does not read is left out, logged once per episode through unreadable.
func hubDriveWaits(prime *lymcwd.Location, unreadable *logger.EpisodeSet) ([]dobbyengine.RunView, error) {
	slugs, err := battencli.HubRunSlugs(prime)
	if err != nil {
		return nil, err
	}
	var waits []dobbyengine.RunView
	for _, slug := range slugs {
		pair, err := pairLocation(prime, slug)
		if err != nil {
			return nil, err
		}
		for _, run := range []struct {
			location *lymcwd.Location
			id       string
		}{{pair, shedrun.SelfRunID}, {prime, slug}} {
			record := shedrun.DriveWaitRecord(run.location, run.id)
			wait, standing, driverGone, err := sheddriver.ReadDriveWait(record, shedrun.DriverLock(run.location, run.id))
			if err != nil {
				unreadable.Fail(record, "beamcli: a run's network wait is unreadable and is left out of the push summary", "slug", slug, "error", err)
				continue
			}
			unreadable.Recover(record, "beamcli: a run's network wait reads again", "slug", slug)
			if standing && !driverGone {
				waits = append(waits, dobbyengine.RunView{Slug: slug, Waiting: true, WaitProducer: wait.Producer, WaitClass: wait.Class, WaitSince: wait.Since})
			}
		}
	}
	return waits, nil
}

// hubBeamCatchUpWaits returns the catch-up wait each live beam dobby of the hub of prime records, keyed by beam slug.
// A beam whose dobby is not live is left out, since nothing retries or clears its record.
// A beam whose record does not read is left out too, logged once per episode through unreadable.
func hubBeamCatchUpWaits(prime *lymcwd.Location, unreadable *logger.EpisodeSet) (map[string]dobbyengine.CatchUpWait, error) {
	beams, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return nil, err
	}
	waits := map[string]dobbyengine.CatchUpWait{}
	for _, beam := range beams {
		wait, err := liveCatchUpWait(prime, beam.Slug)
		if err != nil {
			unreadable.Fail(beam.Slug, "beamcli: a beam dobby's catch-up wait is unreadable and is left out of the push summary", "beam", beam.Slug, "error", err)
			continue
		}
		unreadable.Recover(beam.Slug, "beamcli: a beam dobby's catch-up wait reads again", "beam", beam.Slug)
		if wait != nil {
			waits[beam.Slug] = *wait
		}
	}
	return waits, nil
}

// liveCatchUpWait returns the catch-up wait the dobby of the beam slug records while it is live, nil otherwise.
func liveCatchUpWait(prime *lymcwd.Location, slug string) (*dobbyengine.CatchUpWait, error) {
	geom, err := hubgeom.DobbyGeometry(prime, slug)
	if err != nil {
		return nil, err
	}
	live, err := dobbyengine.Live(geom)
	if err != nil || !live {
		return nil, err
	}
	record, _, err := dobbyengine.ReadState(geom.StatePath)
	return record.CatchUp, err
}

// beamDescribeDeps fills the description rewrite's seams for the beam slug over the hub of prime.
// beam is the beam's worktree.
// The describer agent runs on the landing.describe model for its landing.describe_timeout_min.
// Its shuttle run is gated on its draft passing as a change description, so a draft with a finding re-prompts it.
func beamDescribeDeps(prime, beam *lymcwd.Location, slug string, dirs configengine.Dirs) (dobbyengine.BeamDescribeDeps, error) {
	cfg, err := landingshed.LoadConfig(dirs, "landing")
	if err != nil {
		return dobbyengine.BeamDescribeDeps{}, err
	}
	choice, err := resolveModel(dirs, cfg.Describe)
	if err != nil {
		return dobbyengine.BeamDescribeDeps{}, err
	}
	return dobbyengine.BeamDescribeDeps{
		Shuttle:  &runnerAgentShuttle{worktree: beam, gate: describeGate(cfg.ForbiddenTerms())},
		Model:    choice,
		Timeout:  time.Duration(cfg.DescribeTimeoutMin) * time.Minute,
		Facts:    func() (dobbyengine.BeamDescribeFacts, error) { return beamDescribeFacts(prime, beam, slug) },
		Findings: func(path string) ([]string, error) { return descriptionFindings(path, cfg.ForbiddenTerms()) },
		Record: func(ctx context.Context, draftPath string) (bool, error) {
			_, err := rewriteDescription(ctx, hubLandDeps(prime), slug, draftPath)
			var afterRecord afterRecordError
			return errors.As(err, &afterRecord), err
		},
	}, nil
}

// beamDescribeFacts reads what the describer agent's prompt names about the beam slug, whose worktree is beam:
// its recorded description, its branch, its parent branch and the commits on its line that the parent lacks.
// The commits leave out the records-only ones, as the beam's draft does.
func beamDescribeFacts(prime, beam *lymcwd.Location, slug string) (dobbyengine.BeamDescribeFacts, error) {
	origin, found, err := fabricengine.ReadOriginFor(prime, slug)
	if err != nil {
		return dobbyengine.BeamDescribeFacts{}, err
	}
	if !found {
		return dobbyengine.BeamDescribeFacts{}, fmt.Errorf("the pair of the beam %s has no readable origin record; way forward: run `lym fabric reconcile` to restore its records worktree", slug)
	}
	pair, err := fabricengine.Open(beam)
	if err != nil {
		return dobbyengine.BeamDescribeFacts{}, err
	}
	branch, err := pair.CurrentBranch()
	if err != nil {
		return dobbyengine.BeamDescribeFacts{}, err
	}
	commits, err := beamDraftDeps(beam, slug, origin.ParentBranch).Commits()
	if err != nil {
		return dobbyengine.BeamDescribeFacts{}, err
	}
	return dobbyengine.BeamDescribeFacts{
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(beam)),
		Branch:          branch,
		ParentBranch:    origin.ParentBranch,
		Changes:         landingshed.RenderBeamChanges(commits),
	}, nil
}

// describeGate returns the gate builder of the describer agent's run, over its one output file, the draft.
// The gate refuses a line of the draft that one of forbidden matches.
func describeGate(forbidden []*regexp.Regexp) func(spec shuttleengine.Spec) shuttleengine.GateSpec {
	return func(spec shuttleengine.Spec) shuttleengine.GateSpec {
		return shuttleengine.GateSpec{{Name: describeGateName, Gate: landingshed.NewDescriptionGate(spec.OutputFiles[0], forbidden), Attempts: describeGateAttempts}}
	}
}

// tearDownLeftovers tears down each of pairs by its kind in a goroutine of its own.
// opts bounds the origin reconcile that tears down the ended runs' pairs.
// It then regenerates the hub workspace file once, so it no longer lists a removed pair.
// The channel delivers once, mapping each slug to nil when its pair was removed and to the reason otherwise.
func tearDownLeftovers(prime *lymcwd.Location, pairs []dobbyengine.LeftoverPair, opts fabricengine.ReconcileOptions) <-chan map[string]error {
	result := make(chan map[string]error, 1)
	go func() {
		results := make(map[string]error, len(pairs))
		var ended []string
		for _, pair := range pairs {
			if pair.Kind == dobbyengine.LeftoverEnded {
				ended = append(ended, pair.Slug)
				continue
			}
			results[pair.Slug] = tearDownLeftover(prime, pair)
		}
		if len(ended) > 0 {
			maps.Copy(results, reconcileEnded(prime, ended, opts))
		}
		if _, err := fabricengine.RegenerateHubWorkspace(fabricengine.NewMutations(prime.HubPath), prime); err != nil {
			logger.Warn(`beamcli: the hub workspace file was not regenerated after a leftover teardown; run "lym fabric reconcile" once the cause is fixed`, "error", err)
		}
		result <- results
	}()
	return result
}

// tearDownLeftover tears down one leftover pair that is not an ended run's.
// A handed-over run's local pair is removed in local mode, and an orphan's after its tips are archived.
// A landed or cancelled run's pair goes through the pair teardown with its task branch on origin;
// the landed run's claim is marked done, and the cancelled run's batten record goes with its pair.
// The cancel chose the removal, so the cancelled run's teardown answers dirtiness with force.
// A landed run's pair is left for a later tick, with no error, while origin lacks it or its landing is still owed.
func tearDownLeftover(prime *lymcwd.Location, pair dobbyengine.LeftoverPair) error {
	if pair.Kind == dobbyengine.LeftoverLanded {
		stays, err := landedPairStays(prime, pair.Slug)
		if err != nil || stays {
			return err
		}
	}
	switch pair.Kind {
	case dobbyengine.LeftoverHandedOver:
		_, err := pairteardown.RemoveHandedOverPair(prime, pair.Slug)
		return err
	case dobbyengine.LeftoverOrphan:
		_, err := pairteardown.RemoveOrphanPair(context.Background(), prime, pair.Slug)
		return err
	}
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return err
	}
	_, err = teardown.Run(context.Background(), pairteardown.Request{
		Slug:    pair.Slug,
		Remote:  true,
		Force:   pair.Kind == dobbyengine.LeftoverCancelled,
		Landed:  pair.Kind == dobbyengine.LeftoverLanded,
		Verdict: fabriclayout.EntryEnded,
	})
	return err
}

// landedPairStays reports whether the landed run's pair of slug must not be removed yet:
// origin lacks one of its tips, or a landing it carries or holds is still owed.
// With git or pushes off no tip is read, and a pair whose task worktree is gone has none to read.
func landedPairStays(prime *lymcwd.Location, slug string) (bool, error) {
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return false, err
	}
	options := fabricengine.EnvSyncOptions()
	_, err = os.Stat(pair.WorktreePath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, err
	case !options.SkipGit && !options.SkipPush:
		onOrigin, err := fabricengine.TipsOnOrigin(pair)
		if err != nil || !onOrigin {
			return true, err
		}
	}
	return pairteardown.LandingStands(prime.HubPath, slug, pair)
}

// reconcileEnded runs the hub-scope origin reconcile from prime, as `lym cleanup --apply` does.
// That reconcile tears down the local pair of every run origin has ended, and moves the hub's branches behind origin.
// Each entry of its mutation record is logged at Info.
// It maps each of slugs to nil when the reconcile removed its pair, and to the reason otherwise.
// opts bounds the reconcile's remote calls and its retries of an unreachable origin.
func reconcileEnded(prime *lymcwd.Location, slugs []string, opts fabricengine.ReconcileOptions) map[string]error {
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return endedTeardownResults(slugs, fabriclayout.OriginReport{}, err)
	}
	report, err := fabricengine.ReconcileOrigin(prime, fabriclayout.ReconcileScope{}, teardown.OriginSeams(pairteardown.HubSeamOptions(prime)), opts)
	for _, mutation := range report.Mutations.Entries() {
		logger.Info("beamcli: origin reconcile mutation", "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
	}
	return endedTeardownResults(slugs, report, err)
}

// endedTeardownResults maps each of slugs to nil when report shows its pair removed, and to the reason otherwise.
// reconcileErr is the error the reconcile behind report returned, which every slug then carries.
func endedTeardownResults(slugs []string, report fabriclayout.OriginReport, reconcileErr error) map[string]error {
	results := make(map[string]error, len(slugs))
	for _, slug := range slugs {
		results[slug] = endedTeardownResult(slug, report, reconcileErr)
	}
	return results
}

// endedTeardownResult reads slug's ended entry in report: nil when its pair was removed, and the reason otherwise.
func endedTeardownResult(slug string, report fabriclayout.OriginReport, reconcileErr error) error {
	switch {
	case reconcileErr != nil:
		return fmt.Errorf("the hub's origin reconcile failed: %w", reconcileErr)
	case report.Unreachable:
		return errors.New("origin was unreachable, so the reconcile judged no run ended")
	}
	for _, entry := range report.Entries {
		if entry.Slug != slug || entry.State != fabriclayout.EntryEnded {
			continue
		}
		switch {
		case entry.Removed:
			return nil
		case entry.Busy:
			return fmt.Errorf("its pair is busy; %s", entry.WayForward)
		case entry.Error != "":
			return fmt.Errorf("its pair's teardown failed: %s", entry.Error)
		}
	}
	return fmt.Errorf("origin's board does not show %s ended, so the reconcile left its pair; way forward: read lym batten status %s", slug, slug)
}

// pullBolt brings the Bolt at boardDir, in the hub at hubRoot, up to date from origin.
// A pull that skips, as a dirty or diverged Bolt does, is an error carrying the skip's way forward.
func pullBolt(hubRoot, boardDir string) error {
	skip, detail, err := fabricengine.NewBolt(boardDir).Pull(fabricengine.EnvSyncOptions(), fabricengine.NewMutations(hubRoot))
	if err != nil {
		return err
	}
	if skip != "" {
		return errors.New(detail)
	}
	return nil
}

// waitReminderDeps fills the wait reminder seams of the dobby of worktree, whose beam slug is slug, over the hub of prime.
// Only the wait rows that wait on the operator alone get reminders:
// a row such as Beam-Await also waits on the beam's members, so it gets none.
// The Slack geometry resolves at each read, so a hub it fails for loses its reminders and never its dobby.
func waitReminderDeps(prime, worktree *lymcwd.Location, slug, pauseRecordPath string, dirs configengine.Dirs) dobbyengine.WaitReminderDeps {
	return dobbyengine.WaitReminderDeps{
		Rows: []string{loomshed.NamePRReview, loomshed.NameSamplerSession},
		LatestResume: func(slug string) (time.Time, error) {
			record, err := pauserecord.Read(pauseRecordPath)
			if err != nil {
				return time.Time{}, err
			}
			entry, _ := record.LatestResumed(slug)
			return entry.ResumedAt, nil
		},
		Away: func() (bool, error) {
			geometry, err := hubgeom.SlackGeometry(prime.HubPath)
			if err != nil {
				return false, err
			}
			presence, err := slackengine.ReadPresence(geometry.PresencePath)
			return presence == slackengine.PresenceSlack, err
		},
		Interval: func() (time.Duration, error) {
			config, err := slackengine.LoadConfig(dirs)
			return time.Duration(config.RemindAfterMin) * time.Minute, err
		},
		Post: func(_ context.Context, line string) error {
			geometry, err := hubgeom.SlackGeometry(prime.HubPath)
			if err != nil {
				return err
			}
			relay, err := agentname.FormatProgram(geometry.Shortname, slackengine.SlackProgram)
			if err != nil {
				return err
			}
			client, err := creelcli.NewScopedProgramClient(worktree, slug, dobbyProgram)
			if err != nil {
				return err
			}
			_, err = client.Post(relay, line)
			return err
		},
	}
}

// holdReexec returns a binary check that runs check only while holds reports false.
// It keeps a re-exec from cutting what holds guards.
func holdReexec(holds func() bool, check func(beforeExec func(), failed func(error))) func(beforeExec func(), failed func(error)) {
	return func(beforeExec func(), failed func(error)) {
		if holds() {
			return
		}
		check(beforeExec, failed)
	}
}

// slackRelayGaveUpEpisode is the episode key of a start of the Slack relay that gave up.
const slackRelayGaveUpEpisode = "slack-relay-gave-up"

// ensureSlackRelay brings the hub's Slack relay back on a tick of the prime's dobby, while the operator is away.
// It starts nothing for a hub with no Slack setup and nothing while the presence is terminal.
// A start that gave up is logged at Warn once per episode through episodes.
// It gave up because the relay died young too many starts in a row, and it is not an error, so the tick goes on.
func ensureSlackRelay(prime *lymcwd.Location, episodes *logger.EpisodeSet) error {
	geometry, err := hubgeom.SlackGeometry(prime.HubPath)
	if err != nil {
		return err
	}
	outcome, err := slackengine.EnsureRelayWhileAway(geometry, prime.WorktreeName)
	if err != nil {
		return err
	}
	if outcome != slackengine.OutcomeGaveUp {
		episodes.Recover(slackRelayGaveUpEpisode, "beamcli: the Slack relay start no longer gives up")
		return nil
	}
	episodes.Fail(slackRelayGaveUpEpisode, "beamcli: the Slack relay start gave up after the relay died young five starts in a row; way forward: read the relay log, then run `lym slack start`", "log", geometry.LogPath)
	return nil
}

// beamLandReading reads whether the beam slug can land.
// A beam can land once it is flagged ready and nothing works under it.
// For a beam that can land, it reports the owner of its parent when another machine holds it.
// That report also carries the subject `lym adopt` takes to move the beam.
func beamLandReading(prime *lymcwd.Location, slug string) (dobbyengine.LandReading, error) {
	ready, err := hubLandDeps(prime).Ready(slug)
	if err != nil || !ready {
		return dobbyengine.LandReading{}, err
	}
	blockers, err := battencli.ReadBeamLandBlockers(prime, slug)
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	if !blockers.Empty() {
		return dobbyengine.LandReading{}, nil
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	owners, err := boardengine.ReadOwners(fabriclayout.BoardDir(prime.HubPath))
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	parentOf := func(entry string) (string, error) { return hubgeom.ParentOf(prime, entry) }
	parent, err := parentOf(slug)
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	owner, subject := owners.Prime, boardengine.RootSubject
	if parent != "" {
		if owner, err = owners.Effective(parent, parentOf); err != nil {
			return dobbyengine.LandReading{}, err
		}
		if subject, err = owners.AdoptSubject(parent, parentOf); err != nil {
			return dobbyengine.LandReading{}, err
		}
	}
	reading := dobbyengine.LandReading{Passes: true}
	if boardengine.OwnedElsewhere(owner, owners.Registry, here) {
		reading.ParentOwner, reading.ParentSubject = owner, subject
	}
	return reading, nil
}

// runShedDuration reads key from the batten recipe's Run-Shed row as a count of unit.
// It returns fallback when the row names no count or a count that is not positive.
// The row's notice_probe_s is the cadence the run's own wait probes at.
// Its notice_quiet_min is the quiet window of the run notices.
func runShedDuration(key string, unit, fallback time.Duration) (time.Duration, error) {
	recipe, err := shedbuild.Parse(recipes.BattenRecipe)
	if err != nil {
		return 0, err
	}
	for _, row := range recipe.Producers {
		if row.Name != battenrecipe.NameRunShed {
			continue
		}
		return durationFromCount(row.Config[key], unit, fallback), nil
	}
	return fallback, nil
}

// durationFromCount reads a recipe value as a count of unit.
// It returns fallback when the value is absent, not a number or not positive.
func durationFromCount(value any, unit, fallback time.Duration) time.Duration {
	switch count := value.(type) {
	case int:
		if count > 0 {
			return time.Duration(count) * unit
		}
	case int64:
		if count > 0 {
			return time.Duration(count) * unit
		}
	case float64:
		if count > 0 {
			return time.Duration(count * float64(unit))
		}
	}
	return fallback
}

// sleepContext waits d, or returns early when ctx ends.
func sleepContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// fileExists reports whether a file or directory exists at path.
func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// pairLocation returns slug's task pair as a location at the pair's own anchor, with no git call.
func pairLocation(prime *lymcwd.Location, slug string) (*lymcwd.Location, error) {
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return nil, fmt.Errorf("locate the pair %s: %w", slug, err)
	}
	return pair, nil
}

// spawnPass starts `<lym> batten run <slug>` for geom as its own process, in the prime's anchor.
// It delivers the process's exit once, which reaps it.
// The pass registers itself in the run-process registry as the run's batten, so dobby registers nothing.
func spawnPass(geom dobbyengine.Geometry, slug string, continuePass, noReopen bool) (<-chan dobbyengine.PassExit, error) {
	if testing.Testing() {
		return nil, dobbyengine.ErrSpawnUnderTest
	}
	args := []string{"batten", "run", slug}
	if continuePass {
		args = append(args, "--continue-pass")
	}
	if noReopen {
		args = append(args, "--no-reopen")
	}
	output := &tailBuffer{limit: passErrorTailBytes}
	cmd := exec.Command(geom.LymExecutable, args...)
	cmd.Dir = geom.PrimeAnchor
	cmd.Stdout = output
	cmd.Stderr = output
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		logger.Warn("beamcli: spawn batten pass failed", "exe", geom.LymExecutable, "args", args, "dir", geom.PrimeAnchor, "error", err)
		return nil, err
	}
	logger.Info("beamcli: spawned batten pass", "exe", geom.LymExecutable, "args", args, "dir", geom.PrimeAnchor, "pid", cmd.Process.Pid)
	exit := make(chan dobbyengine.PassExit, 1)
	go func() {
		waitErr := cmd.Wait()
		exit <- dobbyengine.PassExit{Code: cmd.ProcessState.ExitCode(), Error: passErrorText(output.Bytes(), waitErr)}
	}()
	return exit, nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if extra := len(b.data) - b.limit; extra > 0 {
		b.data = b.data[extra:]
	}
	return len(p), nil
}

// Bytes returns a copy of what the buffer holds.
func (b *tailBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.data)
}

// passErrorText reads the error text of a pass from what it printed:
// the error of the last JSON envelope line, else the tail of the output, else the wait error.
func passErrorText(printed []byte, waitErr error) string {
	lines := strings.Split(strings.TrimSpace(string(printed)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var envelope struct {
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Error != "" {
			return envelope.Error
		}
	}
	if text := strings.TrimSpace(string(printed)); text != "" {
		runes := []rune(text)
		if len(runes) > passErrorTextRunes {
			runes = runes[len(runes)-passErrorTextRunes:]
		}
		return string(runes)
	}
	if waitErr != nil {
		return waitErr.Error()
	}
	return ""
}

// resolveModel resolves the model spec through the hub's model registry.
func resolveModel(dirs configengine.Dirs, spec string) (shuttleengine.ModelChoice, error) {
	parsed, err := modelspec.Parse(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	resolved, err := registry.Resolve(parsed)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// runnerAgentShuttle adapts the shuttle runner over a worktree to dobbyengine.AgentShuttle, for the agents dobby spawns.
// The runner is built on the first start, so a config that does not load fails the agent's step and never the dobby.
type runnerAgentShuttle struct {
	worktree *lymcwd.Location
	// gate builds the gate a started run carries from its spec; nil starts the run ungated.
	gate   func(spec shuttleengine.Spec) shuttleengine.GateSpec
	once   sync.Once
	runner *shuttleengine.Runner
	err    error
}

// Start launches spec in a fresh window and returns its strand's GUID once the agent is running.
// The run is waited on in the background, so shuttle enforces the spec's timeout and gate and settles the run.
func (s *runnerAgentShuttle) Start(spec shuttleengine.Spec) (string, error) {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return "", s.err
	}
	var gate shuttleengine.GateSpec
	if s.gate != nil {
		gate = s.gate(spec)
	}
	run, err := s.runner.StartGated(spec, gate)
	if err != nil {
		return "", err
	}
	go func() {
		result, err := run.Wait()
		logger.Debug("beamcli: a dobby agent's run ended", "role", spec.Role, "outcome", result.Outcome, "error", err)
	}()
	return run.StrandGUID(), nil
}

// EngineName names the engine choice launches on under the runner's engine set.
// A runner that cannot be built returns the build's error, so the step reports it instead of rendering a prompt.
func (s *runnerAgentShuttle) EngineName(choice shuttleengine.ModelChoice) (string, error) {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return "", s.err
	}
	return s.runner.EngineName(choice), nil
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s *runnerAgentShuttle) StopStrand(guid string) error {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return s.err
	}
	return s.runner.StopStrand(guid)
}

// machineLandings returns a Landings seam that reads this machine's names on each call.
// It passes the current name and then the former names to read.
// A name that cannot be read fails the call with the read's own way forward,
// so no landing is told apart from another machine's by a missing name.
func machineLandings(names func() (string, []string, error), read func(machines []string) ([]string, error)) func() ([]string, error) {
	return func() ([]string, error) {
		current, former, err := names()
		if err != nil {
			return nil, fmt.Errorf("this machine's name is unknown, so its landings cannot be told apart: %w", err)
		}
		return read(append([]string{current}, former...))
	}
}
