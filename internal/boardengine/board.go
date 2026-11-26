// board.go implements Board, the facade that callers use over a board directory.

package boardengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	flock "github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// Board is the high-level facade over a board directory.
// Every mutating method acquires an exclusive file lock, mutates the store, and renders output files;
// the remote backup (commit + push) is detached.
type Board struct {
	boardPath string
	out       Outputs
	vocab     Vocabulary
	skipGit   bool
	skipPush  bool
	now       func() time.Time
	parentOf  func(slug string) (string, error)
	// pairExists is copied onto every store the board builds.
	pairExists func(slug string) bool
	// validateSlug is applied to the task and project slugs a write makes.
	validateSlug func(slug string) error
	// strandName is the writing session's agent name, which the write's task-creation check judges.
	strandName string
	// expected is the entry revision the write section checks, nil for a board that expects none; Expecting sets it.
	expected *expectedRevision
}

// New returns a Board that runs with cfg, and uses the wall clock when cfg.Clock is nil.
func New(cfg Config) *Board {
	now := cfg.Clock
	if now == nil {
		now = time.Now
	}
	return &Board{
		boardPath:    cfg.Path,
		out:          cfg.Outputs(),
		vocab:        cfg.Vocabulary(),
		skipGit:      cfg.SkipGit,
		skipPush:     cfg.SkipPush,
		now:          now,
		parentOf:     cfg.ParentOf,
		pairExists:   cfg.PairExists,
		validateSlug: cfg.ValidateSlug,
		strandName:   cfg.StrandName,
	}
}

// RecipeOf returns the recipe task runs: its own recipe field, or the board's default_recipe when the field is empty.
func (b *Board) RecipeOf(task Task) string {
	return b.out.recipeOf(task)
}

// noWrite is returned as a mutation's result to end the write without a save, render or sync;
// result is what the caller receives.
type noWrite struct{ result any }

// promoted is returned as a mutation's result when it turns a note into a task;
// the write unwraps it, and the promotion keeps the note's dates.
type promoted struct{ task Task }

// boardCriticalSection runs the locked write of boardCriticalSectionRollingBack with no rollback.
func (b *Board) boardCriticalSection(fn func(store *Store) (any, error), afterSave func() error) (any, error) {
	return b.boardCriticalSectionRollingBack(fn, afterSave, nil)
}

