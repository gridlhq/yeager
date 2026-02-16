package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/spf13/cobra"
)

const iamConsoleURL = "https://console.aws.amazon.com/iam/home#/users/create"

// iamPolicyJSON is the minimal IAM policy required by yeager.
const iamPolicyJSON = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:DescribeInstances",
        "ec2:StartInstances",
        "ec2:StopInstances",
        "ec2:TerminateInstances",
        "ec2:CreateSecurityGroup",
        "ec2:DescribeSecurityGroups",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:CreateTags",
        "ec2:DescribeImages"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:PutBucketLifecycleConfiguration",
        "s3:HeadBucket",
        "s3:PutObject",
        "s3:GetObject"
      ],
      "Resource": ["arn:aws:s3:::yeager-*", "arn:aws:s3:::yeager-*/*"]
    },
    {
      "Effect": "Allow",
      "Action": ["ec2-instance-connect:SendSSHPublicKey"],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": ["sts:GetCallerIdentity"],
      "Resource": "*"
    }
  ]
}`

func newConfigureCmd(f *flags) *cobra.Command {
	var accessKeyID, secretAccessKey, profile string

	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Set up AWS credentials for yeager",
		Long: `Configures AWS credentials so yeager can create and manage VMs.

Writes credentials to ~/.aws/credentials. This is the same file used by
the AWS CLI and all AWS SDKs.

If you already have valid AWS credentials configured, yg configure will
detect them and verify permissions.

Interactive:
  yg configure

