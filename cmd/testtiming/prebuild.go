// prebuild.go builds `lym` once for a tagged harness run,
// so every test binary the run starts shares the one binary through gateslot.PrebuiltLymEnv.

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

// prebuildLym returns the environment for the `go test` children of a run:
// base without an inherited gateslot.PrebuiltLymEnv.
// It also returns the cleanup that removes what the build left.
// A tagged run builds `<root>/cmd/lym` with goBinary under base, passing parallel as its `-p` cap.
// It exports the binary through gateslot.PrebuiltLymEnv.
// The build inherits the gate slot's environment and acquires no slot of its own.
// An offline run, untagged or `tier0`, builds nothing, since its tests spawn no `lym`.
// A tagged run whose root has no `cmd/lym` builds nothing either.
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
