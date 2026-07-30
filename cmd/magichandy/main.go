// Command magichandy starts the MagicHandy local HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	setUILocale := flags.String("set-ui-locale", "", "set the app UI locale and exit")
	setChatLocale := flags.String("set-chat-locale", "", "set the built-in chat reply locale and exit")
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
	if handled, err := configureLanguagesAndExit(store, loadStatus, *setUILocale, *setChatLocale, stdout); handled {
		return err
	}
	if loadStatus.Recovered {
		logger.Warn(
			"settings or datastore recovered",
			"source", loadStatus.Source,
			"message", loadStatus.Message,
			"datastore_backup", loadStatus.DatastoreRecoveredPath,
		)
	} else if loadStatus.UsingDefaults {
		logger.Info("settings using defaults", "data_dir", loadStatus.DataDir)
	}

	runtime := httpapi.Runtime{
		Traces:         diagnostics.NewTraceRing(512),
		Transport:      transport.NewFake(),
		ExecutablePath: executablePath(),
	}

	api, err := httpapi.New(web.FS(), logger, store, runtime, httpapi.VersionInfo{
		Version: version,
		Commit:  commit,
	})
	if err != nil {
		_ = store.Close()
		return err
	}
	defer api.Close()

	server, err := newHTTPServer(listenAddress(defaults.Server.Address, settings.Server.Port, *addr), api.Handler())
	if err != nil {
		return err
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

	api.Quiesce()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	logger.Info("server stopped")

	return nil
}

func listenAddress(defaultAddress string, settingsPort int, override string) string {
	if override != "" {
		return override
	}
	if settingsPort != 0 {
		return fmt.Sprintf("127.0.0.1:%d", settingsPort)
	}
	return defaultAddress
}

func validateListenAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid HTTP listen address %q: %w", address, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf(
			"HTTP listen address %q is not loopback; LAN access is unavailable until authentication and HTTPS support are implemented",
			address,
		)
	}
	return nil
}

func newHTTPServer(address string, handler http.Handler) (*http.Server, error) {
	if err := validateListenAddress(address); err != nil {
		return nil, err
	}
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}, nil
}

func configureLanguagesAndExit(store *config.Store, loadStatus config.LoadStatus, uiLocale, chatLocale string, stdout io.Writer) (bool, error) {
	if uiLocale == "" && chatLocale == "" {
		return false, nil
	}
	configureErr := configureLanguages(store, loadStatus, uiLocale, chatLocale)
	closeErr := store.Close()
	if configureErr != nil {
		return true, configureErr
	}
	if closeErr != nil {
		return true, closeErr
	}
	_, err := fmt.Fprintf(stdout, "language settings updated: ui=%s chat=%s\n", uiLocale, chatLocale)
	return true, err
}

func configureLanguages(store *config.Store, loadStatus config.LoadStatus, uiLocale, chatLocale string) error {
	if uiLocale == "" || chatLocale == "" {
		return errors.New("set-ui-locale and set-chat-locale must be provided together")
	}
	if !config.IsSupportedLocale(uiLocale) {
		return fmt.Errorf("unsupported UI locale %q", uiLocale)
	}
	promptSet, ok := config.PromptSetForLocale(chatLocale)
	if !ok {
		return fmt.Errorf("unsupported chat locale %q", chatLocale)
	}
	if loadStatus.Recovered && loadStatus.UsingDefaults {
		return errors.New("language settings cannot be changed while recovered defaults are active; review and save recovered settings in the app first")
	}
	_, _, err := store.Update(func(settings config.Settings) (config.Settings, error) {
		settings.UI.Locale = uiLocale
		settings.LLM.PromptSet = promptSet
		return settings, nil
	})
	if err != nil {
		return fmt.Errorf("save language settings: %w", err)
	}
	return nil
}

func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}
