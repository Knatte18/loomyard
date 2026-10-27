// ownerset.go — the owner setter, the only writer of a beam project's owner machine and of the hub's prime owner, and the local owner read.
//
// SetOwners records a machine as the owner of named beam projects in one pull-first Bolt commit and pushes it;
// ReadOwners reads the owners a board directory's local board.json records.

package boardengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/state"
)

// OwnerWritePrime is the key under which OwnerWrite names the hub's prime owner in Recorded and Lost.
const OwnerWritePrime = "prime"

// ownerPushRetryWait is how long SetOwners waits before it pushes again after a held lock deferred the first push.
const ownerPushRetryWait = time.Second

// ErrOwnerOriginUnreachable matches, through errors.Is, an owner write that needs origin and found it unreachable.
var ErrOwnerOriginUnreachable = errors.New("board: origin is unreachable, so the owner write was not pushed")

// OwnerRequest asks SetOwners to record Machine as the owner of the root project and of beam projects, or to claim one entry for it.
type OwnerRequest struct {
	// Machine is the owner to record, this machine's name; empty is refused.
	Machine string
	// Slugs names the entries to write; an absent, done or note entry is left alone.
	// Only a beam project records an owner, so a task or grouping project named without Claim or Beam is not written.
	Slugs []string
	// Prime records Machine as the hub's prime owner, the root project's, as well.
	Prime bool
	// Takeover writes over an owner that reads as another machine; without it such an entry is left alone and reported in OwnerWrite.Lost.
	Takeover bool
	// Claim is a run status written on the request's one slug in the same commit, on a task or a beam project alone, and it records no owner; it needs exactly one slug.
	// A claim on an entry whose effective owner is another machine writes nothing and is reported in OwnerWrite.Lost.
	Claim string
	// Beam sets the beam flag and the project recipe on each named project the write records, in the same commit as the owner; only the beam verbs ask for it.
	Beam bool
	// ParentOf names the slug of an entry's parent pair, "" for the prime, so the effective owner is judged as a read judges it; nil reads every parent as the prime.
	ParentOf func(slug string) (string, error)
	// Sleep waits out the retry delay after a held lock deferred the push; nil sleeps.
	Sleep func(time.Duration)
	// BeforePush runs once between the commit and the first push; nil in production.
	BeforePush func()
}

// OwnerWrite reports what SetOwners recorded.
type OwnerWrite struct {
	// Recorded lists the slugs, and OwnerWritePrime when asked, whose owner reads as the request's Machine after the push.
	Recorded []string
	// Lost maps a slug, or OwnerWritePrime, to the owner that stands instead of the request's Machine.
	Lost map[string]string
}

