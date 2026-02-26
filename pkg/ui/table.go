package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StateStyle returns a lipgloss style for color-coding sandbox states.
func StateStyle(state string) lipgloss.Style {
	switch strings.ToLower(state) {
	case "running":
		return lipgloss.NewStyle().Foreground(GreenColor)
	case "stopped", "shutoff":
		return lipgloss.NewStyle().Foreground(GrayColor)
	case "error", "failed":
		return lipgloss.NewStyle().Foreground(RedColor)
	case "starting", "stopping":
		return lipgloss.NewStyle().Foreground(YellowColor)
	default:
		return lipgloss.NewStyle().Foreground(DimColor)
	}
}

// Table renders a styled table with headers and rows.
// Headers are rendered bold; the column at StateCol (if >= 0) gets color-coded.
type Table struct {
	Headers  []string
	Rows     [][]string
	StateCol int // index of the STATE column; -1 to disable color-coding
}

// Render writes the styled table to w.
func (t *Table) Render(w io.Writer) {
	if len(t.Rows) == 0 {
		return
	}

	colCount := len(t.Headers)
	widths := make([]int, colCount)
	for i, h := range t.Headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, row := range t.Rows {
		for i, cell := range row {
			if i < colCount && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Header row.
	var headerParts []string
	for i, h := range t.Headers {
		headerParts = append(headerParts, HeaderStyle.Render(padRight(h, widths[i])))
	}
	_, _ = fmt.Fprintln(w, strings.Join(headerParts, "  "))

	// Data rows.
	for _, row := range t.Rows {
		var parts []string
		for i, cell := range row {
			if i >= colCount {
				break
			}
			padded := padRight(cell, widths[i])
			switch i {
			case t.StateCol:
				parts = append(parts, StateStyle(cell).Render(padded))
			case 0:
				parts = append(parts, NameStyle.Render(padded))
			default:
				parts = append(parts, padded)
			}
		}
		_, _ = fmt.Fprintln(w, strings.Join(parts, "  "))
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
