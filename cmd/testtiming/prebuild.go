// prebuild.go builds `lyx` once for a tagged harness run, so every test binary the run starts shares the one binary through gateslot.PrebuiltLyxEnv.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
)

// prebuildLyx returns the environment every `go test` child of a run takes, base less an inherited gateslot.PrebuiltLyxEnv, and the cleanup that removes what the build left.
// A tagged run builds `<root>/cmd/lyx` with go from PATH in a gate slot it holds for the build alone, capped at the slot's `-p`, and exports the binary through gateslot.PrebuiltLyxEnv;
// the returned environment carries none of the slot's variables.
// An untagged run, or a tagged one whose root has no `cmd/lyx`, builds nothing and takes no slot.
// A build failure returns an error carrying the build's output.
func prebuildLyx(tags, root string, base []string) (env []string, cleanup func(), err error) {
	env = gateslot.StripPrebuilt(base)
	if tags == "" {
		return env, func() {}, nil
	}
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return env, func() {}, nil
	}
	slotEnv, parallel, release, err := takeGateSlot(context.Background(), root, env)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	dir, err := os.MkdirTemp("", "lyx-prebuilt-")
	if err != nil {
		return nil, nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	bin := filepath.Join(dir, "lyx")
	cmd := exec.Command("go", "build", "-C", root, "-p", strconv.Itoa(parallel), "-o", bin, "./cmd/lyx")
	cmd.Env = slotEnv
	logger.Info("testtiming: spawning go build of lyx", "root", root, "bin", bin, "parallel", parallel)
	out, buildErr := cmd.CombinedOutput()
	logger.Info("testtiming: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("go build -C %s ./cmd/lyx: %w\n%s", root, buildErr, out)
	}
	return append(env, gateslot.PrebuiltLyxEnv+"="+bin), func() { os.RemoveAll(dir) }, nil
}
