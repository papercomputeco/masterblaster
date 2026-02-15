package ui

import (
	"fmt"
	"os"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// Status prints a cyan status message (informational progress).
func Status(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s==> %s%s\n", colorCyan, msg, colorReset)
}

// Success prints a green success message.
func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s==> %s%s\n", colorGreen, msg, colorReset)
}

// Warn prints a yellow warning message.
func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%sWarning: %s%s\n", colorYellow, msg, colorReset)
}

// Error prints a red error message.
func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%sError: %s%s\n", colorRed, msg, colorReset)
}

// Info prints an uncolored info message (for details after status messages).
func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "    %s\n", msg)
}

// Bold prints a bold message.
func Bold(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s%s%s\n", colorBold, msg, colorReset)
}
