// Package agentname is the sole former, parser and validator of agent names.
//
// A full name has the form "<shortname>:<role>" in the prime and in a standalone run.
// In a task worktree it has the form "<shortname>:<slug>:<role>".
// The shortname is 2-6 characters of [a-z][a-z0-9];
// the slug and the role are [a-z][a-z0-9-]*.
// Every other package forms, parses, validates and matches names through this one (PATTERN-agent-addressing).
// This package imports the standard library only.
//
// # Where names are formed
//
// A program sender is "<shortname>:<NAME>", with an upper-case NAME matching [A-Z][A-Z0-9_]*.
// FormatProgram forms it and ParseProgram parses it.
// A program acting for one beam signs "<shortname>:<slug>:<NAME>".
// FormatScopedProgram forms it, and ParseProgram reads it as the same (shortname, name) pair.
// EXTERNAL is the reserved name of a sender creel cannot place.
// Reed forms a strand's full name once, in AddStrand under its state lock.
// It stores the name in the strand record, the only key that delivery and lookup resolve through.
// Pane titles and Claude session names are display mirrors.
// A full name never names a tmux session or socket, since tmux rewrites ':' and '.' there.
// A per-strand file, such as creel's mail flag, is named by FileName.
// A name shown across machines takes the machine's name in front, as "<machine>::<name>".
// FormatMachineAddress forms it.
//
// The module that spawns an agent owns its role name as a constant declared in that module.
// It is never an inline literal in the spawn spec.
// There is no shared list of roles, and the role set stays open.
// cmd/lym/spawnrole_test.go is a tripwire.
// It fails a shuttleengine.Spec or reedengine.AddSpec composite literal whose Role is a string literal.
// A seat table's role is its prefix, declared in the spawning module.
// Its lead's role is the prefix alone.
// A later seat's role suffix may come from the recipe or config.
// The role grammar and the table's uniqueness check bound it.
//
// A run's parent is the worktree its pair was created from, recorded once in fabric's origin record.
// Its agent name is resolved at use as the orch role's name in that worktree.
// It is never stored in a seed or read from the caller's environment.
// The hub's shortname is recorded in .lym-shortname at the board root.
// Only internal/fabricengine reads and writes that file; internal/hubgeom tells the shortname to reed.
// internal/standalonegeom tells reed a standalone run's derived shortname.
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

	// CallerOperator is the caller label of a process with no strand, the operator at a terminal.
	CallerOperator = "operator"

	// StrandNameEnv is the env key reed injects with a strand's full name.
	StrandNameEnv = "LYM_STRAND_NAME"
	// ParentEnv is the env key reed injects with the full name of the session that spawned the worktree's run.
	ParentEnv = "LYM_PARENT"
	// LymBinEnv is the env key reed's launch script exports with the absolute path of the lym binary, not its directory.
	// The Claude engine's env file reads the same key.
	LymBinEnv = "LYM_BIN"

	sep = ":"
)

// Name is a parsed agent name.
// Slug is empty in the prime and in a standalone run.
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

// ValidateRole checks the role grammar [a-z][a-z0-9-]*;
// the set of roles is open.
func ValidateRole(role string) error { return validateDashed("role", role) }

// Format validates each segment and joins them;
// an empty slug gives the two-segment form.
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

// Parse accepts exactly two segments (prime) or three (task);
// anything else is an error.
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

// External is the reserved program-sender name of a sender that cannot be placed.
const External = "EXTERNAL"

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }

func validateProgramName(name string) error {
	ok := name != "" && isUpper(name[0])
	for i := 1; ok && i < len(name); i++ {
		ok = isUpper(name[i]) || isDigit(name[i]) || name[i] == '_'
	}
	if !ok {
		return fmt.Errorf("agent program name %q is invalid: must match [A-Z][A-Z0-9_]*", name)
	}
	return nil
}

// FormatProgram validates the shortname and the upper-case program name and joins them as "<shortname>:<NAME>".
// The upper-case name tells a program sender, such as a batten row, from a strand role.
func FormatProgram(shortname, name string) (string, error) {
	if err := ValidateShortname(shortname); err != nil {
		return "", err
	}
	if err := validateProgramName(name); err != nil {
		return "", err
	}
	return shortname + sep + name, nil
}

