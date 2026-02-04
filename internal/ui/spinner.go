package ui

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Spinner provides a simple animated progress indicator for long operations.
type Spinner struct {
	message string
	frames  []string
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		frames:  []string{"|", "/", "-", "\\"},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation in a background goroutine.
func (s *Spinner) Start() {
	go func() {
		defer close(s.done)
		i := 0
		for {
			select {
			case <-s.stop:
				// Clear the spinner line
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			default:
				s.mu.Lock()
				fmt.Fprintf(os.Stderr, "\r%s %s %s", colorCyan, s.frames[i%len(s.frames)], s.message+colorReset)
				s.mu.Unlock()
				i++
				time.Sleep(100 * time.Millisecond)
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
