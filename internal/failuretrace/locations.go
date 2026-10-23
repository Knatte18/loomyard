// locations.go extracts the `path:line` locations a failed verify log names and places each relative to the repository root.

package failuretrace

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// maxPlacedLocations caps the placed locations one trace answers.
const maxPlacedLocations = 20

// goLocation matches one `<path>.go:<line>` token of Go test or build output, with an optional `:<col>` that the match leaves out.
var goLocation = regexp.MustCompile(`((?:[A-Za-z]:)?[^\s:()"'\[\]]+\.go):(\d+)`)

// Placement tells Locations where the verified tree sits and what it holds.
type Placement struct {
	// WorktreeRoot is the absolute root of the verified worktree.
	WorktreeRoot string
	// ModulePath is the module path from the worktree's go.mod, empty when unknown.
	ModulePath string
	// ExistsAtHead reports whether the slash-separated, root-relative path rel names a file at the verified head.
	// Nil reads as a tree holding no file.
	ExistsAtHead func(rel string) bool
}

// Locations extracts the `path:line` locations of every failure's Tail and places each relative to the repository root.
// placed holds the placed locations, deduplicated in first-seen order and capped at twenty.
// untraced holds the bare and `./`-relative locations of an opaque identity, as written, which no package directory places.
// A location that places nowhere, such as a standard-library or dependency frame, is dropped.
func Locations(failures []websterengine.VerifyFailure, p Placement) (placed, untraced []string) {
	seenPlaced := map[string]bool{}
	seenUntraced := map[string]bool{}
	for _, failure := range failures {
		for _, match := range goLocation.FindAllStringSubmatch(failure.Tail, -1) {
			rel, outcome := p.place(match[1], failure.Package)
			switch outcome {
			case placedOutcome:
				location := rel + ":" + match[2]
				if !seenPlaced[location] && len(placed) < maxPlacedLocations {
					seenPlaced[location] = true
					placed = append(placed, location)
				}
			case untracedOutcome:
				location := match[1] + ":" + match[2]
				if !seenUntraced[location] {
					seenUntraced[location] = true
					untraced = append(untraced, location)
				}
			}
		}
	}
	return placed, untraced
}

// placeOutcome is what placing one location comes to.
type placeOutcome int

const (
	droppedOutcome placeOutcome = iota
	placedOutcome
	untracedOutcome
)

// place maps raw, a path as a log wrote it, to a repository-relative path.
// pkg is the import path of the failure identity the path came from, empty for an opaque identity.
func (p Placement) place(raw, pkg string) (string, placeOutcome) {
	slashed := filepath.ToSlash(raw)
	absolute := path.IsAbs(slashed) || filepath.IsAbs(raw) || (len(slashed) > 1 && slashed[1] == ':')
	relativeToPackage := !strings.Contains(slashed, "/") || strings.HasPrefix(slashed, "./")

	if !absolute && !relativeToPackage && p.ExistsAtHead != nil && p.ExistsAtHead(slashed) {
		return slashed, placedOutcome
	}
	if root := strings.TrimSuffix(filepath.ToSlash(p.WorktreeRoot), "/"); root != "" {
		if rel, ok := strings.CutPrefix(slashed, root+"/"); ok {
			return rel, placedOutcome
		}
	}
	if p.ModulePath != "" {
		if rel, ok := strings.CutPrefix(slashed, p.ModulePath+"/"); ok {
			return rel, placedOutcome
		}
	}
	if !relativeToPackage {
		return "", droppedOutcome
	}
	dir, ok := p.packageDir(pkg)
	if !ok {
		return "", untracedOutcome
	}
	return path.Join(dir, strings.TrimPrefix(slashed, "./")), placedOutcome
}

// packageDir maps the import path pkg to its directory relative to the module root, "." for the module's root package.
// It reports false for an empty pkg, an unknown module path, or a pkg outside the module.
func (p Placement) packageDir(pkg string) (string, bool) {
	if p.ModulePath == "" || pkg == "" {
		return "", false
	}
	if pkg == p.ModulePath {
		return ".", true
	}
	rel, ok := strings.CutPrefix(pkg, p.ModulePath+"/")
	return rel, ok && rel != ""
}
