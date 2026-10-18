// command.go declares the closed set of commands a creel inject may carry instead of a text line, and the rules that validate one.

package creelengine

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// CommandName names one command of the closed set a command inject carries.
// The engine knows only the neutral name; the delivery implementation knows how the provider spells it.
type CommandName string

const (
	// CommandClear starts a fresh conversation in the receiver's session.
	CommandClear CommandName = "clear"
	// CommandCompact compacts the receiver's conversation, with an optional focus as its argument.
	CommandCompact CommandName = "compact"
	// CommandReloadPlugins reloads the plugins of the receiver's session.
	CommandReloadPlugins CommandName = "reload-plugins"
	// CommandColor sets the receiver's session color to the segment color its argument names.
	CommandColor CommandName = "color"
	// CommandLoadSkills loads the skills its space-separated argument lists into the receiver's session.
	CommandLoadSkills CommandName = "load-skills"
)

// Command is one command inject: a name from the closed set and the argument that name takes.
type Command struct {
	Name     CommandName
	Argument string
}

// IsZero reports whether the command is unset, which marks a text inject.
func (c Command) IsZero() bool { return c.Name == "" }

// validate refuses a name outside the closed set and an argument the name does not take.
func (c Command) validate() error {
	switch c.Name {
	case CommandClear, CommandReloadPlugins:
		if c.Argument != "" {
			return fmt.Errorf("creel: the %s command takes no argument", c.Name)
		}
	case CommandCompact:
		if c.Argument == "" || strings.IndexFunc(c.Argument, unicode.IsControl) >= 0 {
			return errors.New("creel: the compact command takes a focus line without control characters")
		}
	case CommandColor, CommandLoadSkills:
		if c.Argument == "" {
			return fmt.Errorf("creel: the %s command needs an argument", c.Name)
		}
	default:
		return fmt.Errorf("creel: unknown command %q; the commands are clear, compact, reload-plugins, color and load-skills", c.Name)
	}
	return nil
}
