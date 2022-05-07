package devicepluginserver

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Device struct {
	path   string
	device *pluginapi.Device
}

type DevicePluginServer struct {
	devices      []Device
	socket       string
	resourceName string
	server       *grpc.Server
	restart      bool
	wg           *sync.WaitGroup
	healthUpd    chan bool
}

// NewDevicePluginServer returns an initialized DevicePluginServer
func NewDevicePluginServer(devicePaths []string, deviceGroupName string, wg *sync.WaitGroup) *DevicePluginServer {
	srv := DevicePluginServer{
		resourceName: "device-plugin-server/" + deviceGroupName,
		socket:       pluginapi.DevicePluginPath + "dps-" + deviceGroupName + ".sock",
		wg:           wg,
	}

	log.Infof("%s has %d paths", srv.resourceName, len(devicePaths))
	log.Infof("Paths: %s", devicePaths)

	for _, path := range devicePaths {
		srv.devices = append(srv.devices, Device{
			device: &pluginapi.Device{
				ID:     uuid.NewString(),
				Health: pluginapi.Healthy,
			},
			path: path,
		})
	}

	log.Infof("%s has %d devices", srv.resourceName, len(srv.devices))

	return &srv
}

// Run makes the plugin go, plugin can be terminated using the provided context
func (m *DevicePluginServer) Run(ctx context.Context) error {
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()
		defer m.stop()

		for {
			m.restart = false
			restartCtx, restartFunc := context.WithCancel(ctx)

			if err := m.start(restartCtx, restartFunc); err != nil {
				return
			}

			select {
			case <-restartCtx.Done():
				if m.restart {
					continue
				}
				return
			}
		}
	}()

	return nil
}

// start sets up kubelet and device watchers and registers the plugin
func (m *DevicePluginServer) start(ctx context.Context, restart context.CancelFunc) error {
	log.Info("Cleanup previous runs")

	if err := m.stop(); err != nil {
		return err
	}

	log.Info("Starting kubelet watcher")

	if err := m.startKubeletWatcher(ctx, restart); err != nil {
		return err
	}

	log.Info("Starting device watcher")

	if err := m.startDeviceWatcher(ctx); err != nil {
		return err
	}

	log.Info("Registering with Kubelet")

	if err := m.register(ctx); err != nil {
		return err
	}

	return nil
}

// stop shuts down kubelet communication and cleans up
func (m *DevicePluginServer) stop() error {
	if m.server != nil {
		log.Info("Stopping socket server")

		m.server.Stop()
		m.server = nil

		log.Info("Socket server stopped")
	}

	log.Infof("Removing file socket file: %s", m.socket)

	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	log.Info("Socket file removed")

	return nil
}

// startKubeletWatcher watches the Kubelet socket path for changes and restarts the plugin if add
// or remove is detected
func (m *DevicePluginServer) startKubeletWatcher(ctx context.Context, restart context.CancelFunc) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	m.wg.Add(1)

	go func() {
		defer m.wg.Done()
		defer watcher.Close()

		watcher.Add(pluginapi.KubeletSocket)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		select {
		case event := <-watcher.Events:
			if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
				log.Infof("inotify: %s created, restarting.", event.Name)
				m.restart = true
				restart()
			}

		case err := <-watcher.Errors:
			log.Infof("inotify: %s", err)
			m.restart = true
			restart()

		case <-ctx.Done():
			return
		}
	}()

	return nil
}

// startDeviceWatcher starts watching for changes in the device list, the list is immutable once
// started so inotify removed is treated as the device becoming unhealth, added is treated as
// becoming healthy again.
func (m *DevicePluginServer) startDeviceWatcher(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	for _, device := range m.devices {
		path := device.path

		info, err := os.Stat(path)

		mode := info.Mode()

		log.Infof("mode: %X, %X, %X", mode, fs.ModeSymlink, mode&fs.ModeSymlink)

		switch {
		case mode&fs.ModeSymlink != 0:
			log.Infof("Skipping watch on: %s", path)

			// Found symlink, follow it and see if it's pointing at a device directly
			symPath, err := os.Readlink(path)
			if err != nil {
				log.Warnf("Bad symlink: %v", err)
				return err
			}

			if !filepath.IsAbs(symPath) {
				path = filepath.Join(filepath.Dir(path), symPath)
				log.Tracef("Sympath: %s", symPath)
			}
		}

		log.Infof("Adding watch on: %s", path)

		err = watcher.Add(path)
		if err != nil {
			log.Warnf("Failed to setup watcher: %v", err)
			watcher.Close()
			return err
		}
	}

	m.wg.Add(1)

	go func() {
		defer m.wg.Done()
		defer close(m.healthUpd)
		defer watcher.Close()

		m.healthUpd = make(chan bool)

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				m.healthUpd <- false
				return

			case event := <-watcher.Events:
				log.Infof("FS watch event for: %s", event.Name)

				initHealth, err := m.getDeviceHealth(event.Name)
				if err != nil {
					return
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
					return
				}

				log.Infof("Device: %s, was: %s, now %s", event.Name, initHealth, newHealth)

				// Notify that there was a health update
				m.healthUpd <- true
			}
		}
	}()

	return nil
}

