package dockersync

import (
	"context"
	"github.com/yarlson/ftl/tests"
	"io"
	"os"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"github.com/yarlson/ftl/pkg/executor/ssh"
)

const (
	testImage = "golang:1.21-alpine"
)

func setupTestImage(t *testing.T, dockerClient *client.Client) error {
	ctx := context.Background()

	// Pull test image
	reader, err := dockerClient.ImagePull(ctx, testImage, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)

	return nil
}

func TestImageSync(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set up test container
	t.Log("Setting up test container...")
	tc, err := tests.SetupTestContainer(t)
	require.NoError(t, err)
	defer func() { _ = tc.Container.Terminate(context.Background()) }()

	// Create SSH client
	t.Log("Creating SSH client...")
	sshClient, err := ssh.ConnectWithUserPassword("127.0.0.1", tc.SshPort.Port(), "root", "testpassword")
	require.NoError(t, err)
	defer sshClient.Close()

	// Create Docker client
	t.Log("Creating Docker client...")
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)
	defer dockerClient.Close()

	// Set up test image
	t.Log("Setting up test image...")
	err = setupTestImage(t, dockerClient)
	require.NoError(t, err)

	// Create temporary directories for test
	t.Log("Creating temporary directories...")
	localStore, err := os.MkdirTemp("", "dockersync-local")
	require.NoError(t, err)
	defer os.RemoveAll(localStore)

	remoteStore := "/tmp/dockersync-remote"

	// Initialize ImageSync
	cfg := Config{
		ImageName:   testImage,
		LocalStore:  localStore,
		RemoteStore: remoteStore,
		MaxParallel: 4,
	}

	sync := NewImageSync(cfg, sshClient)

	// Run sync
	t.Log("Running sync...")
	ctx := context.Background()
	err = sync.Sync(ctx)
	require.NoError(t, err)

	// Verify image exists on remote
	t.Log("Verifying image exists on remote...")
	output, err := sshClient.RunCommandOutput("docker images --format '{{.Repository}}:{{.Tag}}'")
	require.NoError(t, err)
	require.Contains(t, output, testImage)

	// Test image comparison
	t.Log("Comparing images...")
	needsSync, err := sync.compareImages(ctx)
	require.NoError(t, err)
	require.False(t, needsSync, "Images should be identical after sync")

	// Test re-sync with no changes
	t.Log("Re-syncing...")
	err = sync.Sync(ctx)
	require.NoError(t, err)
}
