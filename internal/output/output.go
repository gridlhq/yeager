package output

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	prefix    = "yeager | "
	separator = "─────────────────────────────────────────────"
)

// Mode controls output format.
type Mode int

const (
	ModeText Mode = iota
	ModeJSON
	ModeQuiet
)

// Writer handles all user-facing output.
type Writer struct {
	out  io.Writer
	err  io.Writer
	mode Mode
	now  func() time.Time // injectable clock for testing
}

// New creates a Writer with the given mode, writing to stdout/stderr.
func New(mode Mode) *Writer {
	return &Writer{
		out:  os.Stdout,
		err:  os.Stderr,
		mode: mode,
		now:  time.Now,
	}
}

// NewWithWriters creates a Writer with explicit output targets (for testing).
func NewWithWriters(out, errOut io.Writer, mode Mode) *Writer {
	return &Writer{
		out:  out,
		err:  errOut,
		mode: mode,
		now:  time.Now,
	}
}

// SetClock overrides the time source (for testing).
func (w *Writer) SetClock(fn func() time.Time) {
	w.now = fn
}

// Info prints a yeager-prefixed informational message.
func (w *Writer) Info(msg string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("info", msg)
	case ModeQuiet:
		// suppress
	default:
		fmt.Fprintf(w.out, "%s%s\n", prefix, msg)
	}
}

// Infof prints a formatted yeager-prefixed informational message.
func (w *Writer) Infof(format string, args ...any) {
	w.Info(fmt.Sprintf(format, args...))
}

// Error prints a yeager-prefixed error message with an optional fix suggestion.
func (w *Writer) Error(msg, fix string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSONError(msg, fix)
	default:
		fmt.Fprintf(w.err, "%serror: %s\n", prefix, msg)
		if fix != "" {
			fmt.Fprintf(w.err, "%s%s\n", prefix, fix)
		}
	}
}

// Separator prints a visual separator line.
func (w *Writer) Separator() {
	switch w.mode {
	case ModeJSON:
		// no separator in JSON mode
	case ModeQuiet:
		// suppress
	default:
		fmt.Fprintf(w.out, "%s%s\n", prefix, separator)
	}
}

// Stream writes raw command output without any prefix.
func (w *Writer) Stream(data []byte) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("output", string(data))
	default:
		w.out.Write(data) //nolint:errcheck // output writer errors are not actionable
	}
}

// StreamLine writes a single line of command output without prefix.
func (w *Writer) StreamLine(line string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("output", line)
	default:
		fmt.Fprintln(w.out, line)
	}
}

func (w *Writer) writeJSON(msgType, msg string) {
	msg = strings.TrimRight(msg, "\n")
	obj := map[string]string{
		"type":      msgType,
		"message":   msg,
		"timestamp": w.now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(obj)
	if err != nil {
		slog.Error("failed to marshal JSON output", "error", err)
		return
	}
	fmt.Fprintln(w.out, string(data))
}

func (w *Writer) writeJSONError(msg, fix string) {
	msg = strings.TrimRight(msg, "\n")
	obj := map[string]string{
		"type":      "error",
		"message":   msg,
		"timestamp": w.now().UTC().Format(time.RFC3339),
	}
	if fix != "" {
		obj["fix"] = fix
	}
	data, err := json.Marshal(obj)
	if err != nil {
		slog.Error("failed to marshal JSON output", "error", err)
		return
	}
	fmt.Fprintln(w.out, string(data))
}

// SetupSlog configures slog for the given verbosity level.
// When verbose is true, debug-level messages are shown.
func SetupSlog(verbose bool) {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}
