// newbinary.go times the first and second run of a freshly linked program in a folder,
// so an endpoint agent's hold on an unseen binary, and a folder exception's effect on it, show as figures.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The program the probe links: it does nothing, so its run time is the start cost and any hold alone.
const (
	probeProgramSource = "package main\n\nfunc main() {}\n"
	probeProgramModule = "module machinecostprobe\n\ngo 1.22\n"
)

// buildCacheName is the folder, inside the probe's temp folder, that holds the probe build's cache.
const buildCacheName = "gocache"

// buildIDBytes is the length of the random build ID each linked program gets, so each has a hash never seen before.
const buildIDBytes = 20

// measureNewBinaries links count programs, each with a random build ID, into a folder it makes inside dir.
// It runs each twice and adds the median first and second run under newbinary/<name>/.
// The source is written into tempDir, so only the binaries land in dir.
func measureNewBinaries(ctx context.Context, tempDir string, dir NamedDir, count int, measurements *collector) error {
	sourceDir := filepath.Join(tempDir, "probe-program")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return fmt.Errorf("create probe program folder: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte(probeProgramSource), 0o644); err != nil {
		return fmt.Errorf("write probe program: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "go.mod"), []byte(probeProgramModule), 0o644); err != nil {
		return fmt.Errorf("write probe program module: %w", err)
	}
	binaryDir, err := os.MkdirTemp(dir.Path, probeFolderPrefix)
	if err != nil {
		return fmt.Errorf("create folder in %s: %w", dir.Path, err)
	}
	defer os.RemoveAll(binaryDir)

	firstRuns := make([]time.Duration, 0, count)
	secondRuns := make([]time.Duration, 0, count)
	for range count {
		binary, err := linkProbeProgram(ctx, tempDir, sourceDir, binaryDir)
		if err != nil {
			return err
		}
		first, err := timeRun(ctx, binary)
		if err != nil {
			return err
		}
		second, err := timeRun(ctx, binary)
		if err != nil {
			return err
		}
		firstRuns = append(firstRuns, first)
		secondRuns = append(secondRuns, second)
	}
	prefix := groupNewBinary + "/" + dir.Name + "/"
	measurements.add(prefix+"first_run_ms", unitMilliseconds, milliseconds(median(firstRuns)))
	measurements.add(prefix+"second_run_ms", unitMilliseconds, milliseconds(median(secondRuns)))
	return nil
}

// linkProbeProgram builds the program in sourceDir into binaryDir under a random build ID and returns the binary's path.
// So that every machine links it the same way, the build turns off the workspace, the go env file, GOFLAGS and cgo.
// It uses the local toolchain with its default experiments and amd64 level.
// The go command's work folder, build cache and user config are inside tempDir, and its telemetry is off.
// A ctx cancelled during the build lets the build finish and then returns the context's error.
func linkProbeProgram(ctx context.Context, tempDir, sourceDir, binaryDir string) (string, error) {
	buildID := make([]byte, buildIDBytes)
	if _, err := rand.Read(buildID); err != nil {
		return "", fmt.Errorf("draw build ID: %w", err)
	}
	buildIDHex := hex.EncodeToString(buildID)
	binary := filepath.Join(binaryDir, "probe-"+buildIDHex)
	configEntry, err := goConfigWithoutTelemetry(tempDir)
	if err != nil {
		return "", err
	}
	// The build does not take ctx.
	// A signalled go command exits without waiting for its compiler and linker.
	// Those would still write into tempDir.
	build := exec.Command(goBinary, "build", "-o", binary, "-ldflags=-B=0x"+buildIDHex, ".")
	build.Dir = sourceDir
	// An empty value falls back to the go env file, so GOENV=off is what makes the empty ones mean the default.
	build.Env = append(os.Environ(), "GOWORK=off", "GOENV=off", "GOFLAGS=", "CGO_ENABLED=0",
		"GOTOOLCHAIN=local", "GOEXPERIMENT=", "GOAMD64=", "GOTMPDIR="+tempDir,
		"GOCACHE="+filepath.Join(tempDir, buildCacheName), "GOCACHEPROG=", configEntry)
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build probe program: %w: %s", err, out)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("build probe program: %w", err)
	}
	return binary, nil
}

func timeRun(ctx context.Context, binary string) (time.Duration, error) {
	started := time.Now()
	if err := exec.CommandContext(ctx, binary).Run(); err != nil {
		return 0, fmt.Errorf("run %s: %w", binary, err)
	}
	return time.Since(started), nil
}
