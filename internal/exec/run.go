package exec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

const (
	markerPrefix = "yg-run-"
	exitPrefix   = "yg-exit-"
	logPrefix    = "yg-log-"
	tmuxPrefix   = "yg-"
	markerDir    = "/tmp"
)

// validRunID matches exactly 8 lowercase hex characters.
var validRunID = regexp.MustCompile(`^[0-9a-f]{8}$`)

// ActiveRun represents a currently running command on the VM.
type ActiveRun struct {
	RunID     string
	Command   string
	StartTime time.Time
}

// RunID is a short unique identifier for a command execution.
type RunID string

// GenerateRunID creates a unique, human-readable run ID (8 hex chars).
func GenerateRunID() RunID {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return RunID(fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF))
	}
	return RunID(hex.EncodeToString(b))
}

// ValidateRunID checks that a run ID is valid (8 hex chars).
func ValidateRunID(id string) error {
	if !validRunID.MatchString(id) {
		return fmt.Errorf("invalid run ID %q: must be 8 hex characters", id)
	}
	return nil
}

// String returns the run ID as a string.
func (r RunID) String() string {
	return string(r)
}

// RunResult holds the outcome of a remote command execution.
type RunResult struct {
	RunID     RunID
	Command   string
	ExitCode  int
	StartTime time.Time
	EndTime   time.Time
}

// Duration returns the wall-clock duration of the run.
func (r *RunResult) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// RunOpts configures a remote command execution.
type RunOpts struct {
	Command string // the shell command to run
	WorkDir string // working directory on the VM
	RunID   RunID  // unique run identifier
}

// LogPath returns the path to the tmux log file for a run.
func LogPath(runID RunID) string {
	return fmt.Sprintf("%s/%s%s", markerDir, logPrefix, runID)
}

// TmuxSession returns the tmux session name for a run.
func TmuxSession(runID RunID) string {
	return fmt.Sprintf("%s%s", tmuxPrefix, runID)
}

// Run executes a command on the remote VM inside a tmux session.
//
// The command runs in a tmux session that:
// - Survives SSH disconnects (the core product promise)
// - Pipes all output to a log file for replay
// - Creates a marker file to track the running process
// - Captures exit code to a file for later retrieval
// - Cleans up the marker file on completion
//
// stdout and stderr writers receive real-time output streamed via tail -f.
// If the SSH connection drops, the tmux session keeps running on the VM.
func Run(client *gossh.Client, opts RunOpts, stdout, stderr io.Writer) (*RunResult, error) {
	result := &RunResult{
		RunID:     opts.RunID,
		Command:   opts.Command,
		StartTime: time.Now().UTC(),
	}

	// Step 1: Start the tmux session with the command.
	launchSession, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("creating SSH session: %w", err)
	}
	defer launchSession.Close()

	tmuxCmd := buildTmuxCommand(opts)
	if err := launchSession.Run(tmuxCmd); err != nil {
		return nil, fmt.Errorf("starting tmux session: %w", err)
	}

	// Step 2: Tail the log file to stream output back to the caller.
	// This is a separate SSH session so that if it drops, the tmux keeps running.
	tailSession, err := client.NewSession()
	if err != nil {
		// tmux is running, but we can't tail. The command will complete on its own.
		slog.Debug("could not create tail session, tmux continues in background", "error", err)
		return nil, fmt.Errorf("creating tail session: %w", err)
	}
	defer tailSession.Close()

	tailSession.Stdout = stdout
	tailSession.Stderr = stderr

	logFile := LogPath(opts.RunID)
	sessionName := TmuxSession(opts.RunID)

	// Wait for the log file to be created by the tmux session (tee opens it).
	// Then tail -f the log file. The loop waits for the tmux session to end,
	// then does a final read to capture any remaining output.
	tailCmd := fmt.Sprintf(
		`for i in $(seq 1 50); do [ -f %s ] && break; sleep 0.1; done;`+
			` tail -n +1 -f %s --pid=$$ 2>/dev/null &`+
			` TAIL_PID=$!;`+
			` while tmux has-session -t %s 2>/dev/null; do sleep 1; done;`+
			` sleep 0.5;`+
			` kill $TAIL_PID 2>/dev/null; wait $TAIL_PID 2>/dev/null; true`,
		logFile,
		logFile,
		sessionName,
	)

	err = tailSession.Run(tailCmd)
	result.EndTime = time.Now().UTC()

	// Read exit code from the file the tmux command wrote.
	exitCode, exitErr := readExitCode(client, opts.RunID)
	if exitErr != nil {
		// If we can't read the exit code and the tail also errored,
		// the SSH connection likely dropped — tmux keeps running.
		if err != nil {
			return result, fmt.Errorf("running command: %w", err)
		}
		slog.Debug("could not read exit code", "error", exitErr)
	}
	result.ExitCode = exitCode

	return result, nil
}

