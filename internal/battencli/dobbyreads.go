// dobbyreads.go reads over the hub what the dobby of a worktree drives and may start:
// the batten runs whose parent is that worktree, their landings, and the board's tasks.
// It also builds the dobbyengine.View of that dobby, as a reader outside its process sees it.

package battencli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// DobbyParents caches the slug each pair was forked from, read once per slug.
// A run's parent never changes, and each answer reads the worktree list.
// It is safe for concurrent use.
type DobbyParents struct {
	prime *lymcwd.Location
	mu    sync.Mutex
	known map[string]string
}

// NewDobbyParents returns an empty cache over the hub of prime.
func NewDobbyParents(prime *lymcwd.Location) *DobbyParents {
	return &DobbyParents{prime: prime, known: map[string]string{}}
}

// Of returns the slug that slug was forked from, or "" for the prime.
func (c *DobbyParents) Of(slug string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if parent, ok := c.known[slug]; ok {
		return parent, nil
	}
	parent, err := hubgeom.ParentOf(c.prime, slug)
	if err != nil {
		return "", err
	}
	c.known[slug] = parent
	return parent, nil
}

// dobbyRun is one batten run whose parent worktree is the dobby's own, with its status as last written.
type dobbyRun struct {
	slug   string
	status shedengine.Status
}

