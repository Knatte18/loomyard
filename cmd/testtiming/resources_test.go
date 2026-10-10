package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseKernelCounters(t *testing.T) {
	t.Parallel()

	t.Run("fork count", func(t *testing.T) {
		t.Parallel()
		stat := "cpu  1 2 3\nctxt 99\nprocesses 4242\nprocs_running 1\n"
		if got, ok := parseForkCount(stat); !ok || got != 4242 {
			t.Errorf("parseForkCount = %d, %v; want 4242, true", got, ok)
		}
		if _, ok := parseForkCount("cpu  1 2 3\n"); ok {
			t.Error("parseForkCount over text with no processes line reported ok")
		}
		if _, ok := parseForkCount("processes lots\n"); ok {
			t.Error("parseForkCount over a non-numeric count reported ok")
		}
	})

	t.Run("cpu stat", func(t *testing.T) {
		t.Parallel()
		got, err := parseCPUStat("usage_usec 1500000\nuser_usec 1000000\nsystem_usec 500000\n")
		if err != nil || got != 1500*time.Millisecond {
			t.Errorf("parseCPUStat = %v, %v; want 1.5s, nil", got, err)
		}
		if _, err := parseCPUStat("user_usec 1\n"); err == nil {
			t.Error("parseCPUStat over text with no usage_usec line returned no error")
		}
	})

	t.Run("scope cgroup path", func(t *testing.T) {
		t.Parallel()
		rows := []struct {
			name    string
			listing string
			want    string
		}{
			{"scope", "0::/user.slice/user-1000.slice/user@1000.service/app.slice/run-r1f.scope\n", "/user.slice/user-1000.slice/user@1000.service/app.slice/run-r1f.scope"},
			{"not yet moved", "0::/user.slice/user-1000.slice/session-3.scope\n", ""},
			{"empty", "", ""},
		}
		for _, row := range rows {
			if got := scopeCgroupPath(row.listing); got != row.want {
				t.Errorf("%s: scopeCgroupPath = %q; want %q", row.name, got, row.want)
			}
		}
	})
}

func TestReferencesDir(t *testing.T) {
	t.Parallel()

	const dir = "/tmp/testtiming-resources-abc"
	rows := []struct {
		name string
		cwd  string
		exe  string
		argv []string
		want bool
	}{
		{"cwd inside", dir + "/work", "/usr/bin/tmux", nil, true},
		{"exe inside", "/", dir + "/lyx", nil, true},
		{"argv element inside", "/", "/usr/bin/tmux", []string{"tmux", "-S", dir + "/sock"}, true},
		{"unrelated", "/", "/usr/bin/tmux", []string{"tmux", "-L", "other"}, false},
		{"empty directory matches nothing", "/", "/usr/bin/tmux", []string{"tmux"}, false},
	}
	for _, row := range rows {
		target := dir
		if strings.HasPrefix(row.name, "empty") {
			target = ""
		}
		if got := referencesDir(target, row.cwd, row.exe, row.argv); got != row.want {
			t.Errorf("%s: referencesDir = %v; want %v", row.name, got, row.want)
		}
	}
}

func TestRenderResources(t *testing.T) {
	t.Parallel()

	rows := []resourceRow{
		{pkg: testPkgPrefix + "internal/quick", wall: time.Second, cpu: 2 * time.Second, peakBytes: 64 << 20, procs: 10, procsKnown: true,
			census: gitCensus{fixture: 5, code: 7}},
		{pkg: testPkgPrefix + "internal/slow", wall: 3 * time.Second, cpu: 4 * time.Second, peakBytes: 128 << 20, rusage: true, failed: true,
			leftovers: []string{"4321 tmux -S /tmp/x/sock"}},
	}

	got := renderResources("Tags: tmux", rows, "0.50", "1.25", false)

	for _, want := range []string{
		"Resources  —  Tags: tmux",
		"PACKAGE", "WALL", "CPU", "PEAK_MEM", "PROCS", "LEFTOVER", "GIT_FIXTURE", "GIT_CODE",
		"internal/slow", "3.00s", "128.0M", "rusage  FAIL",
		"internal/quick", "64.0M",
		"TOTAL", "4.00s", "6.00s",
		"Leftover processes (pid argv):", "internal/slow: 4321 tmux -S /tmp/x/sock",
		"rusage",
		"0.50 at start, 1.25 at end",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render lacks %q:\n%s", want, got)
		}
	}
	if slow, quick := strings.Index(got, "internal/slow"), strings.Index(got, "internal/quick"); slow > quick {
		t.Errorf("rows are not ordered by wall time, slowest first:\n%s", got)
	}
	if cgroup := renderResources("Tags: tmux", rows, "0", "0", true); !strings.Contains(cgroup, "systemd scope cgroup") {
		t.Errorf("cgroup trailer missing:\n%s", cgroup)
	}
}
