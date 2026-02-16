package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gridlhq/yeager/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockValidateCreds(accessKeyID, secretAccessKey string) (string, error) {
	return "123456789012", nil
}

func TestRunConfigure_Flags(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
	})
	require.NoError(t, err)

	credPath := filepath.Join(homeDir, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "[default]")
	assert.Contains(t, content, "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, content, "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
}

func TestRunConfigure_Interactive(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	stdin := strings.NewReader("AKIAIOSFODNN7EXAMPLE\nwJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n")

	err := RunConfigure(ConfigureOpts{
		Mode:          output.ModeText,
		Profile:       "default",
		Stdin:         stdin,
		HomeDir:       homeDir,
		ValidateCreds: mockValidateCreds,
	})
	require.NoError(t, err)

	credPath := filepath.Join(homeDir, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "[default]")
	assert.Contains(t, content, "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, content, "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
}

func TestRunConfigure_CustomProfile(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "yeager",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
	})
	require.NoError(t, err)

	credPath := filepath.Join(homeDir, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "[yeager]")
	assert.Contains(t, content, "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
}

func TestRunConfigure_ReplaceExistingProfile(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	// Write an existing credentials file with a default profile.
	awsDir := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))
	existing := "[default]\naws_access_key_id = OLD_KEY\naws_secret_access_key = OLD_SECRET\n"
	require.NoError(t, os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(existing), 0o600))

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "NEW_KEY",
		SecretAccessKey:  "NEW_SECRET",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "aws_access_key_id = NEW_KEY")
	assert.Contains(t, content, "aws_secret_access_key = NEW_SECRET")
	assert.NotContains(t, content, "OLD_KEY")
	assert.NotContains(t, content, "OLD_SECRET")
}

func TestRunConfigure_PreservesOtherProfiles(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	awsDir := filepath.Join(homeDir, ".aws")
	require.NoError(t, os.MkdirAll(awsDir, 0o700))
	existing := "[production]\naws_access_key_id = PROD_KEY\naws_secret_access_key = PROD_SECRET\n"
	require.NoError(t, os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(existing), 0o600))

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "DEV_KEY",
		SecretAccessKey:  "DEV_SECRET",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(awsDir, "credentials"))
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "[production]")
	assert.Contains(t, content, "aws_access_key_id = PROD_KEY")
	assert.Contains(t, content, "[default]")
	assert.Contains(t, content, "aws_access_key_id = DEV_KEY")
}

func TestRunConfigure_EmptyCredentialsFails(t *testing.T) {
	t.Parallel()

	err := RunConfigure(ConfigureOpts{
		Mode:          output.ModeText,
		AccessKeyID:   "",
		SecretAccessKey: "",
		Profile:       "default",
		Stdin:         strings.NewReader("\n\n"),
		HomeDir:       t.TempDir(),
		ValidateCreds: mockValidateCreds,
	})
	assert.Error(t, err)
}

func TestRunConfigure_FilePermissions(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey:  "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
	})
	require.NoError(t, err)

	credPath := filepath.Join(homeDir, ".aws", "credentials")
	info, err := os.Stat(credPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "credentials file should be 0600")
}

func TestReplaceProfileSection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		profile  string
		wantKey  string
		dontWant string
	}{
		{
			name:     "replaces existing profile",
			input:    "[default]\naws_access_key_id = OLD\naws_secret_access_key = OLD_SECRET\n",
			profile:  "default",
			wantKey:  "aws_access_key_id = NEW_KEY",
			dontWant: "OLD",
		},
		{
			name:     "preserves other profiles",
			input:    "[other]\naws_access_key_id = KEEP\n\n[default]\naws_access_key_id = OLD\n",
			profile:  "default",
			wantKey:  "aws_access_key_id = KEEP",
			dontWant: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := replaceProfileSection(tt.input, tt.profile, "NEW_KEY", "NEW_SECRET")
			assert.Contains(t, result, tt.wantKey)
			if tt.dontWant != "" {
				assert.NotContains(t, result, tt.dontWant)
			}
		})
	}
}

func TestDisplayedErrorDetection(t *testing.T) {
	t.Parallel()

	raw := assert.AnError
	var de *displayedError
	assert.False(t, false, "raw error should not be displayedError")

	wrapped := displayed(raw)
	require.ErrorAs(t, wrapped, &de)
	assert.Equal(t, raw.Error(), wrapped.Error())

	// Double-wrapped via fmt.Errorf.
	doubleWrapped := fmt.Errorf("context: %w", wrapped)
	require.ErrorAs(t, doubleWrapped, &de)

	// Nil returns nil.
	assert.Nil(t, displayed(nil))
}

// ── New tests for guided setup, existing cred detection, and permission validation ──

