package ui

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestMark(t *testing.T) {
	if got := Mark(nil); got != SuccessMark {
		t.Errorf("Mark(nil) = %q, want %q", got, SuccessMark)
	}
	if got := Mark(errors.New("fail")); got != FailMark {
		t.Errorf("Mark(err) = %q, want %q", got, FailMark)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0ms"},
		{50 * time.Millisecond, "50ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{65 * time.Second, "65.0s"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.d); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestStateStyle(t *testing.T) {
	// Verify each state maps to a non-empty style render.
	states := []string{"running", "stopped", "shutoff", "error", "failed", "starting", "stopping", "unknown"}
	for _, s := range states {
		style := StateStyle(s)
		rendered := style.Render(s)
		if rendered == "" {
			t.Errorf("StateStyle(%q).Render() returned empty string", s)
		}
	}
}

func TestTableRender(t *testing.T) {
	var buf bytes.Buffer

	tbl := &Table{
		Headers:  []string{"NAME", "STATE"},
		StateCol: 1,
	}
	tbl.Rows = append(tbl.Rows, []string{"sandbox1", "running"})
	tbl.Rows = append(tbl.Rows, []string{"sandbox2", "stopped"})
	tbl.Render(&buf)

	out := buf.String()
	if out == "" {
		t.Fatal("Table.Render() produced empty output")
	}
	// Headers and data should appear in the output (may have ANSI codes).
	for _, want := range []string{"NAME", "STATE", "sandbox1", "sandbox2", "running", "stopped"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("Table.Render() missing %q in output", want)
		}
	}
}

func TestTableRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	tbl := &Table{
		Headers:  []string{"NAME"},
		StateCol: -1,
	}
	tbl.Render(&buf)
	if buf.Len() != 0 {
		t.Errorf("Table.Render() with no rows should produce no output, got %q", buf.String())
	}
}

func TestTableRenderNoState(t *testing.T) {
	var buf bytes.Buffer
	tbl := &Table{
		Headers:  []string{"NAME", "TAG", "SIZE"},
		StateCol: -1,
	}
	tbl.Rows = append(tbl.Rows, []string{"mixtape-a", "latest", "1.2 GiB"})
	tbl.Render(&buf)

	out := buf.String()
	for _, want := range []string{"NAME", "TAG", "SIZE", "mixtape-a", "latest", "1.2 GiB"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("Table.Render() missing %q in output", want)
		}
	}
}

func TestStepNonTTY(t *testing.T) {
	// bytes.Buffer is not a TTY, so Step should skip the spinner
	// and just print the result line.
	var buf bytes.Buffer
	err := Step(&buf, "test step", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("Step() returned unexpected error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("test step")) {
		t.Errorf("Step() output missing message, got %q", out)
	}
}

func TestStepNonTTYError(t *testing.T) {
	var buf bytes.Buffer
	sentinel := errors.New("step failed")
	err := Step(&buf, "failing step", func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Step() returned %v, want %v", err, sentinel)
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"hi", 5, "hi   "},
		{"hello", 5, "hello"},
		{"longer", 3, "longer"},
		{"", 4, "    "},
	}
	for _, tt := range tests {
		if got := padRight(tt.s, tt.width); got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}
