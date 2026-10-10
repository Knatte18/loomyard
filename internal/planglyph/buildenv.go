// buildenv.go classifies an ambiguous resolve answer as a constraint-partitioned member or a
// genuine ambiguity.
// A member declared once per mutually exclusive build-constraint set (a `//go:build linux` file
// beside a `//go:build !linux` one) is one member to the compiler, because no build environment
// compiles two of its declarations.
// Cost bound: one read per candidate file, then in-memory matcher calls, at most the pinned pairs
// times two cgo values times sixteen tag assignments per candidate.

package planglyph

import (
	"bytes"
	"go/build"
	"go/build/constraint"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/quarry/quarry"
)

// buildPair is one GOOS and GOARCH pair.
type buildPair struct {
	GOOS   string
	GOARCH string
}

// pinnedBuildPairs is every pair `go tool dist list` prints today.
var pinnedBuildPairs = []buildPair{
	{"aix", "ppc64"},
	{"android", "386"}, {"android", "amd64"}, {"android", "arm"}, {"android", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"dragonfly", "amd64"},
	{"freebsd", "386"}, {"freebsd", "amd64"}, {"freebsd", "arm"}, {"freebsd", "arm64"},
	{"illumos", "amd64"},
	{"ios", "amd64"}, {"ios", "arm64"},
	{"js", "wasm"},
	{"linux", "386"}, {"linux", "amd64"}, {"linux", "arm"}, {"linux", "arm64"}, {"linux", "loong64"},
	{"linux", "mips"}, {"linux", "mips64"}, {"linux", "mips64le"}, {"linux", "mipsle"},
	{"linux", "ppc64"}, {"linux", "ppc64le"}, {"linux", "riscv64"}, {"linux", "s390x"},
	{"netbsd", "386"}, {"netbsd", "amd64"}, {"netbsd", "arm"}, {"netbsd", "arm64"},
	{"openbsd", "386"}, {"openbsd", "amd64"}, {"openbsd", "arm"}, {"openbsd", "arm64"},
	{"openbsd", "ppc64"}, {"openbsd", "riscv64"},
	{"plan9", "386"}, {"plan9", "amd64"}, {"plan9", "arm"},
	{"solaris", "amd64"},
	{"wasip1", "wasm"},
	{"windows", "386"}, {"windows", "amd64"}, {"windows", "arm64"},
}

// maxFreeTags caps the free tags one candidate set may name; the classifier evaluates every
// assignment of them, so a larger set is reported as not partitioned.
const maxFreeTags = 4

// noConstraint is the description of a candidate file carrying no build constraint.
const noConstraint = "no constraint"

// constrainedCandidate is an ambiguous answer's candidate with the text of its file's build constraint.
type constrainedCandidate struct {
	quarry.Symbol
	Constraint string
}

// ambiguity is the classification of one ambiguous answer.
// Partitioned reports that no single build environment selects two candidate files;
// Candidates holds every candidate in the answer's order, each described.
type ambiguity struct {
	Partitioned bool
	Candidates  []constrainedCandidate
}

// classifyAmbiguity reads each candidate's file once, under worktreeRoot, and reports the answer
// partitioned when no single build environment selects two candidate files.
// Fewer than two candidates, two candidates in one file, a candidate set naming more than
// maxFreeTags free tags, a file that cannot be read and a constraint line that cannot be parsed
// all yield not partitioned, with every candidate still described.
func classifyAmbiguity(worktreeRoot string, candidates []quarry.Symbol) ambiguity {
	result := ambiguity{Candidates: make([]constrainedCandidate, len(candidates))}
	sources := make(map[string][]byte, len(candidates))
	readable := true
	for i, candidate := range candidates {
		path := filepath.Clean(filepath.FromSlash(candidate.File))
		source, seen := sources[path]
		if !seen {
			var err error
			source, err = os.ReadFile(filepath.Join(worktreeRoot, path))
			if err != nil {
				readable = false
			}
			sources[path] = source
		}
		result.Candidates[i] = constrainedCandidate{
			Symbol:     candidate,
			Constraint: candidateConstraint(filepath.Base(path), source),
		}
	}
	if len(candidates) < 2 || !readable || len(sources) < len(candidates) {
		return result
	}

	freeTags, ok := freeTagsMentioned(sources)
	if !ok || len(freeTags) > maxFreeTags {
		return result
	}
	result.Partitioned = !someEnvironmentSelectsTwo(sources, freeTags)
	return result
}

// candidateConstraint describes one candidate file's build constraint for a finding detail: the
// file's `//go:build` line verbatim, else its `// +build` lines, else the GOOS or GOARCH filename
// suffix, else "no constraint".
func candidateConstraint(name string, source []byte) string {
	goBuild, plusBuild := constraintLines(source)
	if goBuild != "" {
		return goBuild
	}
	if len(plusBuild) > 0 {
		return strings.Join(plusBuild, "; ")
	}
	if suffix := filenameConstraintSuffix(name); suffix != "" {
		return suffix
	}
	return noConstraint
}

