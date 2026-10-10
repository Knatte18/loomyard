//go:build integration

// These tests set the prebuilt-binary variable through t.Setenv, so none of them calls t.Parallel: the process environment is the global state they share.

package lyxbin

import (
	"debug/buildinfo"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

func requireExecutable(t *testing.T, bin string) {
	t.Helper()
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("built binary %s is not executable: mode %v", bin, info.Mode())
	}
}

func TestBuild_ProducesExecutableBinary(t *testing.T) {
	t.Setenv(gateslot.PrebuiltLyxEnv, "")
	built := Build(t)

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"unset: two calls share one executable build", func(t *testing.T) {
			t.Setenv(gateslot.PrebuiltLyxEnv, "")
			first, second := Build(t), Build(t)
			if first != second {
				t.Errorf("second Build = %q; want the first call's %q", second, first)
			}
			requireExecutable(t, first)
		}},
		{"set: Build returns the named binary", func(t *testing.T) {
			data, err := os.ReadFile(built)
			if err != nil {
				t.Fatal(err)
			}
			copyPath := filepath.Join(t.TempDir(), "prebuilt-lyx")
			if err := os.WriteFile(copyPath, data, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv(gateslot.PrebuiltLyxEnv, copyPath)
			if got := Build(t); got != copyPath {
				t.Errorf("Build = %q; want the prebuilt %q", got, copyPath)
			}
		}},
		{"set to a missing path: BuildInto is stale", func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "gone")
			t.Setenv(gateslot.PrebuiltLyxEnv, missing)
			_, err := BuildInto(t.TempDir(), "")
			if !errors.Is(err, ErrStalePrebuilt) || !strings.Contains(err.Error(), missing) {
				t.Errorf("BuildInto error = %v; want ErrStalePrebuilt naming %q", err, missing)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestBuildWithLDFlags_ReachTheBuild(t *testing.T) {
	prebuiltPath := Build(t)

	tests := []struct {
		name     string
		prebuilt string
	}{
		{"variable unset", ""},
		{"variable set", prebuiltPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(gateslot.PrebuiltLyxEnv, tt.prebuilt)
			bin := BuildWithLDFlags(t, "-s -w")

			info, err := buildinfo.ReadFile(bin)
			if err != nil {
				t.Fatalf("read build info of %s: %v", bin, err)
			}
			for _, s := range info.Settings {
				if s.Key == "-ldflags" {
					if s.Value != "-s -w" {
						t.Errorf("-ldflags = %q, want %q", s.Value, "-s -w")
					}
					return
				}
			}
			t.Errorf("build info carries no -ldflags setting: %v", info.Settings)
		})
	}
}

func TestBuildInto_UnwritableDirReturnsError(t *testing.T) {
	t.Setenv(gateslot.PrebuiltLyxEnv, "")
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	bin, err := BuildInto(filepath.Join(blocker, "out"), "")
	if err == nil {
		t.Fatalf("BuildInto into a path under a regular file succeeded: %s", bin)
	}
}
