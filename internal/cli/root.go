package cli

import (
	"context"
	"errors"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/provider"
	"github.com/spf13/cobra"
)

// displayedError wraps an error that has already been printed to the user.
// Execute() checks for this to avoid double-printing.
type displayedError struct {
	err error
}

func (e *displayedError) Error() string { return e.err.Error() }
func (e *displayedError) Unwrap() error { return e.err }

// displayed wraps an error to mark it as already shown to the user.
func displayed(err error) error {
	if err == nil {
		return nil
	}
	return &displayedError{err: err}
}

// flags holds per-invocation flag state (no package globals).
type flags struct {
	json    bool
	quiet   bool
	verbose bool
}

func (f *flags) outputMode() output.Mode {
	if f.json {
		return output.ModeJSON
	}
	if f.quiet {
		return output.ModeQuiet
	}
	return output.ModeText
}

// exitCodeError wraps an exit code for propagation through cobra.
type exitCodeError struct {
	code int
}

func (e *exitCodeError) Error() string {
	return ""
}

// Execute runs the CLI with the given version and args. Returns exit code.
func Execute(version string, args []string) int {
	root := newRootCmd(version)
	root.SetArgs(args)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		// Check for exit code propagation from remote commands.
		var ece *exitCodeError
		if errors.As(err, &ece) {
			return ece.code
		}

		// If the error was already displayed inline, don't print again.
		var de *displayedError
		if !errors.As(err, &de) {
			// Safety net: always print something so users never see silent failures.
			w := output.New(output.ModeText)

			// Improve error message for flag parsing conflicts.
			if strings.Contains(err.Error(), "unknown shorthand flag") || strings.Contains(err.Error(), "unknown flag") {
				w.Error(err.Error(), "")
				w.Hint("use -- to pass flags to the remote command: yg -- ls -al")
			} else if ce := provider.ClassifyAWSError(err); ce != nil {
				w.Error(ce.Message, ce.Fix)
			} else {
				w.Error(err.Error(), "")
			}
		}
		return 1
	}
	return 0
}

func newRootCmd(version string) *cobra.Command {
	f := &flags{}

	root := &cobra.Command{
		Use:   "yg <command> [args...]",
		Short: "Remote execution for local AI coding agents",
		Long: `yeager runs commands on a remote Linux VM in your AWS account.
Your laptop stays free for editing. The cloud does the compute.`,
		Example: `  yg echo 'hello world'  # verify setup — run your first command
  yg cargo test          # run Rust tests
  yg npm run build       # build a Node.js project
  yg go test ./...       # run Go tests
  yg pytest              # run Python tests
  yg make build          # run make targets
  yg -- ls -al           # use -- to pass flags to remote command`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Allow arbitrary args so unrecognized commands (e.g. "yg echo hi")
		// fall through to RunE instead of erroring as "unknown command".
		Args: cobra.ArbitraryArgs,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			output.SetupSlog(f.verbose)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteCommand(cmd, args, f)
		},
	}

	root.PersistentFlags().BoolVarP(&f.json, "json", "j", false, "output in JSON format")
	root.PersistentFlags().BoolVarP(&f.quiet, "quiet", "q", false, "suppress yeager messages, show only command output")
	root.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "enable debug logging")

	root.AddCommand(
		// Daily-use commands (ordered by frequency).
		newStatusCmd(f),
		newLogsCmd(f),
		newKillCmd(f),
		newStopCmd(f),
		newUpCmd(f),
		newDestroyCmd(f),
		// Setup commands (typically run once).
		newConfigureCmd(f),
		newInitCmd(f),
		// Hidden internal commands.
		newMonitorDaemonCmd(),
	)

	root.SetHelpFunc(renderHelp)

	return root
}

// runRemoteCommand handles `yg <command...>` — the variadic remote execution path.
func runRemoteCommand(cmd *cobra.Command, args []string, f *flags) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	// Handle -- separator: everything after -- is passed verbatim to remote command.
	// This allows: yg -- ls -al (where -al won't be parsed as yeager flags).
	argsBeforeDash := cmd.ArgsLenAtDash()
	if argsBeforeDash >= 0 {
		// There was a -- separator. Use only args after it.
		args = cmd.Flags().Args()
	}

	command := strings.Join(args, " ")

	cc, err := resolveCmdContext(cmd.Context(), f.outputMode())
	if err != nil {
		return err
	}

	exitCode, err := RunCommand(cmd.Context(), cc, command)
	if err != nil {
		return err
	}

	// Propagate non-zero exit code from remote command.
	if exitCode != 0 {
		return &exitCodeError{code: exitCode}
	}
	return nil
}
