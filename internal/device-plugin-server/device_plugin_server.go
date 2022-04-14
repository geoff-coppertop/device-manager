package devicepluginserver

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DeviceState int

const (
	Unallocated DeviceState = iota
	Allocated
	Error
)

type Device struct {
	path   string
	device *pluginapi.Device
}

type DevicePluginServer struct {
	devices       []Device
	deviceWatcher *fsnotify.Watcher
	socket        string
	resourceName  string
	server        *grpc.Server
	context       context.Context
	cancel        context.CancelFunc
	wg            *sync.WaitGroup
}

// NewDevicePluginServer returns an initialized DevicePluginServer
func NewDevicePluginServer(wg *sync.WaitGroup, ctx context.Context, devicePaths []string, deviceGroupName string) *DevicePluginServer {
	cancelCtx, cancel := context.WithCancel(ctx)

	srv := DevicePluginServer{
		deviceWatcher: nil,
		resourceName:  "device-plugin-server/" + deviceGroupName,
		socket:        pluginapi.DevicePluginPath + "dps" + deviceGroupName + ".sock",
		context:       cancelCtx,
		cancel:        cancel,
		wg:            wg,
	}

	for _, path := range devicePaths {
		srv.devices = append(srv.devices, Device{
			device: &pluginapi.Device{
				ID:     uuid.NewString(),
				Health: pluginapi.Healthy,
			},
			path: path,
		})
	}

	return &srv
}

func (m *DevicePluginServer) Start() (err error) {
	log.Info("Cleanup previous runs")

	if err = m.Stop(); err != nil {
		return err
	}

	log.Info("Starting device watcher")

	if err = m.setupDeviceWatcher(); err != nil {
		// Stop could fail too but we're making our best effort to cleanup as we crash and burn now
		m.Stop()
		return err
	}

	log.Info("Registering with Kubelet")

	if err = m.register(); err != nil {
		// Stop could fail too but we're making our best effort to cleanup as we crash and burn now
		m.Stop()
		return err
	}

	return nil
}

func (m *DevicePluginServer) Stop() error {
	log.Info("Stopping socket server")

	if m.server != nil {
		m.server.Stop()
		m.server = nil

		m.cancel()

		log.Info("Socket server stopped")
	}

	log.Infof("Removing file socket file: %s", m.socket)

	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	log.Info("Socket file removed")

	return nil
}

func (m *DevicePluginServer) setupDeviceWatcher() (err error) {
	log.Info("Starting FS watcher")

	// Setup and start watching the device files. If one goes away that's bad news.
	m.deviceWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, device := range m.devices {
		log.Infof("Adding watch on: %s", device.path)

		err = m.deviceWatcher.Add(device.path)
		if err != nil {
			m.deviceWatcher.Close()
			return err
		}
	}

	return nil
}

// connect establishes the gRPC communication with the registered device plugin.
func (m *DevicePluginServer) connect(socketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	log.Infof("Connecting to socket: %s", socketPath)

	ctx, cancel := context.WithDeadline(m.context, time.Now().Add(timeout))
	defer cancel()

	c, err := grpc.DialContext(
		ctx,
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithContextDialer(
			func(ctx context.Context, addr string) (net.Conn, error) {
				var d net.Dialer

				d.LocalAddr = nil
				raddr := net.UnixAddr{Name: addr, Net: "unix"}
				return d.DialContext(ctx, "unix", raddr.String())
			}),
	)

	if err != nil {
		return nil, err
	}

	log.Infof("Connected to socket: %s", socketPath)

	return c, nil
}

