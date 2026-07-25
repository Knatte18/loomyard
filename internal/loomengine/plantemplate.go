// plantemplate.go — plan producer autonomous prompt asset.
//
// Embeds the generic plan-template.md asset directly at build time,
// mirroring prompttemplate.go's discussionTemplate accessor pattern. The
// template carries the compact plan-format-v3 spec (on-disk layout,
// overview frontmatter, card fields, Rename mechanic block, a minimal
// skeleton) as static prose around three top-level markers that plan.go's
// composePlanPrompt fills via internal/stencil.

package loomengine

import _ "embed"

//go:embed plan-template.md
var planTemplate []byte
