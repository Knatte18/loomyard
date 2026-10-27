// dobbyreads.go reads over the hub what the dobby of a worktree drives and may start: the batten runs whose parent is that worktree, their landings, the board's tasks, and the View of that dobby as a reader outside its process sees it.

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
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// DobbyParents answers hubgeom.ParentOf once per slug, since a run's parent never changes and the answer reads the worktree list.
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

// Of returns the slug of the pair slug was forked from, "" for the prime.
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

// dobbyParentRuns lists the batten runs of the prime whose parent worktree is geom's, read from the prime's local status files whatever each run's state.
// A run whose seed or parent cannot be read is left out with one Warn, so one broken run never stops the rest.
// A run whose pair holds an invalid anchor marker is no such read failure: it returns an error naming the pair and wrapping lymcwd.ErrInvalidAnchor.
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

// DobbyRuns builds the RunView of every batten run whose parent worktree is geom's, with the pause coverage read from the record at pauseRecord.
// A run's slot reading comes from its board entry on the local board at geom.BoardDir, which runTakesNoSlot judges.
func DobbyRuns(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents, pauseRecord string) ([]dobbyengine.RunView, error) {
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
	views := make([]dobbyengine.RunView, 0, len(runs))
	for _, run := range runs {
		slug := run.slug
		view := dobbyengine.RunView{
			Slug:     slug,
			State:    string(run.status.State),
			Producer: run.status.CurrentProducer,
			Covered:  record.CoverageOf(slug, now).Covered,
			NoSlot:   runTakesNoSlot(entryOf[slug]),
		}
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
			func() (err error) { view.CancelMarker, err = markerExists(shedrun.CancelMarker(prime, slug)); return },
			func() (err error) { view.OriginOwed, err = markerExists(shedrun.OriginOwedMarker(prime, slug)); return },
			func() (err error) { view.DriverLockHeld, err = lockHeld(shedrun.DriverLock(prime, slug)); return },
		}
		for _, read := range reads {
			if err := read(); err != nil {
				return nil, fmt.Errorf("read the run %s: %w", slug, err)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// runTakesNoSlot reports whether the run behind task takes no slot under its dobby's max_runs: a beam project whose status is a run status and whose run does not hold the beam.
// A pawl run stands at Worktree-Create, Beam-Up and Beam-Await with no hold, waiting on its members, and takes a slot from Parent-Align on.
func runTakesNoSlot(task boardengine.Task) bool {
	return task.Kind == boardengine.KindProject && task.Beam && boardengine.IsRunStatus(task.Status) && !boardengine.RunHolds(task)
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

// DobbyLandings lists the slugs of the runs of geom's parent worktree whose batten history holds a Land that finished on machine.
// An entry with no machine, written before the history stamped one, and an entry naming another machine are never counted.
func DobbyLandings(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents, machine string) ([]string, error) {
	runs, err := dobbyParentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	var landed []string
	for _, run := range runs {
		if slices.ContainsFunc(run.status.History, func(entry shedengine.HistoryEntry) bool {
			return entry.Producer == battenrecipe.NameLand && entry.Outcome == shedengine.Done && entry.Machine == machine
		}) {
			landed = append(landed, run.slug)
		}
	}
	return landed, nil
}

// DobbyTasks reads the local board under boardDir as TaskViews.
// Only a released task, one with an empty status, gets its parent worktree and owner read; the others carry neither.
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
	here, err := machinename.Name()
	if err != nil {
		return nil, err
	}
	statusOf := make(map[string]*string, len(tasks))
	for _, task := range tasks {
		statusOf[task.Slug] = task.Status
	}
	views := make([]dobbyengine.TaskView, 0, len(tasks))
	for _, task := range tasks {
		view := dobbyengine.TaskView{Slug: task.Slug, Kind: task.Kind, Status: task.Status}
		for _, dep := range task.DependsOn {
			if status, present := statusOf[dep]; present && (status == nil || *status != "done") {
				view.PendingDependencies = append(view.PendingDependencies, dep)
			}
		}
		if task.Kind == boardengine.KindTask && boardengine.IsEmptyStatus(task.Status) {
			parent, err := parents.Of(task.Slug)
			if err != nil {
				return nil, fmt.Errorf("read the parent of %s: %w", task.Slug, err)
			}
			if parent == "" {
				parent = prime.WorktreeName
			}
			view.Parent = parent
			owner, err := owners.Effective(task.Slug, parents.Of)
			if err != nil {
				return nil, fmt.Errorf("read the owner of %s: %w", task.Slug, err)
			}
			view.OwnedElsewhere = boardengine.OwnedElsewhere(owner, here)
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

// Read reads the dobby's liveness, its state record, beam.yaml, its runs, the board and the pause record, and builds its View at now.
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
	runs, err := DobbyRuns(r.prime, r.geom, r.parents, r.pauseRecord)
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
		Alive:    alive,
		Config:   cfg,
		Record:   record,
		Runs:     runs,
		Tasks:    tasks,
		Covers:   func(slug string) bool { return pauses.CoverageOf(slug, now).Covered },
		Now:      now,
	}), nil
}
