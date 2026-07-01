// Command magichandy starts the MagicHandy local HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/diagnostics"
	"github.com/mapledaemon/MagicHandy/internal/httpapi"
	"github.com/mapledaemon/MagicHandy/internal/logging"
	"github.com/mapledaemon/MagicHandy/internal/transport"
	"github.com/mapledaemon/MagicHandy/web"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "magichandy: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	defaults := config.Default()

	flags := flag.NewFlagSet("magichandy", flag.ContinueOnError)
	flags.SetOutput(stderr)

	addr := flags.String("addr", "", "HTTP listen address override")
	dataDir := flags.String("data-dir", "", "app data directory for settings and diagnostics")
	logLevel := flags.String("log-level", "info", "structured log level: debug, info, warn, or error")
	showVersion := flags.Bool("version", false, "print version and exit")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "magichandy %s (%s)\n", version, commit)
		return err
	}

	level, err := logging.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	logger := logging.New(stderr, level)

	resolvedDataDir, err := config.ResolveDataDir(*dataDir)
	if err != nil {
		return err
	}
	store, err := config.OpenStore(resolvedDataDir)
	if err != nil {
		return err
	}
	settings, loadStatus := store.Snapshot()
	if loadStatus.Recovered {
		logger.Warn("settings recovered with defaults", "source", loadStatus.Source, "message", loadStatus.Message)
	} else if loadStatus.UsingDefaults {
		logger.Info("settings using defaults", "data_dir", loadStatus.DataDir)
	}

	runtime := httpapi.Runtime{
		Traces:    diagnostics.NewTraceRing(512),
		Transport: transport.NewFake(),
	}

	api, err := httpapi.New(web.FS(), logger, store, runtime, httpapi.VersionInfo{
		Version: version,
		Commit:  commit,
	})
	if err != nil {
		return err
	}

	listenAddr := defaults.Server.Address
	if settings.Server.Port != 0 {
		listenAddr = fmt.Sprintf("127.0.0.1:%d", settings.Server.Port)
	}
	if *addr != "" {
		listenAddr = *addr
	}

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		stopSignals()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api.Close()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	logger.Info("server stopped")

	return nil
}
