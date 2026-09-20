// bootstrapverb.go declares batten's bootstrap-verb capability constant.

package battencli

// BootstrapVerb records that batten has no bootstrap verb.
//
// This is a deliberate declaration, not an unfilled placeholder: batten has no "lyx batten start", so
// "lyx batten run <slug>"'s driver IS the process the operator typed, and there is no spawn seam to
// branch on a seed's driver. batten's own --driver validator (arm.go) reads this constant directly,
// because battencli cannot import shedcli without an import cycle. The value changes only if batten
// grows a bootstrap verb -- at which point driver: llm becomes supportable for batten with no edit to
// either validator.
const BootstrapVerb = ""
