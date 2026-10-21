// prebuild.go builds `lyx` once for a tagged harness run, so every test binary the run starts shares the one binary through gateslot.PrebuiltLyxEnv.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
)

// prebuildLyx returns the environment every `go test` child of a run takes, base less an inherited gateslot.PrebuiltLyxEnv, and the cleanup that removes what the build left.
// A tagged run builds `<root>/cmd/lyx` with goBinary under base and the `-p` cap parallel, and exports the binary through gateslot.PrebuiltLyxEnv.
// The build inherits the slot base names, if any, and acquires none of its own.
// An untagged run, or a tagged one whose root has no `cmd/lyx`, builds nothing.
// A build failure returns an error carrying the build's output.
func prebuildLyx(goBinary, tags, root string, base []string, parallel int) (env []string, cleanup func(), err error) {
	env = gateslot.StripPrebuilt(base)
	if tags == "" {
		return env, func() {}, nil
	}
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return env, func() {}, nil
	}

	dir, err := lyxtemp.MkdirTemp("lyx-prebuilt-")
	if err != nil {
		return nil, nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	bin := filepath.Join(dir, "lyx")
	cmd := exec.Command(goBinary, "build", "-C", root, "-p", strconv.Itoa(parallel), "-o", bin, "./cmd/lyx")
	cmd.Env = env
	logger.Info("testtiming: spawning go build of lyx", "binary", goBinary, "root", root, "bin", bin, "parallel", parallel)
	out, buildErr := cmd.CombinedOutput()
	logger.Info("testtiming: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("%s build -C %s ./cmd/lyx: %w\n%s", goBinary, root, buildErr, out)
	}
	return append(env, gateslot.PrebuiltLyxEnv+"="+bin), func() { os.RemoveAll(dir) }, nil
}
