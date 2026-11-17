// redundant.go implements `lym gate redundant`, a read-only view of one package's coverage map.
// It lists the package's redundancy candidates: the tests whose covered blocks the package's other tests cover.

package gatecli

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testcover"
)

// redundantCandidate is one contained test of a `lym gate redundant` answer.
type redundantCandidate struct {
	Name    string  `json:"name"`
	Tier    int     `json:"tier"`
	Seconds float64 `json:"seconds"`
	// Removable is set for a test of the set the greedy pass can delete together while every block stays covered.
	Removable bool `json:"removable"`
	// Covering names the tests that together cover the candidate's blocks, largest overlap first.
	Covering []string `json:"covering"`
}

// redundantSkip is one test a `lym gate redundant` answer did not judge, with the keep reason or why not.
type redundantSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// redundantCmd builds `lym gate redundant`.
func redundantCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "redundant <package>",
		Short:       "list the tests of one package whose covered blocks its other tests cover, from its coverage map",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `redundant judges the coverage map of one package, named by its directory relative to the module root, and lists its redundancy candidates.
A candidate is a tier0 or tier1 test whose covered blocks of the package's own files the package's other judged tests cover.
Each comes with its standalone seconds and the tests that cover it; removable marks the set a greedy pass, slowest first, can delete together while every block stays covered.
A test marked //testtiming:keep is neither a candidate nor a cover, and nor is a test the spawn scan marks or one whose entry records no passing run.
It reads the map as the last lym gate testmap call left it and the keep markers from this worktree; it runs no test and takes no slot.
It prints the map's commit and build time, map_build_failure when a later coverage build failed, the candidates, the kept tests with their reasons and the tests not judged with why.

Examples:

  lym gate redundant internal/foo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), runRedundant(cmd.Context(), cmd.OutOrStdout(), args[0]))
			return nil
		},
	}
}

// runRedundant writes the redundancy candidates of the package pkg.
// It reads them from the package's coverage map in the hub of the seam cwd.
// It returns the verb's exit code.
func runRedundant(ctx context.Context, out io.Writer, pkg string) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot read the working directory: %v; way forward: re-run the same command", err))
	}
	location, inHub, err := resolveHub(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: %v; way forward: re-run the same command", err))
	}
	if !inHub {
		return output.Err(out, fmt.Sprintf("gate redundant: %s is in no hub, and the coverage maps live in a hub; way forward: run lym gate redundant from a worktree of the hub", cwd))
	}
	cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: cannot read the hub's gate config: %v; way forward: take the step it names, then re-run the same command", err))
	}
	module, err := redundancyModule(cfg.WithDefaultDir(location.AnchorRel), location.WorktreePath())
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: %v; way forward: declare tier0 or tier1 with its dir at the Go module's root with lym config gate from the prime, then re-run the same command", err))
	}

	pkg = path.Clean(filepath.ToSlash(pkg))
	rebuild := fmt.Sprintf("way forward: build it with lym gate testmap %s from the hub's prime worktree, then re-run the same command", pkg)
	m, found, err := testcover.ReadMap(testcover.Dir(fabricengine.BoltDir(location.HubPath)), pkg)
	if err == nil && found {
		err = checkMapBlocks(m)
	}
	switch {
	case err != nil:
		return output.Err(out, fmt.Sprintf("gate redundant: the coverage map of %s is unreadable: %v; %s", pkg, err, rebuild))
	case !found:
		return output.Err(out, fmt.Sprintf("gate redundant: %s has no coverage map; %s", pkg, rebuild))
	case m.BuildFailure != "":
		return output.Err(out, fmt.Sprintf("gate redundant: the coverage map of %s records only a failed build: %s; %s", pkg, m.BuildFailure, rebuild))
	}
	keeps, err := testcover.ScanKeeps(filepath.Join(module.dir, filepath.FromSlash(pkg)))
	if err != nil {
		return output.Err(out, fmt.Sprintf("gate redundant: the keep scan of %s failed: %v; way forward: fix the //testtiming:keep line it names, then re-run the same command", pkg, err))
	}

	candidates, kept, notJudged := []redundantCandidate{}, []redundantSkip{}, []redundantSkip{}
	for i, verdict := range testcover.JudgeMap(m, keeps) {
		test := m.Tests[i]
		switch {
		case verdict.Reason == testcover.ReasonKept:
			kept = append(kept, redundantSkip{Name: test.Name, Reason: keeps[test.Name]})
		case verdict.Reason != "":
			notJudged = append(notJudged, redundantSkip{Name: test.Name, Reason: verdict.Reason})
		case verdict.Contained:
			candidates = append(candidates, redundantCandidate{
				Name: test.Name, Tier: test.Tier, Seconds: test.Seconds, Removable: verdict.Removable, Covering: verdict.Covering,
			})
		}
	}
	answer := map[string]any{
		"package":      pkg,
		"map_commit":   m.Commit,
		"map_built_at": m.BuiltAt.Format(time.RFC3339),
		"tests":        len(m.Tests),
		"candidates":   candidates,
		"kept":         kept,
		"not_judged":   notJudged,
	}
	if m.Failure != nil {
		answer["map_build_failure"] = map[string]string{"commit": m.Failure.Commit, "error": m.Failure.Error}
	}
	return output.Ok(out, answer)
}
