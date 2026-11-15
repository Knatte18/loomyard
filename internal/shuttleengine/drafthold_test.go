// drafthold_test.go holds IsDraftHold, the oracle the send tests classify a send's error against.

package shuttleengine

import "errors"

// IsDraftHold reports whether err is a send held by text in the input box:
// a *SessionBusyError whose hold is a draft, or an error wrapping ErrSentTextInDraft or ErrForeignBoxText.
func IsDraftHold(err error) bool {
	var busy *SessionBusyError
	if errors.As(err, &busy) && busy.Draft {
		return true
	}
	return errors.Is(err, ErrSentTextInDraft) || errors.Is(err, ErrForeignBoxText)
}
