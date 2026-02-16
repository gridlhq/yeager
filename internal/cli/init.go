package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gridlhq/yeager/internal/config"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/spf13/cobra"
)

func newInitCmd(f *flags) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a .yeager.toml with sensible defaults",
		Long: `Creates a .yeager.toml in the current directory with all settings
commented out. Uncomment and modify values you want to change.

Running yg without init works fine — this is only needed if you want
to customize settings before your first run.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			return RunInit(cwd, force, f.outputMode())
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .yeager.toml")

	return cmd
}

// RunInit creates a .yeager.toml in the given directory.
func RunInit(dir string, force bool, mode output.Mode) error {
	w := output.New(mode)
	target := filepath.Join(dir, config.FileName)

	if !force {
		if _, err := os.Stat(target); err == nil {
			w.Error(
				fmt.Sprintf("%s already exists", config.FileName),
				"use --force to overwrite",
			)
			return fmt.Errorf("%s already exists (use --force to overwrite)", config.FileName)
		}
	}

	if err := os.WriteFile(target, []byte(config.Template), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.FileName, err)
	}

	w.Infof("created %s", config.FileName)
	return nil
}
