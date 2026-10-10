// housekeeping.go keeps the hub's two object stores from auto-gc'ing under live steps:
// disableStoreAutoGC turns git's automatic gc and maintenance off in both, and housekeepStores runs gc in the foreground where lyx is already doing slow work, after a pair's removal.
// It removes no worktree, branch or file the destruction gate protects: gc deletes only unreachable objects and expired reflog entries past git's own default expiry.

package fabricengine

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// gcAutoThreshold is git's default gc.auto: the loose-object estimate above which a foreground `gc --auto` actually packs.
// It is a variable only so a test can lower it.
var gcAutoThreshold = 6700

// namedStore is one of the hub's two object stores with the name its log lines use.
type namedStore struct {
	name string
	path string
}

// hubStores returns the code store and the records store of l's hub, the working directories git resolves each store's shared config and object database from.
func hubStores(l *lyxcwd.Location) ([]namedStore, error) {
	recordsPath, err := RecordsRepoRoot(l)
	if err != nil {
		return nil, fmt.Errorf("resolve records repo root: %w", err)
	}
	return []namedStore{{name: "code", path: l.WorktreePath()}, {name: "records", path: recordsPath}}, nil
}

// disableStoreAutoGC sets gc.auto=0 and maintenance.auto=false in the shared config of the code store and of the records store.
// gc.auto=0 stops git's post-command auto gc, and maintenance.auto=false stops its post-command `git maintenance run --auto`, whose repack tasks gc.auto does not govern.
// It attempts both stores whatever one returns, and its error joins every failure.
func disableStoreAutoGC(l *lyxcwd.Location) error {
	stores, err := hubStores(l)
	if err != nil {
		return err
	}
	var failures []error
	for _, store := range stores {
		for _, setting := range [][2]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}} {
			if _, err := gitexec.Run([]string{"config", setting[0], setting[1]}, store.path); err != nil {
				failures = append(failures, fmt.Errorf("set %s in the %s store: %w", setting[0], store.name, err))
			}
		}
	}
	return errors.Join(failures...)
}

// housekeepStores disables auto gc in both stores, then runs `git gc --auto` in the foreground in each.
// The gc runs only when a store is above gcAutoThreshold, and never prunes a worktree's admin entry, which stays under the destruction gate.
// It is best-effort and returns nothing: a failure is logged as a warning and never fails the caller.
// A store another gc already holds is skipped silently, since git exits 0 without packing then.
func housekeepStores(l *lyxcwd.Location) {
	if err := disableStoreAutoGC(l); err != nil {
		logger.Warn("fabricengine: disabling store auto gc failed (non-fatal)", "error", err)
	}

	stores, err := hubStores(l)
	if err != nil {
		logger.Warn("fabricengine: store housekeeping skipped (non-fatal)", "error", err)
		return
	}
	args := []string{
		"-c", "gc.auto=" + strconv.Itoa(gcAutoThreshold),
		"-c", "gc.autoDetach=false",
		"-c", "gc.worktreePruneExpire=never",
		"gc", "--auto", "--quiet",
	}
	for _, store := range stores {
		logger.Info("fabricengine: running foreground gc", "store", store.name, "path", store.path)
		if _, err := gitexec.Run(args, store.path); err != nil {
			logger.Warn("fabricengine: store gc failed (non-fatal)", "store", store.name, "path", store.path, "error", err)
			continue
		}
		logger.Info("fabricengine: foreground gc finished", "store", store.name, "path", store.path)
	}
}
