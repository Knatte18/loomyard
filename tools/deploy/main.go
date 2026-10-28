// Command deploy is the only route by which lym and the loomyard plugins are installed.
//
// Production is the default and is what update-plugins.sh / update-plugins.cmd run: it puts the checked-out, committed code into production in one step.
// It refuses a dirty working tree or a HEAD that is not pushed to origin/main, mirrors every installed loomyard plugin's git-tracked files into the Claude Code plugin cache, and builds lym into the Go bin dir (`go env GOBIN`, else GOPATH/bin).
// Nothing reaches production except through this step.
// Every deploy installs lym by building it beside its target and renaming it over the target, so a process starting lym while it deploys never finds the file open for writing.
// No pointer on origin records the deployed commit: each machine deploys on its own, so a shared one could not say what any one machine runs.
//
// -dev (deploy-dev / deploy-dev.cmd) builds the working tree as-is into <repoRoot>/.dev-bin (see
// tools/internal/devbin) for internal tests, and touches nothing else.
//
//	go run ./tools/deploy        # production: plugins + lym into the Go bin dir
//	go run ./tools/deploy -dev   # dev: lym into <repoRoot>/.dev-bin
//
// Run it from anywhere — it locates the module root itself.
package main

import (
	"debug/buildinfo"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

func main() {
	dev := flag.Bool("dev", false, "build the working tree as-is into the derived .dev-bin directory instead of deploying to production")
	flag.Parse()

	if err := run(*dev); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(1)
	}
}

func run(dev bool) error {
	root, err := devbin.RepoRoot()
	if err != nil {
		return err
	}

	if !dev {
		porcelain, err := gitOut(root, "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("git status: %w", err)
		}
		if err := dirtyError(porcelain); err != nil {
			return err
		}
		if err := pushedGate(root); err != nil {
			return err
		}
		if err := syncPlugins(root); err != nil {
			return err
		}
	}

	destDir, err := resolveDest(dev)
	if err != nil {
		return err
	}

	name := binaryName()
	dest := filepath.Join(destDir, name)

	tag := gitTag(root)
	fmt.Printf("Building lym @ %s -> %s\n", tag, dest)

	// Stamp internal/buildinfo.Channel so a dev-installed binary seeds but does not refresh an untouched stencil, and only a production-stamped binary refreshes one.
	ldflags := "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=production"
	if dev {
		ldflags = "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"
	}
	err = installBinary(dest, func(out string) error {
		build := exec.Command("go", "build", "-o", out, "-ldflags", ldflags, "./cmd/lym")
		build.Dir = root
		// lym links quarry's tree-sitter grammars through cgo, so pin CGO_ENABLED=1
		// explicitly: an environment that has disabled cgo must fail here at the
		// compiler rather than produce a binary silently missing that linkage.
		build.Env = append(os.Environ(), "CGO_ENABLED=1")
		build.Stdout, build.Stderr = os.Stdout, os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("go build: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("stat built binary: %w", err)
	}
	fmt.Printf("Deployed lym @ %s  (%d KB)  %s\n", tag, info.Size()/1024, dest)

	if dev {
		return nil
	}

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	if !onPath(pathDirs, destDir) {
		fmt.Printf("  WARNING: %s is not on PATH - add it so 'lym' resolves globally.\n", destDir)
	}
	for _, other := range otherLymBinaries(pathDirs, destDir, name, lymMainPackage) {
		fmt.Printf("  WARNING: another %s on PATH: %s - remove it; production lym lives only in %s.\n", name, other, destDir)
	}
	return nil
}

// installBinary has build write the binary to a temporary file beside dest and renames it over dest, so a process starting or running the old binary never finds dest open for writing, which exec refuses with ETXTBSY.
// The rename stays on dest's filesystem, where it replaces the directory entry at once and a running process keeps the old file.
// A failed build or rename removes the temporary file and leaves dest as it was.
func installBinary(dest string, build func(out string) error) error {
	tmp := filepath.Join(filepath.Dir(dest), "."+filepath.Base(dest)+".deploy-"+strconv.Itoa(os.Getpid()))
	if err := build(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install the built binary over %s: %w", dest, err)
	}
	return nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "lym.exe"
	}
	return "lym"
}

// dirtyError refuses a production deploy from a working tree with any change git reports, since
// production must be exactly a commit.
func dirtyError(porcelain string) error {
	if strings.TrimSpace(porcelain) == "" {
		return nil
	}
	return fmt.Errorf("working tree is dirty; commit or stash before deploying to production:\n%s", porcelain)
}

// workBranch is where all work lands, and the only branch production deploys from.
const workBranch = "main"

// lymMainPackage is the main package path lym's Go build info carries, which tells a copy of lym on PATH from a foreign program of the same name.
const lymMainPackage = "github.com/Knatte18/loomyard/cmd/lym"

// pushedGate fetches origin and refuses a HEAD that is not on origin's workBranch, so production only ever runs pushed work.
func pushedGate(root string) error {
	if _, err := gitOut(root, "fetch", "--quiet", "origin"); err != nil {
		return fmt.Errorf("git fetch origin: %w", err)
	}
	head, err := gitOut(root, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	isAncestor := exec.Command("git", "merge-base", "--is-ancestor", head, "origin/"+workBranch)
	isAncestor.Dir = root
	if isAncestor.Run() != nil {
		return fmt.Errorf("HEAD %s is not on origin/%s; push it to %s before deploying to production", head, workBranch, workBranch)
	}
	return nil
}

// resolveDest picks the install directory: .dev-bin for dev, the Go bin dir for production.
func resolveDest(dev bool) (string, error) {
	if dev {
		return devbin.Dir()
	}
	return goBinDir()
}

// goBinDir returns the production install directory: GOBIN or GOPATH/bin.
func goBinDir() (string, error) {
	if b := goEnv("GOBIN"); b != "" {
		return b, nil
	}
	gp := goEnv("GOPATH")
	if gp == "" {
		return "", fmt.Errorf("GOBIN and GOPATH both empty; set one with `go env -w`")
	}
	return filepath.Join(filepath.SplitList(gp)[0], "bin"), nil
}

func goEnv(name string) string {
	out, _ := exec.Command("go", "env", name).Output()
	return strings.TrimSpace(string(out))
}

// marketplace is the subset of .claude-plugin/marketplace.json the plugin sync reads.
type marketplace struct {
	Name    string `json:"name"`
	Plugins []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Source  string `json:"source"`
	} `json:"plugins"`
}

func parseMarketplace(data []byte) (marketplace, error) {
	var m marketplace
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse marketplace.json: %w", err)
	}
	if m.Name == "" {
		return m, fmt.Errorf("marketplace.json has no name")
	}
	return m, nil
}