// readExitCode reads the exit code written by the tmux command wrapper.
func readExitCode(client *gossh.Client, runID RunID) (int, error) {
	session, err := client.NewSession()
	if err != nil {
		return 0, fmt.Errorf("creating session to read exit code: %w", err)
	}
	defer session.Close()

	exitFile := fmt.Sprintf("%s/%s%s", markerDir, exitPrefix, runID)
	output, err := session.CombinedOutput(fmt.Sprintf("cat %s 2>/dev/null", exitFile))
	if err != nil {
		return 0, fmt.Errorf("reading exit code file: %w", err)
	}

	code := 0
	trimmed := strings.TrimSpace(string(output))
	if trimmed != "" {
		if _, err := fmt.Sscanf(trimmed, "%d", &code); err != nil {
			return 0, fmt.Errorf("parsing exit code %q: %w", trimmed, err)
		}
	}
	return code, nil
}

// ReadRemoteFile reads the contents of a file on the VM via SSH.
// Returns the file contents, or an error if the file doesn't exist or can't be read.
// The path is single-quoted to prevent shell injection.
// Uses stdout only — stderr is discarded to prevent shell warnings from
// contaminating the file data.
func ReadRemoteFile(client *gossh.Client, remotePath string) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	// Single-quote the path and escape embedded single quotes for safety.
	// Redirect stderr to /dev/null so shell warnings don't mix into the data.
	cmd := fmt.Sprintf("cat '%s' 2>/dev/null", shellEscape(remotePath))
	output, err := session.Output(cmd)
	if err != nil {
		return nil, fmt.Errorf("reading remote file %s: %w", remotePath, err)
	}
	return output, nil
}

// Kill terminates a running command by killing its tmux session.
func Kill(client *gossh.Client, runID RunID) error {
	if err := ValidateRunID(runID.String()); err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	// Kill the tmux session. This sends SIGHUP to all processes in the session.
	// RunID is validated as hex-only above, so this is safe from injection.
	sessionName := TmuxSession(runID)
	cmd := fmt.Sprintf("tmux kill-session -t %s 2>/dev/null || true", sessionName)

	return session.Run(cmd)
}

// ListRuns checks for active yeager runs on the VM.
// Uses tmux session list combined with marker file metadata.
func ListRuns(client *gossh.Client) ([]ActiveRun, error) {
	if client == nil {
		return nil, fmt.Errorf("SSH client is nil")
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	// List active tmux sessions and read their marker files for metadata.
	// tmux ls gives us active sessions; marker files give us command + start time.
	cmd := fmt.Sprintf(
		`for s in $(tmux ls -F '#{session_name}' 2>/dev/null | grep '^%s'); do `+
			`RID="${s#%s}"; `+
			`MF="%s/%s${RID}"; `+
			`echo "===TMUX:${RID}"; `+
			`[ -f "$MF" ] && cat "$MF"; `+
			`done`,
		tmuxPrefix, tmuxPrefix,
		markerDir, markerPrefix,
	)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, err
	}

	return parseTmuxOutput(string(output)), nil
}

