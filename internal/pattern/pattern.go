// pattern.go implements the PATTERN active check and Directive's role-keyed stencil read: which
// stencil name a Role selects, and the stencilstore.Read + StripLeadingComment call that turns it
// into directive text.
// See doc.go for the package-level rationale.

package pattern

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// patternFileName is the PATTERN entry-point filename. It is this package's
// single declaration of the filename; File and PathspecFile both build from
// it so the literal is written exactly once.
const patternFileName = "PATTERN.md"

// PathspecFile is the worktree-relative git-pathspec spelling of the PATTERN entry point.
// internal/pattern is its single declarer;
// internal/fabricengine consumes it for the PatternResidue pathspec.
const PathspecFile = lyxdirs.LyxDirName + "/" + patternFileName

// PathspecDir is the worktree-relative git-pathspec spelling of the PATTERN detail-docs directory.
// internal/pattern is its single declarer;
// internal/fabricengine consumes it for the PatternResidue pathspec.
const PathspecDir = lyxdirs.LyxDirName + "/pattern"

// File returns the path to the PATTERN.md file within a baseDir.
func File(baseDir string) string {
	return filepath.Join(baseDir, lyxdirs.LyxDirName, patternFileName)
}

// Role identifies which agent-facing directive variant Directive should render.
type Role int

// The three directive variants Directive knows how to render, one per agent shape.
const (
	// RoleImplementer selects the pre-edit checklist for any agent that edits code.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the combined review+fix variant for the burler round.
	RoleReviewFix
	// RoleOrchestrator selects the forking-only variant for webster's Master session.
	RoleOrchestrator
)

// The literal pointers "_lyx/PATTERN.md" and "_lyx/pattern/" now live in the three stencil files
// below rather than in Go, but they remain plain fixed literals there too — never interpolated from
// PathspecFile/PathspecDir or any lyxdirs.LyxDirName concatenation.
// That is what keeps this package's own tests' and every consumer template test's fixed-string
// equality and substring comparisons meaningful.
//
// implementerDirectiveStencil, reviewFixDirectiveStencil, and orchestratorDirectiveStencil name the
// stencil Directive reads for each Role, one constant per role so each name is written exactly once.
const (
	implementerDirectiveStencil  = "pattern-directive-implementer"
	reviewFixDirectiveStencil    = "pattern-directive-review-fix"
	orchestratorDirectiveStencil = "pattern-directive-orchestrator"
)

// statFile is the stat implementation isActive calls. It is a package-level
// variable — rather than a hardcoded os.Stat call — purely so this
// package's own test suite can simulate a non-"not exist" stat error (a
// permission or I/O failure) portably across platforms, without depending
// on process privilege or a POSIX-only permission trick. Production code
// never reassigns it.
var statFile = os.Stat

// Directive reports whether PATTERN is active and returns the role's directive text to inject into
// the agent's prompt, read from stencilsDir and stripped of its leading banner.
// It returns ("", nil) with no read attempted at all for an empty anchorPath, an inactive PATTERN, or
// an unknown or zero role.
// It returns ("", err) when PATTERN is active, role is known, and the stencil read fails.
func Directive(anchorPath, stencilsDir string, role Role) (string, error) {
	if anchorPath == "" {
		return "", nil
	}
	if !isActive(anchorPath) {
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
	default:
		// An unknown or zero Role renders no directive and attempts no read;
		// this default case is what makes that behaviour defined and
		// documented rather than an unhandled fall-through.
		return "", nil
	}

	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		// stencilstore.Read's own error already names both the stencil and
		// the base directory, so this wrap adds only the calling package's
		// house prefix, not the stencil name a second time.
		return "", fmt.Errorf("pattern: directive stencil: %w", err)
	}
	return stencil.StripLeadingComment(string(content)), nil
}

// isActive reports whether PATTERN is active: an absent File(anchorPath) means inactive; a directory in its place is also inactive; otherwise active.
func isActive(anchorPath string) bool {
	info, err := statFile(File(anchorPath))
	if err != nil {
		// os.IsNotExist is the normal, common inactive case: PATTERN.md was
		// never created. Any other error — permission denied, I/O failure —
		// is treated as active per Directive's doc comment: the ambiguity
		// resolves loud, in the agent's own read of the file, rather than
		// silently disabling the constraints.
		return !os.IsNotExist(err)
	}
	// A directory named PATTERN.md is not a readable index; treat it the
	// same as absent rather than reading something that isn't the file.
	return !info.IsDir()
}