// SetOwners records req.Machine as the owner of the beam projects among req.Slugs, and of the prime when req.Prime is set, in one commit of the board at boardDir, then pushes it.
// With req.Claim it writes the run status of the one slug instead, and records no owner.
// The commit is made after the board is brought up to date with its upstream, so a stale copy is never written over.
// Without req.Takeover an entry whose effective owner is another machine is neither written nor claimed and is reported in Lost.
// A competing write origin already holds wins the replay of this one, and the re-read after the push reports what stands.
// An unreachable origin returns ErrOwnerOriginUnreachable, wrapped with its detail, and a pull that cannot bring the board up to date, or a push that stays deferred, is refused with its way forward.
// BOARD_SKIP_GIT writes board.json with no pull, commit or push, and BOARD_SKIP_PUSH commits with no pull or push.
func SetOwners(boardDir string, req OwnerRequest) (OwnerWrite, error) {
	if req.Machine == "" {
		return OwnerWrite{}, errors.New("board: the owner write names no machine: take this machine's name from internal/machinename.Name, which fails when the hostname cannot be read")
	}
	if req.Claim != "" && len(req.Slugs) != 1 {
		return OwnerWrite{}, fmt.Errorf("board: a claim status goes with exactly one slug, and the owner write names %d", len(req.Slugs))
	}

	cfg := ApplySkipEnv(Config{Path: boardDir})
	opts := fabricengine.SyncOptions{SkipGit: cfg.SkipGit, SkipPush: cfg.SkipPush}
	bolt := fabricengine.NewBolt(boardDir)

	var written []string
	lost := map[string]string{}
	write := func() ([]string, error) {
		var paths []string
		var err error
		written, paths, err = writeOwners(boardDir, req, lost)
		return paths, err
	}

	subjects := slices.Clone(req.Slugs)
	if req.Prime {
		subjects = append(subjects, OwnerWritePrime)
	}
	message := fmt.Sprintf("board: owner %s for %s", req.Machine, strings.Join(subjects, ", "))

	if cfg.SkipGit || cfg.SkipPush {
		if _, _, err := bolt.CommitWritten(message, write, opts); err != nil {
			return OwnerWrite{}, err
		}
	} else {
		rec := fabricengine.NewMutations(filepath.Dir(boardDir))
		result, err := bolt.PullThenCommitWritten([]fabricengine.BoltWrite{{Message: message, Write: write}}, rec)
		if err != nil {
			return OwnerWrite{}, err
		}
		switch result.Skipped {
		case "":
		case fabricengine.BoltSkipFetchFailed:
			return OwnerWrite{}, fmt.Errorf("%w: %s", ErrOwnerOriginUnreachable, result.SkipDetail)
		default:
			return OwnerWrite{}, fmt.Errorf("board: the owner write was refused, since the board could not be brought up to date: %s", result.SkipDetail)
		}
		if req.BeforePush != nil {
			req.BeforePush()
		}
		sleep := req.Sleep
		if sleep == nil {
			sleep = time.Sleep
		}
		if err := pushOwnerCommit(bolt, opts, rec, sleep); err != nil {
			return OwnerWrite{}, err
		}
	}

	owners, err := ReadOwners(boardDir)
	if err != nil {
		return OwnerWrite{}, err
	}
	var recorded []string
	for _, slug := range written {
		standing := owners.Entries[slug].Owner
		switch {
		case slug == OwnerWritePrime:
			standing = owners.Prime
		case req.Claim != "":
			// A claim records no owner, so it stands while the entry's effective owner is nobody else's.
			if standing, err = owners.Effective(slug, parentOfOrPrime(req.ParentOf)); err != nil {
				return OwnerWrite{}, fmt.Errorf("entry %q: resolve its owner: %w", slug, err)
			}
			if standing == "" {
				standing = req.Machine
			}
		}
		if standing == req.Machine {
			recorded = append(recorded, slug)
			continue
		}
		lost[slug] = standing
	}
	if len(lost) == 0 {
		lost = nil
	}
	return OwnerWrite{Recorded: recorded, Lost: lost}, nil
}

// writeOwners loads the board at boardDir, sets the owners req asks for, saves and renders it, and returns the names it wrote, OwnerWritePrime among them, with the board-relative paths changed.
// An entry left alone for an owner elsewhere is added to lost with that owner.
// The caller holds the board write lock.
func writeOwners(boardDir string, req OwnerRequest, lost map[string]string) (written, paths []string, err error) {
	store := NewStore(boardDir)
	store.parentOf = req.ParentOf
	if err := store.Load(); err != nil {
		return nil, nil, err
	}

	if req.Prime {
		if !req.Takeover && OwnedElsewhere(store.primeOwner, req.Machine) {
			lost[OwnerWritePrime] = store.primeOwner
		} else {
			store.primeOwner = req.Machine
			written = append(written, OwnerWritePrime)
		}
	}
	for _, slug := range req.Slugs {
		index := slices.IndexFunc(store.tasks, func(task Task) bool { return task.Slug == slug })
		if index < 0 || isDone(store.tasks[index]) || (store.tasks[index].Kind != KindTask && store.tasks[index].Kind != KindProject) {
			continue
		}
		effective, err := store.effectiveOwner(slug)
		if err != nil {
			return nil, nil, fmt.Errorf("entry %q: resolve its owner: %w", slug, err)
		}
		if !req.Takeover && OwnedElsewhere(effective, req.Machine) {
			lost[slug] = effective
			continue
		}
		entry := &store.tasks[index]
		if req.Beam && entry.Kind == KindProject {
			entry.Beam = true
			entry.Recipe = ProjectRecipe
		}
		isBeam := entry.Kind == KindProject && entry.Beam
		switch {
		// A claim writes the run status alone; only a task, or a project that is a beam, is claimable, and any other project keeps its status.
		case req.Claim != "":
			if entry.Kind != KindTask && !isBeam {
				continue
			}
			claim := req.Claim
			entry.Status = &claim
		case isBeam:
			entry.OwnerMachine = req.Machine
		default:
			continue
		}
		written = append(written, slug)
	}
	if len(written) == 0 {
		return nil, nil, nil
	}
	if err := store.Save(); err != nil {
		return nil, nil, fmt.Errorf("save store: %w", err)
	}
	rendered, err := renderOwnerWrite(boardDir, store.Tasks(), store.primeOwner)
	if err != nil {
		return nil, nil, err
	}
	return written, append([]string{boardFile}, rendered...), nil
}

