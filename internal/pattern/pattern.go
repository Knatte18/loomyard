// pattern.go implements the PATTERN active check and Directive's role-keyed stencil read: which
// stencil name a Role selects, and the stencilstore.Read, StripLeadingComment and Fill calls that
// turn it into directive text carrying the PATTERN overview.
// See doc.go for the package-level rationale.

package pattern

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// patternFileName is the PATTERN overview filename at the repository's worktree root.
// It is this package's single declaration of the filename.
const patternFileName = "PATTERN.md"

// patternDirName is the background-file directory beside the overview, relative to the repository's worktree root.
// It is this package's single declaration of the directory name;
// the directive stencils spell it as the fixed literal "pattern/".
const patternDirName = "pattern"

// overviewMarker is the one marker every directive stencil carries, which Directive fills with the overview.
const overviewMarker = "pattern_overview"

// File returns the path to the PATTERN.md overview within a repository's worktree root.
func File(worktreeRoot string) string {
	return filepath.Join(worktreeRoot, patternFileName)
}

// Role identifies which agent-facing directive variant Directive should render.
type Role int

// The directive variants Directive knows how to render, one per agent shape.
const (
	// RoleImplementer selects the pre-edit checklist for any agent that edits code.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the combined review+fix variant for the burler round.
	RoleReviewFix
	// RoleOrchestrator selects the forking-only variant for webster's Master session.
	RoleOrchestrator
	// RoleDesigner selects the design variant for the agent that decides a task before any plan exists.
	RoleDesigner
	// RoleJudge selects the variant for the Bouncer judge, which weighs findings and never opens the artifacts.
	RoleJudge
)

// The background pointer "pattern/" lives in the directive stencil files below rather than in Go, and
// stays a plain fixed literal there too — never interpolated from patternDirName or any path.
// That is what keeps this package's own tests' and every consumer template test's fixed-string
// equality and substring comparisons meaningful.
//
// implementerDirectiveStencil, reviewFixDirectiveStencil, orchestratorDirectiveStencil and
// designerDirectiveStencil and judgeDirectiveStencil name the stencil Directive reads for each Role,
// one constant per role so each name is written exactly once.
const (
	implementerDirectiveStencil  = "pattern-directive-implementer"
	reviewFixDirectiveStencil    = "pattern-directive-review-fix"
	orchestratorDirectiveStencil = "pattern-directive-orchestrator"
	designerDirectiveStencil     = "pattern-directive-designer"
	judgeDirectiveStencil        = "pattern-directive-judge"
)

// statFile and readFile are the stat and read implementations Directive calls.
// They are package-level variables — rather than hardcoded os calls — purely so this
// package's own test suite can simulate a stat or read failure (a
// permission or I/O failure) portably across platforms, without depending
// on process privilege or a POSIX-only permission trick. Production code
// never reassigns them.
var (
	statFile = os.Stat
	readFile = os.ReadFile
)

// Directive returns the role's directive text to inject into the agent's prompt, read from
// stencilsDir, stripped of its leading banner and carrying the PATTERN overview inlined at its marker.
// It returns ("", nil) with no read attempted for an empty worktreeRoot, and for an inactive PATTERN:
// an absent File(worktreeRoot), a directory in its place, or whitespace-only content.
// An unknown or zero role also returns ("", nil), and the stencil is never read.
// It returns ("", err) when the file's stat fails for a reason other than not-existing, when reading
// an existing file fails, or when PATTERN is active, role is known, and the stencil read or fill fails.
// Any other content is inlined verbatim, however malformed; format violations are the checker's job.
func Directive(worktreeRoot, stencilsDir string, role Role) (string, error) {
	if worktreeRoot == "" {
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
	case RoleDesigner:
		name = designerDirectiveStencil
	case RoleJudge:
		name = judgeDirectiveStencil
	default:
		// An unknown or zero Role renders no directive and attempts no read;
		// this default case is what makes that behaviour defined and
		// documented rather than an unhandled fall-through.
		return "", nil
	}

	overview, err := readOverview(worktreeRoot)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(overview) == "" {
		return "", nil
	}

	content, err := stencilstore.Read(stencilsDir, name)
	if err != nil {
		// stencilstore.Read's own error already names both the stencil and
		// the base directory, so this wrap adds only the calling package's
		// house prefix, not the stencil name a second time.
		return "", fmt.Errorf("pattern: directive stencil: %w", err)
	}
	filled, err := stencil.Fill([]byte(stencil.StripLeadingComment(string(content))), map[string]string{overviewMarker: overview})
	if err != nil {
		return "", fmt.Errorf("pattern: fill directive stencil %q: %w", name, err)
	}
	return string(filled), nil
}

// readOverview returns the content of File(worktreeRoot), or "" when PATTERN has no overview there:
// the file is absent or a directory sits in its place.
// A stat error that is not "not exist", or a read error on an existing file, is returned.
func readOverview(worktreeRoot string) (string, error) {
	path := File(worktreeRoot)
	info, err := statFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("pattern: stat %s: %w", path, err)
	}
	// A directory named PATTERN.md is not a readable overview; treat it the
	// same as absent rather than reading something that isn't the file.
	if info.IsDir() {
		return "", nil
	}
	data, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("pattern: read %s: %w", path, err)
	}
	return string(data), nil
}
