package cli

import (
	"fmt"
	"strings"

	"github.com/gridlhq/yeager/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// style wraps text in lipgloss styles when color is enabled.
type style struct {
	enabled bool
}

func (s style) bold(text string) string {
	if !s.enabled {
		return text
	}
	return output.BoldStyle.Render(text)
}

func (s style) cyan(text string) string {
	if !s.enabled {
		return text
	}
	return output.CyanStyle.Render(text)
}

func (s style) cyanBold(text string) string {
	if !s.enabled {
		return text
	}
	return output.BrandStyle.Render(text)
}

func (s style) green(text string) string {
	if !s.enabled {
		return text
	}
	return output.GreenStyle.Render(text)
}

func (s style) greenBold(text string) string {
	if !s.enabled {
		return text
	}
	return output.GreenStyle.Bold(true).Render(text)
}

func (s style) yellow(text string) string {
	if !s.enabled {
		return text
	}
	return output.YellowStyle.Render(text)
}

func (s style) dim(text string) string {
	if !s.enabled {
		return text
	}
	return output.DimStyle.Render(text)
}

func (s style) success(text string) string {
	if !s.enabled {
		return text
	}
	return output.SuccessStyle.Render(text)
}

// renderHelp is the custom help function for the root command.
func renderHelp(cmd *cobra.Command, _ []string) {
	w := cmd.OutOrStdout()
	s := style{enabled: output.SupportsColor(w)}

	// Header with brand emoji.
	if s.enabled {
		fmt.Fprintf(w, "🛩️ %s %s %s\n", s.cyanBold("yeager"), s.dim("—"), s.dim("remote execution for local AI coding agents"))
	} else {
		fmt.Fprintf(w, "yeager — remote execution for local AI coding agents\n")
	}
	fmt.Fprintf(w, "   %s\n", s.dim("Your laptop stays free for editing. The cloud does the compute."))
	fmt.Fprintln(w)

	// Usage.
	fmt.Fprintf(w, "%s\n", s.bold("Usage:"))
	fmt.Fprintf(w, "  %s\n", s.dim("yg <command> [args...]"))
	fmt.Fprintln(w)

	// Commands — grouped by purpose (gh-style layout).
	mainOrder := []string{"status", "logs", "kill", "stop", "up", "destroy"}
	setupOrder := []string{"configure", "init"}

	// Build name→command lookup from registered subcommands.
	subByName := make(map[string]*cobra.Command)
	for _, sub := range cmd.Commands() {
		subByName[sub.Name()] = sub
	}

	// Calculate padding based on longest command name.
	maxLen := 0
	for _, names := range [][]string{mainOrder, setupOrder} {
		for _, name := range names {
			full := "yg " + name
			if len(full) > maxLen {
				maxLen = len(full)
			}
		}
	}

	// Daily-use commands.
	fmt.Fprintf(w, "%s\n", s.bold("Commands:"))
	for _, name := range mainOrder {
		sub, ok := subByName[name]
		if !ok {
			continue
		}
		full := "yg " + name
		padded := full + strings.Repeat(" ", maxLen-len(full))
		fmt.Fprintf(w, "  %s   %s\n", s.greenBold(padded), s.dim(sub.Short))
	}
	fmt.Fprintln(w)

	// Setup commands (run once).
	fmt.Fprintf(w, "%s\n", s.bold("Setup:"))
	for _, name := range setupOrder {
		sub, ok := subByName[name]
		if !ok {
			continue
		}
		full := "yg " + name
		padded := full + strings.Repeat(" ", maxLen-len(full))
		fmt.Fprintf(w, "  %s   %s\n", s.greenBold(padded), s.dim(sub.Short))
	}
	fmt.Fprintln(w)

	// Examples.
	if cmd.HasExample() {
		fmt.Fprintf(w, "%s\n", s.bold("Examples:"))
		for _, line := range strings.Split(cmd.Example, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Split on # comment, preserving original alignment.
			if idx := strings.Index(trimmed, "#"); idx >= 0 {
				cmdPart := trimmed[:idx]
				comment := trimmed[idx:]
				fmt.Fprintf(w, "  %s%s\n", s.yellow(cmdPart), s.dim(comment))
			} else {
				fmt.Fprintf(w, "  %s\n", s.yellow(trimmed))
			}
		}
		fmt.Fprintln(w)
	}

	// Flags.
	fmt.Fprintf(w, "%s\n", s.bold("Flags:"))
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		short := ""
		if f.Shorthand != "" {
			short = fmt.Sprintf("-%s, ", f.Shorthand)
		}
		flagName := fmt.Sprintf("%s--%s", short, f.Name)
		fmt.Fprintf(w, "  %s   %s\n", s.green(rpad(flagName, 16)), s.dim(f.Usage))
	})
	fmt.Fprintln(w)

	// Footer with hints.
	fmt.Fprintf(w, "  %s  %s\n", s.dim("→ Getting started?"), s.cyan("yg configure"))
	fmt.Fprintf(w, "  %s  %s\n", s.dim("→ Need to pass flags?"), s.cyan("yg -- ls -al"))
	fmt.Fprintf(w, "  %s\n", s.dim("  Use \"yg <command> --help\" for more information about a command."))
}

// rpad right-pads a string to the given minimum width.
func rpad(s string, minWidth int) string {
	if len(s) >= minWidth {
		return s
	}
	return s + strings.Repeat(" ", minWidth-len(s))
}
