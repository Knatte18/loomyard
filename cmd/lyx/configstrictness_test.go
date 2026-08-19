// configstrictness_test.go enforces the Config Strictness Invariant: a pinned set of
// module package directories call internal/configengine's degrading entry point,
// configengine.LoadOrTemplate, and a second pinned set calls its strict entry point,
// configengine.Load. Following cmd/lyx/gitrepoboundary_test.go's pinned-set style,
// this guard walks every non-test *.go file under the module root, collects the
// package directory of every configengine.LoadOrTemplate( call site and every
// configengine.Load( call site, and asserts each collected set equals its pinned set
// exactly in both directions. See CONSTRAINTS.md's Config Strictness Invariant.
//
// internal/configengine itself is excluded from both collected sets as the
// declaration site: its own Load and LoadOrTemplate function bodies never contain the
// qualified configengine.Load( / configengine.LoadOrTemplate( call form, since a
// package never qualifies a call to its own function with its own package name, but
// the exclusion is applied explicitly rather than relied upon structurally.
//
// The three own-loader modules -- internal/burlerengine, internal/modelspec, and
// internal/scoutengine -- call neither entry point at all: they resolve their config
// path with configengine.ConfigFile and read the file themselves with their own
// absent-file fallback. They are structurally invisible to this substring scan, so
// this guard makes no assertion about them one way or the other -- they are outside
// its subject by construction, not by exclusion.
//
// This guard's own file is skipped as a _test.go file, so the literal
// "configengine.Load(" and "configengine.LoadOrTemplate(" tokens it carries as scan
// data above are harmless -- the same self-reference every other pinned-set guard in
// this package documents about itself.
//
// # The one blind spot this guard cannot see
//
// A substring scan cannot see a call reached through an alias (a local name bound to
// configengine.Load or configengine.LoadOrTemplate) or a function value (the function
// passed around and invoked indirectly). Such a call would not contain the literal
// substring this guard matches on, and would pass undetected.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// configStrictnessDegradingSet is the pinned set of module-relative, slash-separated
// package directories that call configengine.LoadOrTemplate -- the degrading loading
// policy, where an absent _lyx/ directory or absent config file resolves the caller's
// embedded template instead of erroring. See CONSTRAINTS.md's Config Strictness
// Invariant membership rule: a module belongs here when it has, or is slated to have,
// a standalone entry point.
var configStrictnessDegradingSet = map[string]bool{
	"internal/shuttleengine": true,
	"internal/reedengine":    true,
	"internal/perchengine":   true,
	"internal/websterengine": true,
	"internal/batcher":       true,
}

// configStrictnessStrictSet is the pinned set of module-relative, slash-separated
// package directories that call configengine.Load -- the strict loading policy, where
// an absent _lyx/ directory or absent config file is an error. See CONSTRAINTS.md's
// Config Strictness Invariant: a module stays here when it only ever runs inside a
// hub, where an absent config means the hub is broken.
var configStrictnessStrictSet = map[string]bool{
	"internal/fabricengine": true,
	"internal/boardengine":  true,
	"internal/loomengine":   true,
}

// configStrictnessMinScannedFiles is the vacuous-scan floor for this guard's
// whole-module walk. The module has many hundreds of non-test .go files; fewer than
// 50 found means the walk or module-root resolution is misconfigured rather than the
// module having genuinely shrunk.
const configStrictnessMinScannedFiles = 50

// TestConfigStrictness_PinnedCallSiteSets walks every non-test *.go file under the
// module root and asserts that the set of package directories containing a
// configengine.LoadOrTemplate( call equals configStrictnessDegradingSet exactly, and
// that the set of package directories containing a configengine.Load( call equals
// configStrictnessStrictSet exactly -- in both directions, so a pinned-set member
// with no matching call anywhere fails just as loudly as an unpinned package that
// gained a call.
func TestConfigStrictness_PinnedCallSiteSets(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring
	// gitrepoboundary_test.go, crosscompile_test.go, and tierpurity_test.go.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	collectedDegrading := map[string]bool{}
	collectedStrict := map[string]bool{}
	var scanned int

	walkErr := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != moduleRoot && (strings.HasPrefix(d.Name(), ".") || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		scanned++

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		content := string(data)

		relDir, relErr := filepath.Rel(moduleRoot, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		relDir = filepath.ToSlash(relDir)
		if relDir == "internal/configengine" {
			return nil
		}

		if strings.Contains(content, "configengine.LoadOrTemplate(") {
			collectedDegrading[relDir] = true
		}
		if strings.Contains(content, "configengine.Load(") {
			collectedStrict[relDir] = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk module tree: %v", walkErr)
	}

	if scanned < configStrictnessMinScannedFiles {
		t.Fatalf("config strictness guard: only scanned %d non-test .go file(s) under %s; expected at least %d -- the walk may be misconfigured", scanned, moduleRoot, configStrictnessMinScannedFiles)
	}

	if diff := configStrictnessDiffSets(configStrictnessDegradingSet, collectedDegrading); diff != "" {
		t.Errorf("Config Strictness Invariant violated (see CONSTRAINTS.md): configengine.LoadOrTemplate( call-site package set drifted from the pinned degrading set:\n%s", diff)
	}
	if diff := configStrictnessDiffSets(configStrictnessStrictSet, collectedStrict); diff != "" {
		t.Errorf("Config Strictness Invariant violated (see CONSTRAINTS.md): configengine.Load( call-site package set drifted from the pinned strict set:\n%s", diff)
	}
}

// configStrictnessDiffSets returns a description of how got differs from want, or ""
// if identical. Distinct from gitrepoboundary_test.go's diffMethodSets, whose failure
// text is worded for that guard, so package main has no redeclaration conflict.
func configStrictnessDiffSets(want, got map[string]bool) string {
	var missing, unexpected []string
	for name := range want {
		if !got[name] {
			missing = append(missing, name)
		}
	}
	for name := range got {
		if !want[name] {
			unexpected = append(unexpected, name)
		}
	}
	if len(missing) == 0 && len(unexpected) == 0 {
		return ""
	}
	sort.Strings(missing)
	sort.Strings(unexpected)

	var b strings.Builder
	if len(missing) > 0 {
		b.WriteString("  pinned but no matching call found: " + strings.Join(missing, ", ") + "\n")
	}
	if len(unexpected) > 0 {
		b.WriteString("  matching call found but not pinned: " + strings.Join(unexpected, ", ") + "\n")
	}
	return b.String()
}
