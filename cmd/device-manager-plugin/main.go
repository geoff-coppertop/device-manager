package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	/* Get configuration */

	// log.SetLevel(cfg.Debug)
	log.Info("Starting")

	// log.Debug(cfg)

	// ctx, cancel := context.WithCancel(context.Background())
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	/* do the things */

	WaitProcess(&wg, nil, cancel)
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
