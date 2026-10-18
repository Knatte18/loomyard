// rules.go implements the `rules` orch verb: replace the hub-wide orch rules file on Bolt with a draft, committing it and pushing before and after.

package orchcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// hubRulesCommitMessage is the commit message the verb gives the hub-wide rules file.
const hubRulesCommitMessage = "orch: replace the hub-wide rules"

// rulesBolt is the slice of Bolt the verb calls, so untagged tests substitute a fake.
type rulesBolt interface {
	Push() error
	PullThenCommitWritten(write fabricengine.BoltWrite) (fabricengine.BoltWriteResult, error)
}

// boardRulesBolt adapts *fabricengine.Bolt to rulesBolt.
type boardRulesBolt struct {
	bolt *fabricengine.Bolt
}

// Push pushes Bolt, honoring the FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH bypass variables.
func (b boardRulesBolt) Push() error {
	return b.bolt.Push(fabricengine.EnvSyncOptions())
}

// PullThenCommitWritten brings Bolt up to date, then runs write and commits the path it returns.
func (b boardRulesBolt) PullThenCommitWritten(write fabricengine.BoltWrite) (fabricengine.BoltWriteResult, error) {
	return b.bolt.PullThenCommitWritten([]fabricengine.BoltWrite{write}, fabricengine.NewMutations(""))
}

// rulesCmd builds the `rules` subcommand.
func (c *orchCLI) rulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "rules --from <file>",
		Short:       "replace the hub-wide orch rules file on Bolt",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: fmt.Sprintf(`rules replaces the whole hub-wide orch rules file, <board>/%s,
with the content of --from, commits it on Bolt under a pull first, and pushes
before and after. A draft therefore starts as a copy of the current file at that
path, and an empty file clears the rules. The envelope carries path, committed
and unpushed; unpushed true means the commit is made and the next run pushes it.`, filepath.ToSlash(rulesRel())),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			cwd, err := lyxcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.Abort(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
				return nil
			}
			from, err := cmd.Flags().GetString("from")
			if err != nil {
				clihelp.Abort(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
				return nil
			}
			bolt := boardRulesBolt{bolt: fabricengine.NewBolt(fabricengine.BoardDir(c.location.HubPath))}
			clihelp.Abort(cmd.Context(), writeHubRules(cmd.OutOrStdout(), cwd, from, c.paths.HubRulesPath, bolt))
			return nil
		},
	}
	cmd.Flags().String("from", "", "file whose content replaces the hub-wide rules file")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

// writeHubRules replaces the file at rulesPath, inside Bolt's checkout, with the content of from, resolved against cwd when relative.
// It pushes Bolt, writes and commits the file under a pull first, and pushes again, returning 0 on success and 1 on any refusal.
// Every envelope after the draft is read carries path, committed and unpushed.
func writeHubRules(out io.Writer, cwd, from, rulesPath string, bolt rulesBolt) int {
	if from == "" {
		return output.Err(out, "lyx orch rules needs --from <file>")
	}
	draft := from
	if !filepath.IsAbs(draft) {
		draft = filepath.Join(cwd, draft)
	}
	data, err := os.ReadFile(draft)
	if err != nil {
		return output.Err(out, fmt.Sprintf("cannot read --from %s: %v", draft, err))
	}

	refuse := func(msg string, committed, unpushed bool) int {
		return output.ErrFields(out, msg, map[string]any{"path": rulesPath, "committed": committed, "unpushed": unpushed})
	}
	rerun := fmt.Sprintf("lyx orch rules --from %s", from)
	boardDir := filepath.Clean(strings.TrimSuffix(rulesPath, rulesRel()))
	diverged := fmt.Sprintf("the recovery rebase conflicted with an upstream change; run `git pull --rebase` in %s and resolve the conflict, then `lyx board sync`, then re-run %s", boardDir, rerun)

	if err := bolt.Push(); err != nil {
		wayForward := fmt.Sprintf("re-run %s once Bolt pushes", rerun)
		if errors.Is(err, gitrepo.ErrPullRebaseFailed) {
			wayForward = diverged
		}
		return refuse(fmt.Sprintf("pushing Bolt failed (%v); nothing was written, %s", err, wayForward), false, true)
	}

	wrote := false
	var writeErr error
	res, err := bolt.PullThenCommitWritten(fabricengine.BoltWrite{
		Message: hubRulesCommitMessage,
		Write: func() ([]string, error) {
			wrote = true
			if err := fsx.AtomicWriteBytes(rulesPath, data); err != nil {
				writeErr = err
				return nil, err
			}
			return []string{rulesRel()}, nil
		},
	})
	switch {
	case err != nil && !wrote:
		return refuse(fmt.Sprintf("%v; %s unchanged, nothing written", err, rulesPath), false, false)
	case writeErr != nil:
		return refuse(fmt.Sprintf("%v; %s unchanged", err, rulesPath), false, false)
	case err != nil:
		return refuse(fmt.Sprintf("%v; %s is written but not committed, re-run %s to commit it", err, rulesPath, rerun), false, false)
	case res.Skipped != "":
		return refuse(fmt.Sprintf("Bolt was not brought up to date and nothing was written: %s", res.SkipDetail), false, false)
	}

	if err := bolt.Push(); err != nil {
		wayForward := fmt.Sprintf("the next run of %s pushes it", rerun)
		if errors.Is(err, gitrepo.ErrPullRebaseFailed) {
			wayForward = diverged
		}
		return refuse(fmt.Sprintf("%s is committed on Bolt but pushing failed (%v); %s", rulesPath, err, wayForward), res.Committed, true)
	}
	return output.Ok(out, map[string]any{"path": rulesPath, "committed": res.Committed, "unpushed": false})
}