// renderOwnerWrite renders the board's output files when its config loads, and returns the paths the render wrote or removed.
// A board whose config does not load is left unrendered, and the next write renders it.
func renderOwnerWrite(boardDir string, tasks []Task, primeOwner string) ([]string, error) {
	cfg, err := LoadConfig(configengine.Dirs{Board: boardDir}, "board")
	if err != nil {
		logger.Warn("boardengine: the owner write is not rendered, since the board config did not load", "path", boardDir, "error", err)
		return nil, nil
	}
	now := time.Now()
	files, err := Render(tasks, primeOwner, cfg.Outputs(), now)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	previous := readRenderManifest(boardDir)
	if err := RenderToDisk(boardDir, tasks, primeOwner, cfg.Outputs(), now); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	var paths []string
	for name := range files {
		paths = append(paths, name)
	}
	for _, name := range previous {
		if _, kept := files[name]; !kept {
			paths = append(paths, name)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// pushOwnerCommit pushes the committed owner write, and pushes once more after sleep when a held lock deferred the first push.
// An unreachable origin is ErrOwnerOriginUnreachable wrapped with the push's error, and any other failed push is returned wrapped with its way forward.
func pushOwnerCommit(bolt *fabricengine.Bolt, opts fabricengine.SyncOptions, rec *fabricengine.Mutations, sleep func(time.Duration)) error {
	for attempt := 1; ; attempt++ {
		if err := bolt.PushRecorded(opts, rec); err != nil {
			var failed *fabricengine.PushFailedError
			if errors.As(err, &failed) && failed.Record.Class == fabricengine.PushFailureTransport {
				return fmt.Errorf("%w: %w", ErrOwnerOriginUnreachable, err)
			}
			return fmt.Errorf("board: the owner commit was kept locally and not pushed: %w", err)
		}
		ahead, err := bolt.HasUnpushed()
		if err != nil {
			return fmt.Errorf("board: read whether the owner commit was pushed: %w", err)
		}
		if !ahead {
			return nil
		}
		if attempt == 2 {
			return errors.New("board: a held board lock deferred the push of the owner commit, and the board is still ahead of origin: run `lym board sync`, then re-run the verb")
		}
		sleep(ownerPushRetryWait)
	}
}

// ReadOwners reads the owners the local board.json at boardDir records, and an empty Owners when the board has none.
func ReadOwners(boardDir string) (Owners, error) {
	path := filepath.Join(boardDir, boardFile)
	if !fileExists(path) {
		return Owners{}, nil
	}
	raw, found, err := state.ReadJSON[json.RawMessage](path, path+swapLockSuffix)
	if err != nil {
		return Owners{}, fmt.Errorf("read owners: %w", err)
	}
	if !found {
		return Owners{}, nil
	}
	return ParseOwners(raw)
}
