// prebuild.go builds `lyx` once for a tagged `lyx gate test` run, so every package of the run shares the one binary through gateslot.PrebuiltLyxEnv, and inside a hub keeps it in the hub's content-addressed store.

package gatecli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
)

// prebuiltRetention is how long a stored binary stays after its last use before a later prebuild prunes it.
// It is far longer than any gate run, so no run loses its binary while it still runs.
const prebuiltRetention = 2 * time.Hour

// prebuiltNamePrefix starts every stored binary's name, followed by the leading hex digits of its SHA-256.
const prebuiltNamePrefix = "lyx-"

// buildRoot returns the directory whose `cmd/lyx` a run builds: the worktree root inside a hub, dir outside one.
func buildRoot(location *lyxcwd.Location, dir string) string {
	if location != nil {
		return location.WorktreePath()
	}
	return dir
}

// prebuildLyx builds `<root>/cmd/lyx` into a fresh directory in the process run directory with goBinary under the run's `-p` cap and `-trimpath`, and returns the binary path and a cleanup that removes what the run does not keep.
// With storeDir non-empty the binary is published into storeDir under a name derived from its content and that path is returned, so two runs on an unchanged tree export the same path and go test's cache keys stay stable;
// the build directory is then removed at once, and the store keeps the binary for later runs.
// With storeDir empty the returned path is the build directory's, which the cleanup removes.
// A root without `cmd/lyx` builds nothing and returns an empty path and a cleanup that does nothing.
// A build failure returns an error carrying the build's output.
func prebuildLyx(ctx context.Context, goBinary, root string, parallel int, storeDir string) (bin string, cleanup func(), err error) {
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return "", func() {}, nil
	}
	dir, err := lyxtemp.MkdirTemp("lyx-prebuilt-")
	if err != nil {
		return "", nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	cleanup = func() { os.RemoveAll(dir) }

	bin = filepath.Join(dir, "lyx")
	cmd := exec.CommandContext(ctx, goBinary, "build", "-C", root, "-p", strconv.Itoa(parallel), "-trimpath", "-o", bin, "./cmd/lyx")
	logger.Info("gate test: spawning go build of lyx", "binary", goBinary, "args", strings.Join(cmd.Args[1:], " "))
	out, buildErr := cmd.CombinedOutput()
	logger.Info("gate test: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("%s build -C %s ./cmd/lyx: %w\n%s", goBinary, root, buildErr, out)
	}
	if storeDir == "" {
		return bin, cleanup, nil
	}
	stored, err := storePrebuilt(bin, storeDir, time.Now())
	cleanup()
	if err != nil {
		return "", nil, fmt.Errorf("keep the prebuilt lyx in %s: %w", storeDir, err)
	}
	return stored, func() {}, nil
}

// storePrebuilt publishes the binary at bin into storeDir under `lyx-` and the first 16 hex digits of its SHA-256, and returns the stored path.
// A binary already stored under that name is kept and its modification time set to now, which marks its last use.
// A new one, or an existing one a concurrent prune removes around that refresh, is copied in under a name unique to this process and renamed into place, so a concurrent run never sees half a binary.
// It then prunes every other entry of storeDir last modified more than prebuiltRetention before now; a failed removal is logged, never returned.
func storePrebuilt(bin, storeDir string, now time.Time) (string, error) {
	sum, err := fileSHA256(bin)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return "", err
	}
	name := prebuiltNamePrefix + sum[:16]
	stored := filepath.Join(storeDir, name)
	refreshed, err := refreshStored(stored, now)
	if err != nil {
		return "", err
	}
	if !refreshed {
		if err := copyIntoPlace(bin, stored, now); err != nil {
			return "", err
		}
	}
	pruneStore(storeDir, name, now)
	return stored, nil
}

// refreshStored sets the modification time of the entry at stored to now, which marks its last use, and reports whether the entry is there to use.
// An entry that is missing, or that a concurrent prune removes right after the refresh, reports false with no error, so the caller publishes it again.
func refreshStored(stored string, now time.Time) (bool, error) {
	if err := os.Chtimes(stored, now, now); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if _, err := os.Stat(stored); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// copyIntoPlace copies the executable at src to dst through a sibling staging file named for this process, with its modification time set to now.
// On a platform whose rename refuses an existing target, a dst another run published first is kept and the staging file removed.
func copyIntoPlace(src, dst string, now time.Time) error {
	staging := dst + ".partial-" + strconv.Itoa(os.Getpid())
	if err := copyExecutable(src, staging); err != nil {
		os.Remove(staging)
		return err
	}
	if err := os.Chtimes(staging, now, now); err != nil {
		os.Remove(staging)
		return err
	}
	if err := os.Rename(staging, dst); err != nil {
		os.Remove(staging)
		if _, statErr := os.Stat(dst); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

// copyExecutable writes the content of src to a new file dst with mode 0755.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// fileSHA256 returns the lowercase hex SHA-256 of the file at path.
func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// pruneStore removes every entry of storeDir other than keep whose modification time is more than prebuiltRetention before now.
// A failure is logged and the entry left for a later prune.
func pruneStore(storeDir, keep string, now time.Time) {
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		logger.Warn("gate test: could not list the prebuilt lyx store", "dir", storeDir, "cause", err)
		return
	}
	for _, entry := range entries {
		if entry.Name() == keep {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) <= prebuiltRetention {
			continue
		}
		path := filepath.Join(storeDir, entry.Name())
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("gate test: could not prune a prebuilt lyx", "path", path, "cause", err)
		}
	}
}
