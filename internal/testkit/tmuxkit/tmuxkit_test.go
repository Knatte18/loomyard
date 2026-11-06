package tmuxkit

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCheckSocketPath(t *testing.T) {
	tiergate.Require(t, 0)
	runDir, err := lymtemp.Dir()
	if err != nil {
		t.Fatal(err)
	}
	const tenDigitUID = 1856216086
	tests := []struct {
		name    string
		dir     string
		uid     int
		wantErr bool
	}{
		{"short directory", "/tmp/lym123", 1000, false},
		{"directory at the limit", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000, false},
		{"directory one byte over", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes+1), 1000, true},
		{"run directory of the widest uid", "/tmp/lym-4294967295/r0a1b", 4294967295, false},
		{"run directory of a ten-digit uid under /dev/shm", filepath.Join("/dev/shm", "lym-"+strconv.Itoa(tenDigitUID), filepath.Base(runDir)), tenDigitUID, false},
		{"longer uid widens the path", "/tmp/" + strings.Repeat("d", 107-len("/tmp/")-len("/tmux-1000/")-maxKeyBytes), 1000000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSocketPath(tt.dir, tt.uid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkSocketPath(%q, %d) error = %v, wantErr %v", tt.dir, tt.uid, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "TMPDIR") {
				t.Errorf("error %q does not name TMPDIR as the remedy", err)
			}
		})
	}
}

func TestSetEnv(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("TMUX", "/outer/tmux,1,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("LYM_REED_TMUX_CONF", "/operator/tmux.conf")
	t.Setenv("TMUX_TMPDIR", "/elsewhere")
	// TMP and TEMP are the Windows temp variables; setting them first has t.Setenv restore them.
	for _, name := range []string{"TMPDIR", "TMP", "TEMP", "GIT_CEILING_DIRECTORIES", "XDG_STATE_HOME", "LOCALAPPDATA"} {
		t.Setenv(name, os.Getenv(name))
	}
	dir := filepath.Join(t.TempDir(), dirPrefix+"0")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	setEnv(dir)

	for _, name := range []string{"TMUX_TMPDIR", "TMPDIR", "GIT_CEILING_DIRECTORIES", "XDG_STATE_HOME"} {
		if got := os.Getenv(name); got != dir {
			t.Errorf("%s = %q, want %q", name, got, dir)
		}
	}
	// A subtest's TempDir resolves its base afresh, so it sees the TMPDIR setEnv just set.
	t.Run("temp directory lands under TMPDIR", func(t *testing.T) {
		if got := t.TempDir(); !strings.HasPrefix(got, dir+string(filepath.Separator)) {
			t.Errorf("t.TempDir() = %q, want it under %q", got, dir)
		}
	})
	for _, name := range []string{"TMUX", "TMUX_PANE", "LYM_REED_TMUX_CONF"} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s still set to %q", name, v)
		}
	}
}

func TestListSockets_MissingDirectory(t *testing.T) {
	tiergate.Require(t, 0)
	if got := listSockets(t.TempDir(), 1000); len(got) != 0 {
		t.Errorf("listSockets of a directory with no tmux subdirectory = %v, want none", got)
	}
}

func TestMaxKeyBytes_HoldsReedServerName(t *testing.T) {
	tiergate.Require(t, 0)
	hub := "/" + strings.Repeat("h", 300) + "-LYMHUB"
	if got := len(reedengine.ServerName(hub)); got > maxKeyBytes {
		t.Errorf("reedengine.ServerName of an over-long hub path is %d bytes, over the kit's %d-byte key bound", got, maxKeyBytes)
	}
}

func TestSharedTempDirs_AreTheRootAndTheSystemTempDirectoryAboveIt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	system := t.TempDir()
	root := filepath.Join(system, "lym-1000")
	got := sharedTempDirs(filepath.Join(root, "rabcd"))
	if len(got) != 2 || got[0] != root || got[1] != system {
		t.Errorf("sharedTempDirs(%q) = %q; want [%q %q]", filepath.Join(root, "rabcd"), got, root, system)
	}
}

func TestCheckNewGitEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// seed creates the `.git` entry at path; a nil seed creates none.
		seed        func(t *testing.T, path string)
		preexisting bool
		// inFirst seeds the first watched directory instead of the second.
		inFirst bool
		// skipWindows skips the row where creating its entry may need a privilege.
		skipWindows bool
		wantCode    int
		wantOutput  []string
		wantKept    bool
	}{
		{name: "NoEntryKeepsTheCode", wantCode: 0},
		{
			name:        "EntryPresentBeforeTheRunKeepsTheCodeAndTheEntry",
			seed:        mkdirGit,
			preexisting: true,
			wantCode:    0,
			wantKept:    true,
		},
		{
			name:       "NewEmptyDirectoryFailsAndIsRemoved",
			seed:       mkdirGit,
			wantCode:   1,
			wantOutput: []string{"appeared during the run", "removed the empty directory"},
		},
		{
			name:       "NewEmptyDirectoryInFirstDirectoryFailsAndIsRemoved",
			seed:       mkdirGit,
			inFirst:    true,
			wantCode:   1,
			wantOutput: []string{"appeared during the run", "removed the empty directory"},
		},
		{
			name: "NewNonEmptyDirectoryFailsAndIsKept",
			seed: func(t *testing.T, path string) {
				mkdirGit(t, path)
				if err := os.WriteFile(filepath.Join(path, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantCode:   1,
			wantOutput: []string{"appeared during the run", "is not an empty directory", "remove it by hand"},
			wantKept:   true,
		},
		{
			name: "NewGitfileFailsAndIsKept",
			seed: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantCode:   1,
			wantOutput: []string{"appeared during the run", "is not a directory", "remove it by hand"},
			wantKept:   true,
		},
		{
			name: "NewSymlinkFailsAndIsKept",
			seed: func(t *testing.T, path string) {
				if err := os.Symlink(filepath.Join(t.TempDir(), "target"), path); err != nil {
					t.Fatal(err)
				}
			},
			skipWindows: true,
			wantCode:    1,
			wantOutput:  []string{"appeared during the run", "is not a directory", "remove it by hand"},
			wantKept:    true,
		},
	}
	for _, tt := range tests {
		if tt.skipWindows && runtime.GOOS == "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := []string{t.TempDir(), t.TempDir()}
			dir := dirs[1]
			if tt.inFirst {
				dir = dirs[0]
			}
			path := filepath.Join(dir, gitEntryName)
			if tt.seed != nil && tt.preexisting {
				tt.seed(t, path)
			}
			before := presentGitEntries(dirs)
			if tt.seed != nil && !tt.preexisting {
				tt.seed(t, path)
			}

			var out bytes.Buffer
			got := checkNewGitEntries(&out, dirs, before, 0)

			if got != tt.wantCode {
				t.Errorf("checkNewGitEntries code = %d; want %d", got, tt.wantCode)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(out.String(), want) {
					t.Errorf("checkNewGitEntries output = %q; want it to hold %q", out.String(), want)
				}
			}
			if len(tt.wantOutput) == 0 && out.Len() != 0 {
				t.Errorf("checkNewGitEntries output = %q; want none", out.String())
			}
			if len(tt.wantOutput) != 0 && !strings.Contains(out.String(), path) {
				t.Errorf("checkNewGitEntries output = %q; want it to name %s", out.String(), path)
			}
			_, err := os.Lstat(path)
			if kept := err == nil; kept != tt.wantKept {
				t.Errorf("%s kept = %v; want %v", path, kept, tt.wantKept)
			}
		})
	}
}

func TestRunCheckingGitEntries(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// preexisting creates the `.git` before the run starts.
		preexisting bool
		// duringRun makes the fake run create the `.git`.
		duringRun bool
		runCode   int
		wantCode  int
	}{
		{name: "RunLeavingNoEntryKeepsTheCode", runCode: 3, wantCode: 3},
		{name: "RunLeavingNewEntryFails", duringRun: true, wantCode: 1},
		{name: "RunLeavingNewEntryKeepsAFailingCodeFailing", duringRun: true, runCode: 2, wantCode: 1},
		{name: "EntryPresentBeforeTheRunKeepsTheCode", preexisting: true, runCode: 0, wantCode: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := []string{t.TempDir(), t.TempDir()}
			path := filepath.Join(dirs[0], gitEntryName)
			if tt.preexisting {
				mkdirGit(t, path)
			}
			run := func() int {
				if tt.duringRun {
					mkdirGit(t, path)
				}
				return tt.runCode
			}

			var out bytes.Buffer
			got := runCheckingGitEntries(&out, dirs, run)

			if got != tt.wantCode {
				t.Errorf("runCheckingGitEntries code = %d; want %d", got, tt.wantCode)
			}
			if named := strings.Contains(out.String(), path); named != tt.duringRun {
				t.Errorf("runCheckingGitEntries output = %q; names %s = %v, want %v", out.String(), path, named, tt.duringRun)
			}
		})
	}
}

// mkdirGit creates path as an empty directory.
func mkdirGit(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestAfterRun_ReportsFailedRemovalKeepingTheCode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for _, code := range []int{0, 1} {
		dir := t.TempDir()
		removeErr := errors.New("directory in use")

		var out bytes.Buffer
		got := afterRun(&out, dir, code, 100*time.Millisecond, func() {}, func() error { return removeErr })

		if got != code {
			t.Errorf("afterRun code = %d, want the input code %d unchanged", got, code)
		}
		if want := "tmuxkit: remove run directory " + dir + ": directory in use; the next start's sweep removes it\n"; out.String() != want {
			t.Errorf("afterRun output = %q, want %q", out.String(), want)
		}
	}
}
