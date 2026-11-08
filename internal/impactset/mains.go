// mains.go lists the `package main` packages of a module, which a scope adds when it must reach the module's programs.

package impactset

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// mainPackagesFormat is the `go list -f` template for one NUL-terminated record per package.
// Each record is the package directory, a tab, the package name, a tab, and the list error, if any.
const mainPackagesFormat = `{{.Dir}}{{"\t"}}{{.Name}}{{"\t"}}{{with .Error}}{{.Err}}{{end}}{{"\x00"}}`

// ModuleMainPackages returns the `package main` packages of the module at moduleDir, sorted.
// Each is a module-relative `go` package argument, such as `./cmd/tool`, or `.` for the module root.
// It lists the packages with `go list -e ./...`, so a package that fails to build stays listed under its name.
// A failed `go list` run is an error naming moduleDir and the enumeration's own error.
// So are an entry with no directory and a listed directory outside moduleDir.
// ctx bounds the run, so a stalled `go list` is killed when it ends.
func ModuleMainPackages(ctx context.Context, moduleDir string) ([]string, error) {
	absolute, err := filepath.Abs(moduleDir)
	if err != nil {
		return nil, fmt.Errorf("impactset: resolve %s: %w", moduleDir, err)
	}
	cmd := exec.CommandContext(ctx, "go", "list", "-e", "-f", mainPackagesFormat, "./...")
	cmd.Dir = moduleDir
	logger.Info("impactset: spawning go list for the main packages", "dir", moduleDir)
	output, err := cmd.Output()
	logger.Info("impactset: go list for the main packages exited", "dir", moduleDir, "err", err)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("impactset: list the main packages of %s: %w: %s", moduleDir, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("impactset: list the main packages of %s: %w", moduleDir, err)
	}

	var mains []string
	for _, record := range strings.Split(string(output), "\x00") {
		record = strings.TrimPrefix(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		for len(fields) < 3 {
			fields = append(fields, "")
		}
		dir, name, listError := fields[0], fields[1], fields[2]
		if dir == "" {
			return nil, fmt.Errorf("impactset: list the main packages of %s: %s", moduleDir, strings.TrimSpace(listError))
		}
		if name != "main" {
			continue
		}
		relative, err := filepath.Rel(absolute, dir)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("impactset: list the main packages of %s: package directory %s lies outside the module directory", absolute, dir)
		}
		target := "."
		if relative != "." {
			target = "./" + filepath.ToSlash(relative)
		}
		mains = append(mains, target)
	}
	sort.Strings(mains)
	return mains, nil
}