// boardCriticalSectionRollingBack runs the locked write scaffolding shared by every mutating Board method.
// It takes the write lock and loads the store.
// A board made by Expecting then checks its entry's revision and refuses a stale one before anything is written.
// It then calls fn to mutate the store, and it gives a new task the draft status except a promoted raddle task.
// It stamps the entry dates and assigns the handles the mutation left missing or of another letter;
// it refuses a net change to a run-held entry's scope and a run agent's new task.
// It saves board.json and runs afterSave when non-nil.
// It then moves or removes the decision record files of the entries the write renamed or removed;
// it renders the outputs and spawns a detached sync.
// A fn returning a RenameResult has its dates and run lock judged against the loaded entries with that rename applied.
// afterSave runs under the lock after the save and before the render.
// When afterSave or the render fails, a non-nil rollback runs before the lock is released,
// and its own failure is joined to the returned error.
// A *DecisionRecordError does not stop the render or the sync;
// it is returned last, with the write saved and the mutation's result.
// A fn returning a noWrite skips every step after the mutation.
func (b *Board) boardCriticalSectionRollingBack(fn func(store *Store) (any, error), afterSave, rollback func() error) (any, error) {
	// Guard before any disk mutation.
	// A Board built without output filenames, the --board-path shape, carries only the data dir;
	// it can sync and read but must not write.
	// Otherwise the store would be saved and then the render would fail on an empty filename;
	// that would leave a half-applied write that is never synced.
	if b.out.Readme == "" {
		return nil, fmt.Errorf("board outputs not configured; write commands require board config (not --board-path)")
	}

	if err := os.MkdirAll(b.boardPath, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir board: %w", err)
	}

	lockPath, err := fabricengine.BoltLockPath(b.boardPath, writeLockFile)
	if err != nil {
		return nil, err
	}
	lock, err := flock.AcquireWriteLock(lockPath)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer lock.Release()

	store := NewStore(b.boardPath)
	store.vocab = &b.vocab
	store.parentOf = b.parentOf
	store.pairExists = b.pairExists
	if err := store.Load(); err != nil {
		return nil, err
	}

	loaded := cloneTasks(store.Tasks())
	if err := b.checkExpected(loaded, store.primeOwner); err != nil {
		return nil, err
	}
	// Read before fn mutates, so the slug rule judges the raddle store as it stood before the write.
	raddleStore, raddleStoreErr := ReadRaddleStore(b.boardPath)

	result, err := fn(store)
	if err != nil {
		return nil, err
	}
	if skipped, ok := result.(noWrite); ok {
		return skipped.result, nil
	}

	now := b.now()
	promotedSlug := ""
	var released []string
	if p, ok := result.(promoted); ok {
		promotedSlug = p.task.Slug
		// A decision record promoted to a raddle run's entry is released in its own write,
		// so its parent's dobby starts the run.
		if p.task.Recipe == shedrun.RecipeRaddle {
			released = []string{p.task.Slug}
		}
		result = p.task
	}
	// A renamed slug is the same entry, so the loaded entries are judged under the new slug.
	renamedFrom, renamedTo := "", ""
	if r, ok := result.(RenameResult); ok {
		renameSlug(loaded, r.From, r.Task.Slug)
		renamedFrom, renamedTo = r.From, r.Task.Slug
	}
	applyDraftRule(loaded, store.tasks, released)
	stampDates(loaded, store.tasks, now, promotedSlug)
	assignHandles(store.tasks, store.vocab)
	// A returned entry was copied before the stamp, so it is read again to carry its dates and handle.
	switch r := result.(type) {
	case Task:
		result, _ = store.GetTask(r.Slug)
	case ImportResult:
		r.Entry, _ = store.GetTask(r.Entry.Slug)
		result = r
	case RenameResult:
		r.Task, _ = store.GetTask(r.Task.Slug)
		result = r
	}

	if err := checkRunLocks(loaded, store.Tasks(), b.vocab); err != nil {
		return nil, err
	}
	if err := checkProjectRules(loaded, store.Tasks(), store.pairExists); err != nil {
		return nil, err
	}
	if err := checkDiscussedPromotion(loaded, store.Tasks(), b.recordStored); err != nil {
		return nil, err
	}
	if err := checkNewSlugs(loaded, store.Tasks(), renamedTo, b.validateSlug, raddleStore, raddleStoreErr); err != nil {
		return nil, err
	}
	if err := checkTaskCreation(b.strandName, loaded, store.Tasks()); err != nil {
		return nil, err
	}

	// Save before the derived .md view (JSON is authoritative).
	if err := store.Save(); err != nil {
		return nil, fmt.Errorf("save store: %w", err)
	}

	rolledBack := func(failure error) error {
		if rollback == nil {
			return failure
		}
		if err := rollback(); err != nil {
			return errors.Join(failure, fmt.Errorf("roll back the saved store: %w", err))
		}
		return failure
	}
	if afterSave != nil {
		if err := afterSave(); err != nil {
			return nil, rolledBack(err)
		}
	}

	recordErr := followDecisionRecords(b.boardPath, loaded, store.Tasks(), renamedFrom, renamedTo)

	if err := RenderToDisk(b.boardPath, store.Tasks(), store.primeOwner, b.out, now); err != nil {
		return nil, rolledBack(fmt.Errorf("render: %w", err))
	}

	if !b.skipGit {
		_ = spawnSync(b.boardPath)
	}

	return result, recordErr
}

// UpsertTask creates or updates the task identified by fields["slug"] under the write lock;
// field validation (allowlist, slug shape) is the store's job.
func (b *Board) UpsertTask(fields map[string]any) (Task, error) {
	result, err := b.UpsertTaskReporting(fields)
	return result.Task, err
}

