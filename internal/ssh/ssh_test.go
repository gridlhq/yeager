package ssh

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	gossh "golang.org/x/crypto/ssh"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock EC2 Instance Connect ---

type mockIC struct {
	sendSSHPublicKeyFn func(ctx context.Context, params *ec2instanceconnect.SendSSHPublicKeyInput, optFns ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error)
	calls              []ec2instanceconnect.SendSSHPublicKeyInput
}

func (m *mockIC) SendSSHPublicKey(ctx context.Context, params *ec2instanceconnect.SendSSHPublicKeyInput, optFns ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error) {
	m.calls = append(m.calls, *params)
	if m.sendSSHPublicKeyFn != nil {
		return m.sendSSHPublicKeyFn(ctx, params, optFns...)
	}
	return &ec2instanceconnect.SendSSHPublicKeyOutput{}, nil
}

// --- Tests ---

func TestNewConnector(t *testing.T) {
	t.Parallel()

	ic := &mockIC{}
	c := NewConnector(ic, "us-east-1", "us-east-1a")
	assert.NotNil(t, c)
	assert.Equal(t, "us-east-1", c.region)
	assert.Equal(t, "us-east-1a", c.az)
}

func TestGenerateEphemeralKey(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := generateEphemeralKey()
	require.NoError(t, err)
	assert.NotNil(t, pubKey)
	assert.NotNil(t, privKey)
	assert.Equal(t, ed25519.PrivateKeySize, len(privKey))

	// Public key should be valid SSH format.
	authorizedKey := gossh.MarshalAuthorizedKey(pubKey)
	assert.Contains(t, string(authorizedKey), "ssh-ed25519")
}

func TestGenerateEphemeralKey_Uniqueness(t *testing.T) {
	t.Parallel()

	pub1, _, err := generateEphemeralKey()
	require.NoError(t, err)
	pub2, _, err := generateEphemeralKey()
	require.NoError(t, err)

	key1 := gossh.MarshalAuthorizedKey(pub1)
	key2 := gossh.MarshalAuthorizedKey(pub2)
	assert.NotEqual(t, key1, key2, "each key pair must be unique")
}

func TestMarshalPrivateKey(t *testing.T) {
	t.Parallel()

	_, privKey, err := generateEphemeralKey()
	require.NoError(t, err)

	pemBytes, err := MarshalPrivateKey(privKey)
	require.NoError(t, err)
	assert.Contains(t, string(pemBytes), "BEGIN OPENSSH PRIVATE KEY")
	assert.Contains(t, string(pemBytes), "END OPENSSH PRIVATE KEY")
}

func TestPushKey_Success(t *testing.T) {
	t.Parallel()

	ic := &mockIC{}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	err := c.pushKey(context.Background(), "i-test123", "ssh-ed25519 AAAA...")
	require.NoError(t, err)

	require.Len(t, ic.calls, 1)
	assert.Equal(t, "i-test123", *ic.calls[0].InstanceId)
	assert.Equal(t, "ubuntu", *ic.calls[0].InstanceOSUser)
	assert.Equal(t, "ssh-ed25519 AAAA...", *ic.calls[0].SSHPublicKey)
	assert.Equal(t, "us-east-1a", *ic.calls[0].AvailabilityZone)
}

func TestPushKey_Error(t *testing.T) {
	t.Parallel()

	ic := &mockIC{
		sendSSHPublicKeyFn: func(ctx context.Context, params *ec2instanceconnect.SendSSHPublicKeyInput, optFns ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	err := c.pushKey(context.Background(), "i-test123", "ssh-ed25519 AAAA...")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pushing SSH public key")
	assert.Contains(t, err.Error(), "access denied")
}

func TestConnect_PushKeyError(t *testing.T) {
	t.Parallel()

	ic := &mockIC{
		sendSSHPublicKeyFn: func(ctx context.Context, params *ec2instanceconnect.SendSSHPublicKeyInput, optFns ...func(*ec2instanceconnect.Options)) (*ec2instanceconnect.SendSSHPublicKeyOutput, error) {
			return nil, fmt.Errorf("unauthorized")
		},
	}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	_, err := c.Connect(context.Background(), ConnectOpts{
		InstanceID: "i-test",
		PublicIP:   "1.2.3.4",
		Port:       22,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pushing SSH public key")
}

func TestConnect_ContextCancelled(t *testing.T) {
	t.Parallel()

	ic := &mockIC{}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Connect(ctx, ConnectOpts{
		InstanceID: "i-test",
		PublicIP:   "1.2.3.4",
		Port:       22,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDial_InvalidHost(t *testing.T) {
	t.Parallel()

	_, privKey, err := generateEphemeralKey()
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromKey(privKey)
	require.NoError(t, err)

	_, err = dial("192.0.2.1", 22, signer) // RFC 5737 TEST-NET, won't connect
	require.Error(t, err)
}

func TestGenerateEphemeralKeyForSync(t *testing.T) {
	t.Parallel()

	authorizedKey, privKey, err := GenerateEphemeralKeyForSync()
	require.NoError(t, err)
	assert.Contains(t, authorizedKey, "ssh-ed25519")
	assert.NotNil(t, privKey)
	assert.Equal(t, ed25519.PrivateKeySize, len(privKey))
}

func TestPushKeyDirect(t *testing.T) {
	t.Parallel()

	ic := &mockIC{}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	err := c.PushKeyDirect(context.Background(), "i-sync001", "ssh-ed25519 BBBB...")
	require.NoError(t, err)
	require.Len(t, ic.calls, 1)
	assert.Equal(t, "i-sync001", *ic.calls[0].InstanceId)
	assert.Equal(t, "ssh-ed25519 BBBB...", *ic.calls[0].SSHPublicKey)
}

func TestConnect_ContextCancelledDuringRetry(t *testing.T) {
	t.Parallel()

	// Verify that Connect respects context cancellation during the retry loop
	// (after the initial dial failure, before re-attempting).
	ic := &mockIC{}
	c := NewConnector(ic, "us-east-1", "us-east-1a")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Connect(ctx, ConnectOpts{
		InstanceID: "i-test",
		PublicIP:   "192.0.2.1", // TEST-NET, won't connect
		Port:       22,
	})
	require.Error(t, err)
	// Should fail with context error, not "SSH connection failed after N attempts".
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
