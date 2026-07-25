// Command deploy builds lyx and installs it into a directory on PATH.
//
// It is a dev/build tool, not part of the lyx product surface. It is general: the
// install directory comes from -dest, falling back to the Go bin dir, or -dev to
// target the derived .dev-bin directory (see tools/internal/devbin). Machine-
// specific paths belong in the caller (e.g. deploy.cmd), not here.
//
//	go run ./tools/deploy                 # install into `go env GOBIN` (or GOPATH/bin)
//	go run ./tools/deploy -dest D:\bin     # install into a chosen directory
//	go run ./tools/deploy -dev             # install into <repoRoot>/.dev-bin
//
// Run it from anywhere — it locates the module root itself.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/tools/internal/devbin"
)

func main() {
	dest := flag.String("dest", "", "destination directory for the lyx binary (default: `go env GOBIN`, else GOPATH/bin)")
	dev := flag.Bool("dev", false, "install into the derived .dev-bin directory instead of -dest / GOBIN (mutually exclusive with -dest)")
	flag.Parse()

	if err := run(*dev, *dest); err != nil {
		fmt.Fprintln(os.Stderr, "deploy:", err)
		os.Exit(1)
	}
}

func run(dev bool, destArg string) error {
	root, err := devbin.RepoRoot()
	if err != nil {
		return err
	}

	destDir, err := resolveDest(dev, destArg)
	if err != nil {
		return err
	}

	name := "lyx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(destDir, name)

	tag := gitTag(root)
	fmt.Printf("Building lyx @ %s -> %s\n", tag, dest)

	build := exec.Command("go", "build", "-o", dest, "./cmd/lyx")
	build.Dir = root
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("stat built binary: %w", err)
	}
	fmt.Printf("Deployed lyx @ %s  (%d KB)  %s\n", tag, info.Size()/1024, dest)

	if onPath(destDir) {
		fmt.Printf("  %s is on PATH - 'lyx' is globally available.\n", destDir)
	} else {
		fmt.Printf("  WARNING: %s is not on PATH - add it so 'lyx' resolves globally.\n", destDir)
	}
	return nil
}

// resolveDest picks the install directory for the built binary. -dev and a
// non-empty dest are mutually exclusive: passing both is almost certainly a
// mistake (the caller asked for two different destinations at once), so it
// is rejected rather than silently preferring one. When dev is set, the
// derived .dev-bin directory (see devbin.Dir) is used; otherwise dest is
// used verbatim if non-empty, falling back to the Go bin dir.
func resolveDest(dev bool, dest string) (string, error) {
	if dev && dest != "" {
		return "", fmt.Errorf("-dev and -dest are mutually exclusive")
	}
	if dev {
		return devbin.Dir()
	}
	if dest != "" {
		return dest, nil
	}
	return goBinDir()
}

// goBinDir returns the default install directory: `go env GOBIN`, or GOPATH/bin.
func goBinDir() (string, error) {
	if b := goEnv("GOBIN"); b != "" {
		return b, nil
	}
	gp := goEnv("GOPATH")
	if gp == "" {
		return "", fmt.Errorf("GOBIN and GOPATH both empty; pass -dest")
	}
	return filepath.Join(filepath.SplitList(gp)[0], "bin"), nil
}

func goEnv(name string) string {
	out, _ := exec.Command("go", "env", name).Output()
	return strings.TrimSpace(string(out))
}

// gitTag returns the short HEAD SHA, suffixed " (dirty)" when the tree is dirty.
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

// onPath reports whether dir is one of the PATH entries (case-insensitive,
// trailing-separator-insensitive).
func onPath(dir string) bool {
	want := strings.ToLower(strings.TrimRight(dir, `\/`))
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.ToLower(strings.TrimRight(p, `\/`)) == want {
			return true
		}
	}
	return false
}
