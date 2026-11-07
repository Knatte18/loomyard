// amend.go — the `drift amend` verb and the caller check it shares with `drift defer`.

package driftcli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/driftengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/orchcli"
)

// amendCmd returns the `drift amend` verb.
func amendCmd() *cobra.Command {
	var (
		entriesPath string
		restore     bool
		discard     bool
	)
	cmd := &cobra.Command{
		Use:         "amend <slug>",
		Short:       "amend the sealed expectation or put back the drift records",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `amend is the run's parent's way to settle a drift halt that the code is right
about. Exactly one form is given, and the amendment is recorded in scope.json
with who made it and when, in one commit, and never edits the seal.

--entries <file> adds entries to the expectation, in the shape of expected.yaml,
checked by the same rules as the expect agent's file; the stored rulings of the
cells or cases they cover are dropped, so the judge rules on them again.
--restore writes back whatever failed its check: the sealed copy of an edited
expected.yaml, and the records that differ from their commit; it is refused when
neither failed. --discard sets the whole expectation aside, so the run is
checked against its amendments alone.

It is admitted at a blocked Darn-Write, Webster, Drift-Verify or Drift-Recheck,
and at a blocked Drift-Expect for --restore alone, and run lym loom resume after it. Only the run's parent or the operator's own
shell runs it, from any worktree of the hub: <slug> names the task.

Example:
  lym drift amend my-task --entries amended.yaml`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerbAt(cmd, "amend", args[0], func(w *worktree) (any, error) {
				by, err := w.callerBy("amend")
				if err != nil {
					return nil, err
				}
				request := driftengine.AmendRequest{By: by, Restore: restore, Discard: discard}
				shape := request
				shape.Entries = entriesPath
				if err := shape.Check(); err != nil {
					return nil, err
				}
				if entriesPath != "" {
					if request.Entries, err = readEntriesFile(entriesPath); err != nil {
						return nil, err
					}
				}
				st, err := w.verbStatus()
				if err != nil {
					return nil, err
				}
				deps, err := w.checkDeps()
				if err != nil {
					return nil, err
				}
				if err := driftengine.Amend(deps, st, request); err != nil {
					return nil, err
				}
				return map[string]any{"amended": true, "by": by}, nil
			})
		},
	}
	cmd.Flags().StringVar(&entriesPath, "entries", "", "a YAML file of entries to add, shaped like expected.yaml")
	cmd.Flags().BoolVar(&restore, "restore", false, "write back the sealed expectation and the records that failed their check")
	cmd.Flags().BoolVar(&discard, "discard", false, "set the whole expectation aside")
	return cmd
}

// readEntriesFile reads the entries file of an amendment.
// It refuses a file it cannot read or one that holds nothing, and the error names the path.
func readEntriesFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("drift amend refused: cannot read the --entries file %s: %w; way forward: pass a readable YAML file of entries shaped like expected.yaml", path, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("drift amend refused: the --entries file %s is empty; way forward: write the entries to add, shaped like expected.yaml", path)
	}
	return string(data), nil
}

// callerBy runs the caller check of verb in this worktree's run and returns who the caller is for a record.
// It returns the operator for an unnamed shell and the run's parent otherwise.
func (w *worktree) callerBy(verb string) (string, error) {
	strand := os.Getenv(agentname.StrandNameEnv)
	parent := func() (hubgeom.Parent, error) { return orchcli.NearestLiveParent(w.location) }
	if err := checkParentCaller(verb, strand, parent); err != nil {
		return "", err
	}
	if strand == "" {
		return driftengine.AmendedByOperator, nil
	}
	return driftengine.AmendedByParent, nil
}

// checkParentCaller is the caller check that `drift amend` and `drift defer` run.
// The caller's strand name must equal the run's parent's name; an empty name, the operator's unnamed shell, passes.
// An empty name passes without resolving the parent.
// Any other name is refused, and the refusal names the parent and the operator's shell.
// A parent that cannot be resolved refuses a named caller naming the error and the operator's shell.
// Bound: the check rests on the strand name and on the shell guard that refuses setting lym's strand variables.
func checkParentCaller(verb, strand string, parent func() (hubgeom.Parent, error)) error {
	if strand == "" {
		return nil
	}
	resolved, err := parent()
	if err != nil {
		return fmt.Errorf("drift %s refused: the caller %q is a named session and the run's parent could not be resolved (%w); way forward: the operator's own unnamed shell may run it", verb, strand, err)
	}
	if strand == resolved.Name {
		return nil
	}
	way := "the operator's own unnamed shell may always run it"
	if resolved.Name != "" {
		way = fmt.Sprintf("ask %s to run it; %s", resolved.Name, way)
	}
	return fmt.Errorf("drift %s refused: the caller %q is not the run's parent; way forward: %s", verb, strand, way)
}
