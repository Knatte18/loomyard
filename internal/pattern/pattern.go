// pattern.go holds the PATTERN active check and the role-keyed directive read.
// Directive picks the stencil for a Role and fills it with the PATTERN overview.

package pattern

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// patternFileName is the filename of the PATTERN overview at the worktree root.
const patternFileName = "PATTERN.md"

// patternDirName is the directory of background files beside the overview, relative to the worktree root.
// The directive stencils spell the directory as the fixed literal "pattern/" rather than using this constant.
const patternDirName = "pattern"

// overviewMarker is the marker every directive stencil carries, filled with the PATTERN overview.
const overviewMarker = "pattern_overview"

// changesMarker is the optional marker every directive stencil carries, filled with the rendered declared changes.
const changesMarker = "pattern_changes"

// changesFragmentStencil is the name of the stencil that renders the declared changes;
// its one required marker is changesListMarker.
const changesFragmentStencil = "pattern-fragment-changes"

// changesListMarker is the fragment's marker, filled with the declared bullet lines.
const changesListMarker = "pattern_change_list"

// File returns the path to the PATTERN.md overview within a repository's worktree root.
func File(worktreeRoot string) string {
	return filepath.Join(worktreeRoot, patternFileName)
}

// Role identifies which agent-facing directive variant to render.
type Role int

// The directive variants the package renders, one per agent shape.
const (
	// RoleImplementer selects the pre-edit checklist for any agent that edits code.
	RoleImplementer Role = iota + 1
	// RoleReviewFix selects the reviewer variant for the burler review half.
	RoleReviewFix
	// RoleOrchestrator selects the forking-only variant for webster's Master session.
	RoleOrchestrator
	// RoleDesigner selects the design variant for the agent that decides a task before any plan exists.
	RoleDesigner
	// RoleJudge selects the variant for the Bouncer judge, which weighs findings and never opens the artifacts.
	RoleJudge
)

// The background pointer "pattern/" stays a fixed literal in the directive stencil files;
// it is never interpolated from patternDirName or any path.
// That keeps fixed-string equality and substring comparisons meaningful in tests, including consumer template tests.
//
// Each Role's directive stencil name is a constant, so each name is written exactly once.
const (
	implementerDirectiveStencil  = "pattern-directive-implementer"
	reviewFixDirectiveStencil    = "pattern-directive-review-fix"
	orchestratorDirectiveStencil = "pattern-directive-orchestrator"
	designerDirectiveStencil     = "pattern-directive-designer"
	judgeDirectiveStencil        = "pattern-directive-judge"
)

// statFile and readFile are the stat and read calls the overview read makes.
// They are variables rather than direct os calls so that tests can simulate a stat or read failure.
// This works on every platform and needs no process privilege.
// Production code never reassigns them.
var (
	statFile = os.Stat
	readFile = os.ReadFile
)

// Directive returns the role's directive text to inject into the agent's prompt.
// It is read from stencilsDir, stripped of its leading banner, and carries the PATTERN overview inlined at its marker.
// declared is the task's declared PATTERN changes as rendered bullet lines;
// a non-empty declared adds the declared-changes fragment after the overview, and nil or empty adds nothing.
// It returns ("", nil) with no read attempted for an empty worktreeRoot, and for an inactive PATTERN:
// an absent File(worktreeRoot), a directory in its place, or whitespace-only content.
// An unknown or zero role also returns ("", nil), and the stencil is never read.
// It returns ("", err) when stat fails with an error other than not-existing, or when reading the overview fails.
// It also returns ("", err) when PATTERN is active, the role is known, and a stencil read or fill fails.
// Any other content is inlined verbatim, however malformed; format violations are the checker's job.
func Directive(worktreeRoot, stencilsDir string, role Role, declared []string) (string, error) {
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
		// An unknown or zero Role renders no directive and attempts no read.
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
		// stencilstore.Read's error already names the stencil and its base directory,
		// so this wrap adds only the package prefix.
		return "", fmt.Errorf("pattern: directive stencil: %w", err)
	}
	changes, err := renderChanges(stencilsDir, declared)
	if err != nil {
		return "", err
	}
	return fillDirective(name, string(content), overview, changes)
}

// FillDirective renders a directive from stencil text and overview text already in hand.
// It strips the stencil's leading banner and fills the overview marker.
// Its optional changes marker is filled with nothing.
// A whitespace-only overview is an inactive PATTERN and returns ("", nil).
// A fill failure is returned wrapped with stencilName, which appears only in that error text.
func FillDirective(stencilName, stencilText, overview string) (string, error) {
	return fillDirective(stencilName, stencilText, overview, "")
}

// fillDirective strips the stencil's leading banner and fills the overview and changes markers.
// A whitespace-only overview returns ("", nil).
func fillDirective(stencilName, stencilText, overview, changes string) (string, error) {
	if strings.TrimSpace(overview) == "" {
		return "", nil
	}
	values := map[string]string{overviewMarker: overview, changesMarker: changes}
	filled, err := stencil.FillOptional([]byte(stencil.StripLeadingComment(stencilText)), values, []string{changesMarker})
	if err != nil {
		return "", fmt.Errorf("pattern: fill directive stencil %q: %w", stencilName, err)
	}
	return string(filled), nil
}

// renderChanges returns the declared-changes fragment filled with the declared bullet lines joined by newlines.
// It returns ("", nil) with no stencil read when declared is empty.
// A fragment read or fill failure is returned wrapped.
func renderChanges(stencilsDir string, declared []string) (string, error) {
	if len(declared) == 0 {
		return "", nil
	}
	content, err := stencilstore.Read(stencilsDir, changesFragmentStencil)
	if err != nil {
		return "", fmt.Errorf("pattern: changes fragment stencil: %w", err)
	}
	filled, err := stencil.Fill([]byte(stencil.StripLeadingComment(string(content))), map[string]string{changesListMarker: strings.Join(declared, "\n")})
	if err != nil {
		return "", fmt.Errorf("pattern: fill changes fragment stencil %q: %w", changesFragmentStencil, err)
	}
	return string(filled), nil
}

// readOverview returns the content of File(worktreeRoot), or "" when that path is absent or a directory.
// A stat error other than not-existing, or a read error on an existing file, is returned.
func readOverview(worktreeRoot string) (string, error) {
	path := File(worktreeRoot)
	info, err := statFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("pattern: stat %s: %w", path, err)
	}
	// A directory named PATTERN.md is not an overview, so it is treated as absent.
	if info.IsDir() {
		return "", nil
	}
	data, err := readFile(path)
	if err != nil {
		return "", fmt.Errorf("pattern: read %s: %w", path, err)
	}
	return string(data), nil
}