// MachineAddressSeparator joins a machine's name to an agent's or program's name in the cross-machine address form.
const MachineAddressSeparator = "::"

// FormatMachineAddress returns name in the cross-machine address form "<machine>::<name>".
// The form tells a reader on another machine, or in Slack, which machine the agent or program lives on.
// It validates neither part: name is a full name this package already formed.
func FormatMachineAddress(machine, name string) string {
	return machine + MachineAddressSeparator + name
}

// FormatScopedProgram validates the shortname, the slug and the upper-case program name.
// It joins them as "<shortname>:<slug>:<NAME>".
// An empty slug gives the two-segment form.
// A scoped sender is a program that acts for one beam, such as the beam's dobby.
func FormatScopedProgram(shortname, slug, name string) (string, error) {
	if slug == "" {
		return FormatProgram(shortname, name)
	}
	if err := ValidateShortname(shortname); err != nil {
		return "", err
	}
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	if err := validateProgramName(name); err != nil {
		return "", err
	}
	return shortname + sep + slug + sep + name, nil
}

// ParseProgram accepts "<shortname>:<NAME>" or "<shortname>:<slug>:<NAME>".
// It returns the shortname and the upper-case program name.
func ParseProgram(full string) (shortname, name string, err error) {
	parts := strings.Split(full, sep)
	switch len(parts) {
	case 2:
		_, err = FormatProgram(parts[0], parts[1])
	case 3:
		if parts[1] == "" {
			err = fmt.Errorf("empty slug segment")
		} else {
			_, err = FormatScopedProgram(parts[0], parts[1], parts[2])
		}
	default:
		return "", "", fmt.Errorf("agent program name %q is invalid: want <shortname>:<NAME> or <shortname>:<slug>:<NAME>", full)
	}
	if err != nil {
		return "", "", fmt.Errorf("agent program name %q: %w", full, err)
	}
	return parts[0], parts[len(parts)-1], nil
}

// IsProgram reports whether full parses as a program sender, such as "<shortname>:EXTERNAL".
func IsProgram(full string) bool {
	_, _, err := ParseProgram(full)
	return err == nil
}

// TaskSlug returns the slug of a strand name that parses and carries one.
// It returns the empty string for a name with no slug and for a name that does not parse.
func TaskSlug(strandName string) string {
	name, err := Parse(strandName)
	if err != nil {
		return ""
	}
	return name.Slug
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

// MatchesRole reports whether role is base itself or a numbered form of it: base-N with N >= 2.
func MatchesRole(role, base string) bool {
	if role == base {
		return true
	}
	suffix, ok := strings.CutPrefix(role, base+"-")
	if !ok {
		return false
	}
	n, err := strconv.Atoi(suffix)
	return err == nil && n >= 2 && strconv.Itoa(n) == suffix
}

// CallerLabel classifies a caller's full strand name into the label the shed history records.
// An empty strand reads as "operator";
// a strand whose role is "orch" or "driver", or a numbered form of either, reads as that role;
// any other strand reads as its own role.
// A strand that does not parse is never refused.
// Its label is the text after its last ":", or the whole strand when it has none.
// The label is for reports only and no decision reads it, which bounds a caller choosing its own strand.
func CallerLabel(strand string) string {
	if strand == "" {
		return CallerOperator
	}
	role := strand[strings.LastIndex(strand, sep)+1:]
	if n, err := Parse(strand); err == nil {
		role = n.Role
	}
	for _, base := range []string{RoleOrch, RoleDriver} {
		if MatchesRole(role, base) {
			return base
		}
	}
	return role
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

// Matches reports whether query addresses the strand named full.
// It matches when query equals full, or when query is a bare role segment equal to full's role.
// A full name that does not parse matches by equality only.
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

// fileNameSep is the character that stands in for ":" in a per-strand file name.
// No name segment holds ".", so the mapping is one to one.
const fileNameSep = "."

// FileName returns the file name a per-strand file of full takes: full with every ":" turned into ".".
// A full name is not a valid file name on Windows, where ":" is reserved.
func FileName(full string) string {
	return strings.ReplaceAll(full, sep, fileNameSep)
}