// Register the device plugin for the given resourceName with Kubelet.
func (m *DevicePluginServer) register() error {
	log.Infof("Creating plugin socket: %s", m.socket)

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	log.Info("Starting socket server")

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connexion
	conn, err := m.connect(m.socket, 60*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	// Try to connect to the kubelet API
	conn, err = m.connect(pluginapi.KubeletSocket, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Infof("Making device plugin registration request for: %s", m.resourceName)

	client := pluginapi.NewRegistrationClient(conn)
	req := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: m.resourceName,
	}

	log.Info("Registering device plusing")

	_, err = client.Register(m.context, req)
	if err != nil {
		return err
	}

	return nil
}

func (m *DevicePluginServer) getDeviceList() (devs []*pluginapi.Device) {
	for _, dev := range m.devices {
		devs = append(devs, dev.device)
	}

	log.Infof("Found %d devce(s)", len(devs))

	return devs
}

func (m *DevicePluginServer) getDeviceHealth(path string) (string, error) {
	for _, dev := range m.devices {
		if dev.path == path {
			health := dev.device.GetHealth()
			log.Infof("Device: %s, is: %s", path, health)
			return health, nil
		}
	}
	return "", fmt.Errorf("device not found")
}

func (m *DevicePluginServer) setDeviceHealth(path string, health string) error {
	if health != pluginapi.Healthy && health != pluginapi.Unhealthy {
		return fmt.Errorf(fmt.Sprintf("unrecognized health: %s", health))
	}

	for _, dev := range m.devices {
		if dev.path == path {
			log.Infof("Device: %s, now: %s", path, health)
			dev.device.Health = health
			return nil
		}
	}

	return fmt.Errorf("device not found")
}

func (m *DevicePluginServer) getDeviceById(id string) (dev Device, err error) {
	for _, d := range m.devices {
		if d.device.ID == id {
			log.Infof("Found device with ID: %s", id)
			return d, nil
		}
	}

	return Device{}, fmt.Errorf("unknown device: %s", id)
}

// API Functions

// GetDevicePluginOptions returns options to be communicated with Device
// Manager
func (m *DevicePluginServer) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// ListAndWatch returns a stream of List of Devices
// Whenever a Device state change or a Device disapears, ListAndWatch
// returns the new list
func (m *DevicePluginServer) ListAndWatch(empty *pluginapi.Empty, srv pluginapi.DevicePlugin_ListAndWatchServer) error {
	m.wg.Add(1)
	defer m.wg.Done()

	for {
		log.Info("Getting device list")

		devs := m.getDeviceList()

		log.Infof("Found %d device(s), sending to kubelet", len(devs))

		srv.Send(&pluginapi.ListAndWatchResponse{Devices: devs})

		select {
		case <-m.context.Done():
			log.Info("Context done")
			return nil

		case event := <-m.deviceWatcher.Events:
			log.Infof("FS watch event for: %s", event.Name)

			initHealth, err := m.getDeviceHealth(event.Name)
			if err != nil {
				return err
			}

			var newHealth = initHealth

			switch {
			case event.Op&fsnotify.Create == fsnotify.Create:
				newHealth = pluginapi.Healthy

			case event.Op&fsnotify.Remove == fsnotify.Remove:
				newHealth = pluginapi.Unhealthy

			default:
				log.Infof("Device: %s, encountered: %d", event.Name, event.Op)
				continue
			}

			if err := m.setDeviceHealth(event.Name, newHealth); err != nil {
				return err
			}

			log.Infof("Device: %s, was: %s, now %s", event.Name, initHealth, newHealth)
		}
	}

	return nil
}

// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
func (m *DevicePluginServer) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (resp *pluginapi.AllocateResponse, err error) {
	log.Infof("%d container(s) requesting devices", len(req.ContainerRequests))

	for _, containerReq := range req.ContainerRequests {
		log.Infof("\tContainer is requesting %d device(s)", len(containerReq.DevicesIDs))

		for _, id := range containerReq.DevicesIDs {
			dev, err := m.getDeviceById(id)
			if err != nil {
				return nil, err
			}

			var paths []string

			paths = append(paths, dev.path)

			fi, err := os.Stat(dev.path)
			if err != nil {
				return nil, err
			}

			if (fi.Mode() & fs.ModeSymlink) != 0 {
				symPath, err := os.Readlink(dev.path)
				if err != nil {
					return nil, err
				}

				paths = append(paths, symPath)
			}

			containerResp := pluginapi.ContainerAllocateResponse{}

			for _, path := range paths {
				log.Infof("\t\tpath: %s", path)

				containerResp.Devices = append(containerResp.Devices, &pluginapi.DeviceSpec{
					ContainerPath: path,
					HostPath:      path,
					Permissions:   "rw",
				})
			}

			resp.ContainerResponses = append(resp.ContainerResponses, &containerResp)
		}
	}

	return resp, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase,
// before each container start. Device plugin can run device specific operations
// such as reseting the device before making devices available to the container
func (m *DevicePluginServer) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *DevicePluginServer) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}
