// rules.go implements the `rules` orch verb:
// it stores a draft as the repository's orch rules, one heddle entry aimed at the orch-prompt target.

package orchcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/heddlecli"
	"github.com/Knatte18/loomyard/internal/heddleengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// orchRulesSlug is the slug of the heddle entry that holds the repository's orch rules.
const orchRulesSlug = "orch-rules"

// orchRulesOrder is the render order a new orch rules entry takes:
// late, so the rules follow the passages every session of the engine shares.
const orchRulesOrder = 1000

// rulesCmd builds the `rules` subcommand.
func (c *orchCLI) rulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:         "rules --from <file>",
		Short:       "replace the repository's orch rules, kept in heddle under the orch-prompt target",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: fmt.Sprintf(`rules replaces the repository's orch rules with the content of --from. The
rules are the heddle entry %s, aimed at the %s target, whose text is
appended to the system prompt of every orch session launched afterwards. A
draft therefore starts as a copy of the current text, which
"lym heddle get %s" prints, and an empty file clears the rules.

A rules file kept at _lym/orch/rules.md, hub-wide on Bolt or per orch, is no
longer read. Its content carries over by copying it to a draft file and running
lym orch rules --from on that draft.

The entry is saved through heddle's write path: under the Bolt write lock,
followed by the board's detached Bolt sync, which commits and pushes it. A new
entry takes render order %d; an existing one keeps its order. The envelope
carries slug and cleared, and a warning when the sync did not start.`, orchRulesSlug, heddleengine.OrchPromptTarget, orchRulesSlug, orchRulesOrder),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			cwd, err := lymcwd.CwdFrom(cmd.Context())
			if err != nil {
				clihelp.Abort(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
				return nil
			}
			from, err := cmd.Flags().GetString("from")
			if err != nil {
				clihelp.Abort(cmd.Context(), output.Err(cmd.OutOrStdout(), err.Error()))
				return nil
			}
			clihelp.Abort(cmd.Context(), writeOrchRules(cmd.OutOrStdout(), cwd, from, heddlecli.ChangeStore))
			return nil
		},
	}
	cmd.Flags().String("from", "", "file whose content replaces the orch rules")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

// writeOrchRules stores the content of from, resolved against cwd when relative, as the orch rules entry of the hub cwd lies in.
// changeStore applies the change to the store of the hub cwd lies in; a test replaces it to reach the sync warning.
// It returns 0 on success and 1 on any refusal, whose message names the way forward.
func writeOrchRules(out io.Writer, cwd, from string, changeStore func(cwd string, change func(*heddleengine.Store) error) (string, error)) int {
	if from == "" {
		return output.Err(out, "lym orch rules needs --from <file>")
	}
	draft := from
	if !filepath.IsAbs(draft) {
		draft = filepath.Join(cwd, draft)
	}
	data, err := os.ReadFile(draft)
	if err != nil {
		return output.Err(out, fmt.Sprintf("cannot read --from %s: %v; way forward: name a readable draft file", draft, err))
	}
	text := string(data)
	warning, err := changeStore(cwd, func(store *heddleengine.Store) error { return applyOrchRules(store, text) })
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := map[string]any{"slug": orchRulesSlug, "cleared": strings.TrimSpace(text) == ""}
	if warning != "" {
		fields["warning"] = warning
	}
	return output.Ok(out, fields)
}

// applyOrchRules makes text the orch rules entry's text, aimed at the orch-prompt target alone.
// A blank text removes the entry instead.
// A new entry takes orchRulesOrder and an existing one keeps its order.
func applyOrchRules(store *heddleengine.Store, text string) error {
	_, exists := store.Get(orchRulesSlug)
	if strings.TrimSpace(text) == "" {
		if !exists {
			return nil
		}
		return store.Remove(orchRulesSlug)
	}
	patch := heddleengine.Patch{Text: &text, Targets: []string{heddleengine.OrchPromptTarget}}
	if !exists {
		order := orchRulesOrder
		patch.Order = &order
	}
	return store.Set(orchRulesSlug, patch)
}
