// Package cmdinfo provides introspection over a cobra command tree,
// producing serializable metadata for web UI display.
package cmdinfo

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandInfo holds the metadata for a single command.
type CommandInfo struct {
	FullName    string           // e.g. "gosilo user add"
	Name        string           // e.g. "add"
	Short       string
	Long        string
	Usage       string // cobra Use field
	Example     string
	GroupID     string
	GroupTitle  string
	Flags       []FlagInfo
	Subcommands []CommandSummary
	Parent      string // parent's FullName, empty for root
}

// FlagInfo describes a single flag.
type FlagInfo struct {
	Name      string
	Shorthand string
	Usage     string
	DefValue  string
	Type      string
}

// CommandSummary is a lightweight reference to a command (for index listings).
type CommandSummary struct {
	Name     string
	FullName string
	Short    string
}

// WalkCommands traverses the cobra command tree and returns a flat list of
// CommandInfo for every command (including the root).
func WalkCommands(root *cobra.Command) []CommandInfo {
	var out []CommandInfo
	walkCmd(root, "", &out)
	return out
}

func walkCmd(cmd *cobra.Command, parentFullName string, out *[]CommandInfo) {
	fullName := cmd.Name()
	if parentFullName != "" {
		fullName = parentFullName + " " + cmd.Name()
	}

	info := CommandInfo{
		FullName: fullName,
		Name:     cmd.Name(),
		Short:    cmd.Short,
		Long:     cmd.Long,
		Usage:    cmd.Use,
		Example:  cmd.Example,
		GroupID:  cmd.GroupID,
		Parent:   parentFullName,
	}

	// Resolve group title.
	if cmd.GroupID != "" && cmd.Parent() != nil {
		for _, g := range cmd.Parent().Groups() {
			if g.ID == cmd.GroupID {
				info.GroupTitle = g.Title
				break
			}
		}
	}

	// Collect local flags (non-inherited).
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		info.Flags = append(info.Flags, FlagInfo{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Usage:     f.Usage,
			DefValue:  f.DefValue,
			Type:      f.Value.Type(),
		})
	})

	// Collect subcommands.
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		subFull := fullName + " " + sub.Name()
		info.Subcommands = append(info.Subcommands, CommandSummary{
			Name:     sub.Name(),
			FullName: subFull,
			Short:    sub.Short,
		})
	}

	*out = append(*out, info)

	// Recurse into subcommands.
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		walkCmd(sub, fullName, out)
	}
}

// CommandByPath finds a command by its full name (e.g. "gosilo user add").
func CommandByPath(commands []CommandInfo, fullName string) *CommandInfo {
	for i := range commands {
		if commands[i].FullName == fullName {
			return &commands[i]
		}
	}
	return nil
}

// URLPath converts a full command name to a URL path segment.
// "gosilo user add" → "user/add", "gosilo" → ""
func URLPath(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) > 1 {
		return strings.Join(parts[1:], "/")
	}
	return ""
}