// dobbyParentRuns lists the batten runs of the prime whose parent worktree is geom's.
// It reads them from the prime's local status files, whatever each run's state.
// A run whose seed or parent cannot be read is left out with one Warn, so one broken run never stops the rest.
// An invalid anchor marker in a run's pair is not skipped like a read failure:
// the call returns an error naming the pair and wrapping lymcwd.ErrInvalidAnchor.
func dobbyParentRuns(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents) ([]dobbyRun, error) {
	slugs, err := shedrun.List(prime)
	if err != nil {
		return nil, err
	}
	var runs []dobbyRun
	for _, slug := range slugs {
		seed, found, err := shedrun.ReadSeed(prime, slug)
		if err != nil {
			logger.Warn("battencli: a run's seed is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if !found || seed.Recipe != shedrun.RecipeBatten {
			continue
		}
		parent, err := parents.Of(slug)
		if errors.Is(err, lymcwd.ErrInvalidAnchor) {
			return nil, err
		}
		if err != nil {
			logger.Warn("battencli: a run's parent is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if parent != geom.Slug {
			continue
		}
		status, found, err := readDobbyRunStatus(shedrun.StatusFile(prime, slug))
		if err != nil {
			logger.Warn("battencli: a run's status is unreadable and the run is left out", "slug", slug, "error", err)
			continue
		}
		if found {
			runs = append(runs, dobbyRun{slug: slug, status: status})
		}
	}
	return runs, nil
}

// readDobbyRunStatus reads the status file at path, reporting false when it does not exist.
func readDobbyRunStatus(path string) (shedengine.Status, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return shedengine.Status{}, false, nil
	}
	if err != nil {
		return shedengine.Status{}, false, err
	}
	status, err := shedengine.ParseStatusTolerant(data)
	if err != nil {
		return shedengine.Status{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return status, true, nil
}

// unreadableReachedParent keeps one failure episode per paused run whose reached-parent judgment does not read.
// It is keyed by slug, so a persistent failure logs one Warn, not one line per dobby tick.
var unreadableReachedParent logger.EpisodeSet

// unreadableRunRecords keeps one failure episode per run whose tracked records or act inputs do not read.
// It is keyed by slug, so a persistent failure logs one Warn, not one line per dobby tick.
var unreadableRunRecords logger.EpisodeSet

// unsettledCodeTip keeps one failure episode per run whose code tip does not settle.
// It is keyed by slug, so a persistent failure logs one Warn, not one line per dobby tick.
var unsettledCodeTip logger.EpisodeSet

// DobbyRuns builds the RunView of every batten run whose parent worktree is geom's.
// The pause coverage is read from the record at pauseRecord.
// Each run also carries its FSM state and the act rule's verdict, judged with nothing set aside.
// A run whose records or act inputs do not read is left out of the call, with one Warn per episode.
// A run this machine owns, with its pair here, is first handed to startPusher for its pair.
// startPusher decides whether the pair needs a pusher.
// A failed start only warns, and the run stays in the call.
// It then settles its code tip:
// a tip missing from origin is pushed or, past its grace, given up, and a code branch origin moved past is brought level.
// A run whose code tip does not settle is left out of the call, with one Warn per episode.
// A run's place against its Land row is read from its status,
// and a landed run carries the landing commit its landing recorded, empty when none is readable.
// Whether the run takes no slot is judged from its board entry on the local board at geom.BoardDir.
// Whether that entry ended, done or gone from the board, is judged from the same board.
// Whether its pair is present is read from its task worktree's path.
// It also carries whether a cancel kept its pair and when that cancel finished.
// It carries whether the registry reads its owner as another machine and whether a Bolt hold holds it.
// It carries whether this machine's hand-over mark names it.
// A paused run also carries whether its work already reached its parent.
// That is read from the child's status in the task worktree,
// and a read that fails leaves the run unreached, with one Warn, so the run stays paused.
func DobbyRuns(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents, pauseRecord string, startPusher func(runpusher.Target) (bool, error)) ([]dobbyengine.RunView, error) {
	runs, err := dobbyParentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	record, err := pauserecord.Read(pauseRecord)
	if err != nil {
		return nil, err
	}
	entries, err := boardengine.ReadTasks(geom.BoardDir)
	if err != nil {
		return nil, fmt.Errorf("read the board: %w", err)
	}
	entryOf := make(map[string]boardengine.Task, len(entries))
	for _, entry := range entries {
		entryOf[entry.Slug] = entry
	}
	now := time.Now()
	codeTip := hubCodeTipDeps(prime)
	views := make([]dobbyengine.RunView, 0, len(runs))
	for _, run := range runs {
		slug := run.slug
		coverage := record.CoverageOf(slug, now)
		entry, onBoard := entryOf[slug]
		view := dobbyengine.RunView{
			Slug:        slug,
			State:       string(run.status.State),
			Producer:    run.status.CurrentProducer,
			HistoryLen:  len(run.status.History),
			LastRepeats: lastRepeats(run.status.History),
			Error:       run.status.Error,
			Covered:     coverage.Covered,
			Held:        coverage.Held,
			NoSlot:      runTakesNoSlot(entry),
			EntryEnded:  entryEnded(entry, onBoard),
		}
		outcome := battenrecipe.LandOutcomeOf(run.status)
		view.PastLand, view.Landed = outcome != battenrecipe.LandOutcomeAhead, outcome == battenrecipe.LandOutcomeLanded
		records, inputs, err := pairteardown.ReadRun(prime, slug)
		if err != nil {
			unreadableRunRecords.Fail(slug, "battencli: a run's records and act inputs are unreadable and the run is left out of this tick", "slug", slug, "error", err)
			continue
		}
		unreadableRunRecords.Recover(slug, "battencli: a run's records and act inputs read again", "slug", slug)
		reading, verdict := runfsm.Judge(records, inputs, runfsm.Aside{})
		view.Run, view.Verdict = reading.State, verdict
		view.PairKept, view.KeptSince = reading.PairKept, keptSince(reading, records.Batten)
		view.OwnedElsewhere, view.BoltHeld = inputs.OwnedElsewhere, inputs.Held
		pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
		if err != nil {
			return nil, fmt.Errorf("locate the pair %s: %w", slug, err)
		}
		reads := []func() error{
			func() (err error) { view.StatusModified, err = modTimeOf(shedrun.StatusFile(prime, slug)); return },
			func() (err error) {
				view.ChildStatusModified, err = modTimeOf(shedrun.StatusFile(pair, shedrun.SelfRunID))
				return
			},
			func() error {
				reading, err := readCancelMarker(prime, slug)
				view.CancelMarker = reading.Present
				return err
			},
			func() (err error) { view.DriverLockHeld, err = lockHeld(shedrun.DriverLock(prime, slug)); return },
			func() (err error) { view.PairPresent, err = markerExists(pair.WorktreePath()); return },
			func() (err error) {
				_, view.HandOverMarked, err = fabricengine.ReadHandOverMark(prime.HubPath, slug)
				return
			},
			func() error {
				if !view.Landed {
					return nil
				}
				landed, err := shedrun.ReadLandedCommit(prime, slug)
				if err != nil {
					logger.Warn("battencli: a landed run's landing commit is unreadable and stays unknown", "slug", slug, "error", err)
					return nil
				}
				view.LandingCommit, view.EmptyLanding = landed.Commit, landed.EmptyDiff
				return nil
			},
			func() error {
				if run.status.State != shedengine.StatePaused {
					unreadableReachedParent.Recover(slug, "battencli: the run left paused, so whether its work reached its parent is no longer read", "slug", slug)
					return nil
				}
				reached, err := hubChildReachedParent(prime, slug)(run.status, true)
				if err != nil {
					unreadableReachedParent.Fail(slug, "battencli: whether a paused run's work reached its parent is unreadable, so the run stays paused", "slug", slug, "error", err)
					return nil
				}
				unreadableReachedParent.Recover(slug, "battencli: whether a paused run's work reached its parent reads again", "slug", slug)
				view.ReachedParent = reached
				return nil
			},
		}
		for _, read := range reads {
			if err := read(); err != nil {
				return nil, fmt.Errorf("read the run %s: %w", slug, err)
			}
		}
		if !inputs.OwnedElsewhere && view.PairPresent {
			restartPusher(pair, slug, startPusher)
			if err := settleCodeTip(codeTip, slug, verdict, now); err != nil {
				unsettledCodeTip.Fail(slug, "battencli: a run's code tip does not settle and the run is left out of this tick", "slug", slug, "error", err)
				continue
			}
			unsettledCodeTip.Recover(slug, "battencli: a run's code tip settles again", "slug", slug)
		}
		views = append(views, view)
	}
	return views, nil
}

// lastRepeats returns the folded repeats of the newest entry of history, zero for an empty history.
func lastRepeats(history []shedengine.HistoryEntry) int {
	if len(history) == 0 {
		return 0
	}
	return history[len(history)-1].Repeats
}

// keptSince returns when the cancel that kept a run's pair finished, read from the batten record's cancel intent.
// It is zero for a run whose pair no cancel kept, and for a finish time that does not parse.
func keptSince(reading runfsm.Reading, batten shedengine.Status) time.Time {
	if !reading.PairKept || batten.Cancel == nil || batten.Cancel.Finish == nil {
		return time.Time{}
	}
	at, err := time.Parse(time.RFC3339, batten.Cancel.Finish.EndedAt)
	if err != nil {
		return time.Time{}
	}
	return at
}

// runTakesNoSlot reports whether the run behind task takes no slot under its dobby's max_runs:
// a beam project whose status is a run status and whose run does not hold the beam.
// A pawl run stands at Worktree-Create, Preflight, Record-Write, Beam-Up or Beam-Await with no hold.
// It waits on its members there, and takes a slot from Finalize on.
func runTakesNoSlot(task boardengine.Task) bool {
	return task.Kind == boardengine.KindProject && task.Beam && boardengine.IsRunStatus(task.Status) && !boardengine.RunHolds(task)
}

// entryEnded reports whether a run's board entry ended:
// entry is done, or onBoard is false because the board no longer holds it.
func entryEnded(entry boardengine.Task, onBoard bool) bool {
	return !onBoard || boardengine.IsDoneStatus(entry.Status)
}

// modTimeOf returns the modification time of path, the zero time when it does not exist.
func modTimeOf(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// lockHeld reports whether the lock at path is held, tried without blocking and released at once when it was free.
// A lock whose directory does not exist has never been taken.
func lockHeld(path string) (bool, error) {
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	held, acquired, err := lock.TryAcquireWriteLock(path)
	if err != nil {
		return false, err
	}
	if !acquired {
		return true, nil
	}
	return false, held.Release()
}

// DobbyLandings lists the slugs of the runs whose parent worktree is geom's.
// A run is listed when its batten history holds a Land that finished under one of machines.
// machines holds this machine's current and former names, none of them empty.
// A landing finished just before a rename therefore still reads as this machine's.
// An entry with no machine, and an entry naming another machine, are never counted.
func DobbyLandings(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents, machines []string) ([]string, error) {
	runs, err := dobbyParentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	var landed []string
	for _, run := range runs {
		if slices.ContainsFunc(run.status.History, func(entry shedengine.HistoryEntry) bool {
			return entry.Producer == battenrecipe.NameLand && entry.Outcome == shedengine.Done && slices.Contains(machines, entry.Machine)
		}) {
			landed = append(landed, run.slug)
		}
	}
	return landed, nil
}

// unreadableDoneRunStart tracks failure episodes of released tasks whose done run record's start judgment does not read.
// It keeps one episode per slug, so a persistent failure logs one Warn, not one line per dobby tick.
var unreadableDoneRunStart logger.EpisodeSet

// readTaskPlacement reads the parent worktree of slug and whether another machine owns it.
// The parent is the prime's own worktree name when slug has no parent.
func readTaskPlacement(prime *lymcwd.Location, parents *DobbyParents, owners boardengine.Owners, here machinename.Identity, slug string) (parent string, ownedElsewhere bool, err error) {
	parent, err = parents.Of(slug)
	if err != nil {
		return "", false, fmt.Errorf("read the parent of %s: %w", slug, err)
	}
	if parent == "" {
		parent = prime.WorktreeName
	}
	owner, err := owners.Effective(slug, parents.Of)
	if err != nil {
		return "", false, fmt.Errorf("read the owner of %s: %w", slug, err)
	}
	return parent, boardengine.OwnedElsewhere(owner, owners.Registry, here), nil
}

// DobbyTasks reads the local board under boardDir as TaskViews.
// A released task is a task with an empty status.
// Released tasks get their parent worktree and owner read.
// The other entries carry neither.
// A released task also carries the machine of the adopt hold that holds it, none for the fix task of an adopt-fix hold.
// A parent or owner that does not read fails the whole read.
// A released task whose run record under the prime is done carries a start refusal.
// The exception is a done motif run of a task whose recipe the board resolves to raddle.
// Its start clears the run.
// A released task whose record is cancelled carries the cancelled refusal, since only a cancel with --remove clears it.
// A run status that does not read holds no start.
// A dependency is pending until its entry is done, whatever its kind.
// A plain project is also pending until every landing member of it is done.
// A dependency no longer on the board was pruned as done.
func DobbyTasks(prime *lymcwd.Location, boardDir string, parents *DobbyParents) ([]dobbyengine.TaskView, error) {
	tasks, err := boardengine.ReadTasks(boardDir)
	if err != nil {
		return nil, err
	}
	owners, err := boardengine.ReadOwners(boardDir)
	if err != nil {
		return nil, err
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		return nil, err
	}
	dependencies := boardengine.NewDependencies(tasks)
	views := make([]dobbyengine.TaskView, 0, len(tasks))
	for _, task := range tasks {
		view := dobbyengine.TaskView{Slug: task.Slug, Kind: task.Kind, Status: task.Status, Front: task.Front, Project: task.Project}
		view.PendingDependencies = dependencies.Pending(task)
		released := task.Kind == boardengine.KindTask && boardengine.IsEmptyStatus(task.Status)
		if released {
			parent, ownedElsewhere, err := readTaskPlacement(prime, parents, owners, here, task.Slug)
			if err != nil {
				return nil, err
			}
			view.Parent, view.OwnedElsewhere = parent, ownedElsewhere
			if hold, held := owners.HoldOf(task.Slug); held {
				view.HeldBy = hold.Machine
			}
			status, found, err := readDobbyRunStatus(shedrun.StatusFile(prime, task.Slug))
			if err != nil {
				logger.Warn("battencli: a released task's run status is unreadable and holds no start", "slug", task.Slug, "error", err)
			}
			if err == nil && found && status.State == shedengine.StateCancelled {
				view.StartRefusal = cancelledRunRefusal(task.Slug).Error()
			}
			if err == nil && found && status.State == shedengine.StateDone {
				clears, err := startClearsMotifRun(prime, task)
				if err != nil {
					unreadableDoneRunStart.Fail(task.Slug, "battencli: whether a done run's start clears it is unreadable, so the run holds the start", "slug", task.Slug, "error", err)
				} else {
					unreadableDoneRunStart.Recover(task.Slug, "battencli: whether a done run's start clears it reads again", "slug", task.Slug)
				}
				if !clears {
					view.StartRefusal = doneSlugRefusal(prime, task.Slug).Error()
				}
			}
		}
		encoded, err := json.Marshal(task)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(encoded)
		view.Fingerprint = hex.EncodeToString(sum[:8])
		views = append(views, view)
	}
	return views, nil
}

// DobbyViewReader reads the View of the dobby of one worktree from outside its process, keeping the parents it resolved across reads.
// It is not safe for concurrent use.
type DobbyViewReader struct {
	prime       *lymcwd.Location
	geom        dobbyengine.Geometry
	dirs        configengine.Dirs
	pauseRecord string
	parents     *DobbyParents
}

// NewDobbyViewReader returns the reader of the dobby of the worktree named worktree, in the hub of prime.
func NewDobbyViewReader(prime *lymcwd.Location, worktree string) (*DobbyViewReader, error) {
	geom, err := hubgeom.DobbyGeometry(prime, worktree)
	if err != nil {
		return nil, err
	}
	location, err := lymcwd.ResolveWorktree(geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		return nil, err
	}
	return &DobbyViewReader{
		prime:       prime,
		geom:        geom,
		dirs:        fabricengine.ConfigDirs(location),
		pauseRecord: pause.RecordPath,
		parents:     NewDobbyParents(prime),
	}, nil
}

// Label is the name the dobby's lines carry: dobbyengine.PrimeLabel for the prime, else the beam's slug.
func (r *DobbyViewReader) Label() string {
	if r.geom.Slug == "" {
		return dobbyengine.PrimeLabel
	}
	return r.geom.Slug
}

// Read builds the dobby's View at now from its liveness, state record, beam.yaml, runs, board and pause record.
// It writes nothing; it takes the dobby's lock and each driver lock for an instant to learn whether it is held.
func (r *DobbyViewReader) Read(now time.Time) (dobbyengine.View, error) {
	alive, err := dobbyengine.Live(r.geom)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("read whether the dobby is live: %w", err)
	}
	record, _, err := dobbyengine.ReadState(r.geom.StatePath)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("read the dobby's state record: %w", err)
	}
	cfg, err := dobbyengine.LoadConfig(r.dirs)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("load beam.yaml: %w", err)
	}
	runs, err := DobbyRuns(r.prime, r.geom, r.parents, r.pauseRecord, runpusher.StartIfAhead)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("list the runs: %w", err)
	}
	tasks, err := DobbyTasks(r.prime, r.geom.BoardDir, r.parents)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("read the board: %w", err)
	}
	pauses, err := pauserecord.Read(r.pauseRecord)
	if err != nil {
		return dobbyengine.View{}, fmt.Errorf("read the pause record: %w", err)
	}
	return dobbyengine.BuildView(dobbyengine.ViewInput{
		Worktree: r.geom.WorktreeName,
		Project:  r.geom.Slug,
		Alive:    alive,
		Config:   cfg,
		Record:   record,
		Runs:     runs,
		Tasks:    tasks,
		Covers:   func(slug string) bool { return pauses.CoverageOf(slug, now).Covered },
		Now:      now,
	}), nil
}
