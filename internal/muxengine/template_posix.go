//go:build !windows

// template_posix.go embeds the POSIX variant of the mux.yaml template, whose
// tmux/shell defaults are the PATH-resolved names tmux/bash rather than a
// pinned Windows install path. It deliberately carries no "_linux" filename
// suffix: the explicit "!windows" tag (rather than a linux-only suffix) is
// what makes every non-Windows GOOS pick up these POSIX defaults, per the
// batch's embedded-template-split exception to the repo's usual
// filename-suffix convention. See template_windows.go for the Windows
// counterpart and template.go for the shared, untagged ConfigTemplate()
// accessor.

package muxengine

import _ "embed"

//go:embed template_posix.yaml
var configTemplate string
