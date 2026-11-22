// revival.go drops remote control from an orch strand's stored command line when orch.yaml has it off.

package orchengine

import (
	"regexp"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
)

// remoteControlArgument matches a ` --remote-control <name>` pair in a stored launch line.
// The name may be quoted for either pane shell.
var remoteControlArgument = regexp.MustCompile(`\s--remote-control\s+(?:'(?:[^']|'\\''|'')*'|\S+)`)

// StripRemoteControlArgument returns line without its --remote-control argument and the session name that follows it.
// A line with no such argument is returned unchanged.
func StripRemoteControlArgument(line string) string {
	return remoteControlArgument.ReplaceAllString(line, "")
}

// RevivalFilter returns the rewrite a reed resume or relaunch applies to a stored command line.
// The line keeps its --remote-control argument only while the orch.yaml under dirs sets remote_control on.
// The file is read at each call, so a change to it applies to the next revival.
// A file that cannot be loaded cannot ask for remote control, so the argument is dropped then.
func RevivalFilter(dirs configengine.Dirs) func(line string) string {
	return func(line string) string {
		cfg, err := LoadConfig(dirs, "orch")
		if err != nil {
			logger.Warn("orch: dropping remote control from a revived session, since orch.yaml does not load", "err", err)
			return StripRemoteControlArgument(line)
		}
		if cfg.RemoteControl {
			return line
		}
		return StripRemoteControlArgument(line)
	}
}