// IsRunActive checks if a tmux session exists for the given run ID.
func IsRunActive(client *gossh.Client, runID RunID) (bool, error) {
	if err := ValidateRunID(runID.String()); err != nil {
		return false, err
	}

	session, err := client.NewSession()
	if err != nil {
		return false, fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	sessionName := TmuxSession(runID)
	err = session.Run(fmt.Sprintf("tmux has-session -t %s 2>/dev/null", sessionName))
	return err == nil, nil
}

// TailLog streams the log file for an active run via SSH.
// Blocks until the tmux session ends or the context is cancelled (SSH drops).
func TailLog(client *gossh.Client, runID RunID, stdout io.Writer) error {
	if err := ValidateRunID(runID.String()); err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("creating SSH session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout

	logFile := LogPath(runID)
	sessionName := TmuxSession(runID)

	// Wait for the log file to exist, then replay from beginning and follow
	// until the tmux session ends.
	cmd := fmt.Sprintf(
		`for i in $(seq 1 50); do [ -f %s ] && break; sleep 0.1; done;`+
			` cat %s 2>/dev/null;`+
			` tail -f -n 0 %s --pid=$$ 2>/dev/null &`+
			` TAIL_PID=$!;`+
			` while tmux has-session -t %s 2>/dev/null; do sleep 1; done;`+
			` sleep 0.5;`+
			` kill $TAIL_PID 2>/dev/null; wait $TAIL_PID 2>/dev/null; true`,
		logFile,
		logFile,
		logFile,
		sessionName,
	)

	return session.Run(cmd)
}

// parseTmuxOutput parses the output of the tmux-based ListRuns command.
// Each entry is preceded by "===TMUX:<RUNID>" followed by marker file contents.
func parseTmuxOutput(output string) []ActiveRun {
	if len(output) == 0 {
		return nil
	}

	const prefix = "===TMUX:"
	var runs []ActiveRun
	var currentRunID string
	var contentLines []string

	for _, line := range splitLines(output) {
		if strings.HasPrefix(line, prefix) {
			if currentRunID != "" {
				runs = append(runs, buildActiveRun(currentRunID, contentLines))
			}
			currentRunID = strings.TrimPrefix(line, prefix)
			contentLines = nil
		} else if currentRunID != "" {
			contentLines = append(contentLines, line)
		}
	}
	if currentRunID != "" {
		runs = append(runs, buildActiveRun(currentRunID, contentLines))
	}

	return runs
}

// buildActiveRun creates an ActiveRun from parsed marker file contents.
func buildActiveRun(runID string, lines []string) ActiveRun {
	run := ActiveRun{RunID: runID}
	if len(lines) >= 1 {
		run.Command = lines[0]
	}
	if len(lines) >= 2 {
		if t, err := time.Parse(time.RFC3339, lines[1]); err == nil {
			run.StartTime = t
		}
	}
	return run
}

// buildTmuxCommand constructs the shell command that starts a tmux session on the VM.
// The tmux session runs the user command, captures output to a log file,
// writes exit code, and cleans up the marker file on completion.
func buildTmuxCommand(opts RunOpts) string {
	marker := fmt.Sprintf("%s/%s%s", markerDir, markerPrefix, opts.RunID)
	exitFile := fmt.Sprintf("%s/%s%s", markerDir, exitPrefix, opts.RunID)
	logFile := LogPath(opts.RunID)
	sessionName := TmuxSession(opts.RunID)

	// The tmux session runs a script that:
	// 1. Writes marker file with command metadata
	// 2. Runs the user command with output piped to a log file
	// 3. Writes exit code to a file
	// 4. Cleans up the marker file
	//
	// WorkDir is always the fixed remoteProjectDir (/home/ubuntu/project)
	// set by the caller, not user input. RunID is validated as hex-only.
	// Command is shell-escaped for the marker file content.
	innerScript := fmt.Sprintf(
		`cd %s && `+
			`printf '%%s\n%%s\n' '%s' "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" > %s && `+
			`bash -c '%s' 2>&1 | tee %s; `+
			`EC=${PIPESTATUS[0]}; `+
			`echo $EC > %s; `+
			`rm -f %s`,
		opts.WorkDir,
		shellEscape(opts.Command),
		marker,
		shellEscape(opts.Command),
		logFile,
		exitFile,
		marker,
	)

	// Start a detached tmux session that runs the inner script.
	// The inner script uses PIPESTATUS (bash-only) to capture exit codes
	// through the tee pipe. Ubuntu's default shell is /bin/bash, and tmux
	// inherits $SHELL, so this is safe for our target platform.
	return fmt.Sprintf(
		`tmux new-session -d -s %s '%s'`,
		sessionName,
		shellEscape(innerScript),
	)
}

// shellEscape escapes a command for use inside single-quoted bash -c.
func shellEscape(cmd string) string {
	// Replace single quotes with the standard escape: end quote, escaped quote, start quote.
	var b strings.Builder
	b.Grow(len(cmd))
	for _, c := range cmd {
		if c == '\'' {
			b.WriteString(`'\''`)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
