// dobbydeps.go fills the seams of dobbyengine.Run over a hub.
// It covers the runs and tasks it reads, the pass it spawns, and the notices it sends.
// It also covers the owner pause and board tidy it sends, and the teardown of an ended run's pair it starts.
// It also covers the hibernate request and the coverage-map upkeep pass the prime's dobby carries,
// and the registration of the dobby process itself.

package beamcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/tendercli"
	"github.com/Knatte18/loomyard/internal/tenderengine"
	"github.com/Knatte18/loomyard/internal/testcover"
)

const (
	// dobbyProgram is the program name dobby signs its notices with, scoped to its beam's slug.
	dobbyProgram = "DOBBY"
	// runNoticeTopicPrefix leads the creel topic of a run notice, followed by the run's slug.
	runNoticeTopicPrefix = "dobby-run-notice/"
	// defaultNoticeProbe is the probe interval when the batten recipe's Run-Shed row names none.
	defaultNoticeProbe = 30 * time.Second
	// defaultNoticeQuiet is the run notices' quiet window when the batten recipe's Run-Shed row names none.
	defaultNoticeQuiet = 30 * time.Minute
	// tidyModelSpec is the model spec the board-tidy agent runs on.
	tidyModelSpec = "sonnet[medium]"
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

// registerDobby records this process as the dobby of geom in the run-process registry under runProcDir.
// The owner is the beam's slug, so the pair teardown of a landed beam ends its dobby;
// the prime's dobby has the hub owner instead.
func registerDobby(runProcDir string, geom dobbyengine.Geometry) error {
	owner := geom.Slug
	if owner == "" {
		owner = runproc.HubOwner
	}
	return runproc.Register(runProcDir, owner, runproc.SelfRecord(runproc.RoleDobby))
}

// hubDobbyDeps fills every seam of dobbyengine.Run over the hub of prime, for the dobby of geom.
// Its log lines are appended to the file the dobby tells.
// The dobby's own working directory is the hub root, so every path comes from prime and geom.
// Only the pass spawn starts its process in the prime's anchor.
func hubDobbyDeps(prime *lymcwd.Location, geom dobbyengine.Geometry) (dobbyengine.Deps, error) {
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
	underAdopt := orchcli.UnderAdoptWatch(prime)
	runNotices := battencli.DobbyRunNotices(prime, pause.RecordPath, quiet)
	runNotices.Post = func(_ context.Context, slug, line string) error {
		return requirePosted(orchcli.NotifyRunFrom(worktree, geom.Slug, dobbyProgram, runNoticeTags(slug), line))
	}
	deps := dobbyengine.Deps{
		Now:   time.Now,
		Awake: pauserecord.MonotonicAwake(),
		Sleep: sleepContext,
		Watch: func() (<-chan struct{}, func(), error) {
			return openDobbyWatch(dobbyWatchDirs(prime, pause.RecordPath, boardDir, hibernateRequestPath))
		},
		Machine:       machinename.Name,
		CheckBinary:   selfexec.New().CheckWith,
		ProbeInterval: probe,
		Owner: func() (string, error) {
			read, err := owners()
			if err != nil {
				return "", err
			}
			if geom.Slug == "" {
				return read.Prime, nil
			}
			return read.Effective(geom.Slug, parents.Of)
		},
		UnderAdopt:   func() bool { return underAdopt.Covers(geom.Slug) },
		PullBolt:     func() error { return pullBolt(geom.HubRoot, boardDir) },
		ArchiveOwner: func() error { return archiveOwnerTips(prime, geom.WorktreeName) },
		Runs: func() ([]dobbyengine.RunView, error) {
			return battencli.DobbyRuns(prime, geom, parents, pause.RecordPath)
		},
		SpawnPass: func(slug string, continuePass, noReopen bool) (<-chan dobbyengine.PassExit, error) {
			return spawnPass(geom, slug, continuePass, noReopen)
		},
		TearDownEnded: func(slugs []string) <-chan map[string]error { return tearDownEnded(prime, slugs) },
		Notice: func(_ context.Context, line string) error {
			return requirePosted(orchcli.NotifyFromScoped(worktree, geom.Slug, dobbyProgram, line))
		},
		PauseOwned: func(_ context.Context, slug string) error { return tendercli.PauseOwnedElsewhere(prime, slug) },
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
		LiftOwnerPause: func(_ context.Context, slug string) error {
			return liftOwnerPause(prime, pause.RecordPath, slug)
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
		ClearNotifyOnLand: func(slug string) error {
			board, err := boardengine.OpenHub(prime.HubPath)
			if err != nil {
				return err
			}
			return board.ClearNotifyOnLand(slug)
		},
		SettleFronts: func() error { return fronttask.Settle(settleFronts) },
		Landings: machineLandings(machinename.Names, func(machines []string) ([]string, error) {
			return battencli.DobbyLandings(prime, geom, parents, machines)
		}),
		Log: func(path, line string, rotate bool) {
			if err := dobbyengine.AppendLog(path, line, rotate); err != nil {
				logger.Debug("beamcli: a dobby log line was not written", "path", path, "error", err)
			}
		},
		StencilsDir: fabricengine.StencilsDir(prime.HubPath),
		NearestOrch: func() (string, error) {
			parent, err := hubgeom.NearestLiveOrch(worktree, orchcli.OrchLive)
			return parent.Name, err
		},
	}
	if geom.Slug == "" {
		carrier, err := hibernatecli.NewDobbyCarrier(prime)
		if err != nil {
			return dobbyengine.Deps{}, err
		}
		var relayEpisodes logger.EpisodeSet
		deps.Tick = func(context.Context, dobbyengine.TickInput) (bool, error) {
			return false, errors.Join(carrier.Tick(), ensureSlackRelay(prime, &relayEpisodes))
		}
		deps.CheckBinary = holdReexec(carrier.HoldsReexec, deps.CheckBinary)
		runProcDir := fabricengine.RunProcDir(prime.HubPath)
		mapsDir := testcover.Dir(fabricengine.BoltDir(prime.HubPath))
		deps.StartTestMap = func() (<-chan dobbyengine.PassExit, error) {
			return startTestMap(selfexec.LymBinary, geom.PrimeAnchor)
		}
		deps.TestMapRunning = func() (bool, error) { return testMapRunning(runProcDir) }
		deps.TestMapExitRecord = func() (dobbyengine.TestMapExit, bool, error) {
			record, found, err := testcover.ReadExitRecord(mapsDir)
			return dobbyengine.TestMapExit{PlannedHead: record.PlannedHead, Queued: record.Queued}, found, err
		}
		deps.PrimeHead = func() (string, error) {
			fabric, err := fabricengine.Open(prime)
			if err != nil {
				return "", err
			}
			return fabric.HeadSHA()
		}
		deps.GateSlots = func() (int, error) {
			cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(prime))
			return cfg.Slots, err
		}
	}
	if geom.Slug != "" {
		// The removed worktree's config is gone by the time this stops a strand, so the runner is built over the prime.
		deps.PrimeTidyShuttle = &runnerTidyShuttle{worktree: prime}
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
		deps.SetNotifyOnLand = func(slug string) error {
			board, err := boardengine.OpenHub(prime.HubPath)
			if err != nil {
				return err
			}
			return board.SetNotifyOnLand(slug)
		}
	}
	if choice, err := resolveTidyModel(dirs); err != nil {
		logger.Warn("beamcli: the board-tidy model does not resolve; the board tidy is off", "spec", tidyModelSpec, "error", err)
	} else {
		deps.TidyModel = choice
		deps.TidyShuttle = &runnerTidyShuttle{worktree: worktree}
	}
	return deps, nil
}

// tearDownEnded runs the hub-scope origin reconcile from prime in a goroutine of its own, as `lym cleanup --apply` does.
// That reconcile tears down the local pair of every run origin has ended, and moves the hub's branches behind origin.
// Each entry of its mutation record is logged at Info.
// The channel delivers once.
// It maps each of slugs to nil when the reconcile removed its pair, and to the reason otherwise.
func tearDownEnded(prime *lymcwd.Location, slugs []string) <-chan map[string]error {
	result := make(chan map[string]error, 1)
	go func() {
		teardown, err := pairteardown.New(prime)
		if err != nil {
			result <- endedTeardownResults(slugs, fabricengine.OriginReport{}, err)
			return
		}
		report, err := fabricengine.ReconcileOrigin(prime, fabricengine.ReconcileScope{}, teardown.OriginSeams(pairteardown.HubSeamOptions(prime)), fabricengine.ReconcileOptions{})
		for _, mutation := range report.Mutations.Entries() {
			logger.Info("beamcli: origin reconcile mutation", "kind", string(mutation.Kind), "target", mutation.Target, "detail", mutation.Detail)
		}
		result <- endedTeardownResults(slugs, report, err)
	}()
	return result
}

// endedTeardownResults maps each of slugs to nil when report shows its pair removed, and to the reason otherwise.
// reconcileErr is the error the reconcile behind report returned, which every slug then carries.
func endedTeardownResults(slugs []string, report fabricengine.OriginReport, reconcileErr error) map[string]error {
	results := make(map[string]error, len(slugs))
	for _, slug := range slugs {
		results[slug] = endedTeardownResult(slug, report, reconcileErr)
	}
	return results
}

// endedTeardownResult reads slug's ended entry in report: nil when its pair was removed, and the reason otherwise.
func endedTeardownResult(slug string, report fabricengine.OriginReport, reconcileErr error) error {
	switch {
	case reconcileErr != nil:
		return fmt.Errorf("the hub's origin reconcile failed: %w", reconcileErr)
	case report.Unreachable:
		return errors.New("origin was unreachable, so the reconcile judged no run ended")
	}
	for _, entry := range report.Entries {
		if entry.Slug != slug || entry.State != fabricengine.EntryEnded {
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
	here, err := machinename.Name()
	if err != nil {
		return dobbyengine.LandReading{}, err
	}
	owners, err := boardengine.ReadOwners(fabricengine.BoardDir(prime.HubPath))
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
	if boardengine.OwnedElsewhere(owner, here) {
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

// liftOwnerPause lifts the live pause entry of slug's own scope that carries the owner caller.
func liftOwnerPause(prime *lymcwd.Location, recordPath, slug string) error {
	record, err := pauserecord.Read(recordPath)
	if err != nil {
		return err
	}
	var refs []tenderengine.EntryRef
	for _, entry := range record.Live {
		if entry.Scope == slug && entry.Caller == pauserecord.OwnerCaller {
			refs = append(refs, tenderengine.EntryRef{Scope: entry.Scope, Caller: entry.Caller, PausedAt: entry.PausedAt})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	deps, _, err := tendercli.HubDeps(prime, os.Getenv(agentname.StrandNameEnv))
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := tenderengine.Lift(deps, ref); err != nil {
			return err
		}
	}
	return nil
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

// testMapRunning reports whether a live process registered under the hub owner in runProcDir is the upkeep pass.
func testMapRunning(runProcDir string) (bool, error) {
	records, err := runproc.List(runProcDir, runproc.HubOwner)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(records, func(record runproc.Record) bool { return record.Role == runproc.RoleTestMap }), nil
}

// startTestMap starts the coverage-map upkeep pass, `<lym> gate testmap --upkeep`, in primeAnchor.
// It runs the binary lookupLymBinary returns.
// The process gets its own process group and registers itself for the hub, so the dobby registers nothing.
// Its output goes to a file rather than a pipe the dobby reads, so the pass outlives a dead dobby without a broken pipe.
// The channel delivers its exit once, which reaps it, with the error text the output file carries.
// A binary that does not resolve fails and starts nothing; in a test binary the lym binary does not resolve.
func startTestMap(lookupLymBinary func() (string, error), primeAnchor string) (<-chan dobbyengine.PassExit, error) {
	executable, err := lookupLymBinary()
	if err != nil {
		return nil, err
	}
	args := []string{"gate", "testmap", "--upkeep"}
	outputFile, err := lymtemp.CreateTemp("upkeep-pass-*.log")
	if err != nil {
		return nil, fmt.Errorf("create the upkeep pass's output file: %w", err)
	}
	outputPath := outputFile.Name()
	cmd := exec.Command(executable, args...)
	cmd.Dir = primeAnchor
	cmd.Stdout = outputFile
	cmd.Stderr = outputFile
	proc.Detach(cmd)
	startErr := cmd.Start()
	// The child holds its own descriptor of the file, so this one is not needed once the child started.
	_ = outputFile.Close()
	if startErr != nil {
		_ = os.Remove(outputPath)
		logger.Warn("beamcli: spawn the upkeep pass failed", "exe", executable, "args", args, "dir", primeAnchor, "error", startErr)
		return nil, startErr
	}
	pid := cmd.Process.Pid
	logger.Info("beamcli: spawned the upkeep pass", "exe", executable, "args", args, "dir", primeAnchor, "pid", pid)
	exit := make(chan dobbyengine.PassExit, 1)
	go func() {
		waitErr := cmd.Wait()
		code := cmd.ProcessState.ExitCode()
		logger.Info("beamcli: the upkeep pass exited", "pid", pid, "code", code, "error", waitErr)
		exit <- dobbyengine.PassExit{Code: code, Error: passErrorText(readFileTail(outputPath, passErrorTailBytes), waitErr)}
		_ = os.Remove(outputPath)
	}()
	return exit, nil
}

// readFileTail returns the last limit bytes of the file at path, or nothing when the file cannot be read.
func readFileTail(path string, limit int) []byte {
	file, err := os.Open(path)
	if err != nil {
		logger.Debug("beamcli: a pass's output file is not read", "path", path, "error", err)
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		logger.Debug("beamcli: a pass's output file is not read", "path", path, "error", err)
		return nil
	}
	if _, err := file.Seek(max(info.Size()-int64(limit), 0), io.SeekStart); err != nil {
		logger.Debug("beamcli: a pass's output file is not read", "path", path, "error", err)
		return nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		logger.Debug("beamcli: a pass's output file is not read", "path", path, "error", err)
	}
	return data
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

// resolveTidyModel resolves the board-tidy agent's model spec through the hub's model registry.
func resolveTidyModel(dirs configengine.Dirs) (shuttleengine.ModelChoice, error) {
	spec, err := modelspec.Parse(tidyModelSpec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	registry, err := modelspec.LoadRegistry(dirs)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	resolved, err := registry.Resolve(spec)
	if err != nil {
		return shuttleengine.ModelChoice{}, err
	}
	return shuttleengine.NewModelChoice(resolved), nil
}

// runnerTidyShuttle adapts the shuttle runner over the dobby's worktree to dobbyengine.TidyShuttle.
// The runner is built on the first start, so a config that does not load fails the tidy and never the dobby.
type runnerTidyShuttle struct {
	worktree *lymcwd.Location
	once     sync.Once
	runner   *shuttleengine.Runner
	err      error
}

// Start launches spec in a fresh window and returns its strand's GUID once the agent is running.
// The run is waited on in the background, so shuttle enforces the spec's timeout and settles the run.
func (s *runnerTidyShuttle) Start(spec shuttleengine.Spec) (string, error) {
	s.once.Do(func() { s.runner, s.err = battencli.ShuttleRunner(s.worktree) })
	if s.err != nil {
		return "", s.err
	}
	run, err := s.runner.StartGated(spec, nil)
	if err != nil {
		return "", err
	}
	go func() {
		result, err := run.Wait()
		logger.Debug("beamcli: the board-tidy agent's run ended", "outcome", result.Outcome, "error", err)
	}()
	return run.StrandGUID(), nil
}

// StopStrand ends the strand guid names through shuttle's stop verb.
func (s *runnerTidyShuttle) StopStrand(guid string) error {
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
