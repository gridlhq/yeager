package output

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/lipgloss"
)

const (
	plainPrefix = "yeager | "
	separator    = "─────────────────────────────────────────────"
)

// Lipgloss styles for terminal output.
var (
	BrandStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)  // cyan bold
	DimStyle     = lipgloss.NewStyle().Faint(true)
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)  // red bold
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))             // green
	WarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))             // yellow
	BoldStyle    = lipgloss.NewStyle().Bold(true)
	CyanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))             // cyan
	GreenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	YellowStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
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
	out      io.Writer
	err      io.Writer
	mode     Mode
	now      func() time.Time // injectable clock for testing
	colorOut bool             // whether stdout supports color
	colorErr bool             // whether stderr supports color

	mu   sync.Mutex
	spin *spinner.Spinner // active spinner (nil when none)
}

// New creates a Writer with the given mode, writing to stdout/stderr.
func New(mode Mode) *Writer {
	nc := noColor()
	return &Writer{
		out:      os.Stdout,
		err:      os.Stderr,
		mode:     mode,
		now:      time.Now,
		colorOut: !nc && isTerminal(os.Stdout),
		colorErr: !nc && isTerminal(os.Stderr),
	}
}

// NewWithWriters creates a Writer with explicit output targets (for testing).
func NewWithWriters(out, errOut io.Writer, mode Mode) *Writer {
	nc := noColor()
	return &Writer{
		out:      out,
		err:      errOut,
		mode:     mode,
		now:      time.Now,
		colorOut: !nc && isTerminal(out),
		colorErr: !nc && isTerminal(errOut),
	}
}

// SetClock overrides the time source (for testing).
func (w *Writer) SetClock(fn func() time.Time) {
	w.now = fn
}

// Mode returns the current output mode.
func (w *Writer) Mode() Mode {
	return w.mode
}

// ColorOut returns whether stdout supports color.
func (w *Writer) ColorOut() bool {
	return w.colorOut
}

// WriteJSON marshals v as a single JSON object and writes it to stdout.
func (w *Writer) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Fprintln(w.out, string(data))
	return nil
}

// prefix returns the branded prefix for the given color capability.
func prefix(color bool) string {
	if color {
		return "🛩️ " + BrandStyle.Render("yeager") + " " + DimStyle.Render("·") + " "
	}
	return plainPrefix
}

// Info prints a yeager-prefixed informational message.
func (w *Writer) Info(msg string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("info", msg)
	case ModeQuiet:
		// suppress
	default:
		w.pauseSpinner()
		fmt.Fprintf(w.out, "%s%s\n", prefix(w.colorOut), msg)
		w.resumeSpinner()
	}
}

// Infof prints a formatted yeager-prefixed informational message.
func (w *Writer) Infof(format string, args ...any) {
	w.Info(fmt.Sprintf(format, args...))
}

// Success prints a yeager-prefixed success message with a green checkmark.
func (w *Writer) Success(msg string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("info", msg)
	case ModeQuiet:
		// suppress
	default:
		w.pauseSpinner()
		if w.colorOut {
			fmt.Fprintf(w.out, "🛩️ %s %s %s\n",
				BrandStyle.Render("yeager"),
				SuccessStyle.Render("✓"),
				msg)
		} else {
			fmt.Fprintf(w.out, "yeager | done: %s\n", msg)
		}
		w.resumeSpinner()
	}
}

// Warn prints a yeager-prefixed warning message with a yellow warning symbol.
func (w *Writer) Warn(msg, fix string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSONError(msg, fix)
	default:
		w.pauseSpinner()
		if w.colorErr {
			fmt.Fprintf(w.err, "🛩️ %s %s %s\n",
				BrandStyle.Render("yeager"),
				WarnStyle.Render("⚠"),
				msg)
			if fix != "" {
				fmt.Fprintf(w.err, "%s%s\n", prefix(true), DimStyle.Render(fix))
			}
		} else {
			fmt.Fprintf(w.err, "yeager | warning: %s\n", msg)
			if fix != "" {
				fmt.Fprintf(w.err, "%s%s\n", plainPrefix, fix)
			}
		}
		w.resumeSpinner()
	}
}

// Hint prints a dimmed hint message with an arrow prefix.
func (w *Writer) Hint(msg string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("info", msg)
	case ModeQuiet:
		// suppress
	default:
		w.pauseSpinner()
		if w.colorOut {
			fmt.Fprintf(w.out, "%s%s %s\n", prefix(true), DimStyle.Render("→"), DimStyle.Render(msg))
		} else {
			fmt.Fprintf(w.out, "%s→ %s\n", plainPrefix, msg)
		}
		w.resumeSpinner()
	}
}

