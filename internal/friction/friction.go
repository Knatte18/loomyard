// friction.go implements Role and Directive, the role-keyed stencil read that turns a told note path
// into directive text, plus the two helpers every composer calls alongside it: WarnIfMarkerAbsent and
// EnsureDir.
// See doc.go for the package-level rationale.

package friction

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// notePathToken is the literal template token Directive substitutes with the caller-supplied
// notePath.
const notePathToken = "{{.note_path}}"

// Role identifies which agent-facing friction directive variant Directive should render.
type Role int

// The four directive variants Directive knows how to render, one per agent shape.
const (
	// RoleImplementer selects the directive for any agent that edits code: the webster fork, the
	// webster recovery strand, the webster integration fork, and loom's Plan-Write.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the directive for the Burler round's combined review-then-fix agent.
	RoleReviewFix
	// RoleOrchestrator selects the directive for webster's Master session, which forks rather than
	// edits.
	RoleOrchestrator
	// RoleInterview selects the directive for the Discussion-Write interview agent, whose job is
	// neither editing nor reviewing.
	RoleInterview
)

// implementerDirectiveStencil, reviewFixDirectiveStencil, orchestratorDirectiveStencil, and
// interviewDirectiveStencil name the stencil Directive reads for each Role, one constant per role so
// each name is written exactly once.
const (
	implementerDirectiveStencil  = "friction-directive-implementer"
	reviewFixDirectiveStencil    = "friction-directive-review-fix"
	orchestratorDirectiveStencil = "friction-directive-orchestrator"
	interviewDirectiveStencil    = "friction-directive-interview"
)

// MarkerName is the stencil marker name the seven composers pass to stencil.FillOptional's
// optional-names slice.
const MarkerName = "friction_directive"

// markerLiteral is the literal template token WarnIfMarkerAbsent searches for, derived from
// MarkerName so the two names cannot drift apart.
const markerLiteral = "{{." + MarkerName + "}}"

// ReportFileName is the reflection agent's own mandatory output file name, named once here so
// internal/frictionengine's note scan and its shuttleengine.Spec.OutputFiles entry both read the
// same constant.
const ReportFileName = "reflection-report.md"

// Directive returns the role's friction directive text to inject into the agent's prompt, read from
// stencilsDir and stripped of its leading banner, with the literal notePath substituted into it.
// It returns ("", nil) with no stencil read attempted at all for an empty notePath and for an
// unknown or zero Role, mirroring pattern.Directive's own empty-anchorPath early return and its
// default case.
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
		// An unknown or zero Role renders no directive and attempts no read; this default case is
		// what makes that behaviour defined and documented rather than an unhandled fall-through.
		return "", nil
	}

	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		// stencilstore.Read's own error already names both the stencil and the base directory, so
		// this wrap adds only the calling package's house prefix, not the stencil name a second time.
		return "", fmt.Errorf("friction: directive stencil: %w", err)
	}

	stripped := stencil.StripLeadingComment(string(content))
	// The substitution is a plain strings.ReplaceAll here, not stencil.Fill, because the returned
	// text is itself injected as another template's marker value and must never be passed through
	// Fill a second time -- the same reason pattern.Directive strips its banner and returns raw text.
	return strings.ReplaceAll(stripped, notePathToken, notePath), nil
}

// WarnIfMarkerAbsent warns when template does not carry the literal "{{.friction_directive}}"
// marker but a non-empty directive was computed for it.
// The third parameter is named directive, not notePath, because every composer passes the composed
// directive text rather than the note path -- an empty directive is the "nothing would have been
// rendered anyway" case, which covers both Tier 2 being off and a friction.Directive read error the
// composer swallowed, and neither should warn about a missing marker.
// It no-ops when directive is empty; otherwise, when template does not contain the marker literal,
// it logs a Warn naming the stencil and the marker and pointing the operator at "lyx stencil diff"
// and "lyx stencil sync".
func WarnIfMarkerAbsent(template []byte, stencilName, directive string) {
	if directive == "" {
		return
	}
	if bytes.Contains(template, []byte(markerLiteral)) {
		return
	}
	logger.Warn("friction: stencil is missing the friction directive marker; a computed directive will render as nothing -- see \"lyx stencil diff\" and \"lyx stencil sync\"", "stencil", stencilName, "marker", markerLiteral)
}

// EnsureDir creates dir, including any missing parents, and never returns an error: a failed create
// must never fail `lyx loom run` or `lyx loom drive`, so a failure is reported via logger.Warn
// instead.
// It no-ops on an empty dir.
func EnsureDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.Warn("friction: failed to create friction directory", "dir", dir, "error", err)
	}
}
