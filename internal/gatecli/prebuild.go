// prebuild.go builds `lyx` once for a tagged `lyx gate test` run, so every package of the run shares the one binary through gateslot.PrebuiltLyxEnv.

package gatecli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// buildRoot returns the directory whose `cmd/lyx` a run builds: the worktree root inside a hub, dir outside one.
func buildRoot(location *lyxcwd.Location, dir string) string {
	if location != nil {
		return location.WorktreePath()
	}
	return dir
}

// prebuildLyx builds `<root>/cmd/lyx` into a fresh temporary directory with goBinary and returns the binary path and a cleanup that removes the directory.
// A root without `cmd/lyx` builds nothing and returns an empty path and a cleanup that does nothing.
// A build failure returns an error carrying the build's output.
func prebuildLyx(ctx context.Context, goBinary, root string) (bin string, cleanup func(), err error) {
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return "", func() {}, nil
	}
	dir, err := os.MkdirTemp("", "lyx-prebuilt-")
	if err != nil {
		return "", nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	cleanup = func() { os.RemoveAll(dir) }

	bin = filepath.Join(dir, "lyx")
	cmd := exec.CommandContext(ctx, goBinary, "build", "-C", root, "-o", bin, "./cmd/lyx")
	logger.Info("gate test: spawning go build of lyx", "binary", goBinary, "args", strings.Join(cmd.Args[1:], " "))
	out, buildErr := cmd.CombinedOutput()
	logger.Info("gate test: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("%s build -C %s ./cmd/lyx: %w\n%s", goBinary, root, buildErr, out)
	}
	return bin, cleanup, nil
}
