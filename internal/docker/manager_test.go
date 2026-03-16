package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDockerClient struct {
	createFn       func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error)
	inspectFn      func(ctx context.Context, containerID string) (container.InspectResponse, error)
	listFn         func(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	startFn        func(ctx context.Context, containerID string, options container.StartOptions) error
	stopFn         func(ctx context.Context, containerID string, options container.StopOptions) error
	imageInspectFn func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	pullFn         func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
	return m.createFn(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return m.inspectFn(ctx, containerID)
}

func (m *mockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	return m.listFn(ctx, options)
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return m.startFn(ctx, containerID, options)
}

func (m *mockDockerClient) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return m.stopFn(ctx, containerID, options)
}

func (m *mockDockerClient) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
	return m.imageInspectFn(ctx, imageID)
}

func (m *mockDockerClient) ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	return m.pullFn(ctx, refStr, options)
}

func TestStartContainerPullsMissingImageAndAddsCloudDevLabel(t *testing.T) {
	t.Parallel()

	var createdConfig *container.Config
	var startedID string
	buf := &bytes.Buffer{}

	m := NewManagerWithClient(&mockDockerClient{
		imageInspectFn: func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, errdefs.NotFound(errors.New("not found"))
		},
		pullFn: func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBufferString("pulling...")), nil
		},
		createFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			createdConfig = config
			return container.CreateResponse{ID: "container-123"}, nil
		},
		startFn: func(ctx context.Context, containerID string, options container.StartOptions) error {
			startedID = containerID
			return nil
		},
		inspectFn: func(ctx context.Context, containerID string) (container.InspectResponse, error) {
			return container.InspectResponse{}, nil
		},
		listFn: func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) { return nil, nil },
		stopFn: func(ctx context.Context, containerID string, options container.StopOptions) error { return nil },
	}, buf)

	id, err := m.StartContainer(context.Background(), ContainerOptions{
		Name:        "clouddev-sqs",
		Image:       "softwaremill/elasticmq",
		PortMapping: map[int]int{4576: 4576},
		Labels:      map[string]string{"service": "sqs"},
	})
	require.NoError(t, err)
	assert.Equal(t, "container-123", id)
	assert.Equal(t, "container-123", startedID)
	require.NotNil(t, createdConfig)
	assert.Equal(t, "true", createdConfig.Labels[managedLabel])
	assert.Equal(t, "sqs", createdConfig.Labels["service"])
	assert.Contains(t, buf.String(), "pulling...")
}

func TestStopAllOnlyTargetsManagedContainers(t *testing.T) {
	t.Parallel()

	stopped := make([]string, 0, 2)
	m := NewManagerWithClient(&mockDockerClient{
		listFn: func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
			labels := options.Filters.Get("label")
			assert.Contains(t, labels, "clouddev=true")
			return []container.Summary{{ID: "a"}, {ID: "b"}}, nil
		},
		stopFn: func(ctx context.Context, containerID string, options container.StopOptions) error {
			require.NotNil(t, options.Timeout)
			assert.Equal(t, 0, *options.Timeout)
			stopped = append(stopped, containerID)
			return nil
		},
		createFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{}, nil
		},
		inspectFn: func(ctx context.Context, containerID string) (container.InspectResponse, error) {
			return container.InspectResponse{}, nil
		},
		startFn: func(ctx context.Context, containerID string, options container.StartOptions) error { return nil },
		imageInspectFn: func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, nil
		},
		pullFn: func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(nil)), nil
		},
	}, io.Discard)

	err := m.StopAll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, stopped)
}

func TestIsRunningReturnsFalseForMissingContainer(t *testing.T) {
	t.Parallel()

	m := NewManagerWithClient(&mockDockerClient{
		inspectFn: func(ctx context.Context, containerID string) (container.InspectResponse, error) {
			return container.InspectResponse{}, errdefs.NotFound(errors.New("missing"))
		},
		createFn: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error) {
			return container.CreateResponse{}, nil
		},
		listFn:  func(ctx context.Context, options container.ListOptions) ([]container.Summary, error) { return nil, nil },
		startFn: func(ctx context.Context, containerID string, options container.StartOptions) error { return nil },
		stopFn:  func(ctx context.Context, containerID string, options container.StopOptions) error { return nil },
		imageInspectFn: func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{}, nil, nil
		},
		pullFn: func(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(nil)), nil
		},
	}, io.Discard)

	running, err := m.IsRunning(context.Background(), "clouddev-s3")
	require.NoError(t, err)
	assert.False(t, running)
}