// SetStatus sets or clears the status field of the task identified by idOrSlug.
// It acquires the write lock, mutates the store, and triggers a render.
func (b *Board) SetStatus(idOrSlug any, status *string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.SetStatus(idOrSlug, status)
	}, nil)
	return err
}

// SetStatusPushed sets or clears the status of the task slug names and pushes the change before it returns.
// It brings the Bolt up to date with origin first, then commits the write and waits for the push, and it spawns no sync.
// A landing's done mark takes this path, since the run acts on a status only once origin holds it.
// An unreachable origin returns an error wrapping ErrOwnerOriginUnreachable; a push that failed leaves the commit local.
// BOARD_SKIP_GIT writes board.json with no pull, commit or push, and BOARD_SKIP_PUSH commits with no pull or push.
func (b *Board) SetStatusPushed(slug string, status *string) error {
	opts := fabricengine.SyncOptions{SkipGit: b.skipGit, SkipPush: b.skipPush}
	bolt := fabricengine.NewBolt(b.boardPath)
	write := func() ([]string, error) {
		store := NewStore(b.boardPath)
		store.vocab = &b.vocab
		store.parentOf = b.parentOf
		store.pairExists = b.pairExists
		if err := store.Load(); err != nil {
			return nil, err
		}
		if err := store.SetStatus(slug, status); err != nil {
			return nil, err
		}
		if err := store.Save(); err != nil {
			return nil, fmt.Errorf("save store: %w", err)
		}
		rendered, err := renderOwnerWrite(b.boardPath, store.Tasks(), store.primeOwner)
		if err != nil {
			return nil, err
		}
		return append([]string{boardFile}, rendered...), nil
	}

	message := "board: status of " + slug
	if b.skipGit || b.skipPush {
		_, _, err := bolt.CommitWritten(message, write, opts)
		return err
	}
	rec := fabricengine.NewMutations(filepath.Dir(b.boardPath))
	result, err := bolt.PullThenCommitWritten([]fabricengine.BoltWrite{{Message: message, Write: write}}, rec)
	if err != nil {
		return err
	}
	switch result.Skipped {
	case "":
	case fabricengine.BoltSkipFetchFailed:
		return fmt.Errorf("%w: %s", ErrOwnerOriginUnreachable, result.SkipDetail)
	default:
		return fmt.Errorf("board: the status write was refused, since the board could not be brought up to date: %s", result.SkipDetail)
	}
	return pushOwnerCommit(bolt, opts, rec, time.Sleep)
}

// SetRunStatus sets status on the entry slug under the write lock.
// It writes nothing when the entry is absent, done, or already carries status.
func (b *Board) SetRunStatus(slug, status string) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		task, found := store.GetTask(slug)
		if !found || isDone(task) || (task.Status != nil && *task.Status == status) {
			return noWrite{}, nil
		}
		return nil, store.SetStatus(slug, &status)
	}, nil)
	return err
}

// RemoveTask removes the entry that idOrSlug names under the write lock, and renders the outputs.
func (b *Board) RemoveTask(idOrSlug any) error {
	_, err := b.boardCriticalSection(func(store *Store) (any, error) {
		return nil, store.RemoveTask(idOrSlug)
	}, nil)
	return err
}

// RemoveCommand returns the `lym board remove` command line that removes the entry slug names.
// The JSON payload is single-quoted for a shell.
// A way forward names this command with its JSON payload spelled out.
func RemoveCommand(slug string) string {
	payload, _ := json.Marshal(map[string]string{"slug": slug})
	return "lym board remove '" + string(payload) + "'"
}

// MergeTasks atomically removes slugs, upserts one task, and optionally applies a status update.
// setStatus carries the pre-resolved task selector and status value;
// pass nil to skip the status step.
// A status update that targets a missing task fails the entire merge, and the in-memory mutation is discarded.
// It also returns the siblings the merge created.
func (b *Board) MergeTasks(removeSlugs []string, upsert map[string]any, setStatus *MergeStatusUpdate) (Task, []Sibling, error) {
	result, siblings, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		return store.MergeTasks(removeSlugs, upsert, setStatus)
	})
	if err != nil {
		return Task{}, nil, err
	}
	return result.(Task), siblings, nil
}

