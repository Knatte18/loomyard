//go:build integration

package lyxbin

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_ProducesExecutableBinary(t *testing.T) {
	bin := Build(t)

	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("built binary %s is not executable: mode %v", bin, info.Mode())
	}
}

func TestBuildWithLDFlags_ReachTheBuild(t *testing.T) {
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
}

func TestBuildInto_UnwritableDirReturnsError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	bin, err := BuildInto(filepath.Join(blocker, "out"), "")
	if err == nil {
		t.Fatalf("BuildInto into a path under a regular file succeeded: %s", bin)
	}
}