// Error prints a yeager-prefixed error message with an optional fix suggestion.
func (w *Writer) Error(msg, fix string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSONError(msg, fix)
	default:
		w.pauseSpinner()
		if w.colorErr {
			fmt.Fprintf(w.err, "🛩️ %s %s %s\n",
				BrandStyle.Render("yeager"),
				ErrorStyle.Render("✗"),
				msg)
			if fix != "" {
				fmt.Fprintf(w.err, "%s%s\n", prefix(true), DimStyle.Render(fix))
			}
		} else {
			fmt.Fprintf(w.err, "%serror: %s\n", plainPrefix, msg)
			if fix != "" {
				fmt.Fprintf(w.err, "%s%s\n", plainPrefix, fix)
			}
		}
		w.resumeSpinner()
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
		w.pauseSpinner()
		if w.colorOut {
			fmt.Fprintf(w.out, "%s%s\n", prefix(true), DimStyle.Render(separator))
		} else {
			fmt.Fprintf(w.out, "%s%s\n", plainPrefix, separator)
		}
		w.resumeSpinner()
	}
}

// Stream writes raw command output without any prefix.
func (w *Writer) Stream(data []byte) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("output", string(data))
	default:
		w.pauseSpinner()
		w.out.Write(data) //nolint:errcheck // output writer errors are not actionable
		w.resumeSpinner()
	}
}

// StreamLine writes a single line of command output without prefix.
func (w *Writer) StreamLine(line string) {
	switch w.mode {
	case ModeJSON:
		w.writeJSON("output", line)
	default:
		w.pauseSpinner()
		fmt.Fprintln(w.out, line)
		w.resumeSpinner()
	}
}

// ── Spinner ──────────────────────────────────────────────────────

// StartSpinner starts an animated spinner with the given message.
// In non-TTY/JSON/quiet modes, falls back to a regular Info message.
func (w *Writer) StartSpinner(msg string) {
	if w.mode != ModeText || !w.colorErr {
		// Non-TTY fallback: just print as info.
		w.Info(msg)
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Stop any existing spinner first.
	if w.spin != nil {
		w.spin.Stop()
	}

	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(w.err))
	s.Prefix = "🛩️ " + BrandStyle.Render("yeager") + " "
	s.Suffix = " " + msg
	s.Start()
	w.spin = s
}

// UpdateSpinner updates the spinner's message text.
// No-op if no spinner is active.
func (w *Writer) UpdateSpinner(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.spin != nil {
		w.spin.Suffix = " " + msg
	}
}

// StopSpinner stops the spinner and prints a final status message.
// If success is true, prints with a green ✓; otherwise red ✗.
func (w *Writer) StopSpinner(msg string, success bool) {
	w.mu.Lock()
	s := w.spin
	w.spin = nil
	w.mu.Unlock()

	if s != nil {
		s.FinalMSG = "" // we'll print our own final message
		s.Stop()
	}

	if w.mode == ModeJSON {
		w.writeJSON("info", msg)
		return
	}
	if w.mode == ModeQuiet {
		return
	}

	if !w.colorErr {
		// Non-TTY fallback.
		fmt.Fprintf(w.out, "%s%s\n", plainPrefix, msg)
		return
	}

	sym := SuccessStyle.Render("✓")
	if !success {
		sym = ErrorStyle.Render("✗")
	}
	fmt.Fprintf(w.err, "🛩️ %s %s %s\n",
		BrandStyle.Render("yeager"),
		sym,
		msg)
}

// pauseSpinner temporarily stops the spinner so other output can be printed
// without visual corruption. Call resumeSpinner() after printing.
func (w *Writer) pauseSpinner() {
	w.mu.Lock()
	if w.spin != nil && w.spin.Active() {
		w.spin.Stop()
	}
	w.mu.Unlock()
}

// resumeSpinner restarts a paused spinner.
func (w *Writer) resumeSpinner() {
	w.mu.Lock()
	if w.spin != nil {
		w.spin.Start()
	}
	w.mu.Unlock()
}

// ── JSON output ──────────────────────────────────────────────────

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

// ── Utilities ────────────────────────────────────────────────────

// isTerminal checks if w is a terminal file descriptor.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// noColor returns true if the NO_COLOR env var is set (https://no-color.org/).
func noColor() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

// SupportsColor returns true if the given writer is a color-capable terminal.
func SupportsColor(w io.Writer) bool {
	return !noColor() && isTerminal(w)
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
