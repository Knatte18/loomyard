// reachverb.go implements `lym gate reach`, the read verb over the per-test coverage maps.
// Its function form prints the reach note of one function.
// Its test form lists the landings of a branch whose changed functions a test reaches.

package gatecli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testreach"
)

// reachWalkCommitCap is the most first-parent commits `lym gate reach --test` walks from its --since commit.
const reachWalkCommitCap = 200

// reachShortSHALength is the number of leading SHA characters the verb prints for a commit.
const reachShortSHALength = 7

// The two forms of the verb, as its refusals name them.
const (
	reachFunctionForm = "lym gate reach <file>#<Func>"
	reachTestForm     = "lym gate reach --test <package>.<Test> --since <rev>"
)

// reachRequest is one `lym gate reach` invocation after flag parsing.
type reachRequest struct {
	// function is the `<file>#<Func>` argument of the function form, empty for the test form.
	function string
	// test, since and branch are the values of --test, --since and --branch.
	test, since, branch string
	// limit is the most commits the test form walks from since.
	limit int
}

// reachCmd builds `lym gate reach`.
func reachCmd() *cobra.Command {
	var test, since, branch string
	cmd := &cobra.Command{
		Use:         "reach [<function>]",
		Short:       "list the tests that reach one function, or the landings on a branch that a test reaches, read from the coverage maps",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `reach reads the coverage maps and changes nothing: it builds nothing, takes no gate slot and writes no file, the index included.
reach <file>#<Func> prints the reach note of one function: the worktree-relative file and the function's name, Type.Method for a method.
The note lists the tests that reach it, its own package's first and at most five, the caveats of a function no test reaches, and the commit and age of each map read.
The maps are the hub's shared maps and, outside the prime, the worktree's own.
A function the maps do not hold prints a line saying so, with the commit of its package's map or that no map holds the package.
reach --test <package>.<Test> --since <rev> lists the landings a test reaches: <package> is the module-relative package path, split from the test at the last dot.
It walks the first-parent commits after <rev> of the parent branch, of --branch when given, or of the current branch in a worktree with no parent branch.
It lists each commit that changes a function the test reaches, naming those functions, and says how many commits it checked and which map holds the test.
A test no map holds walks nothing: the output says so and names the commits of the maps read.
A range over the commit cap is refused, and so is a <rev> that is not on the branch's first-parent history.
The maps may be days old, which the output says; a function renamed or removed since its landing matches nothing.

Examples:

  lym gate reach internal/foo/foo.go#Bar
  lym gate reach --test internal/foo.TestBar --since 3f2a9c1`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			request := reachRequest{test: test, since: since, branch: branch, limit: reachWalkCommitCap}
			if len(args) == 1 {
				request.function = args[0]
			}
			clihelp.SetExit(cmd.Context(), runReach(cmd.Context(), cmd.OutOrStdout(), request))
			return nil
		},
	}
	cmd.Flags().StringVar(&test, "test", "", "the test whose landings to list, as <package>.<Test> with the module-relative package path")
	cmd.Flags().StringVar(&since, "since", "", "with --test, the commit after which to walk, such as the last commit known green")
	cmd.Flags().StringVar(&branch, "branch", "", "with --test, the branch to walk instead of the parent branch")
	return cmd
}

// formRefusal returns the refusal for a request whose flags and argument fit no form, or "" when they fit one.
func (r reachRequest) formRefusal() string {
	hasFunction, hasTest := r.function != "", r.test != ""
	bothForms := fmt.Sprintf("run `%s` or `%s`", reachFunctionForm, reachTestForm)
	switch {
	case hasFunction == hasTest:
		return "gate reach: name exactly one form; way forward: " + bothForms
	case !hasTest && (r.since != "" || r.branch != ""):
		return "gate reach: --since and --branch go with --test; way forward: " + bothForms
	case hasTest && r.since == "":
		return fmt.Sprintf("gate reach: --test needs --since; way forward: pass --since with the last commit you know was green, as in `%s`", reachTestForm)
	case hasTest:
		if _, _, ok := splitReachTest(r.test); !ok {
			return fmt.Sprintf("gate reach: --test %q is not <package>.<Test>; way forward: name the module-relative package path, a dot and the test, as in `%s`", r.test, reachTestForm)
		}
	default:
		if file, name, ok := strings.Cut(r.function, "#"); !ok || file == "" || name == "" {
			return fmt.Sprintf("gate reach: %q is not <file>#<Func>; way forward: name the worktree-relative file, a hash sign and the function, as in `%s`", r.function, reachFunctionForm)
		}
	}
	return ""
}

