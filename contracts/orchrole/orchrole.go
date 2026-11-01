// Package orchrole holds the orch role: the hub orchestrator's whole procedure, one fixed text for every orch in every repository.
// The role is rendered from these embedded bytes alone and never seeded to a board copy, so no hub can edit it;
// a repository's own orch rules reach the orch through heddle instead.
// The package imports nothing, so an engine and the stencil tests both import it without a cycle.
package orchrole

import _ "embed"

// Role is the orch role's text: a leading comment, then the procedure with the `command_index` and `decision_note_title` markers its render fills.
//
//go:embed orch-role.md
var Role []byte