// SetDeps replaces the depends_on of the task identified by selector under the write lock.
// It returns the siblings the write created.
func (b *Board) SetDeps(selector any, dependsOn []string) ([]Sibling, error) {
	_, siblings, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		return nil, store.SetDeps(selector, dependsOn)
	})
	return siblings, err
}

// UpsertTasksBatch upserts every task in tasks in one write under the write lock.
// It returns the siblings the write created.
func (b *Board) UpsertTasksBatch(tasks []map[string]any) ([]Sibling, error) {
	_, siblings, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		return nil, store.UpsertTasksBatch(tasks)
	})
	return siblings, err
}

// Rerender re-renders the board's output files under the write lock, without changing any entry.
func (b *Board) Rerender() error {
	_, err := b.boardCriticalSection(func(*Store) (any, error) {
		return nil, nil
	}, nil)
	return err
}

// Sync backs up pending local changes to the remote (commit and push), looping until nothing is left.
// The detached `lym board sync` process runs it.
// It can also be called directly to force a synchronous backup.
func (b *Board) Sync() error {
	return Sync(b.boardPath, b.skipGit, b.skipPush)
}

// HealthCheck verifies the board directory exists and holds a readable board.json.
// A syntactically corrupt but readable file passes the health check.
func (b *Board) HealthCheck() error {
	if _, err := os.Stat(b.boardPath); err != nil {
		return err
	}
	_, err := os.ReadFile(filepath.Join(b.boardPath, boardFile))
	return err
}

// loadStore loads the store for a read and persists nothing.
// It wires in the board's vocabulary, parent lookup and pair check.
func (b *Board) loadStore() (*Store, error) {
	store := NewStore(b.boardPath)
	store.vocab = &b.vocab
	store.parentOf = b.parentOf
	store.pairExists = b.pairExists
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

// View returns the board's view projection as a write would see the board: loaded, with nothing saved.
// An entry whose decision record file is missing reads as undiscussed, as the render treats it.
func (b *Board) View() (BoardView, error) {
	store, err := b.loadStore()
	if err != nil {
		return BoardView{}, err
	}
	return buildView(store.Tasks(), withMissingRecordsUndiscussed(b.boardPath, store.Tasks()), store.primeOwner, b.out, b.now())
}

// GetTask returns the task identified by idOrSlug and reports whether one was found.
// A missing board directory yields no task and no error.
func (b *Board) GetTask(idOrSlug any) (Task, bool, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return Task{}, false, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return Task{}, false, err
	}

	task, found := store.GetTask(idOrSlug)
	return task, found, nil
}

// EffectiveOwner returns the owner machine the entry slug reads as; "" means owned by no other machine.
// It persists nothing.
func (b *Board) EffectiveOwner(slug string) (string, error) {
	store, err := b.loadStore()
	if err != nil {
		return "", err
	}
	return store.effectiveOwner(slug)
}

// PrimeOwner returns the machine that board.json records as owning the hub's prime.
// It returns "" when none is recorded or the board directory is absent, and it persists nothing.
func (b *Board) PrimeOwner() (string, error) {
	store, err := b.loadStore()
	if err != nil {
		return "", err
	}
	return store.primeOwner, nil
}

// checkFilterLabels refuses a filter label that is in neither list of the vocabulary and that no entry carries.
// A path-only Board has no config to check against and accepts every label.
func (b *Board) checkFilterLabels(store *Store, labels []string) error {
	if b.out.Readme == "" {
		return nil
	}
	for _, label := range labels {
		if !b.vocab.Known(label) && !store.anyEntryCarries(label) {
			return fmt.Errorf("filter label %q is in neither %s nor %s in board.yaml and no entry carries it: add it to one of them, or filter by a configured label", label, typesName(), labelsName())
		}
	}
	return nil
}

