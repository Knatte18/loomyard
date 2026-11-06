// Package orchrole holds the orch role: the hub orchestrator's whole procedure, one fixed text for every orch in every repository, and the on-demand files the role sends the session to for rarer events.
// The role and its on-demand files are rendered from these embedded bytes alone and never seeded to a board copy, so no hub can edit them;
// a repository's own orch rules reach the orch through heddle instead.
// The package imports nothing, so an engine and the stencil tests both import it without a cycle.
package orchrole

import _ "embed"

// Role is the orch role's text: a leading comment, then the procedure with the `decision_note_title` marker its render fills.
//
//go:embed orch-role.md
var Role []byte

var (
	//go:embed role-beams.md
	beams []byte
	//go:embed role-hibernate.md
	hibernate []byte
	//go:embed role-intake.md
	intake []byte
	//go:embed role-landing-halts.md
	landingHalts []byte
	//go:embed role-slack.md
	slack []byte
)

// OnDemand holds the texts the session reads only for a rarer event, keyed by the file name each renders to beside the role, the name the role calls it by.
// Each text is a leading comment, then a body with no marker.
var OnDemand = map[string][]byte{
	"role-beams.md":         beams,
	"role-hibernate.md":     hibernate,
	"role-intake.md":        intake,
	"role-landing-halts.md": landingHalts,
	"role-slack.md":         slack,
}
