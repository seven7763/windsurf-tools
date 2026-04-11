package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"windsurf-tools-linux/internal/api"
	"windsurf-tools-linux/internal/store"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	dataDir := flag.String("data-dir", "", "override data directory; defaults to the same WindsurfTools config directory used by the desktop project")
	readOnly := flag.Bool("read-only", false, "serve the dashboard without write operations")
	flag.Parse()

	appStore, err := store.New(*dataDir)
	if err != nil {
		log.Fatalf("load store: %v", err)
	}
	server, err := api.New(appStore, *readOnly)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("windsurf-tools-linux listening on http://%s", *addr)
	log.Printf("data directory: %s", appStore.DataDir())
	log.Printf("read-only: %t", *readOnly)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