// splitReachTest splits a `<package>.<Test>` value at its last dot, and reports whether both parts are there.
func splitReachTest(test string) (pkg, name string, ok bool) {
	i := strings.LastIndex(test, ".")
	if i <= 0 || i == len(test)-1 {
		return "", "", false
	}
	return test[:i], test[i+1:], true
}

// runReach answers the request from the coverage maps of the hub that contains the seam cwd.
// It returns the verb's exit code: 0 for an answer, a note naming why it read nothing included, and 1 for a refusal.
func runReach(ctx context.Context, out io.Writer, request reachRequest) int {
	if refusal := request.formRefusal(); refusal != "" {
		return output.Err(out, refusal)
	}
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		return output.Err(out, fmt.Sprintf("gate reach: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate reach from a worktree of the hub", cwd))
	}
	cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err))
	}
	cfg = cfg.WithDefaultDir(location.AnchorRel)
	if !cfg.HasGoCoverageMaps() {
		return output.Ok(out, map[string]any{"lines": []string{"gate reach: " + fmt.Sprintf(noteScoper, cfg.Scoper)}})
	}
	module, err := redundancyModule(cfg, location.WorktreePath())
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: declare tier0 or tier1 with its dir at the Go module's root with lym config gate from the prime, then re-run the same command", err))
	}
	fabric, err := fabricengine.Open(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: re-run the same command", err))
	}
	prime, err := hubgeom.IsPrime(location)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: re-run the same command", err))
	}
	step := reachStep{cfg: cfg, location: location, root: location.WorktreePath()}
	reader, err := testreach.Open(reachModuleOf(module), testcover.Dir(fabricengine.BoltDir(location.HubPath)), step.worktreeMapsDir(prime))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: re-run the same command", err))
	}
	if request.test != "" {
		return runReachTest(out, request, location, fabric, module, reader)
	}
	return runReachFunction(out, request.function, step, module, reader)
}

// runReachFunction prints the reach note of the function `<file>#<Func>`.
// The note is built as the reach step builds a changed function's line.
func runReachFunction(out io.Writer, function string, step reachStep, module goModule, reader *testreach.Reader) int {
	file, name, _ := strings.Cut(function, "#")
	file = path.Clean(filepath.ToSlash(file))
	dir := path.Dir(file)
	if !isModulePackage(module.rel, dir) {
		line := fmt.Sprintf("gate reach: %s is in no package of the module at %s, so no coverage map holds it", file, module.rel)
		return output.Ok(out, map[string]any{"function": function, "lines": []string{line}})
	}
	pkg := modulePackageOf(module.rel, dir)
	key := testcover.FuncKey{Package: module.importPath(pkg), File: path.Base(file), Func: name}
	report := ReachReport{Written: time.Now()}
	step.describe(&report, module, reader, []testreach.ChangedFunc{{Key: key}})
	lines := report.NoteLines("")
	if !slices.ContainsFunc(reader.Maps(), func(source testreach.MapSource) bool { return source.Package == pkg }) {
		lines = append(lines, fmt.Sprintf("gate reach: no coverage map holds the package %s; way forward: build the maps with `lym gate test --map` from the prime", pkg))
	}
	return output.Ok(out, map[string]any{"function": function, "lines": lines})
}

