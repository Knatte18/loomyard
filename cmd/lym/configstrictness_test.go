// configstrictness_test.go enforces the Config Strictness Invariant.
// A pinned set of module package directories calls configengine.LoadOrTemplate, the degrading entry point.
// A second pinned set calls configengine.Load, the strict entry point.
// The guard walks every non-test *.go file under the module root.
// It collects the package directory of every call site of configengine.LoadOrTemplate( and of configengine.Load(.
// It asserts that each collected set equals its pinned set exactly, in both directions.
// See `PATTERN-config-layers`.
//
// internal/configengine is excluded from both collected sets as the declaration site.
// Its own Load and LoadOrTemplate bodies never contain the qualified call forms.
// A package never qualifies a call to its own function with its own package name,
// but the exclusion is explicit and does not rely on that naming rule.
//
// internal/modelspec, the own-loader module, calls neither entry point.
// It reads its layers through configengine.StackLayers and decodes them itself, with its own absent-file fallback.
// The substring scan cannot see it, so this guard makes no assertion about it either way.
// It is outside the guard's subject by construction, not by exclusion.
//
// The scan skips _test.go files, so the literal call tokens in this file's comments are harmless.
//
// # The one blind spot this guard cannot see
//
// A substring scan cannot see a call reached through an alias or a function value.
// An alias is a local name bound to configengine.Load or configengine.LoadOrTemplate.
// A function value is one passed around and invoked indirectly.
// Such a call does not contain the literal substring this guard matches on, so it passes undetected.
package main

import (
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// configStrictnessDegradingSet is the pinned set of package directories that call configengine.LoadOrTemplate.
// Each entry is a module-relative, slash-separated path.
// That is the degrading loading policy:
// an absent _lym/ directory or absent config file resolves the caller's embedded template instead of erroring.
// See `PATTERN-config-layers` for the membership rule:
// a module belongs here when it has, or is slated to have, a standalone entry point.
var configStrictnessDegradingSet = map[string]bool{
	"internal/shuttleengine":              true,
	"internal/shuttleengine/claudeengine": true,
	"internal/shuttleengine/codexengine":  true,
	"internal/reedengine":                 true,
	"internal/websterengine":              true,
	"internal/batcher":                    true,
	"internal/orchengine":                 true,
	"internal/loggerconfig":               true,
	"internal/battenshed":                 true,
	"internal/sheddriver":                 true,
	"internal/languageconfig":             true,
	"internal/agentconfig":                true,
	"internal/lenses":                     true,
	"internal/desktophook":                true,
	"internal/heddleengine":               true,
	"internal/hibernateconfig":            true,
	"internal/envconfig":                  true,
	"internal/ideconfig":                  true,
	"internal/reportconfig":               true,
	"internal/creelconfig":                true,
	"internal/slackengine":                true,
}

// configStrictnessStrictSet is the pinned set of package directories that call configengine.Load.
// That is the strict loading policy:
// an absent _lym/ directory or absent config file is an error.
// See `PATTERN-config-layers`:
// a module stays here when it only ever runs inside a hub, where an absent config means the hub is broken.
var configStrictnessStrictSet = map[string]bool{
	"internal/fabricengine":  true,
	"internal/boardengine":   true,
	"internal/loomengine":    true,
	"internal/landingshed":   true,
	"internal/darnengine":    true,
	"internal/driftengine":   true,
	"internal/samplerengine": true,
	"internal/dobbyengine":   true,
	"internal/gateslot":      true,
	"internal/configcli":     true,
	"internal/configreg":     true,
}

// configStrictnessMinScannedFiles is the vacuous-scan floor for this guard's whole-module walk.
// The module has far more non-test .go files than the floor.
// A scan that finds fewer than the floor means the walk or module-root resolution is misconfigured.
// It does not mean the module has shrunk.
const configStrictnessMinScannedFiles = 50

// TestConfigStrictness_PinnedCallSiteSets walks every non-test *.go file under the module root.
// It asserts that the directories with a configengine.LoadOrTemplate( call equal configStrictnessDegradingSet exactly.
// It asserts that the directories with a configengine.Load( call equal configStrictnessStrictSet exactly.
// The check runs in both directions,
// so a pinned member with no matching call fails as loudly as an unpinned package that gained a call.
//
//lym:guard
func TestConfigStrictness_PinnedCallSiteSets(t *testing.T) {
	tiergate.Require(t, 0)
	collectedDegrading := map[string]bool{}
	collectedStrict := map[string]bool{}

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		relDir := path.Dir(f.Rel)
		if relDir == "internal/configengine" {
			return
		}
		content := string(f.Data)
		if strings.Contains(content, "configengine.LoadOrTemplate(") {
			collectedDegrading[relDir] = true
		}
		if strings.Contains(content, "configengine.Load(") {
			collectedStrict[relDir] = true
		}
	})

	scankit.RequireFloor(t, scanned, configStrictnessMinScannedFiles, "config strictness guard")

	if diff := configStrictnessDiffSets(configStrictnessDegradingSet, collectedDegrading); diff != "" {
		t.Errorf("`PATTERN-config-layers` violated: configengine.LoadOrTemplate( call-site package set drifted from the pinned degrading set:\n%s", diff)
	}
	if diff := configStrictnessDiffSets(configStrictnessStrictSet, collectedStrict); diff != "" {
		t.Errorf("`PATTERN-config-layers` violated: configengine.Load( call-site package set drifted from the pinned strict set:\n%s", diff)
	}
}

// configStrictnessDiffSets returns a description of how got differs from want, or "" if identical.
// Its failure text is worded for this guard, and its name is unique within package main.
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
