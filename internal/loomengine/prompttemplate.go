// prompttemplate.go — discussion producer interview prompt asset.
//
// Embeds the generic discussion-template.md asset directly at build time,
// mirroring internal/burlerengine's reviewPromptTemplate accessor pattern.
// The template carries the interview discipline (board read, exploration,
// batched questions, decision-record/support-log compaction rules) as
// static prose around four top-level markers that prompt.go's
// composePrompt fills via internal/stencil.

package loomengine

import _ "embed"

//go:embed discussion-template.md
var discussionTemplate []byte