// constraintLines returns the `//go:build` line, and else the `// +build` lines, of a Go file's
// header: the blank and line-comment lines before the first other line.
func constraintLines(source []byte) (goBuild string, plusBuild []string) {
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case constraint.IsGoBuild(line):
			if goBuild == "" {
				goBuild = line
			}
		case constraint.IsPlusBuild(line):
			plusBuild = append(plusBuild, line)
		case strings.HasPrefix(line, "//"):
		default:
			return goBuild, plusBuild
		}
	}
	return goBuild, plusBuild
}

// filenameConstraintSuffix returns the GOOS or GOARCH suffix of a Go file name, such as
// `_windows.go` or `_linux_amd64.go`, or "" when the name carries none.
func filenameConstraintSuffix(name string) string {
	operatingSystems, architectures := pinnedBuildNames()
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".go"), "_test")
	elements := strings.Split(stem, "_")
	if len(elements) < 2 {
		return ""
	}
	tail := elements[1:]
	last := tail[len(tail)-1]
	if len(tail) >= 2 && operatingSystems[tail[len(tail)-2]] && architectures[last] {
		return "_" + tail[len(tail)-2] + "_" + last + ".go"
	}
	if operatingSystems[last] || architectures[last] {
		return "_" + last + ".go"
	}
	return ""
}

// pinnedBuildNames returns the GOOS and GOARCH names of the pinned pairs as sets.
func pinnedBuildNames() (operatingSystems, architectures map[string]bool) {
	operatingSystems = make(map[string]bool)
	architectures = make(map[string]bool)
	for _, pair := range pinnedBuildPairs {
		operatingSystems[pair.GOOS] = true
		architectures[pair.GOARCH] = true
	}
	return operatingSystems, architectures
}

// freeTagsMentioned returns the sorted free tags the sources' constraint lines mention, each by
// the name the matcher looks up; ok is false when a constraint line cannot be parsed.
// A free tag is a mentioned tag that is no pinned GOOS or GOARCH name, `unix` or `cgo`.
func freeTagsMentioned(sources map[string][]byte) (freeTags []string, ok bool) {
	operatingSystems, architectures := pinnedBuildNames()
	free := make(map[string]bool)
	for _, source := range sources {
		goBuild, plusBuild := constraintLines(source)
		lines := plusBuild
		if goBuild != "" {
			lines = []string{goBuild}
		}
		for _, line := range lines {
			expression, err := constraint.Parse(line)
			if err != nil {
				return nil, false
			}
			for _, tag := range tagsOf(expression) {
				if operatingSystems[tag] || architectures[tag] || tag == "unix" || tag == "cgo" {
					continue
				}
				if tag == "boringcrypto" {
					tag = "goexperiment.boringcrypto"
				}
				free[tag] = true
			}
		}
	}
	for tag := range free {
		freeTags = append(freeTags, tag)
	}
	sort.Strings(freeTags)
	return freeTags, true
}

// tagsOf returns every tag a constraint expression mentions.
func tagsOf(expression constraint.Expr) []string {
	switch e := expression.(type) {
	case *constraint.TagExpr:
		return []string{e.Tag}
	case *constraint.NotExpr:
		return tagsOf(e.X)
	case *constraint.AndExpr:
		return append(tagsOf(e.X), tagsOf(e.Y)...)
	case *constraint.OrExpr:
		return append(tagsOf(e.X), tagsOf(e.Y)...)
	}
	return nil
}

// someEnvironmentSelectsTwo reports whether any build environment — every pinned pair, cgo on and
// off, and every assignment of freeTags — selects two of the source files.
// A file that the matcher cannot evaluate counts as selected, so it never partitions an answer.
func someEnvironmentSelectsTwo(sources map[string][]byte, freeTags []string) bool {
	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	for _, pair := range pinnedBuildPairs {
		for _, cgoEnabled := range []bool{false, true} {
			for assignment := 0; assignment < 1<<len(freeTags); assignment++ {
				context := build.Context{
					GOOS:       pair.GOOS,
					GOARCH:     pair.GOARCH,
					CgoEnabled: cgoEnabled,
					BuildTags:  tagsAssignedTrue(freeTags, assignment),
					OpenFile:   func(path string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(sources[path])), nil },
				}
				selected := 0
				for _, path := range paths {
					match, err := context.MatchFile(filepath.Dir(path), filepath.Base(path))
					if match || err != nil {
						selected++
					}
				}
				if selected >= 2 {
					return true
				}
			}
		}
	}
	return false
}

// tagsAssignedTrue returns the freeTags whose bit is set in assignment.
func tagsAssignedTrue(freeTags []string, assignment int) []string {
	var assigned []string
	for i, tag := range freeTags {
		if assignment&(1<<i) != 0 {
			assigned = append(assigned, tag)
		}
	}
	return assigned
}