Non-interactive:
  yg configure --aws-access-key-id=AKIA... --aws-secret-access-key=...`,
		Example: `  yg configure
  yg configure --aws-access-key-id=AKIA... --aws-secret-access-key=...
  yg configure --profile=yeager`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunConfigure(ConfigureOpts{
				Mode:            f.outputMode(),
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
				Profile:         profile,
				Stdin:           os.Stdin,
				CheckExisting:   checkExistingAWSCreds,
				CheckPerms:      checkAWSPermissions,
				OpenURL:         openBrowser,
				CopyClipboard:   copyToClipboard,
			})
		},
	}

	cmd.Flags().StringVar(&accessKeyID, "aws-access-key-id", "", "AWS Access Key ID")
	cmd.Flags().StringVar(&secretAccessKey, "aws-secret-access-key", "", "AWS Secret Access Key")
	cmd.Flags().StringVar(&profile, "profile", "default", "AWS profile name to write")

	return cmd
}

// ConfigureOpts holds options for RunConfigure.
type ConfigureOpts struct {
	Mode            output.Mode
	AccessKeyID     string
	SecretAccessKey string
	Profile         string
	Stdin           io.Reader
	HomeDir         string                               // override for testing
	ValidateCreds   func(string, string) (string, error) // override for testing

	// Optional hooks — nil means skip that feature.
	CheckExisting func() (string, error)     // check default AWS credential chain
	CheckPerms    func(string, string) error // check EC2 permissions ("","" = default chain)
	OpenURL       func(string) error         // open URL in browser
	CopyClipboard func(string) error         // copy text to clipboard
}

// RunConfigure sets up AWS credentials for yeager.
func RunConfigure(opts ConfigureOpts) error {
	w := output.New(opts.Mode)

	accessKeyID := strings.TrimSpace(opts.AccessKeyID)
	secretAccessKey := strings.TrimSpace(opts.SecretAccessKey)

	// ── Phase 1: Check for existing credentials (when no flags provided) ──
	if accessKeyID == "" && secretAccessKey == "" && opts.CheckExisting != nil {
		w.Info("checking for existing AWS credentials...")
		accountID, err := opts.CheckExisting()
		if err == nil {
			w.Infof("found existing AWS credentials (account %s)", accountID)
			if opts.CheckPerms != nil {
				w.Info("checking permissions...")
				if permErr := opts.CheckPerms("", ""); permErr != nil {
					printPermWarning(w, opts)
					return nil
				}
			}
			w.Info("")
			w.Info("ready! try: yg echo 'hello world'")
			w.Info("")
			w.Info("to reconfigure: yg configure --aws-access-key-id=AKIA... --aws-secret-access-key=...")
			return nil
		}
		// No existing creds — continue to guided setup.
	}

	// ── Phase 2: Prompt for credentials (interactive) ──
	if accessKeyID == "" {
		printSetupGuide(w)

		prompt := "AWS Access Key ID: "
		if opts.OpenURL != nil {
			prompt = "AWS Access Key ID [Enter to open AWS console]: "
		}
		fmt.Fprint(os.Stderr, prompt)
		line, err := readLine(opts.Stdin)
		if err != nil {
			return fmt.Errorf("reading access key ID: %w", err)
		}

		// Empty input = open AWS console and copy IAM policy.
		if strings.TrimSpace(line) == "" && opts.OpenURL != nil {
			copyPolicyAndOpenConsole(w, opts)
			fmt.Fprint(os.Stderr, "\nAWS Access Key ID: ")
			line, err = readLine(opts.Stdin)
			if err != nil {
				return fmt.Errorf("reading access key ID: %w", err)
			}
		}

		accessKeyID = strings.TrimSpace(line)
	}

	if secretAccessKey == "" {
		fmt.Fprint(os.Stderr, "AWS Secret Access Key: ")
		line, err := readLine(opts.Stdin)
		if err != nil {
			return fmt.Errorf("reading secret access key: %w", err)
		}
		secretAccessKey = strings.TrimSpace(line)
	}

	if accessKeyID == "" || secretAccessKey == "" {
		w.Error("both access key ID and secret access key are required", "")
		return displayed(fmt.Errorf("missing credentials"))
	}

	// ── Phase 3: Validate credentials ──
	w.Info("validating credentials...")
	validate := opts.ValidateCreds
	if validate == nil {
		validate = validateAWSCredentials
	}
	accountID, err := validate(accessKeyID, secretAccessKey)
	if err != nil {
		w.Error("invalid credentials", "check your access key ID and secret access key")
		return displayed(err)
	}

	// ── Phase 4: Write credentials ──
	homeDir := opts.HomeDir
	if homeDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("finding home directory: %w", err)
		}
	}

	credPath := filepath.Join(homeDir, ".aws", "credentials")
	if err := writeAWSCredentials(credPath, opts.Profile, accessKeyID, secretAccessKey); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}

	w.Infof("credentials saved to %s [%s]", credPath, opts.Profile)
	w.Infof("authenticated as account %s", accountID)

	// ── Phase 5: Check permissions ──
	if opts.CheckPerms != nil {
		w.Info("checking permissions...")
		if err := opts.CheckPerms(accessKeyID, secretAccessKey); err != nil {
			printPermWarning(w, opts)
			return nil // creds saved, but permissions need fixing
		}
	}

	w.Info("")
	w.Info("ready! try: yg echo 'hello world'")
	return nil
}

// printSetupGuide displays setup instructions for new users.
func printSetupGuide(w *output.Writer) {
	w.Info("")
	w.Info("yeager launches EC2 instances in your AWS account to run commands.")
	w.Info("you need an IAM user with EC2, S3, and STS permissions.")
	w.Info("")
	w.Info("quickstart:")
	w.Info("  1. go to IAM > Users > Create user")
	w.Infof("     %s", iamConsoleURL)
	w.Info("  2. attach an inline policy (the yeager IAM policy)")
	w.Info("  3. go to Security credentials > Create access key")
	w.Info("  4. paste the credentials below")
	w.Info("")
}

// printPermWarning displays a warning when credentials are valid but lack required permissions.
func printPermWarning(w *output.Writer, opts ConfigureOpts) {
	w.Error("missing EC2 permissions", "attach the yeager IAM policy to your AWS user/role")
	w.Info("")
	w.Info("  IAM console: https://console.aws.amazon.com/iam/")
	if opts.CopyClipboard != nil {
		if err := opts.CopyClipboard(iamPolicyJSON); err == nil {
			w.Info("  IAM policy copied to clipboard — paste it as an inline policy")
		}
	}
	w.Info("")
	w.Info("after attaching the policy, try: yg echo 'hello world'")
}

// copyPolicyAndOpenConsole copies the IAM policy to clipboard and opens the AWS console.
func copyPolicyAndOpenConsole(w *output.Writer, opts ConfigureOpts) {
	if opts.CopyClipboard != nil {
		if err := opts.CopyClipboard(iamPolicyJSON); err == nil {
			w.Info("IAM policy copied to clipboard")
		}
	}
	if opts.OpenURL != nil {
		if err := opts.OpenURL(iamConsoleURL); err == nil {
			w.Info("opening AWS console...")
		}
	}
	w.Info("")
	w.Info("create a user, paste the policy, then create an access key.")
}

// checkExistingAWSCreds checks the default AWS credential chain for valid credentials.
func checkExistingAWSCreds() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", err
	}

	stsClient := sts.NewFromConfig(cfg)
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Account), nil
}

// checkAWSPermissions verifies EC2 permissions with the given credentials.
// If accessKeyID is empty, uses the default credential chain.
func checkAWSPermissions(accessKeyID, secretAccessKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	loadOpts := []func(*awsconfig.LoadOptions) error{}
	if accessKeyID != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return err
	}

	ec2Client := ec2sdk.NewFromConfig(cfg)
	_, err = ec2Client.DescribeInstances(ctx, &ec2sdk.DescribeInstancesInput{
		MaxResults: aws.Int32(5),
	})
	return err
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported platform for browser open")
	}
}

// copyToClipboard copies text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, lookupErr := exec.LookPath("xclip"); lookupErr == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, lookupErr := exec.LookPath("xsel"); lookupErr == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip or xsel)")
		}
	default:
		return fmt.Errorf("unsupported platform for clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// readLine reads a single line from r, stripping the trailing newline.
func readLine(r io.Reader) (string, error) {
	var buf bytes.Buffer
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				return buf.String(), nil
			}
			buf.WriteByte(b[0])
		}
		if err != nil {
			if buf.Len() > 0 {
				return buf.String(), nil
			}
			return "", err
		}
	}
}

// validateAWSCredentials calls STS GetCallerIdentity to verify credentials.
func validateAWSCredentials(accessKeyID, secretAccessKey string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID, secretAccessKey, "",
		)),
	)
	if err != nil {
		return "", err
	}

	stsClient := sts.NewFromConfig(cfg)
	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Account), nil
}

// writeAWSCredentials writes or updates the credentials file with the given profile.
func writeAWSCredentials(credPath, profile, accessKeyID, secretAccessKey string) error {
	// Create ~/.aws/ directory if needed.
	dir := filepath.Dir(credPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	existing, _ := os.ReadFile(credPath)

	profileHeader := fmt.Sprintf("[%s]", profile)
	if bytes.Contains(existing, []byte(profileHeader)) {
		// Replace the existing profile section.
		content := replaceProfileSection(string(existing), profile, accessKeyID, secretAccessKey)
		return atomicWriteFile(credPath, []byte(content), 0o600)
	}

	// Append new profile.
	var buf bytes.Buffer
	if len(existing) > 0 {
		buf.Write(existing)
		if !bytes.HasSuffix(existing, []byte("\n")) {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	fmt.Fprintf(&buf, "[%s]\n", profile)
	fmt.Fprintf(&buf, "aws_access_key_id = %s\n", accessKeyID)
	fmt.Fprintf(&buf, "aws_secret_access_key = %s\n", secretAccessKey)

	return atomicWriteFile(credPath, buf.Bytes(), 0o600)
}

// replaceProfileSection replaces the credentials for a profile in an INI file.
func replaceProfileSection(content, profile, accessKeyID, secretAccessKey string) string {
	lines := strings.Split(content, "\n")
	header := fmt.Sprintf("[%s]", profile)

	var result []string
	inSection := false
	replaced := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == header {
			inSection = true
			replaced = true
			result = append(result, line)
			result = append(result, fmt.Sprintf("aws_access_key_id = %s", accessKeyID))
			result = append(result, fmt.Sprintf("aws_secret_access_key = %s", secretAccessKey))
			continue
		}

		// A new section header ends the current section.
		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}

		// Skip old key lines within the section being replaced.
		if inSection {
			if strings.HasPrefix(trimmed, "aws_access_key_id") ||
				strings.HasPrefix(trimmed, "aws_secret_access_key") {
				continue
			}
			// Also skip blank lines within the replaced section.
			if trimmed == "" {
				continue
			}
		}

		result = append(result, line)
	}

	if !replaced {
		// Should not happen since we check Contains before calling, but be safe.
		result = append(result, header)
		result = append(result, fmt.Sprintf("aws_access_key_id = %s", accessKeyID))
		result = append(result, fmt.Sprintf("aws_secret_access_key = %s", secretAccessKey))
	}

	return strings.Join(result, "\n")
}

// atomicWriteFile writes data to path via a temp file + rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