// ListTasksBrief returns the tasks carrying every label in labels, or all tasks when labels is empty.
func (b *Board) ListTasksBrief(labels []string) ([]BriefTask, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}
	if err := b.checkFilterLabels(store, labels); err != nil {
		return nil, err
	}

	return store.ListTasksBrief(labels), nil
}

// ListTasksFull returns every task with all its fields, or nil when the board directory is absent.
func (b *Board) ListTasksFull() ([]Task, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}

	return store.ListTasksFull(), nil
}

// PromoteResult is what a promote wrote.
// It holds the task, the siblings the write created, and a warning about its decision record.
type PromoteResult struct {
	Task     Task
	Siblings []Sibling
	// RecordWarning is set when the promoted entry was a Sketch whose body changed after its decision record was stored;
	// it names the upsert that brings the record up to the body.
	// It is empty otherwise, and for an entry with no record_body_sha256 stamp.
	RecordWarning string
}

// PromoteTo makes the entry identified by idOrSlug a task under the write lock.
// It returns the task with the siblings the write created.
// It also returns a warning for a Sketch whose decision record was stored against a different body.
// The body compared is the one loaded before the write; the warning never refuses the promote.
// When recipe is not empty, the same write sets the task's recipe to recipe.
// A non-nil dependsOn replaces the task's depends_on in the same write,
// so a task the write releases is never started ahead of its dependencies.
// An entry that is already a task is returned unchanged without a write;
// a non-nil dependsOn that differs from its depends_on is refused.
// When recipe is empty, a discussed note takes the raddle recipe if its decision record is stored.
// The same holds for a note that carries the sketch label.
// Promoting a note whose record is stored with the raddle recipe keeps the record and the flag.
// It restores the sketch label that a note write dropped, and it releases the task in the same write.
// A Sketch becomes a task only through that promotion; a promotion of it with any other recipe is refused.
func (b *Board) PromoteTo(idOrSlug any, recipe string, dependsOn []string) (PromoteResult, error) {
	var warning string
	result, siblings, err := b.writeReportingSiblings(func(store *Store) (any, error) {
		if current, found := store.GetTask(idOrSlug); found && IsSketch(current.Kind, current.Labels) && recordBodyChanged(current) {
			warning = fmt.Sprintf("the body of Sketch %q changed after its decision record was stored, so the record may not cover it; "+
				"to bring the record up to the body before its raddle run starts, run `lym board upsert '{\"slug\":\"%s\"}' --record-file <record.md>`",
				current.Slug, current.Slug)
		}
		task, changed, err := store.promoteTo(idOrSlug, recipe, dependsOn, b.recordStored)
		if err != nil {
			return nil, err
		}
		if !changed {
			return noWrite{result: task}, nil
		}
		return promoted{task: task}, nil
	})
	if err != nil {
		return PromoteResult{}, err
	}
	return PromoteResult{Task: result.(Task), Siblings: siblings, RecordWarning: warning}, nil
}

// Prune removes every done entry under the write lock, except an abandoned entry and a project a kept entry names.
// It returns the removed slugs; the render that follows drops their design docs.
// With apply false it is a dry run:
// the slugs a pruning run would remove are reported, and the store is neither saved nor rendered.
// A dry run still takes the board write lock, creating the board directory and the lock file when they are absent.
func (b *Board) Prune(apply bool) ([]string, error) {
	result, err := b.boardCriticalSection(func(store *Store) (any, error) {
		removed := store.Prune()
		if !apply {
			return noWrite{result: removed}, nil
		}
		return removed, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// Find returns the entries whose slug, title, brief or body contains text and that carry every label in labels.
// It persists nothing.
func (b *Board) Find(text string, labels []string) ([]BriefTask, error) {
	if _, err := os.Stat(b.boardPath); os.IsNotExist(err) {
		return nil, nil
	}

	store, err := b.loadStore()
	if err != nil {
		return nil, err
	}
	if err := b.checkFilterLabels(store, labels); err != nil {
		return nil, err
	}
	return store.Find(text, labels), nil
}
