// machine.go reads the machine descriptor and the load snapshots a report carries.
// Its sources are /proc, /sys and the tools' versions.
// A figure the machine does not expose, such as CPU pressure on an older kernel, is left absent.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Files the descriptor and the snapshots read.
const (
	loadavgPath       = "/proc/loadavg"
	cpuPressurePath   = "/proc/pressure/cpu"
	cpuinfoPath       = "/proc/cpuinfo"
	kernelReleasePath = "/proc/sys/kernel/osrelease"
	governorPath      = "/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor"
	throttleCountGlob = "/sys/devices/system/cpu/cpu*/thermal_throttle/core_throttle_count"
	// trapsPath is where Cortex XDR installs; its presence marks a machine whose endpoint agent holds new binaries.
	trapsPath = "/opt/traps"
)

// goBinary is the Go toolchain the probe links its new binaries with.
const goBinary = "go"

// goConfigName is the folder, inside the probe's temp folder, that the probe's go commands take as the user config.
const goConfigName = "config"

// Machine describes the machine a report was measured on.
type Machine struct {
	Hostname     string `json:"hostname"`
	CPUModel     string `json:"cpu_model"`
	Cores        int    `json:"cores"`
	Kernel       string `json:"kernel"`
	Governor     string `json:"governor"`
	GoVersion    string `json:"go_version"`
	GitVersion   string `json:"git_version"`
	TrapsPresent bool   `json:"traps_present"`
}

// Snapshot is the machine's load and CPU state at one instant; a nil field is one the machine does not expose.
type Snapshot struct {
	Load1                *float64 `json:"load1"`
	Load5                *float64 `json:"load5"`
	Load15               *float64 `json:"load15"`
	CPUPressureSomeAvg10 *float64 `json:"cpu_pressure_some_avg10"`
	CPUPressureSomeAvg60 *float64 `json:"cpu_pressure_some_avg60"`
	MeanMHz              *float64 `json:"mean_mhz"`
	// ThrottleCount sums every core's thermal throttle count since boot,
	// so the difference between two snapshots counts the throttles in between.
	ThrottleCount *float64 `json:"throttle_count"`
}

// readMachine describes this machine, with the version of the local Go toolchain, asked from inside tempDir.
// tempDir holds no Go module, so no module's toolchain requirement applies.
// The go command's user config is made inside tempDir too, with its telemetry off.
// The probe runs both go and git, so it fails only when the version of either cannot be read.
func readMachine(tempDir string) (Machine, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Machine{}, fmt.Errorf("read host name: %w", err)
	}
	configEntry, err := goConfigWithoutTelemetry(tempDir)
	if err != nil {
		return Machine{}, err
	}
	goVersionQuery := exec.Command(goBinary, "env", "GOVERSION")
	goVersionQuery.Dir = tempDir
	goVersionQuery.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", configEntry)
	goVersion, err := commandOutput(goVersionQuery)
	if err != nil {
		return Machine{}, err
	}
	gitVersion, err := commandOutput(exec.Command(gitBinary, "--version"))
	if err != nil {
		return Machine{}, err
	}
	model, _ := parseCPUInfo(readOptionalFile(cpuinfoPath))
	_, trapsErr := os.Stat(trapsPath)
	return Machine{
		Hostname:     hostname,
		CPUModel:     model,
		Cores:        runtime.NumCPU(),
		Kernel:       readOptionalFile(kernelReleasePath),
		Governor:     readOptionalFile(governorPath),
		GoVersion:    goVersion,
		GitVersion:   gitVersion,
		TrapsPresent: trapsErr == nil,
	}, nil
}

// goConfigWithoutTelemetry makes a user config folder with Go telemetry off inside tempDir.
// It returns the environment entry that points a go command at that folder.
// With telemetry on, a go command writes counters into the user config folder.
// It can also start a detached process there that outlives it.
func goConfigWithoutTelemetry(tempDir string) (string, error) {
	configDir := filepath.Join(tempDir, goConfigName)
	telemetryDir := filepath.Join(configDir, "go", "telemetry")
	if err := os.MkdirAll(telemetryDir, 0o755); err != nil {
		return "", fmt.Errorf("create go telemetry folder: %w", err)
	}
	if err := os.WriteFile(filepath.Join(telemetryDir, "mode"), []byte("off"), 0o644); err != nil {
		return "", fmt.Errorf("turn off go telemetry: %w", err)
	}
	return "XDG_CONFIG_HOME=" + configDir, nil
}

// readSnapshot takes the machine's load and CPU state now.
func readSnapshot() Snapshot {
	var snapshot Snapshot
	snapshot.Load1, snapshot.Load5, snapshot.Load15 = parseLoadavg(readOptionalFile(loadavgPath))
	snapshot.CPUPressureSomeAvg10, snapshot.CPUPressureSomeAvg60 = parseCPUPressure(readOptionalFile(cpuPressurePath))
	_, snapshot.MeanMHz = parseCPUInfo(readOptionalFile(cpuinfoPath))
	snapshot.ThrottleCount = readThrottleCount()
	return snapshot
}

// readOptionalFile returns a file's trimmed content, or "" when the machine does not have it.
func readOptionalFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func commandOutput(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run %s: %w", strings.Join(cmd.Args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseLoadavg reads the 1, 5 and 15 minute load averages from /proc/loadavg's text.
func parseLoadavg(text string) (load1, load5, load15 *float64) {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return nil, nil, nil
	}
	return parseOptionalFloat(fields[0]), parseOptionalFloat(fields[1]), parseOptionalFloat(fields[2])
}

// parseCPUPressure reads the `some` line's avg10 and avg60 from /proc/pressure/cpu's text.
func parseCPUPressure(text string) (avg10, avg60 *float64) {
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "some" {
			continue
		}
		for _, field := range fields[1:] {
			key, value, _ := strings.Cut(field, "=")
			switch key {
			case "avg10":
				avg10 = parseOptionalFloat(value)
			case "avg60":
				avg60 = parseOptionalFloat(value)
			}
		}
	}
	return avg10, avg60
}

// parseCPUInfo reads the first CPU's model name and the mean of every core's current frequency from /proc/cpuinfo's text.
func parseCPUInfo(text string) (model string, meanMHz *float64) {
	var totalMHz float64
	var cores int
	for line := range strings.Lines(text) {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "model name":
			if model == "" {
				model = value
			}
		case "cpu MHz":
			if mhz := parseOptionalFloat(value); mhz != nil {
				totalMHz += *mhz
				cores++
			}
		}
	}
	if cores == 0 {
		return model, nil
	}
	mean := totalMHz / float64(cores)
	return model, &mean
}

// readThrottleCount sums every core's thermal throttle count, or returns nil when the machine exposes none.
func readThrottleCount() *float64 {
	paths, err := filepath.Glob(throttleCountGlob)
	if err != nil {
		return nil
	}
	texts := make([]string, 0, len(paths))
	for _, path := range paths {
		texts = append(texts, readOptionalFile(path))
	}
	return sumThrottleCounts(texts)
}

// sumThrottleCounts sums the per-core counts in texts.
// It returns nil when there are none or one does not parse, since a partial sum would compare with nothing.
func sumThrottleCounts(texts []string) *float64 {
	if len(texts) == 0 {
		return nil
	}
	var total float64
	for _, text := range texts {
		count := parseOptionalFloat(text)
		if count == nil {
			return nil
		}
		total += *count
	}
	return &total
}

func parseOptionalFloat(text string) *float64 {
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return &value
}
