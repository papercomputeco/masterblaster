package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

var (
	spinnerStyle  = lipgloss.NewStyle().Foreground(GreenColor)
	spinnerFrames = []string{"\u28fe", "\u28fd", "\u28fb", "\u28bf", "\u287f", "\u28df", "\u28ef", "\u28f7"}
)

// isTTY reports whether w is connected to a terminal.
func isTTY(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(f.Fd())
	}
	return false
}

// Step prints an animated spinner while fn runs, then replaces it with
// a checkmark or X mark and elapsed time. Matches tapes cliui.Step().
// When the writer is not a TTY (piped output, CI), the spinner is
// skipped and only the final result line is printed.
func Step(w io.Writer, msg string, fn func() error) error {
	interactive := isTTY(w)

	var done chan struct{}
	var wg sync.WaitGroup

	if interactive {
		done = make(chan struct{})
		wg.Go(func() {
			frame := 0
			ticker := time.NewTicker(80 * time.Millisecond)
			defer ticker.Stop()

			for {
				fmt.Fprintf(w, "\r  %s %s",
					spinnerStyle.Render(spinnerFrames[frame%len(spinnerFrames)]),
					msg,
				)

				select {
				case <-done:
					return
				case <-ticker.C:
					frame++
				}
			}
		})
	}

	start := time.Now()
	err := fn()
	elapsed := time.Since(start)

	if interactive {
		close(done)
		wg.Wait()
		fmt.Fprintf(w, "\r")
	}

	fmt.Fprintf(w, "  %s %s %s\n",
		Mark(err),
		msg,
		StepStyle.Render(fmt.Sprintf("(%s)", FormatDuration(elapsed))),
	)

	return err
}

// Mark returns a checkmark for nil errors or X mark for non-nil errors.
func Mark(err error) string {
	if err != nil {
		return FailMark
	}
	return SuccessMark
}

// FormatDuration formats a duration for display (e.g. "12ms" or "3.2s").
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// Spinner provides a standalone animated progress indicator for long operations
// where Step() is not suitable (e.g. the daemon serve loop).
type Spinner struct {
	message string
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation in a background goroutine.
func (s *Spinner) Start() {
	go func() {
		defer close(s.done)
		i := 0
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
				s.mu.Lock()
				fmt.Fprintf(os.Stderr, "\r  %s %s",
					spinnerStyle.Render(spinnerFrames[i%len(spinnerFrames)]),
					s.message,
				)
				s.mu.Unlock()
				i++
			}
		}
	}()
}

// Stop halts the spinner animation and clears the line.
func (s *Spinner) Stop() {
	close(s.stop)
	<-s.done
}

// StopWithMessage halts the spinner and prints a final message.
func (s *Spinner) StopWithMessage(format string, args ...interface{}) {
	s.Stop()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s\n", msg)
}