// runReachTest lists the first-parent commits after request.since whose changed functions the named test reaches.
func runReachTest(out io.Writer, request reachRequest, location *lymcwd.Location, fabric *fabricengine.Fabric, module goModule, reader *testreach.Reader) int {
	pkg, name, _ := splitReachTest(request.test)
	pkg = path.Clean(filepath.ToSlash(pkg))
	if _, _, held := reader.FuncsReachedBy(pkg, name, nil); !held {
		return output.Ok(out, map[string]any{"test": request.test, "lines": []string{noMapHoldsTestLine(request.test, reader)}})
	}
	branch, refusal := reachWalkBranch(location, fabric, request.branch)
	if refusal != "" {
		return output.Err(out, refusal)
	}
	commits, met, err := fabric.FirstParentCommits(branch, request.since, request.limit)
	switch {
	case errors.Is(err, fabricengine.ErrBranchUnresolved):
		return output.Err(out, fmt.Sprintf("gate reach: %q names no branch; way forward: pass a branch of the repository, or drop --branch to walk the parent branch", branch))
	case errors.Is(err, fabricengine.ErrCommitUnresolved):
		return output.Err(out, fmt.Sprintf("gate reach: --since %q names no commit; way forward: pass a commit SHA or a ref that names a commit", request.since))
	case err != nil:
		return output.Err(out, fmt.Sprintf("gate reach: %v; way forward: re-run the same command", err))
	case !met && len(commits) > request.limit:
		return output.Err(out, fmt.Sprintf("gate reach: the range after %s holds more than %d commits; way forward: pass a later --since, the last commit you know was green", request.since, request.limit))
	case !met:
		return output.Err(out, fmt.Sprintf("gate reach: %s is not on the first-parent history of %s; way forward: pass a commit that is, such as one that landed on it", request.since, branch))
	}

	_, source, _ := reader.FuncsReachedBy(pkg, name, nil)
	lines := []string{fmt.Sprintf("gate reach: commits checked: %d, since %s on %s; the map of %s is from commit %s",
		len(commits), request.since, branch, pkg, source.Commit)}
	landings := []map[string]any{}
	for _, commit := range commits {
		changed, err := fabric.ChangedPathsBothSides(commit.Parent, commit.SHA)
		if err != nil {
			return output.Err(out, fmt.Sprintf("gate reach: read the changes of commit %s: %v; way forward: re-run the same command", commit.SHA, err))
		}
		funcs, err := changedFuncsAt(commitTree{fabric: fabric, rev: commit.SHA}, fabric, commit.Parent, module, changed)
		if err != nil {
			return output.Err(out, fmt.Sprintf("gate reach: find the functions commit %s changes: %v; way forward: re-run the same command", commit.SHA, err))
		}
		keys := make([]testcover.FuncKey, len(funcs))
		for i, fn := range funcs {
			keys[i] = fn.Key
		}
		reached, _, _ := reader.FuncsReachedBy(pkg, name, keys)
		if len(reached) == 0 {
			continue
		}
		names := make([]string, len(reached))
		for i, key := range reached {
			names[i] = module.relativePackage(key.Package) + "." + key.Func
		}
		lines = append(lines, fmt.Sprintf("gate reach: %s changes %s", commit.SHA[:min(len(commit.SHA), reachShortSHALength)], strings.Join(names, ", ")))
		landings = append(landings, map[string]any{"commit": commit.SHA, "functions": names})
	}
	if len(landings) == 0 {
		lines = append(lines, fmt.Sprintf("gate reach: no checked commit changes a function %s reaches", request.test))
	}
	return output.Ok(out, map[string]any{
		"test": request.test, "branch": branch, "commits_checked": len(commits), "map_commit": source.Commit,
		"landings": landings, "lines": lines,
	})
}

// noMapHoldsTestLine says no coverage map holds the test, and names the commits of the maps the reader read.
func noMapHoldsTestLine(test string, reader *testreach.Reader) string {
	commits := map[string]bool{}
	for _, source := range reader.Maps() {
		commits[source.Commit] = true
	}
	read := "none"
	if len(commits) > 0 {
		read = strings.Join(slices.Sorted(maps.Keys(commits)), ", ")
	}
	return fmt.Sprintf("gate reach: no coverage map holds %s, so no landing can be blamed on it; the maps read are from commits %s", test, read)
}

// reachWalkBranch returns the branch the test form walks.
// It is branch when given, else the parent branch of the worktree's pair.
// A worktree with no parent branch walks its current branch.
// It returns a refusal when none of them names a branch.
func reachWalkBranch(location *lymcwd.Location, fabric *fabricengine.Fabric, branch string) (string, string) {
	if branch != "" {
		return branch, ""
	}
	origin, found, err := fabricengine.ReadOrigin(location)
	if err != nil {
		return "", fmt.Sprintf("gate reach: read the parent branch: %v; way forward: re-run the same command", err)
	}
	if found && origin.ParentBranch != "" {
		return origin.ParentBranch, ""
	}
	current, err := fabric.CurrentBranch()
	if err != nil {
		return "", fmt.Sprintf("gate reach: the worktree has no parent branch and is on no branch: %v; way forward: pass --branch with the branch to walk", err)
	}
	return current, ""
}
