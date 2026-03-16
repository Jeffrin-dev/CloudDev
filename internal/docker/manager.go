package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

const managedLabel = "clouddev"

// Manager orchestrates docker containers used by CloudDev.
type Manager interface {
	StartContainer(ctx context.Context, opts ContainerOptions) (string, error)
	StopContainer(ctx context.Context, id string) error
	StopAll(ctx context.Context) error
	IsRunning(ctx context.Context, name string) (bool, error)
}

// ContainerOptions defines a desired container run configuration.
type ContainerOptions struct {
	Name        string
	Image       string
	PortMapping map[int]int // host:container
	EnvVars     map[string]string
	Labels      map[string]string
}

type dockerClient interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *specs.Platform, containerName string) (container.CreateResponse, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
}

type dockerManager struct {
	client dockerClient
	out    io.Writer
}

func NewManager(out io.Writer) (Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &dockerManager{client: cli, out: out}, nil
}

func NewManagerWithClient(cli dockerClient, out io.Writer) Manager {
	return &dockerManager{client: cli, out: out}
}

func (m *dockerManager) StartContainer(ctx context.Context, opts ContainerOptions) (string, error) {
	if err := m.ensureImage(ctx, opts.Image); err != nil {
		return "", err
	}

	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for hostPort, containerPort := range opts.PortMapping {
		port := nat.Port(fmt.Sprintf("%d/tcp", containerPort))
		exposedPorts[port] = struct{}{}
		portBindings[port] = []nat.PortBinding{{HostPort: strconv.Itoa(hostPort)}}
	}

	env := make([]string, 0, len(opts.EnvVars))
	for key, value := range opts.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	labels := map[string]string{managedLabel: "true"}
	for key, value := range opts.Labels {
		labels[key] = value
	}

	resp, err := m.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        opts.Image,
			ExposedPorts: exposedPorts,
			Env:          env,
			Labels:       labels,
		},
		&container.HostConfig{PortBindings: portBindings},
		nil,
		nil,
		opts.Name,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container %q: %w", opts.Name, err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container %q: %w", opts.Name, err)
	}

	return resp.ID, nil
}

func (m *dockerManager) StopContainer(ctx context.Context, id string) error {
	noWaitTimeout := 0
	stopOptions := container.StopOptions{Timeout: &noWaitTimeout}
	if err := m.client.ContainerStop(ctx, id, stopOptions); err != nil {
		return fmt.Errorf("failed to stop container %q: %w", id, err)
	}
	return nil
}

func (m *dockerManager) StopAll(ctx context.Context) error {
	args := filters.NewArgs(filters.Arg("label", managedLabel+"=true"))
	containers, err := m.client.ContainerList(ctx, container.ListOptions{Filters: args})
	if err != nil {
		return fmt.Errorf("failed to list managed containers: %w", err)
	}

	for _, c := range containers {
		if err := m.StopContainer(ctx, c.ID); err != nil {
			return err
		}
	}

	return nil
}

func (m *dockerManager) IsRunning(ctx context.Context, name string) (bool, error) {
	resp, err := m.client.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to inspect container %q: %w", name, err)
	}

	return resp.State != nil && resp.State.Running, nil
}

func (m *dockerManager) ensureImage(ctx context.Context, imageRef string) error {
	_, _, err := m.client.ImageInspectWithRaw(ctx, imageRef)
	if err == nil {
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return fmt.Errorf("failed to inspect image %q: %w", imageRef, err)
	}

	pullResp, err := m.client.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %q: %w", imageRef, err)
	}
	defer pullResp.Close()

	if _, err := io.Copy(m.out, pullResp); err != nil {
		return fmt.Errorf("failed to stream image pull output for %q: %w", imageRef, err)
	}

	return nil
}