// register sets up communication between the kubelet and the device plugin for the given
// resourceName
func (m *DevicePluginServer) register(ctx context.Context) error {
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
	conn, err := m.connect(ctx, m.socket, 60*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	// Try to connect to the kubelet API
	conn, err = m.connect(ctx, pluginapi.KubeletSocket, 5*time.Second)
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

	log.Info("Registering device plugin")

	_, err = client.Register(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

// connect establishes the gRPC communication with the provided socket path
func (m *DevicePluginServer) connect(ctx context.Context, socketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	log.Infof("Connecting to socket: %s", socketPath)

	connCtx, cancel := context.WithDeadline(ctx, time.Now().Add(timeout))
	defer cancel()

	c, err := grpc.DialContext(
		connCtx,
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

// getDeviceList returns the device list
func (m *DevicePluginServer) getDeviceList() (devs []*pluginapi.Device) {
	log.Infof("There are %d devce(s)", len(m.devices))

	for _, dev := range m.devices {
		devs = append(devs, dev.device)
	}

	log.Infof("Found %d devce(s)", len(devs))

	return devs
}

// getDeviceByPath returns a Device struct for the device with the matching path in the device list
func (m *DevicePluginServer) getDeviceByPath(path string) (Device, error) {
	for _, dev := range m.devices {
		if dev.path == path {
			return dev, nil
		}
	}

	return Device{}, fmt.Errorf("device not found")
}

// getDeviceById returns a Device struct for the device with the matching id in the device list
func (m *DevicePluginServer) getDeviceById(id string) (dev Device, err error) {
	for _, d := range m.devices {
		if d.device.ID == id {
			log.Infof("Found device with ID: %s", id)
			return d, nil
		}
	}

	return Device{}, fmt.Errorf("unknown device: %s", id)
}

// setDeviceHealth returns the health of the device given by path in the device list
func (m *DevicePluginServer) getDeviceHealth(path string) (string, error) {
	dev, err := m.getDeviceByPath(path)
	if err != nil {
		return "", err
	}

	health := dev.device.GetHealth()
	log.Infof("Device: %s, is: %s", path, health)
	return health, nil
}

// setDeviceHealth sets the health of the device given by path in the device list
func (m *DevicePluginServer) setDeviceHealth(path string, health string) error {
	if health != pluginapi.Healthy && health != pluginapi.Unhealthy {
		return fmt.Errorf(fmt.Sprintf("unrecognized health: %s", health))
	}

	dev, err := m.getDeviceByPath(path)
	if err != nil {
		return err
	}

	log.Infof("Device: %s, now: %s", path, health)
	dev.device.Health = health
	return nil
}

// API Functions

// GetDevicePluginOptions returns options to be communicated with Device Manager
func (m *DevicePluginServer) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

// ListAndWatch returns a stream of List of Devices Whenever a Device state change or a Device
// disapears, ListAndWatch returns the new list
func (m *DevicePluginServer) ListAndWatch(empty *pluginapi.Empty, srv pluginapi.DevicePlugin_ListAndWatchServer) error {
	m.wg.Add(1)
	defer m.wg.Done()

	for {
		log.Info("Getting device list")

		devs := m.getDeviceList()

		log.Infof("Found %d device(s), sending to kubelet", len(devs))

		srv.Send(&pluginapi.ListAndWatchResponse{Devices: devs})

		select {
		case update, ok := <-m.healthUpd:
			if update && ok {
				continue
			}
			return fmt.Errorf("health updates ended")
		}
	}

	return nil
}

// Allocate is called during container creation so that the Device Plugin can run device specific
// operations and instruct Kubelet of the steps to make the Device available in the container
func (m *DevicePluginServer) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	log.Infof("%d container(s) requesting devices", len(req.ContainerRequests))
	resp := pluginapi.AllocateResponse{}

	for _, containerReq := range req.ContainerRequests {
		log.Infof("Container is requesting %d device(s)", len(containerReq.DevicesIDs))

		for _, id := range containerReq.DevicesIDs {
			dev, err := m.getDeviceById(id)
			if err != nil {
				return nil, err
			}

			var paths []string

			log.Infof("Adding %s to the container", dev.path)

			paths = append(paths, dev.path)

			fi, err := os.Stat(dev.path)
			if err != nil {
				return nil, err
			}

			log.Info(fi)

			if (fi.Mode() & fs.ModeSymlink) != 0 {
				log.Infof("Following symlink: %s", dev.path)
				symPath, err := os.Readlink(dev.path)
				if err != nil {
					return nil, err
				}

				if !filepath.IsAbs(symPath) {
					symPath = filepath.Join(filepath.Dir(dev.path), symPath)
					log.Infof("Sympath: %s", symPath)
				}

				paths = append(paths, symPath)
			}

			containerResp := pluginapi.ContainerAllocateResponse{}

			for _, path := range paths {
				log.Infof("path: %s", path)

				containerResp.Devices = append(containerResp.Devices, &pluginapi.DeviceSpec{
					ContainerPath: path,
					HostPath:      path,
					Permissions:   "rw",
				})
			}

			resp.ContainerResponses = append(resp.ContainerResponses, &containerResp)
		}
	}

	return &resp, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registeration phase, before
// each container start. Device plugin can run device specific operations such as reseting the
// device before making devices available to the container
func (m *DevicePluginServer) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

// GetPreferredAllocation returns a preferred set of devices to allocate from a list of available
// ones. The resulting preferred allocation is not guaranteed to be the allocation ultimately
// performed by the devicemanager. It is only designed to help the devicemanager make a more
// informed allocation decision when possible.
func (m *DevicePluginServer) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}
