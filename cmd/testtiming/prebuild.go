// prebuild.go builds `lyx` once for a tagged harness run, so every test binary the run starts shares the one binary through gateslot.PrebuiltLyxEnv.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// prebuildLyx returns the environment every `go test` child of a run takes, and the cleanup that removes what the build left.
// A tagged run builds `./cmd/lyx` of the module root, the working directory, with go from PATH and exports the binary through gateslot.PrebuiltLyxEnv.
// An untagged run, or a tagged one whose root has no `cmd/lyx`, builds nothing and strips an inherited value.
// A build failure returns an error carrying the build's output.
func prebuildLyx(tags string) (env []string, cleanup func(), err error) {
	env = gateslot.StripPrebuilt(os.Environ())
	if tags == "" {
		return env, func() {}, nil
	}
	root, err := lyxcwd.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("read the working directory: %w", err)
	}
	if info, statErr := os.Stat(filepath.Join(root, "cmd", "lyx")); statErr != nil || !info.IsDir() {
		return env, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "lyx-prebuilt-")
	if err != nil {
		return nil, nil, fmt.Errorf("create the prebuilt lyx directory: %w", err)
	}
	bin := filepath.Join(dir, "lyx")
	logger.Info("testtiming: spawning go build of lyx", "root", root, "bin", bin)
	out, buildErr := exec.Command("go", "build", "-C", root, "-o", bin, "./cmd/lyx").CombinedOutput()
	logger.Info("testtiming: go build of lyx ended", "error", buildErr)
	if buildErr != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("go build -C %s ./cmd/lyx: %w\n%s", root, buildErr, out)
	}
	return append(env, gateslot.PrebuiltLyxEnv+"="+bin), func() { os.RemoveAll(dir) }, nil
}