func TestRunConfigure_ExistingCredsValid(t *testing.T) {
	t.Parallel()

	err := RunConfigure(ConfigureOpts{
		Mode:    output.ModeText,
		Profile: "default",
		Stdin:   strings.NewReader(""),
		HomeDir: t.TempDir(),
		CheckExisting: func() (string, error) {
			return "123456789012", nil
		},
		CheckPerms: func(_, _ string) error {
			return nil
		},
	})
	require.NoError(t, err)
}

func TestRunConfigure_ExistingCredsSkipsWrite(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	err := RunConfigure(ConfigureOpts{
		Mode:    output.ModeText,
		Profile: "default",
		Stdin:   strings.NewReader(""),
		HomeDir: homeDir,
		CheckExisting: func() (string, error) {
			return "123456789012", nil
		},
		CheckPerms: func(_, _ string) error {
			return nil
		},
	})
	require.NoError(t, err)

	// Credentials file should NOT exist — we used existing creds, nothing to write.
	credPath := filepath.Join(homeDir, ".aws", "credentials")
	_, err = os.Stat(credPath)
	assert.True(t, os.IsNotExist(err), "should not write credentials when using existing creds")
}

func TestRunConfigure_ExistingCredsBadPerms(t *testing.T) {
	t.Parallel()

	var clipboardContent string
	err := RunConfigure(ConfigureOpts{
		Mode:    output.ModeText,
		Profile: "default",
		Stdin:   strings.NewReader(""),
		HomeDir: t.TempDir(),
		CheckExisting: func() (string, error) {
			return "123456789012", nil
		},
		CheckPerms: func(_, _ string) error {
			return fmt.Errorf("AccessDenied: User is not authorized to perform ec2:DescribeInstances")
		},
		CopyClipboard: func(text string) error {
			clipboardContent = text
			return nil
		},
	})
	require.NoError(t, err) // should not error — just warns

	// IAM policy should have been copied to clipboard.
	assert.Contains(t, clipboardContent, "ec2:RunInstances")
	assert.Contains(t, clipboardContent, "s3:CreateBucket")
	assert.Contains(t, clipboardContent, "sts:GetCallerIdentity")
}

func TestRunConfigure_PermCheckWithNewCreds(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	var checkedKeyID string
	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
		CheckPerms: func(keyID, _ string) error {
			checkedKeyID = keyID
			return nil
		},
	})
	require.NoError(t, err)

	// Permission check should have used the provided credentials.
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", checkedKeyID)

	// Credentials should still be written.
	credPath := filepath.Join(homeDir, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
}

func TestRunConfigure_PermCheckFailsStillSaves(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
		CheckPerms: func(_, _ string) error {
			return fmt.Errorf("AccessDenied")
		},
	})
	require.NoError(t, err) // should not fail — creds saved, just warns

	// Credentials should still be written despite permission failure.
	credPath := filepath.Join(homeDir, ".aws", "credentials")
	data, err := os.ReadFile(credPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "aws_access_key_id = AKIAIOSFODNN7EXAMPLE")
}

func TestRunConfigure_ExistingCredsNoPermCheck(t *testing.T) {
	t.Parallel()

	// When CheckPerms is nil, existing valid creds should still return success.
	err := RunConfigure(ConfigureOpts{
		Mode:    output.ModeText,
		Profile: "default",
		Stdin:   strings.NewReader(""),
		HomeDir: t.TempDir(),
		CheckExisting: func() (string, error) {
			return "123456789012", nil
		},
		// CheckPerms intentionally nil.
	})
	require.NoError(t, err)
}

func TestRunConfigure_FlagsBypassExistingCheck(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	existingChecked := false
	err := RunConfigure(ConfigureOpts{
		Mode:            output.ModeText,
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Profile:         "default",
		Stdin:           strings.NewReader(""),
		HomeDir:         homeDir,
		ValidateCreds:   mockValidateCreds,
		CheckExisting: func() (string, error) {
			existingChecked = true
			return "123456789012", nil
		},
	})
	require.NoError(t, err)

	// When flags are provided, existing cred check should be skipped.
	assert.False(t, existingChecked, "should not check existing creds when flags are provided")
}

func TestIAMPolicyJSON(t *testing.T) {
	t.Parallel()

	// Verify the embedded IAM policy contains all required permissions.
	assert.Contains(t, iamPolicyJSON, "ec2:RunInstances")
	assert.Contains(t, iamPolicyJSON, "ec2:DescribeInstances")
	assert.Contains(t, iamPolicyJSON, "s3:CreateBucket")
	assert.Contains(t, iamPolicyJSON, "s3:PutObject")
	assert.Contains(t, iamPolicyJSON, "ec2-instance-connect:SendSSHPublicKey")
	assert.Contains(t, iamPolicyJSON, "sts:GetCallerIdentity")
	assert.Contains(t, iamPolicyJSON, "arn:aws:s3:::yeager-*")
}
