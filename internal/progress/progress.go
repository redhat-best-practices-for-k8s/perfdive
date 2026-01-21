package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Spinner provides an animated spinner for long-running operations
type Spinner struct {
	mu       sync.Mutex
	message  string
	frames   []string
	current  int
	running  bool
	done     chan bool
	writer   io.Writer
	verbose  bool
}

var defaultFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a new spinner with the given message
func NewSpinner(message string, verbose bool) *Spinner {
	return &Spinner{
		message: message,
		frames:  defaultFrames,
		done:    make(chan bool),
		writer:  os.Stdout,
		verbose: verbose,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	if !s.verbose {
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				s.mu.Lock()
				if s.running {
					_, _ = fmt.Fprintf(s.writer, "\r%s %s", s.frames[s.current], s.message)
					s.current = (s.current + 1) % len(s.frames)
				}
				s.mu.Unlock()
			}
		}
	}()
}

// Stop halts the spinner and clears the line
func (s *Spinner) Stop() {
	if !s.verbose {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.done)

	// Clear the spinner line
	_, _ = fmt.Fprintf(s.writer, "\r%s\r", strings.Repeat(" ", len(s.message)+10))
}

// Success stops the spinner and prints a success message
func (s *Spinner) Success(message string) {
	s.Stop()
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "✓ %s\n", message)
	}
}

// Fail stops the spinner and prints a failure message
func (s *Spinner) Fail(message string) {
	s.Stop()
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "✗ %s\n", message)
	}
}

// Update changes the spinner's message while it's running
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// Progress tracks progress of a multi-step operation
type Progress struct {
	total   int
	current int
	message string
	verbose bool
	writer  io.Writer
	mu      sync.Mutex
}

// NewProgress creates a new progress tracker
func NewProgress(total int, message string, verbose bool) *Progress {
	return &Progress{
		total:   total,
		current: 0,
		message: message,
		verbose: verbose,
		writer:  os.Stdout,
	}
}

// Increment advances the progress by one
func (p *Progress) Increment() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current++
	if p.verbose {
		p.render()
	}
}

// SetCurrent sets the current progress value
func (p *Progress) SetCurrent(current int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current
	if p.verbose {
		p.render()
	}
}

// render displays the current progress
func (p *Progress) render() {
	if p.total <= 0 {
		_, _ = fmt.Fprintf(p.writer, "\r→ %s (%d)...", p.message, p.current)
		return
	}

	percentage := float64(p.current) / float64(p.total) * 100
	barWidth := 20
	filled := int(float64(barWidth) * float64(p.current) / float64(p.total))

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	_, _ = fmt.Fprintf(p.writer, "\r→ %s [%s] %d/%d (%.0f%%)", p.message, bar, p.current, p.total, percentage)
}

// Done completes the progress and prints a final message
func (p *Progress) Done(message string) {
	if p.verbose {
		_, _ = fmt.Fprintf(p.writer, "\r%s\r", strings.Repeat(" ", 60))
		_, _ = fmt.Fprintf(p.writer, "✓ %s\n", message)
	}
}

// StatusLine provides a simple status line that can be updated
type StatusLine struct {
	verbose bool
	writer  io.Writer
}

// NewStatusLine creates a new status line
func NewStatusLine(verbose bool) *StatusLine {
	return &StatusLine{
		verbose: verbose,
		writer:  os.Stdout,
	}
}

// Print prints a status message with an arrow
func (s *StatusLine) Print(format string, args ...any) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "→ "+format+"\n", args...)
	}
}

// Success prints a success message with a checkmark
func (s *StatusLine) Success(format string, args ...any) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "  ✓ "+format+"\n", args...)
	}
}

// Info prints an info message
func (s *StatusLine) Info(format string, args ...any) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "  ℹ "+format+"\n", args...)
	}
}

// Warn prints a warning message
func (s *StatusLine) Warn(format string, args ...any) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "  ⚠ "+format+"\n", args...)
	}
}

// Error prints an error message
func (s *StatusLine) Error(format string, args ...any) {
	if s.verbose {
		_, _ = fmt.Fprintf(s.writer, "  ✗ "+format+"\n", args...)
	}
}
