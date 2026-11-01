//go:build tier0

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
)

// records is the name of the records directory beside the fixture's prime directory, repo.
var records = recordsDirName("repo")

// writeHub lays out a hub as boltrename expects it, with a reed command, a VS Code setting and a workspace folder naming _board, and returns its path.
func writeHub(t *testing.T) string {
	t.Helper()
	hubPath := filepath.Join(t.TempDir(), "repo-LYMHUB")
	files := map[string]string{
		"_board/.git":                             "gitdir: " + filepath.Join(hubPath, records, ".git/worktrees/_board") + "\n",
		"_board/.lym/runproc/_board/12-34.json":   "{}",
		records + "/.git/worktrees/_board/gitdir": filepath.Join(hubPath, "_board/.git") + "\n",
		records + "/.lym/reed.json":               `{"cmd":"lym board --board-path ` + filepath.Join(hubPath, "_board") + ` sync"}`,
		"repo/.vscode/settings.json":              `{"files.exclude":{"` + filepath.Join(hubPath, "_board/.lym") + `":true}}`,
		"_launchers/ide-menu.sh":                  "cd ../repo && lym ide menu\n",
		"repo.code-workspace":                     `{"folders":[{"name":"Bolt","path":"_board"},{"name":"custom_board","path":"repo"}]}`,
	}
	for rel, content := range files {
		path := filepath.Join(hubPath, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return hubPath
}

// TestRenameBolt_MovesTheCheckoutAndRepointsItsPaths covers a hub as it stands before the rename.
func TestRenameBolt_MovesTheCheckoutAndRepointsItsPaths(t *testing.T) {
	t.Parallel()
	hubPath := writeHub(t)
	bolt := lymcwd.BoltDirName

	h, err := inspectHub(hubPath)
	if err != nil {
		t.Fatalf("inspectHub() error = %v", err)
	}
	repointed, err := renameBolt(h)
	if err != nil {
		t.Fatalf("renameBolt() error = %v", err)
	}

	wantRepointed := []string{
		filepath.Join(hubPath, "repo.code-workspace"),
		filepath.Join(hubPath, "repo/.vscode/settings.json"),
		filepath.Join(hubPath, records, ".lym/reed.json"),
	}
	if !slices.Equal(repointed, wantRepointed) {
		t.Errorf("renameBolt() re-pointed %v; want %v", repointed, wantRepointed)
	}
	for _, gone := range []string{"_board", records + "/.git/worktrees/_board"} {
		if _, err := os.Stat(filepath.Join(hubPath, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still present: %v", gone, err)
		}
	}
	wantFiles := map[string]string{
		bolt + "/.git": "gitdir: " + filepath.Join(hubPath, records, ".git/worktrees", bolt) + "\n",
		bolt + "/.lym/runproc/" + bolt + "/12-34.json":  "{}",
		records + "/.git/worktrees/" + bolt + "/gitdir": filepath.Join(hubPath, bolt, ".git") + "\n",
		records + "/.lym/reed.json":                     `{"cmd":"lym board --board-path ` + filepath.Join(hubPath, bolt) + ` sync"}`,
		"repo/.vscode/settings.json":                    `{"files.exclude":{"` + filepath.Join(hubPath, bolt, ".lym") + `":true}}`,
		"_launchers/ide-menu.sh":                        "cd ../repo && lym ide menu\n",
		"repo.code-workspace":                           `{"folders":[{"name":"Bolt","path":"` + bolt + `"},{"name":"custom_board","path":"repo"}]}`,
	}
	for rel, want := range wantFiles {
		data, err := os.ReadFile(filepath.Join(hubPath, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		if got := string(data); got != want {
			t.Errorf("%s = %q; want %q", rel, got, want)
		}
	}
}

// TestInspectHub_StopsOnWhatItDoesNotExpect covers each departure from the expected hub.
func TestInspectHub_StopsOnWhatItDoesNotExpect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		alter    func(hubPath string) error
		wantPart string
	}{
		{"PairStillInTheHub", func(hubPath string) error {
			return os.Mkdir(filepath.Join(hubPath, "some-task"), 0o755)
		}, "some-task"},
		{"NewNameAlreadyPresent", func(hubPath string) error {
			return os.Mkdir(filepath.Join(hubPath, lymcwd.BoltDirName), 0o755)
		}, ": " + lymcwd.BoltDirName},
		{"AdminDirPointsElsewhere", func(hubPath string) error {
			return os.WriteFile(filepath.Join(hubPath, records, ".git/worktrees/_board/gitdir"), []byte("/elsewhere/.git\n"), 0o644)
		}, `reads "/elsewhere/.git"`},
		{"NoBoltCheckout", func(hubPath string) error {
			return os.RemoveAll(filepath.Join(hubPath, "_board"))
		}, "no _board directory"},
		{"StrayFileInTheHub", func(hubPath string) error {
			return os.WriteFile(filepath.Join(hubPath, "notes.txt"), []byte("x"), 0o644)
		}, "notes.txt"},
		{"NoPrimeWithRecords", func(hubPath string) error {
			return os.RemoveAll(filepath.Join(hubPath, "repo"))
		}, "no prime with its"},
		{"CheckoutGitLinkPointsElsewhere", func(hubPath string) error {
			return os.WriteFile(filepath.Join(hubPath, "_board/.git"), []byte("gitdir: /elsewhere\n"), 0o644)
		}, `reads "gitdir: /elsewhere"`},
		{"NewAdminDirAlreadyPresent", func(hubPath string) error {
			return os.Mkdir(filepath.Join(hubPath, records, ".git/worktrees", lymcwd.BoltDirName), 0o755)
		}, lymcwd.BoltDirName + " exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hubPath := writeHub(t)
			if err := tt.alter(hubPath); err != nil {
				t.Fatal(err)
			}
			if _, err := inspectHub(hubPath); err == nil || !strings.Contains(err.Error(), tt.wantPart) {
				t.Errorf("inspectHub() error = %v; want one naming %q", err, tt.wantPart)
			}
		})
	}
}

// TestLiveProcesses covers which processes keep the rename from running.
func TestLiveProcesses(t *testing.T) {
	t.Parallel()
	hubPath := "/code/repo-LYMHUB"
	absPath := "/link/repo-LYMHUB"
	processes := []proc.Process{
		{PID: 900000001, Exe: "/home/u/go/bin/lym (deleted)", Cwd: "/", Argv: []string{"lym", "dobby"}},
		{PID: 900000002, Exe: "/usr/bin/claude", Cwd: "/code/repo-LYMHUB/repo", Argv: []string{"claude"}},
		{PID: 900000003, Exe: "/usr/bin/tmux", Cwd: "/", Argv: []string{"tmux", "-c", "/code/repo-LYMHUB/_board"}},
		{PID: 900000004, Exe: "/usr/bin/bash", Cwd: "/code/repo-LYMHUB-old", Argv: []string{"bash"}},
		{PID: 900000005, Exe: "/usr/bin/code", Cwd: "/home/u", Argv: []string{"code"}},
		{PID: 900000006, Exe: "/usr/bin/vim", Cwd: "/", Argv: []string{"vim", "/link/repo-LYMHUB/repo.code-workspace"}},
		{PID: os.Getpid(), Exe: "/usr/bin/lym", Cwd: "/code/repo-LYMHUB", Argv: []string{"lym"}},
	}
	want := []string{
		"900000001 lym dobby",
		"900000002 claude",
		"900000003 tmux -c /code/repo-LYMHUB/_board",
		"900000006 vim /link/repo-LYMHUB/repo.code-workspace",
	}
	if got := liveProcesses(hubPath, absPath, processes); !slices.Equal(got, want) {
		t.Errorf("liveProcesses() = %v; want %v", got, want)
	}
}
