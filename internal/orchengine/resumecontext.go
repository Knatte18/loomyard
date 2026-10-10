// resumecontext.go renders the resume pointer for the provider's session-start hook and records that it did.

package orchengine

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/state"
)

// ResumeContext renders the one-line pointer the session-start hook delivers after a compaction, from the persisted state, and writes the delivery mark with now and the text.
// A resuming phase with a pending pointer returns it verbatim;
// a compacting phase renders the resume prompt naming the cycle's note, and every other phase the reload prompt naming the role alone.
// Every case but the verbatim one renders the role file first, so the file the pointer names is current when the session reads it.
// A missing state file, or a render failure, returns an error and writes no mark.
func ResumeContext(p Paths, stencilsDir string, now time.Time) (string, error) {
	st, found, err := state.ReadJSON[State](p.StatePath, p.StateLockPath)
	if err != nil {
		return "", fmt.Errorf("orch: load state: %w", err)
	}
	if !found {
		return "", fmt.Errorf("orch: no orch state at %s; start the orch session first", p.StatePath)
	}
	text := st.PendingResume
	if st.Phase != PhaseResuming || text == "" {
		if err := RenderRoleFile(stencilsDir, p.RolePath); err != nil {
			return "", err
		}
		if st.Phase == PhaseCompacting {
			text, err = RenderResumePrompt(stencilsDir, p.RolePath, st.LastHandoff)
		} else {
			text, err = RenderReloadPrompt(stencilsDir, p.RolePath)
		}
		if err != nil {
			return "", err
		}
	}
	if err := WriteResumeMark(p, ResumeMark{At: now, Text: text}); err != nil {
		return "", err
	}
	return text, nil
}
