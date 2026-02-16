package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	gossh "golang.org/x/crypto/ssh"
)

const (
	sshUser         = "ubuntu"
	keyValidWindow  = 55 * time.Second // push key, connect within 60s window
	keepAliveInterval = 30 * time.Second
	connectTimeout  = 10 * time.Second
	maxRetries      = 12              // retry for ~60s total
	retryDelay      = 5 * time.Second
)

// EC2InstanceConnectAPI is the subset of the EC2 Instance Connect client we use.
type EC2InstanceConnectAPI interface {
	SendSSHPublicKey(ctx context.Context, params *ec2instanceconnect.SendSSHPublicKeyInput, optFns ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error)
}

// Connector manages SSH connections to EC2 instances via EC2 Instance Connect.
type Connector struct {
	ic         EC2InstanceConnectAPI
	region     string
	az         string // availability zone (required by SendSSHPublicKey)
}

// NewConnector creates a Connector with the given EC2 Instance Connect client.
func NewConnector(ic EC2InstanceConnectAPI, region, az string) *Connector {
	return &Connector{ic: ic, region: region, az: az}
}

// ConnectOpts configures an SSH connection attempt.
type ConnectOpts struct {
	InstanceID string
	PublicIP   string
	Port       int // 22 or 443
}

// Connect establishes an SSH connection to the instance.
// Generates an ephemeral Ed25519 key pair, pushes the public key via
// EC2 Instance Connect, then dials SSH with the private key.
func (c *Connector) Connect(ctx context.Context, opts ConnectOpts) (*gossh.Client, error) {
	// Generate ephemeral key pair.
	pubKey, privKey, err := generateEphemeralKey()
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral key: %w", err)
	}

	// Push public key to instance metadata.
	authorizedKey := string(gossh.MarshalAuthorizedKey(pubKey))
	if err := c.pushKey(ctx, opts.InstanceID, authorizedKey); err != nil {
		return nil, err
	}

	// Parse private key for SSH client.
	signer, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("creating SSH signer: %w", err)
	}

	// Try connecting, with retries for cloud-init not ready yet.
	var client *gossh.Client
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		client, err = dial(opts.PublicIP, opts.Port, signer)
		if err == nil {
			break
		}

		slog.Debug("SSH connect attempt failed, retrying",
			"attempt", attempt+1,
			"error", err,
		)

		// If the key has expired (60s window), push a new one.
		if attempt > 0 && attempt%3 == 0 {
			pubKey, privKey, err = generateEphemeralKey()
			if err != nil {
				return nil, fmt.Errorf("regenerating ephemeral key: %w", err)
			}
			authorizedKey = string(gossh.MarshalAuthorizedKey(pubKey))
			if err := c.pushKey(ctx, opts.InstanceID, authorizedKey); err != nil {
				return nil, err
			}
			signer, err = gossh.NewSignerFromKey(privKey)
			if err != nil {
				return nil, fmt.Errorf("creating SSH signer: %w", err)
			}
		}

		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed after %d attempts: %w", maxRetries, err)
	}

	// Start keepalive goroutine.
	go keepAlive(client, keepAliveInterval)

	return client, nil
}

// ConnectWithFallback tries port 22, falls back to 443.
func (c *Connector) ConnectWithFallback(ctx context.Context, instanceID, publicIP string) (*gossh.Client, error) {
	client, err := c.Connect(ctx, ConnectOpts{
		InstanceID: instanceID,
		PublicIP:   publicIP,
		Port:       22,
	})
	if err != nil {
		slog.Debug("port 22 failed, trying 443", "error", err)
		return c.Connect(ctx, ConnectOpts{
			InstanceID: instanceID,
			PublicIP:   publicIP,
			Port:       443,
		})
	}
	return client, nil
}

// pushKey sends the ephemeral public key to the instance via EC2 Instance Connect.
func (c *Connector) pushKey(ctx context.Context, instanceID, authorizedKey string) error {
	_, err := c.ic.SendSSHPublicKey(ctx, &ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:       aws.String(instanceID),
		InstanceOSUser:   aws.String(sshUser),
		SSHPublicKey:     aws.String(authorizedKey),
		AvailabilityZone: aws.String(c.az),
	})
	if err != nil {
		return fmt.Errorf("pushing SSH public key via EC2 Instance Connect: %w", err)
	}
	slog.Debug("pushed ephemeral SSH key", "instance_id", instanceID)
	return nil
}

// GenerateEphemeralKeyForSync creates an Ed25519 key pair and returns the
// SSH authorized-key string and the private key. Used for rsync authentication.
func GenerateEphemeralKeyForSync() (authorizedKey string, privKey ed25519.PrivateKey, err error) {
	pubKey, priv, err := generateEphemeralKey()
	if err != nil {
		return "", nil, err
	}
	return string(gossh.MarshalAuthorizedKey(pubKey)), priv, nil
}

// PushKeyDirect pushes an SSH public key via EC2 Instance Connect.
// Used for rsync where we need to push the key separately from connecting.
func (c *Connector) PushKeyDirect(ctx context.Context, instanceID, authorizedKey string) error {
	return c.pushKey(ctx, instanceID, authorizedKey)
}

// generateEphemeralKey creates an Ed25519 key pair in memory.
func generateEphemeralKey() (gossh.PublicKey, ed25519.PrivateKey, error) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	pubKey, err := gossh.NewPublicKey(privKey.Public())
	if err != nil {
		return nil, nil, err
	}
	return pubKey, privKey, nil
}

// MarshalPrivateKey returns the PEM-encoded private key (for rsync -i flag).
func MarshalPrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	// ed25519 keys use OpenSSH format.
	pemBlock, err := gossh.MarshalPrivateKey(key, "")
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// dial creates an SSH connection to host:port with the given signer.
func dial(host string, port int, signer gossh.Signer) (*gossh.Client, error) {
	config := &gossh.ClientConfig{
		User: sshUser,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // ephemeral instances, no TOFU
		Timeout:         connectTimeout,
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	return gossh.Dial("tcp", addr, config)
}

// keepAlive sends periodic keep-alive requests on the SSH connection.
// Runs until the connection is closed.
func keepAlive(client *gossh.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		_, _, err := client.SendRequest("keepalive@yeager", true, nil)
		if err != nil {
			return // connection closed
		}
	}
}
