// hookpython.go implements the hidden hook-python-allowed verb.
// A session's python guard hook runs it on a matching call, to ask whether the hub allows Python now.

package shuttlecli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// hookPythonAllowedCmd builds the hidden hook-python-allowed verb, run only by a session's python guard hook.
// It prints nothing on standard output, and a failure goes to standard error as a JSON error object.
// It exits 1 on a failure as on a deny, so a hook that cannot ask the hub keeps denying.
func hookPythonAllowedCmd() *cobra.Command {
	return &cobra.Command{
		Use:    shuttleengine.PythonAllowedHookVerb,
		Short:  "exit 0 when the shuttle config allows Python now (run by the python guard hook)",
		Hidden: true,
		Annotations: map[string]string{
			clihelp.AudienceAnnotation:        clihelp.AudienceInternal,
			clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled,
		},
		Long: `hook-python-allowed loads the shuttle config the session's launch read, from the
working directory, anywhere under the worktree's anchored directory. In a hub with
a Bolt checkout holding _lym that is the hub's layers; otherwise, as in a standalone
run, it is the state directory derived for the target. It exits 0 when deny_python is false and 1 when
it is true. It also exits 1 when the config cannot be found, so a guard that
cannot ask keeps denying. It prints nothing on standard output; a failure goes to
standard error as a JSON error object.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			allowed, err := pythonAllowed(cmd.Context())
			if err != nil {
				output.Err(cmd.ErrOrStderr(), "shuttle: "+err.Error())
			}
			if !allowed {
				clihelp.Abort(cmd.Context(), 1)
			}
			return nil
		},
	}
}

// pythonAllowed reports whether the shuttle config that the session's launch read allows Python.
// The session's cwd in ctx decides which config that is, as shuttleConfigDirs describes.
// A cwd with no config to find, or a config that does not load, returns the error.
func pythonAllowed(ctx context.Context) (bool, error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return false, err
	}
	dirs, err := shuttleConfigDirs(cwd)
	if err != nil {
		return false, err
	}
	cfg, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		return false, err
	}
	return !cfg.DenyPython, nil
}

// shuttleConfigDirs returns the config directories the launch of a session running at cwd loaded its shuttle config from.
// A cwd under a worktree of a hub whose Bolt checkout holds `_lym` gets the hub's layers, as a launch's hub mode does.
// Otherwise a standalone run's state directory, derived for the repository root above cwd, is used when it exists.
// With neither, the resolution error is returned, or the worktree's own layers when it resolved.
func shuttleConfigDirs(cwd string) (configengine.Dirs, error) {
	layout, resolveErr := lymcwd.ResolveWithin(cwd)
	var hubDirs configengine.Dirs
	if resolveErr == nil {
		hubDirs = fabricengine.ConfigDirs(layout)
		if preflight.BoardLymPresent(layout) {
			return hubDirs, nil
		}
	}
	if stateDir, err := cliwire.StandaloneStateDir(cliwire.RepositoryRootOf(cwd)); err == nil {
		if info, err := os.Stat(stateDir); err == nil && info.IsDir() {
			return configengine.Dirs{Anchor: stateDir}, nil
		}
	}
	return hubDirs, resolveErr
}
