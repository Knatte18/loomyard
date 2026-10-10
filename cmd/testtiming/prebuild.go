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
)

// prebuildLyx returns the environment every `go test` child of a run takes, base less an inherited gateslot.PrebuiltLyxEnv, and the cleanup that removes what the build left.
// A tagged run builds `<root>/cmd/lyx` with go from PATH and base as its environment, and exports the binary through gateslot.PrebuiltLyxEnv.
// A positive parallel caps the build's `-p`, as a slotted run caps its `go test`; zero leaves go's default, as an unslotted run leaves its `go test`.
// An untagged run, or a tagged one whose root has no `cmd/lyx`, builds nothing.
// A build failure returns an error carrying the build's output.
func prebuildLyx(tags, root string, base []string, parallel int) (env []string, cleanup func(), err error) {
	env = gateslot.StripPrebuilt(base)
	if tags == "" {
		return env, func() {}, nil
	}
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return env, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "lyx-prebuilt-")
	if err != nil {
		return nil, nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	bin := filepath.Join(dir, "lyx")
	args := []string{"build", "-C", root}
	if parallel > 0 {
		args = append(args, "-p", strconv.Itoa(parallel))
	}
	cmd := exec.Command("go", append(args, "-o", bin, "./cmd/lyx")...)
	cmd.Env = env
	logger.Info("testtiming: spawning go build of lyx", "root", root, "bin", bin, "parallel", parallel)
	out, buildErr := cmd.CombinedOutput()
	logger.Info("testtiming: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("go build -C %s ./cmd/lyx: %w\n%s", root, buildErr, out)
	}
	return append(env, gateslot.PrebuiltLyxEnv+"="+bin), func() { os.RemoveAll(dir) }, nil
}
