// friction.go implements Role and Directive, which turns a note path into a role's friction directive text.
// It also holds WarnIfMarkerAbsent and EnsureDir, the helpers that composers call alongside Directive.

package friction

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// freeNameScanCeiling bounds NotePath's first-free-suffix scan.
// Exhausting it is not an error this package reports.
// NotePath returns the last candidate the scan reached instead of looping forever.
const freeNameScanCeiling = 1000

// notePathToken is the literal template token that Directive substitutes with the notePath argument.
const notePathToken = "{{.note_path}}"

// Role identifies which agent-facing friction directive variant Directive should render.
type Role int

// The four directive variants Directive knows how to render, one per agent shape.
const (
	// RoleImplementer selects the directive for any agent that edits code.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the directive for the Burler fix half's fixer, the round's only note writer.
	RoleReviewFix
	// RoleOrchestrator selects the directive for webster's Master session, which forks rather than edits.
	RoleOrchestrator
	// RoleInterview selects the directive for the Discussion-Write interview agent.
	// That agent neither edits code nor reviews it.
	RoleInterview
)

// The stencil names Directive reads for each Role.
// Each is its own constant, so each name is written exactly once.
const (
	implementerDirectiveStencil  = "friction-directive-implementer"
	reviewFixDirectiveStencil    = "friction-directive-review-fix"
	orchestratorDirectiveStencil = "friction-directive-orchestrator"
	interviewDirectiveStencil    = "friction-directive-interview"
)

// MarkerName is the stencil marker name that composers pass to stencil.FillOptional's optional-names slice.
const MarkerName = "friction_directive"

// markerLiteral is the literal template token that WarnIfMarkerAbsent searches for.
// It is derived from MarkerName, so the two names cannot drift apart.
const markerLiteral = "{{." + MarkerName + "}}"

// ReportFileName is the reflection agent's mandatory output file name.
// internal/frictionengine's note scan and its shuttleengine.Spec.OutputFiles entry both read this constant,
// so the name is written once.
const ReportFileName = "reflection-report.md"

// Directive returns the role's friction directive text for injection into an agent's prompt.
// The text is read from stencilsDir, stripped of its leading banner, and has the literal notePath substituted into it.
// It returns ("", nil) without attempting a stencil read for an empty notePath, or for an unknown or zero Role.
// It returns ("", err) when notePath is non-empty, role is known, and the stencil read fails.
func Directive(notePath, stencilsDir string, role Role) (string, error) {
	if notePath == "" {
		return "", nil
	}

	var name string
	switch role {
	case RoleImplementer:
		name = implementerDirectiveStencil
	case RoleReviewFix:
		name = reviewFixDirectiveStencil
	case RoleOrchestrator:
		name = orchestratorDirectiveStencil
	case RoleInterview:
		name = interviewDirectiveStencil
	default:
		// An unknown or zero Role renders no directive and attempts no read.
		return "", nil
	}

	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		// stencilstore.Read's error already names the stencil and the base directory.
		// This wrap adds only the package prefix, not the stencil name a second time.
		return "", fmt.Errorf("friction: directive stencil: %w", err)
	}

	stripped := stencil.StripLeadingComment(string(content))
	// The substitution uses strings.ReplaceAll, not stencil.Fill.
	// The returned text is itself injected as another template's marker value,
	// so it must never be passed through Fill a second time.
	return strings.ReplaceAll(stripped, notePathToken, notePath), nil
}

// WarnIfMarkerAbsent warns when template does not carry the literal "{{.friction_directive}}"
// marker but a non-empty directive was computed for it.
// The third parameter is the composed directive text, not a note path.
// An empty directive means nothing would have been rendered anyway: the feature is off,
// or the read failed and the composer dropped the error.
// Neither case should warn about a missing marker.
// It does nothing when directive is empty.
// Otherwise, when template lacks the marker literal, it logs a warning naming the stencil and the marker,
// and it points the operator at "lym stencil diff" and "lym stencil sync".
func WarnIfMarkerAbsent(template []byte, stencilName, directive string) {
	if directive == "" {
		return
	}
	if bytes.Contains(template, []byte(markerLiteral)) {
		return
	}
	logger.Warn("friction: stencil is missing the friction directive marker; a computed directive will render as nothing -- see \"lym stencil diff\" and \"lym stencil sync\"", "stencil", stencilName, "marker", markerLiteral)
}

// NotePath composes a non-clobbering path for a note named by the stem id inside frictionDir.
// It returns "" when frictionDir is empty.
// It also returns "" for an id that fails sanitization.
// An id fails when it is empty, contains a path separator, is "." or "..", or has a ".." path element.
// An id whose id+".md" is ReportFileName fails too, so a caller cannot name a note over the reflection agent's output.
//
// id is a stem, not a final name.
// When frictionDir/id.md is not taken, NotePath returns that path.
// Otherwise it returns the first free path in the sequence id-2.md, id-3.md and so on.
// It takes the first free number, not the highest plus one: with id.md and id-3.md present, it returns id-2.md.
// The scan is bounded: past a fixed ceiling it returns the last candidate rather than looping forever.
// Exhausting the ceiling is not reported as an error.
// A stat error other than "not exist" counts as taken, so an unreadable entry is never overwritten.
//
// The scan is best-effort under concurrency.
// Two spawns racing in one directory can resolve to the same suffix, and one note is lost.
// That loss is accepted, since a note is optional bookkeeping.
// Sequential calls for the same site do not overwrite an earlier note.
func NotePath(frictionDir, id string) string {
	if frictionDir == "" {
		return ""
	}
	if !validNoteID(id) {
		return ""
	}

	candidate := filepath.Join(frictionDir, id+".md")
	if !pathTaken(candidate) {
		return candidate
	}

	for n := 2; n <= freeNameScanCeiling; n++ {
		candidate = filepath.Join(frictionDir, fmt.Sprintf("%s-%d.md", id, n))
		if !pathTaken(candidate) {
			return candidate
		}
	}
	return candidate
}

// validNoteID reports whether id is safe to use as a note file stem.
// It rejects an empty id, an id with a path separator, "." and "..", an id with a ".." path element,
// and an id whose ".md" name would collide with ReportFileName.
func validNoteID(id string) bool {
	if id == "" {
		return false
	}
	if strings.ContainsAny(id, "/\\") {
		return false
	}
	if id == "." || id == ".." {
		return false
	}
	for _, elem := range strings.Split(filepath.ToSlash(id), "/") {
		if elem == ".." {
			return false
		}
	}
	if id+".md" == ReportFileName {
		return false
	}
	return true
}

// pathTaken reports whether path cannot be used as a new note name.
// It is taken when it exists, or when stat fails with an error other than "not exist",
// so an unreadable entry is never silently overwritten.
func pathTaken(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}

// EnsureDir creates dir, including any missing parents, and reports a failure only as a logger.Warn.
// It must never fail `lym loom start` or `lym loom run`, so it returns no error.
// It no-ops on an empty dir.
func EnsureDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("friction: failed to create friction directory", "dir", dir, "error", err)
	}
}
