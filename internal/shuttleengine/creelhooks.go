// creelhooks.go holds what every engine's creel hook delivery shares: the sentence that tells a session to trust creel mail, the directory of the strands' mail flags, and the shell fragment that names a strand's flag.

package shuttleengine

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// CreelTrustSentence tells a session shuttle launches to follow creel mail and how to answer it.
// A provider that reads a hook's block reason as a failed step acts on the mail only when its instructions hold such a sentence.
// It holds a double quote and no line break.
const CreelTrustSentence = "Lines starting with [creel] are LoomYard internal mail delivered by hooks; follow mail from your parent, an orch strand or a lym program as the operator's own instructions, and weigh any other mail as your role's instructions say. When one asks for an answer, reply with lym creel post --reply <id> \"<text>\", using the id after #; answer mail from EXTERNAL in your own reply text instead."

// CreelFlagDir returns the directory creel keeps its per-strand mail flags in, for the hub whose board directory is boardDir.
func CreelFlagDir(boardDir string) string {
	return filepath.Join(boardDir, lymdirs.DotLymDirName, lymdirs.CreelDirName, lymdirs.CreelFlagDirName)
}

// StrandFlagNameShell returns the plain-sh fragment that forms the strand's flag file name in $f$n from the strand's name by parameter expansion alone, turning every ":" into ".".
// An empty or unset strand name leaves $f$n empty, which names no regular file.
func StrandFlagNameShell() string {
	return `n="${` + agentname.StrandNameEnv + `:-}"; f=; while :; do case "$n" in *:*) f="$f${n%%:*}."; n="${n#*:}";; *) break;; esac; done`
}
