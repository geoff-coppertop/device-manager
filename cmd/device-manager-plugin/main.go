package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	cfg "github.com/geoff-coppertop/device-manager-plugin/internal/config"
	fnd "github.com/geoff-coppertop/device-manager-plugin/internal/device-finder"
	srv "github.com/geoff-coppertop/device-manager-plugin/internal/device-plugin-server"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	// log.SetLevel(cfg.Debug)
	log.Info("Starting")

	// log.Debug(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	/* do the things */
	cfg, err := cfg.ParseConfig("config/config.yml")
	if err != nil {
		// log.Error("Oh shit")
		cancel()
	}
	log.Debugf("\nConfig\n------\n%s", cfg.String())

	devMappings, err := fnd.GenerateDeviceMapping(cfg)
	if err != nil {
		cancel()
	}

	for _, devMap := range devMappings {
		dps := srv.NewDevicePluginServer(devMap.Paths, devMap.Group, &wg)
		if err = dps.Run(ctx); err != nil {
			cancel()
			break
		}
	}

	WaitProcess(&wg, ctx.Done(), cancel)
}

func WaitProcess(wg *sync.WaitGroup, ch <-chan struct{}, cancel context.CancelFunc) {
	log.Info("Waiting")

	select {
	case <-OSExit():
		log.Info("signal caught - exiting")

	case <-ch:
		log.Errorf("uh-oh")
	}

	cancel()

	log.Info("cancelled")

	wg.Wait()

	log.Info("goodbye")
}

func OSExit() <-chan os.Signal {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)

	return sig
}
