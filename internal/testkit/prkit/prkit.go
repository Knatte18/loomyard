// prkit.go declares API, the scripted fake of pullrequest.API, and the Answer a script is made of.

package prkit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/pullrequest"
)

// The method names Script and Calls take, one per method of pullrequest.API.
const (
	MethodFindOpen            = "FindOpen"
	MethodCreate              = "Create"
	MethodEdit                = "Edit"
	MethodStatus              = "Status"
	MethodClose               = "Close"
	MethodReopen              = "Reopen"
	MethodSetDraft            = "SetDraft"
	MethodComment             = "Comment"
	MethodMerge               = "Merge"
	MethodRequiresPullRequest = "RequiresPullRequest"
	MethodRequiredChecks      = "RequiredChecks"
)

// Answer is one scripted reply of a method, holding the values the method returns.
// A method reads only the fields of its own result and ignores the rest.
type Answer struct {
	// Ref is what FindOpen and Create return.
	Ref pullrequest.Ref
	// Found is what FindOpen returns as found and RequiresPullRequest as its verdict.
	Found bool
	// Status is what Status returns.
	Status pullrequest.Status
	// Checks is what RequiredChecks returns.
	Checks []pullrequest.Check
	// Err is the error every method returns.
	Err error
}

// API is a pullrequest.API that answers each method from its scripted answers and records every call.
// The zero value is ready: a method with no script answers the zero Answer.
// It is safe for concurrent use.
type API struct {
	mu      sync.Mutex
	scripts map[string][]Answer
	used    map[string]int
	calls   map[string][]string
}

var _ pullrequest.API = (*API)(nil)

// Script sets the answers of method, one per call in order with the last repeating, and restarts its sequence.
func (a *API) Script(method string, answers ...Answer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scripts == nil {
		a.scripts = map[string][]Answer{}
		a.used = map[string]int{}
	}
	a.scripts[method] = answers
	a.used[method] = 0
}

// Calls returns the calls of method so far in order, each one its arguments rendered with fmt and joined by "|".
func (a *API) Calls(method string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.calls[method]...)
}

// answer records one call of method with args and returns its next scripted Answer.
func (a *API) answer(method string, args ...any) Answer {
	a.mu.Lock()
	defer a.mu.Unlock()
	rendered := make([]string, len(args))
	for i, arg := range args {
		rendered[i] = fmt.Sprint(arg)
	}
	if a.calls == nil {
		a.calls = map[string][]string{}
	}
	a.calls[method] = append(a.calls[method], strings.Join(rendered, "|"))

	script := a.scripts[method]
	if len(script) == 0 {
		return Answer{}
	}
	next := min(a.used[method], len(script)-1)
	a.used[method]++
	return script[next]
}

// FindOpen returns the scripted Ref and Found.
func (a *API) FindOpen(_ context.Context, head, base string) (pullrequest.Ref, bool, error) {
	answer := a.answer(MethodFindOpen, head, base)
	return answer.Ref, answer.Found, answer.Err
}

// Create returns the scripted Ref.
func (a *API) Create(_ context.Context, head, base, title, body string) (pullrequest.Ref, error) {
	answer := a.answer(MethodCreate, head, base, title, body)
	return answer.Ref, answer.Err
}

// Edit returns the scripted error.
func (a *API) Edit(_ context.Context, number int, title, body string) error {
	return a.answer(MethodEdit, number, title, body).Err
}

// Status returns the scripted Status.
func (a *API) Status(_ context.Context, number int) (pullrequest.Status, error) {
	answer := a.answer(MethodStatus, number)
	return answer.Status, answer.Err
}

// Close returns the scripted error.
func (a *API) Close(_ context.Context, number int, comment string) error {
	return a.answer(MethodClose, number, comment).Err
}

// Reopen returns the scripted error.
func (a *API) Reopen(_ context.Context, number int) error {
	return a.answer(MethodReopen, number).Err
}

// SetDraft returns the scripted error.
func (a *API) SetDraft(_ context.Context, number int, draft bool) error {
	return a.answer(MethodSetDraft, number, draft).Err
}

// Comment returns the scripted error.
func (a *API) Comment(_ context.Context, number int, body string) error {
	return a.answer(MethodComment, number, body).Err
}

// Merge returns the scripted error.
func (a *API) Merge(_ context.Context, number int, expectedHead string, squash bool, title, message string) error {
	return a.answer(MethodMerge, number, expectedHead, squash, title, message).Err
}

// RequiresPullRequest returns the scripted Found as its verdict.
func (a *API) RequiresPullRequest(_ context.Context, branch string) (bool, error) {
	answer := a.answer(MethodRequiresPullRequest, branch)
	return answer.Found, answer.Err
}

// RequiredChecks returns the scripted Checks.
func (a *API) RequiredChecks(_ context.Context, base, head string) ([]pullrequest.Check, error) {
	answer := a.answer(MethodRequiredChecks, base, head)
	return answer.Checks, answer.Err
}
