// Command boltrename renames a hub's Bolt checkout directory from _board to lymcwd.BoltDirName, once per hub, after a lym binary that knows only the new name is installed.
// It refuses while a lym process, or any process working in the hub, is alive.
// It stops and names what it found when the hub holds anything but a prime, its weft, _launchers, the Bolt checkout and workspace files, or when the checkout's git worktree links are not the ones it expects.
// It renames the checkout and its git worktree admin directory, rewrites the two gitdir links between them, moves the hub's own process records, and re-points every _board path in the hub's workspace files, the prime's .vscode files and the weft's reed.json.
// It never runs git.
//
//	go run ./tools/boltrename -hub <hub dir>
//
// Delete it once the hubs on Neve and OSL-1046 are renamed.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// oldName is the Bolt checkout's directory name before the rename.
const oldName = "_board"

// oldToken matches _board where no letter or digit precedes it, capturing the character before it.
var oldToken = regexp.MustCompile(`(^|[^A-Za-z0-9])` + oldName)

func main() {
	hubFlag := flag.String("hub", "", "the hub directory, such as ~/Code/loomyard-LYMHUB")
	flag.Parse()
	if *hubFlag == "" {
		fail(errors.New("-hub is required"))
	}
	absPath, err := filepath.Abs(*hubFlag)
	if err != nil {
		fail(err)
	}
	hubPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		fail(err)
	}

	processes, err := proc.ListProcesses()
	if err != nil {
		fail(err)
	}
	if live := liveProcesses(hubPath, absPath, processes); len(live) > 0 {
		fail(fmt.Errorf("these processes are alive; end every lym process, session and editor working in the hub, then run again:\n%s", strings.Join(live, "\n")))
	}

	h, err := inspectHub(hubPath)
	if err != nil {
		fail(err)
	}
	repointed, err := renameBolt(h)
	if err != nil {
		fail(err)
	}
	fmt.Printf("renamed %s to %s\n", filepath.Join(hubPath, oldName), filepath.Join(hubPath, lymcwd.BoltDirName))
	for _, path := range repointed {
		fmt.Printf("re-pointed: %s\n", path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "boltrename:", err)
	os.Exit(1)
}

// liveProcesses returns a line for each process, other than this one and its ancestors, that runs a lym binary, works in the hub, or names the hub on its command line.
// hubPath is the hub's symlink-resolved path, which a process's working directory is compared with;
// absPath is the path as the operator gave it, which a command line may name instead.
func liveProcesses(hubPath, absPath string, processes []proc.Process) []string {
	var live []string
	for _, p := range processes {
		if p.PID == os.Getpid() || proc.IsAncestor(p.PID) {
			continue
		}
		exe := strings.TrimSuffix(p.Exe, " (deleted)")
		commandLine := strings.Join(p.Argv, " ")
		if filepath.Base(exe) == "lym" || within(hubPath, p.Cwd) || strings.Contains(commandLine, hubPath) || strings.Contains(commandLine, absPath) {
			live = append(live, fmt.Sprintf("%d %s", p.PID, commandLine))
		}
	}
	return live
}

