package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/golang/glog"

	cfg "github.com/geoff-coppertop/device-manager-plugin/internal/config"
	fnd "github.com/geoff-coppertop/device-manager-plugin/internal/device-finder"
	srv "github.com/geoff-coppertop/device-manager-plugin/internal/device-plugin-server"
)

// init runs early to setup flag parsing which is used to configure logging
func init() {
	flag.Set("logtostderr", "true")
	flag.Set("alsologtostderr", "true")
	flag.Set("stderrthreshold", "WARNING")
	flag.Set("v", "4")
	flag.Parse()
}

// main runs all the things
func main() {
	glog.V(3).Info("Starting")

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	/* do the things */
	cfg, err := cfg.ParseConfig("config/config.yml")
	if err != nil {
		glog.Errorf("Error parsing config: %v", err)
		cancel()
	}
	glog.V(3).Infof("\nConfig\n------\n%s\n", cfg.String())

	devMappings, err := fnd.GenerateDeviceMapping(cfg)
	if err != nil {
		cancel()
	}

	errCh := make(chan error, len(devMappings))

	for _, devMap := range devMappings {
		dps := srv.NewDevicePluginServer(devMap.Paths, devMap.Group, devMap.MapSymPaths)
		dps.Run(ctx, &wg, errCh)
	}

	WaitProcess(ctx, &wg, cancel, errCh)

	close(errCh)
}

// WaitProcess waits for all process to terminate and should be run as the last call in main
func WaitProcess(ctx context.Context, wg *sync.WaitGroup, cancel context.CancelFunc, errCh <-chan error) {
	glog.V(3).Info("Waiting")

	select {
	case <-OSExit():
		glog.V(3).Info("signal caught - exiting")
		cancel()

	case <-ctx.Done():
		glog.V(3).Info("ctx channel closed")

	case err := <-errCh:
		glog.Errorf("Error reported: %v", err)
		cancel()
	}

	glog.V(3).Info("start waiting for everything to terminate")

	wg.Wait()

	glog.V(3).Info("done waiting")
}

// OSExit returns a channel that can be used to wait on OS signals for app termination
func OSExit() <-chan os.Signal {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	glog.V(3).Info("setup a channel to wait on OS signals")

	return sig
}