// syncPlugins mirrors each installed plugin's git-tracked files into its Claude Code cache
// directory, <home>/.claude/plugins/cache/<marketplace>/<plugin>/<version>. `/plugin update` is
// version-gated and skips an unchanged version, so this mirrors regardless of version. A plugin not
// yet installed is skipped with the install command to run.
func syncPlugins(root string) error {
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return err
	}
	m, err := parseMarketplace(data)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cacheBase := filepath.Join(home, ".claude", "plugins", "cache", m.Name)

	for _, p := range m.Plugins {
		target := filepath.Join(cacheBase, p.Name, p.Version)
		if !dirExists(target) {
			fmt.Printf("Skipped (not installed): %s@%s -- run '/plugin install %s@%s' first.\n", p.Name, m.Name, p.Name, m.Name)
			continue
		}
		sourceRel := filepath.ToSlash(filepath.Clean(p.Source))
		listing, err := gitOut(root, "ls-files", "-z", "--", sourceRel)
		if err != nil {
			return fmt.Errorf("git ls-files %s: %w", sourceRel, err)
		}
		files := strings.FieldsFunc(listing, func(r rune) bool { return r == 0 })
		if err := mirrorFiles(root, sourceRel, files, target); err != nil {
			return fmt.Errorf("sync %s: %w", p.Name, err)
		}
		fmt.Printf("Synced plugin: %s@%s (%s)\n", p.Name, m.Name, p.Version)
	}
	return nil
}

// mirrorFiles replaces target with exactly files, each a repo-relative slash path under sourceRel.
// Clearing target first also removes anything built inside the cache (a plugin's build-on-first-run
// binary), forcing a rebuild from the new source instead of running old code.
func mirrorFiles(root, sourceRel string, files []string, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	prefix := strings.TrimSuffix(sourceRel, "/") + "/"
	for _, f := range files {
		rel, ok := strings.CutPrefix(f, prefix)
		if !ok {
			continue
		}
		if err := copyEntry(filepath.Join(root, filepath.FromSlash(f)), filepath.Join(target, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	return nil
}

func copyEntry(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(link, dst)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// otherLymBinaries returns every file called name on PATH outside destDir whose Go build info names mainPackage, in PATH order:
// a copy that shadows or is shadowed by the production binary is a stale install waiting to be run by mistake.
// A foreign program of the same name is skipped,
// and a file reached through a linked directory is reported once, under the first PATH entry that reaches it, or not at all when it is the production binary itself.
func otherLymBinaries(pathDirs []string, destDir, name, mainPackage string) []string {
	var found []string
	seenDirs := map[string]bool{normDir(destDir): true}
	seenFiles := map[string]bool{}
	if resolved, err := filepath.EvalSymlinks(filepath.Join(destDir, name)); err == nil {
		seenFiles[resolved] = true
	}
	for _, d := range pathDirs {
		key := normDir(d)
		if d == "" || seenDirs[key] {
			continue
		}
		seenDirs[key] = true
		p := filepath.Join(d, name)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil || seenFiles[resolved] {
			continue
		}
		seenFiles[resolved] = true
		if bi, err := buildinfo.ReadFile(p); err == nil && bi.Path == mainPackage {
			found = append(found, p)
		}
	}
	return found
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// gitTag returns the short HEAD SHA, suffixed " (dirty)" if tree is dirty.
func gitTag(root string) string {
	sha, err := gitOut(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "unknown"
	}
	if st, _ := gitOut(root, "status", "--porcelain"); st != "" {
		sha += " (dirty)"
	}
	return sha
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// normDir folds case and trailing separators so PATH entries compare the way the OS resolves them.
func normDir(dir string) string {
	return strings.ToLower(strings.TrimRight(dir, `\/`))
}

// onPath reports whether dir is among pathDirs.
func onPath(pathDirs []string, dir string) bool {
	want := normDir(dir)
	for _, p := range pathDirs {
		if normDir(p) == want {
			return true
		}
	}
	return false
}
