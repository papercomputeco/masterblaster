// Package ui provides terminal output helpers styled with charmbracelet/lipgloss.
// Color palette and patterns match the tapes CLI (pkg/cliui).
package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Color palette (ANSI 256, matching tapes/pkg/cliui).
var (
	GreenColor  = lipgloss.Color("82")
	RedColor    = lipgloss.Color("196")
	CyanColor   = lipgloss.Color("39")
	YellowColor = lipgloss.Color("226")
	GrayColor   = lipgloss.Color("245")
	DimColor    = lipgloss.Color("241")
	LightColor  = lipgloss.Color("252")
)

// Shared styles for CLI output formatting.
var (
	SuccessMark = lipgloss.NewStyle().Foreground(GreenColor).Render("\u2713")
	FailMark    = lipgloss.NewStyle().Foreground(RedColor).Render("\u2717")

	StepStyle   = lipgloss.NewStyle().Foreground(GrayColor)
	NameStyle   = lipgloss.NewStyle().Foreground(GreenColor).Bold(true)
	DimStyle    = lipgloss.NewStyle().Foreground(DimColor)
	HashStyle   = lipgloss.NewStyle().Foreground(CyanColor)
	HeaderStyle = lipgloss.NewStyle().Foreground(LightColor).Bold(true)

	statusStyle = lipgloss.NewStyle().Foreground(CyanColor)
	warnStyle   = lipgloss.NewStyle().Foreground(YellowColor)
	errorStyle  = lipgloss.NewStyle().Foreground(RedColor)
	boldStyle   = lipgloss.NewStyle().Bold(true)
)

// Status prints a cyan status message to stderr.
func Status(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", statusStyle.Render("==>"), msg)
}

// Success prints a green checkmark + message to stderr.
func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", SuccessMark, msg)
}

// Warn prints a yellow warning to stderr.
func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s\n", warnStyle.Render(msg))
}

// Error prints a red error to stderr.
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s %s\n", FailMark, errorStyle.Render(msg))
}

// Info prints a dim indented detail line to stderr.
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "    %s\n", DimStyle.Render(msg))
}

// Bold prints a bold message to stderr.
func Bold(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s\n", boldStyle.Render(msg))
}

// Box renders content inside a rounded-border box to the given writer.
// The box fits to content width with 1-line vertical and 2-char horizontal padding,
// matching the gum style --border rounded --padding "1 2" pattern.
func Box(w io.Writer, content string, borderColor lipgloss.Color) {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2)
	_, _ = fmt.Fprintln(w, boxStyle.Render(content))
}

// Label renders a cyan label followed by a value, for use inside info panels.
func Label(label, value string) string {
	return fmt.Sprintf("%s %s",
		lipgloss.NewStyle().Foreground(CyanColor).Render(label),
		value,
	)
}
