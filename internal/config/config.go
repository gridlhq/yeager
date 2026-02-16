package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	FileName  = ".yeager.toml"
	EnvPrefix = "YEAGER"
)

// Config is the full yeager configuration.
type Config struct {
	Compute   ComputeConfig   `mapstructure:"compute"`
	Lifecycle LifecycleConfig `mapstructure:"lifecycle"`
	Setup     SetupConfig     `mapstructure:"setup"`
	Sync      SyncConfig      `mapstructure:"sync"`
	Artifacts ArtifactsConfig `mapstructure:"artifacts"`
}

// ComputeConfig controls VM size and region.
type ComputeConfig struct {
	Size   string `mapstructure:"size"`
	Region string `mapstructure:"region"`
}

// LifecycleConfig controls VM lifecycle timers.
// Duration fields are stored as strings (e.g. "10m", "7d") for Viper compatibility.
type LifecycleConfig struct {
	IdleStop            string `mapstructure:"idle_stop"`
	StoppedTerminate    string `mapstructure:"stopped_terminate"`
	TerminatedDeleteAMI string `mapstructure:"terminated_delete_ami"`
}

// SetupConfig controls extra packages and setup commands.
type SetupConfig struct {
	Packages []string `mapstructure:"packages"`
	Run      []string `mapstructure:"run"`
}

// SyncConfig controls file sync overrides.
type SyncConfig struct {
	Include []string `mapstructure:"include"`
	Exclude []string `mapstructure:"exclude"`
}

// ArtifactsConfig controls which paths are uploaded to S3 after each run.
type ArtifactsConfig struct {
	Paths []string `mapstructure:"paths"`
}

// ParseDuration parses a duration string with support for "Nd" day syntax.
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		var days float64
		if _, err := fmt.Sscanf(numStr, "%f", &days); err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}

// IdleStopDuration returns the parsed idle_stop duration.
func (lc *LifecycleConfig) IdleStopDuration() (time.Duration, error) {
	return ParseDuration(lc.IdleStop)
}

// StoppedTerminateDuration returns the parsed stopped_terminate duration.
func (lc *LifecycleConfig) StoppedTerminateDuration() (time.Duration, error) {
	return ParseDuration(lc.StoppedTerminate)
}

// TerminatedDeleteAMIDuration returns the parsed terminated_delete_ami duration.
func (lc *LifecycleConfig) TerminatedDeleteAMIDuration() (time.Duration, error) {
	return ParseDuration(lc.TerminatedDeleteAMI)
}

// Defaults returns a Config with all default values.
func Defaults() Config {
	return Config{
		Compute: ComputeConfig{
			Size:   "medium",
			Region: "us-east-1",
		},
		Lifecycle: LifecycleConfig{
			IdleStop:            "10m",
			StoppedTerminate:    "7d",
			TerminatedDeleteAMI: "30d",
		},
	}
}

// ValidSizes is the set of allowed compute sizes.
var ValidSizes = map[string]bool{
	"small":  true,
	"medium": true,
	"large":  true,
	"xlarge": true,
}

// Validate checks the config for invalid values.
func (c *Config) Validate() error {
	if c.Compute.Size != "" && !ValidSizes[c.Compute.Size] {
		return fmt.Errorf("invalid compute.size %q (must be small, medium, large, or xlarge)", c.Compute.Size)
	}
	if c.Lifecycle.IdleStop != "" {
		if _, err := ParseDuration(c.Lifecycle.IdleStop); err != nil {
			return fmt.Errorf("invalid lifecycle.idle_stop: %w", err)
		}
	}
	if c.Lifecycle.StoppedTerminate != "" {
		if _, err := ParseDuration(c.Lifecycle.StoppedTerminate); err != nil {
			return fmt.Errorf("invalid lifecycle.stopped_terminate: %w", err)
		}
	}
	if c.Lifecycle.TerminatedDeleteAMI != "" {
		if _, err := ParseDuration(c.Lifecycle.TerminatedDeleteAMI); err != nil {
			return fmt.Errorf("invalid lifecycle.terminated_delete_ami: %w", err)
		}
	}
	return nil
}

// Load reads configuration from .yeager.toml (discovered by walking up from startDir),
// environment variables (YEAGER_*), and applies defaults.
// CLI flag overrides should be applied by the caller after Load returns.
func Load(startDir string) (Config, string, error) {
	cfg := Defaults()

	v := viper.New()
	v.SetConfigType("toml")
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setViperDefaults(v, cfg)

	configPath := FindConfig(startDir)
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return Config{}, "", fmt.Errorf("reading %s: %w", configPath, err)
		}
	}

	decoderOpt := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToBasicTypeHookFunc(),
	))
	if err := v.Unmarshal(&cfg, decoderOpt); err != nil {
		return Config{}, "", fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, "", err
	}

	return cfg, configPath, nil
}

// FindConfig walks up from startDir looking for .yeager.toml.
// Returns the path if found, empty string otherwise.
func FindConfig(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func setViperDefaults(v *viper.Viper, cfg Config) {
	v.SetDefault("compute.size", cfg.Compute.Size)
	v.SetDefault("compute.region", cfg.Compute.Region)
	v.SetDefault("lifecycle.idle_stop", cfg.Lifecycle.IdleStop)
	v.SetDefault("lifecycle.stopped_terminate", cfg.Lifecycle.StoppedTerminate)
	v.SetDefault("lifecycle.terminated_delete_ami", cfg.Lifecycle.TerminatedDeleteAMI)
}