// within reports whether path is dir or lies under it.
func within(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// recordsDirName returns the name of the weft directory beside the prime directory named prime.
func recordsDirName(prime string) string {
	return prime + weftname.Suffix
}

// hub is the layout boltrename expects and acts on.
type hub struct {
	path  string
	prime string
	weft  string
}

// inspectHub reads hubPath's layout and the Bolt checkout's git worktree links, and returns an error naming every entry or link that differs from what boltrename expects.
func inspectHub(hubPath string) (hub, error) {
	entries, err := os.ReadDir(hubPath)
	if err != nil {
		return hub{}, err
	}
	h := hub{path: hubPath}
	var dirs []string
	var unexpected []string
	for _, entry := range entries {
		switch {
		case entry.IsDir():
			dirs = append(dirs, entry.Name())
		case entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".code-workspace"):
		default:
			unexpected = append(unexpected, entry.Name())
		}
	}
	for _, name := range dirs {
		if slices.Contains(dirs, recordsDirName(name)) {
			h.prime = filepath.Join(hubPath, name)
			h.weft = filepath.Join(hubPath, recordsDirName(name))
		}
	}
	for _, name := range dirs {
		path := filepath.Join(hubPath, name)
		if name != oldName && name != "_launchers" && path != h.prime && path != h.weft {
			unexpected = append(unexpected, name)
		}
	}
	if !slices.Contains(dirs, oldName) {
		unexpected = append(unexpected, "no "+oldName+" directory")
	}
	if h.weft == "" {
		unexpected = append(unexpected, "no prime with its weft")
	}
	if len(unexpected) > 0 {
		return h, fmt.Errorf("the hub %s holds what boltrename does not expect, so fix it by hand: %s", hubPath, strings.Join(unexpected, ", "))
	}

	checks := map[string]string{
		filepath.Join(hubPath, oldName, ".git"):                                               "gitdir: " + h.adminDir(oldName),
		filepath.Join(h.adminDir(oldName), "gitdir"):                                          filepath.Join(hubPath, oldName, ".git"),
		h.adminDir(lymcwd.BoltDirName):                                                        "",
		filepath.Join(hubPath, oldName, lymdirs.DotLymDirName, "runproc", lymcwd.BoltDirName): "",
	}
	for path, want := range checks {
		data, err := os.ReadFile(path)
		switch {
		case want == "" && errors.Is(err, os.ErrNotExist):
		case want == "":
			unexpected = append(unexpected, path+" exists")
		case err != nil:
			unexpected = append(unexpected, err.Error())
		case strings.TrimSpace(string(data)) != want:
			unexpected = append(unexpected, fmt.Sprintf("%s reads %q, not %q", path, strings.TrimSpace(string(data)), want))
		}
	}
	if len(unexpected) > 0 {
		slices.Sort(unexpected)
		return h, fmt.Errorf("the Bolt checkout's links are not as boltrename expects, so fix them by hand: %s", strings.Join(unexpected, "; "))
	}
	return h, nil
}

// adminDir returns the weft repository's git admin directory of the worktree named name.
func (h hub) adminDir(name string) string {
	return filepath.Join(h.weft, ".git", "worktrees", name)
}

// renameBolt moves the Bolt checkout, its git admin directory and the hub's process records to the new name, rewrites the gitdir links between checkout and admin directory, and re-points each _board path in the hub's workspace files, the prime's .vscode files and the weft's reed.json.
// It returns the paths of the files it re-pointed.
func renameBolt(h hub) ([]string, error) {
	bolt := filepath.Join(h.path, lymcwd.BoltDirName)
	if err := os.Rename(filepath.Join(h.path, oldName), bolt); err != nil {
		return nil, err
	}
	if err := os.Rename(h.adminDir(oldName), h.adminDir(lymcwd.BoltDirName)); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(bolt, ".git"), []byte("gitdir: "+h.adminDir(lymcwd.BoltDirName)+"\n"), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(h.adminDir(lymcwd.BoltDirName), "gitdir"), []byte(filepath.Join(bolt, ".git")+"\n"), 0o644); err != nil {
		return nil, err
	}
	runproc := filepath.Join(bolt, lymdirs.DotLymDirName, "runproc")
	if err := os.Rename(filepath.Join(runproc, oldName), filepath.Join(runproc, lymcwd.BoltDirName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	var candidates []string
	for _, pattern := range []string{
		filepath.Join(h.path, "*.code-workspace"),
		filepath.Join(h.prime, ".vscode", "*"),
		filepath.Join(h.weft, lymdirs.DotLymDirName, "reed.json"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, matches...)
	}
	var repointed []string
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil {
			return repointed, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return repointed, err
		}
		out := oldToken.ReplaceAllString(string(data), "${1}"+lymcwd.BoltDirName)
		if out == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(out), info.Mode().Perm()); err != nil {
			return repointed, err
		}
		repointed = append(repointed, path)
	}
	return repointed, nil
}
