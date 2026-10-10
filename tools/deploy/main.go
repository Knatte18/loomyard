// Command deploy is the only route by which lyx and the loomyard plugins are installed.
//
// Production is the default and is what update-plugins.sh / update-plugins.cmd run: it puts the
// checked-out, committed code into production in one step. It refuses a dirty working tree or a
// HEAD that is not pushed to origin/main, mirrors every installed loomyard plugin's git-tracked
// files into the Claude Code plugin cache, builds lyx into the Go bin dir (`go env GOBIN`, else
// GOPATH/bin), and then moves the `prod` branch forward to the deployed commit. Nothing reaches
// production except through this step.
//
// -dev (deploy-dev / deploy-dev.cmd) builds the working tree as-is into <repoRoot>/.dev-bin (see
// tools/internal/devbin) for internal tests, and touches nothing else.
//
//	go run ./tools/deploy        # production: plugins + lyx into the Go bin dir
//	go run ./tools/deploy -dev   # dev: lyx into <repoRoot>/.dev-bin
//
// Run it from anywhere — it locates the module root itself.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	var head string
	if !dev {
		porcelain, err := gitOut(root, "status", "--porcelain")
		if err != nil {
			return fmt.Errorf("git status: %w", err)
		}
		if err := dirtyError(porcelain); err != nil {
			return err
		}
		if head, err = prodGate(root); err != nil {
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
	fmt.Printf("Building lyx @ %s -> %s\n", tag, dest)

	// Stamp internal/buildinfo.Channel so a dev-installed binary seeds but does not refresh an untouched stencil,
	// and only a production-stamped binary refreshes one.
	ldflags := "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=production"
	if dev {
		ldflags = "-X github.com/Knatte18/loomyard/internal/buildinfo.Channel=dev"
	}
	args := []string{"build", "-o", dest, "-ldflags", ldflags, "./cmd/lyx"}
	build := exec.Command("go", args...)
	build.Dir = root
	// lyx links quarry's tree-sitter grammars through cgo, so pin CGO_ENABLED=1
	// explicitly: an environment that has disabled cgo must fail here at the
	// compiler rather than produce a binary silently missing that linkage.
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("stat built binary: %w", err)
	}
	fmt.Printf("Deployed lyx @ %s  (%d KB)  %s\n", tag, info.Size()/1024, dest)

	if dev {
		return nil
	}
	// The prod pointer moves only after everything above succeeded, so it never names a commit
	// that did not actually reach production.
	if _, err := gitOut(root, "push", "origin", head+":refs/heads/"+prodBranch); err != nil {
		return fmt.Errorf("deployed, but moving %s to %s failed: %w", prodBranch, head, err)
	}
	fmt.Printf("Moved %s -> %s\n", prodBranch, head)

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	if !onPath(pathDirs, destDir) {
		fmt.Printf("  WARNING: %s is not on PATH - add it so 'lyx' resolves globally.\n", destDir)
	}
	for _, other := range otherBinaries(pathDirs, destDir, name, fileExists) {
		fmt.Printf("  WARNING: another %s on PATH: %s - remove it; production lyx lives only in %s.\n", name, other, destDir)
	}
	return nil
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "lyx.exe"
	}
	return "lyx"
}

// dirtyError refuses a production deploy from a working tree with any change git reports, since
// production must be exactly a commit.
func dirtyError(porcelain string) error {
	if strings.TrimSpace(porcelain) == "" {
		return nil
	}
	return fmt.Errorf("working tree is dirty; commit or stash before deploying to production:\n%s", porcelain)
}

// workBranch is where all work lands; prodBranch is the pointer a production deploy moves to the
// commit it deployed, so `git log -1 prod` always names what is in production.
const (
	workBranch = "main"
	prodBranch = "prod"
)

// prodGate fetches origin and returns HEAD's full SHA when HEAD may go to production: it must be
// pushed work (on origin's workBranch), and prodBranch, when it exists, must be its ancestor, so
// production history only ever moves forward.
func prodGate(root string) (string, error) {
	if _, err := gitOut(root, "fetch", "--quiet", "origin"); err != nil {
		return "", fmt.Errorf("git fetch origin: %w", err)
	}
	head, err := gitOut(root, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	onWork := isAncestor(root, head, "origin/"+workBranch)
	_, prodErr := gitOut(root, "rev-parse", "--verify", "--quiet", "origin/"+prodBranch)
	prodExists := prodErr == nil
	prodBehind := prodExists && isAncestor(root, "origin/"+prodBranch, head)
	return head, prodAdvanceError(head, onWork, prodExists, prodBehind)
}

// prodAdvanceError is prodGate's decision, separated from the git calls that feed it.
func prodAdvanceError(head string, onWork, prodExists, prodBehind bool) error {
	if !onWork {
		return fmt.Errorf("HEAD %s is not on origin/%s; push it to %s before deploying to production", head, workBranch, workBranch)
	}
	if prodExists && !prodBehind {
		return fmt.Errorf("origin/%s is not an ancestor of HEAD %s; production only moves forward -- revert on %s and deploy that instead", prodBranch, head, workBranch)
	}
	return nil
}

func isAncestor(root, ancestor, descendant string) bool {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", ancestor, descendant)
	cmd.Dir = root
	return cmd.Run() == nil
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

// otherBinaries returns every name on PATH outside destDir, in PATH order: a copy that shadows or
// is shadowed by the production binary is a stale install waiting to be run by mistake.
func otherBinaries(pathDirs []string, destDir, name string, exists func(string) bool) []string {
	var found []string
	seen := map[string]bool{normDir(destDir): true}
	for _, d := range pathDirs {
		key := normDir(d)
		if d == "" || seen[key] {
			continue
		}
		seen[key] = true
		if p := filepath.Join(d, name); exists(p) {
			found = append(found, p)
		}
	}
	return found
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
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
