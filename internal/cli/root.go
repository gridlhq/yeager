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
			if ce := provider.ClassifyAWSError(err); ce != nil {
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
Your laptop stays free for editing. The cloud does the compute.

  yg configure        set up AWS credentials
  yg cargo test       run a command on the VM
  yg status           show VM state and active commands
  yg logs             replay + stream output from the last run
  yg stop             stop the VM (fast restart later)
  yg destroy          terminate the VM and clean up
  yg init             create a .yeager.toml with defaults`,
		Example: `  yg cargo test          # run Rust tests
  yg npm run build       # build a Node.js project
  yg go test ./...       # run Go tests
  yg pytest              # run Python tests
  yg make build          # run make targets`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			output.SetupSlog(f.verbose)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemoteCommand(cmd, args, f)
		},
	}

	root.PersistentFlags().BoolVar(&f.json, "json", false, "output in JSON format")
	root.PersistentFlags().BoolVar(&f.quiet, "quiet", false, "suppress yeager messages, show only command output")
	root.PersistentFlags().BoolVarP(&f.verbose, "verbose", "v", false, "enable debug logging")

	root.AddCommand(
		newConfigureCmd(f),
		newStatusCmd(f),
		newLogsCmd(f),
		newKillCmd(f),
		newStopCmd(f),
		newDestroyCmd(f),
		newInitCmd(f),
		newUpCmd(f),
	)

	return root
}

// runRemoteCommand handles `yg <command...>` — the variadic remote execution path.
func runRemoteCommand(cmd *cobra.Command, args []string, f *flags) error {
	if len(args) == 0 {
		return cmd.Help()
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
