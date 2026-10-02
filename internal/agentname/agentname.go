// Package agentname is the sole former, parser and validator of agent names.
//
// A full name is "<shortname>:<role>" in the prime and in a standalone run, and "<shortname>:<slug>:<role>" in a task worktree.
// The shortname is 2-6 characters of [a-z][a-z0-9]; the slug and the role are [a-z][a-z0-9-]*.
// Every other package forms, parses, validates and matches names through this one,
// and the package imports the standard library only.
package agentname

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// RoleOrch is the role other processes address the orchestrator strand by.
	RoleOrch = "orch"
	// RoleDriver is the role other processes address a run's driver strand by.
	RoleDriver = "driver"

	// StrandNameEnv is the env key reed injects with a strand's full name.
	StrandNameEnv = "LYX_STRAND_NAME"
	// ParentEnv is the env key reed injects with the full name of the session that spawned the worktree's run.
	ParentEnv = "LYX_PARENT"

	sep = ":"
)

// Name is a parsed agent name. Slug is empty in the prime and in a standalone run.
type Name struct {
	Shortname string
	Slug      string
	Role      string
}

// String joins the segments with ":", omitting an empty slug.
func (n Name) String() string {
	if n.Slug == "" {
		return n.Shortname + sep + n.Role
	}
	return n.Shortname + sep + n.Slug + sep + n.Role
}

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// ValidateShortname checks the repo shortname grammar [a-z][a-z0-9]{1,5}.
func ValidateShortname(shortname string) error {
	ok := len(shortname) >= 2 && len(shortname) <= 6 && isLower(shortname[0])
	for i := 1; ok && i < len(shortname); i++ {
		ok = isLower(shortname[i]) || isDigit(shortname[i])
	}
	if !ok {
		return fmt.Errorf("agent name shortname %q is invalid: must match [a-z][a-z0-9]{1,5} (2-6 characters, no '-', ':', '.' or '@')", shortname)
	}
	return nil
}

func validateDashed(kind, v string) error {
	ok := v != "" && isLower(v[0])
	for i := 1; ok && i < len(v); i++ {
		ok = isLower(v[i]) || isDigit(v[i]) || v[i] == '-'
	}
	if !ok {
		return fmt.Errorf("agent name %s %q is invalid: must match [a-z][a-z0-9-]*", kind, v)
	}
	return nil
}

// ValidateSlug checks the task worktree slug grammar [a-z][a-z0-9-]*.
func ValidateSlug(slug string) error { return validateDashed("slug", slug) }

// ValidateRole checks the role grammar [a-z][a-z0-9-]*; the set of roles is open.
func ValidateRole(role string) error { return validateDashed("role", role) }

// Format validates each segment and joins them; an empty slug gives the two-segment form.
func Format(shortname, slug, role string) (string, error) {
	if err := ValidateShortname(shortname); err != nil {
		return "", err
	}
	if slug != "" {
		if err := ValidateSlug(slug); err != nil {
			return "", err
		}
	}
	if err := ValidateRole(role); err != nil {
		return "", err
	}
	return Name{Shortname: shortname, Slug: slug, Role: role}.String(), nil
}

// Parse accepts exactly two segments (prime) or three (task); anything else is an error.
func Parse(name string) (Name, error) {
	parts := strings.Split(name, sep)
	var n Name
	switch len(parts) {
	case 2:
		n = Name{Shortname: parts[0], Role: parts[1]}
	case 3:
		n = Name{Shortname: parts[0], Slug: parts[1], Role: parts[2]}
	default:
		return Name{}, fmt.Errorf("agent name %q is invalid: want <shortname>:<role> or <shortname>:<slug>:<role>", name)
	}
	if _, err := Format(n.Shortname, n.Slug, n.Role); err != nil {
		return Name{}, fmt.Errorf("agent name %q: %w", name, err)
	}
	if len(parts) == 3 && n.Slug == "" {
		return Name{}, fmt.Errorf("agent name %q is invalid: empty slug segment", name)
	}
	return n, nil
}

// NumberRole returns role when held does not contain it, else role-N for the lowest N >= 2 not held.
// held is the role segments of every strand in the worktree's state, live or dormant.
func NumberRole(role string, held []string) string {
	taken := make(map[string]bool, len(held))
	for _, h := range held {
		taken[h] = true
	}
	if !taken[role] {
		return role
	}
	for n := 2; ; n++ {
		if c := role + "-" + strconv.Itoa(n); !taken[c] {
			return c
		}
	}
}

// Resolve reads a --name-style query: a bare role segment is formed under (shortname, slug);
// a query containing ":" is parsed as a full name and refused when its shortname or slug differ.
func Resolve(shortname, slug, query string) (Name, error) {
	if !strings.Contains(query, sep) {
		if _, err := Format(shortname, slug, query); err != nil {
			return Name{}, err
		}
		return Name{Shortname: shortname, Slug: slug, Role: query}, nil
	}
	n, err := Parse(query)
	if err != nil {
		return Name{}, err
	}
	if n.Shortname != shortname || n.Slug != slug {
		return Name{}, fmt.Errorf("agent name %q belongs to %q, not to %q; way forward: pass the role segment alone, or run the command from the worktree the full name belongs to",
			query, n.prefix(), Name{Shortname: shortname, Slug: slug}.prefix())
	}
	return n, nil
}

func (n Name) prefix() string {
	if n.Slug == "" {
		return n.Shortname
	}
	return n.Shortname + sep + n.Slug
}

// Matches reports whether query addresses the strand named full: equal to it,
// or a bare role segment equal to full's role. A full that does not Parse matches by equality only.
func Matches(full, query string) bool {
	if full == query {
		return true
	}
	if strings.Contains(query, sep) {
		return false
	}
	n, err := Parse(full)
	return err == nil && n.Role == query
}

// StandaloneShortname derives a standalone run's shortname: "s" plus the first five characters of hash8.
func StandaloneShortname(hash8 string) string {
	if len(hash8) > 5 {
		hash8 = hash8[:5]
	}
	return "s" + hash8
}
