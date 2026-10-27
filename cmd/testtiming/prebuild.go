// prebuild.go builds `lym` once for a tagged harness run, so every test binary the run starts shares the one binary through gateslot.PrebuiltLymEnv.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// prebuildLym returns the environment every `go test` child of a run takes, base less an inherited gateslot.PrebuiltLymEnv, and the cleanup that removes what the build left.
// A tagged run builds `<root>/cmd/lym` with goBinary under base and the `-p` cap parallel, and exports the binary through gateslot.PrebuiltLymEnv.
// The build inherits the slot base names, if any, and acquires none of its own.
// An untagged run, a `tier0` run, whose tests spawn no `lym`, or a tagged one whose root has no `cmd/lym`, builds nothing.
// A build failure returns an error carrying the build's output.
func prebuildLym(goBinary, tags, root string, base []string, parallel int) (env []string, cleanup func(), err error) {
	env = gateslot.StripPrebuilt(base)
	if isOfflineRun(tags) {
		return env, func() {}, nil
	}
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lym")); statErr != nil || !info.IsDir() {
		return env, func() {}, nil
	}

	dir, err := lymtemp.MkdirTemp("lym-prebuilt-")
	if err != nil {
		return nil, nil, fmt.Errorf("create the prebuilt lym directory: %w", err)
	}
	bin := filepath.Join(dir, "lym")
	cmd := exec.Command(goBinary, "build", "-C", root, "-p", strconv.Itoa(parallel), "-o", bin, "./cmd/lym")
	cmd.Env = env
	logger.Info("testtiming: spawning go build of lym", "binary", goBinary, "root", root, "bin", bin, "parallel", parallel)
	out, buildErr := cmd.CombinedOutput()
	logger.Info("testtiming: go build of lym ended", "error", buildErr)
	if buildErr != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("%s build -C %s ./cmd/lym: %w\n%s", goBinary, root, buildErr, out)
	}
	return append(env, gateslot.PrebuiltLymEnv+"="+bin), func() { os.RemoveAll(dir) }, nil
}
