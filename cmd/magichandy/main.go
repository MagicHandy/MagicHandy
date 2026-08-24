// Command magichandy starts the MagicHandy local HTTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
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
	flags := flag.NewFlagSet("magichandy", flag.ContinueOnError)
	flags.SetOutput(stderr)

	addr := flags.String("addr", "", "HTTP listen address override")
	dataDir := flags.String("data-dir", "", "app data directory for settings and diagnostics")
	simulateMotion := flags.Bool("simulate-motion", false, "route all motion to the in-process simulator instead of a configured device")
	languageFlags := addLanguageFlags(flags)
	browserFlags := addBrowserFlags(flags)
	ttsFlags := addTTSModuleFlags(flags)
	logLevel := flags.String("log-level", "info", "structured log level: debug, info, warn, or error")
	showVersion := flags.Bool("version", false, "print version and exit")
	prepareUninstall := flags.Bool("prepare-uninstall", false, "stop the installed app and its managed workers before uninstall")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "magichandy %s (%s)\n", version, commit)
		return err
	}
	if *prepareUninstall {
		return prepareForUninstall(stdout)
	}

	level, err := logging.ParseLevel(*logLevel)
	if err != nil {
		return err
	}
	logger := logging.New(stderr, level)
	installerShutdown, closeInstallerShutdown := installerShutdownListener(logger)
	defer closeInstallerShutdown()

	store, settings, loadStatus, err := openSettingsStore(*dataDir)
	if err != nil {
		return err
	}
	if handled, err := configureLanguagesAndExit(store, loadStatus, *languageFlags.ui, *languageFlags.chat, *languageFlags.completeSetup, stdout); handled {
		return err
	}
	ttsConfiguration := ttsFlags.configuration()
	if handled, err := configureTTSModuleAndExit(store, loadStatus, ttsConfiguration, stdout); handled {
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

	runtime := applicationRuntime(*simulateMotion)
	if *simulateMotion {
		logger.Info("motion simulation enabled", "transport", runtime.MotionTransport.Diagnostics().Name)
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

	server, err := newHTTPServer(listenAddress(config.Default().Server.Address, settings.Server.Port, *addr), api.Handler())
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
	launchBrowserWhenReady(*browserFlags.open, *browserFlags.setup, server.Addr, logger)

	select {
	case <-ctx.Done():
		stopSignals()
	case <-installerShutdown:
		logger.Info("shutdown requested by Windows uninstaller")
		stopSignals()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	return shutdownHTTPServer(server, api, logger)
}

// applicationRuntime keeps motion simulation explicit. Production starts with
// no injected motion transport so the selected device owner remains
// authoritative; review and test processes can opt into a credential-free
// in-process transport that exercises the complete shared motion engine.
func applicationRuntime(simulateMotion bool) httpapi.Runtime {
	fake := transport.NewFake()
	runtime := httpapi.Runtime{
		Traces:         diagnostics.NewTraceRing(512),
		Transport:      fake,
		ExecutablePath: executablePath(),
	}
	if simulateMotion {
		runtime.MotionTransport = fake
	}
	return runtime
}

func prepareForUninstall(stdout io.Writer) error {
	requested, err := requestInstallerShutdown(30 * time.Second)
	if err != nil {
		return fmt.Errorf("prepare uninstall: %w", err)
	}
	if requested {
		_, err = fmt.Fprintln(stdout, "The running MagicHandy instance stopped cleanly.")
	} else {
		_, err = fmt.Fprintln(stdout, "No running MagicHandy instance uses this installation path.")
	}
	return err
}

func installerShutdownListener(logger *slog.Logger) (<-chan struct{}, func()) {
	requests, cleanup, err := listenForInstallerShutdown()
	if err != nil {
		logger.Warn("installer shutdown coordination unavailable", "error", err)
		return nil, func() {}
	}
	return requests, cleanup
}

func shutdownHTTPServer(server *http.Server, api *httpapi.Server, logger *slog.Logger) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	api.Quiesce()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

type languageConfigurationFlags struct {
	ui            *string
	chat          *string
	completeSetup *bool
}

func addLanguageFlags(flags *flag.FlagSet) languageConfigurationFlags {
	return languageConfigurationFlags{
		ui:            flags.String("set-ui-locale", "", "set the app UI locale and exit"),
		chat:          flags.String("set-chat-locale", "", "set the built-in chat reply locale and exit"),
		completeSetup: flags.Bool("complete-setup", false, "mark command-line setup complete when applying locales"),
	}
}

func openSettingsStore(dataDir string) (*config.Store, config.Settings, config.LoadStatus, error) {
	resolvedDataDir, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return nil, config.Settings{}, config.LoadStatus{}, err
	}
	store, err := config.OpenStore(resolvedDataDir)
	if err != nil {
		return nil, config.Settings{}, config.LoadStatus{}, err
	}
	settings, status := store.Snapshot()
	return store, settings, status, nil
}

type ttsModuleConfiguration struct {
	Provider       string
	ModuleRoot     string
	BaseURL        string
	Model          string
	Voice          string
	ResponseFormat string
	HealthPath     string
	ReferenceWAV   string
	Language       string
	Device         string
	ServerPort     int
	AutoLaunch     bool
	SpeakReplies   bool
}

type ttsModuleFlagValues struct {
	provider       *string
	moduleRoot     *string
	baseURL        *string
	model          *string
	voice          *string
	responseFormat *string
	healthPath     *string
	referenceWAV   *string
	language       *string
	device         *string
	serverPort     *int
	autoLaunch     *bool
	speakReplies   *bool
}

func addTTSModuleFlags(flags *flag.FlagSet) ttsModuleFlagValues {
	return ttsModuleFlagValues{
		provider:       flags.String("configure-tts-module", "", "configure a scripted TTS provider and exit"),
		moduleRoot:     flags.String("tts-module-root", "", "installed scripted TTS module root"),
		baseURL:        flags.String("tts-base-url", "", "OpenAI-compatible TTS server base URL"),
		model:          flags.String("tts-model", "", "TTS model identifier"),
		voice:          flags.String("tts-voice", "", "TTS voice identifier"),
		responseFormat: flags.String("tts-response-format", config.DefaultTTSResponseFormat, "TTS audio response format"),
		healthPath:     flags.String("tts-health-path", config.DefaultTTSHealthPath, "TTS server health endpoint"),
		referenceWAV:   flags.String("tts-reference-wav", "", "local Chatterbox voice reference WAV"),
		language:       flags.String("tts-language", "Auto", "TTS language"),
		device:         flags.String("tts-device", config.TTSDeviceAuto, "TTS runtime device: auto, cuda, or cpu"),
		serverPort:     flags.Int("tts-server-port", config.DefaultTTSServerPort, "managed TTS loopback port"),
		autoLaunch:     flags.Bool("tts-auto-launch", false, "start the selected TTS module with MagicHandy"),
		speakReplies:   flags.Bool("tts-speak-replies", false, "speak chat replies after module configuration"),
	}
}

func (values ttsModuleFlagValues) configuration() ttsModuleConfiguration {
	return ttsModuleConfiguration{
		Provider:       *values.provider,
		ModuleRoot:     *values.moduleRoot,
		BaseURL:        *values.baseURL,
		Model:          *values.model,
		Voice:          *values.voice,
		ResponseFormat: *values.responseFormat,
		HealthPath:     *values.healthPath,
		ReferenceWAV:   *values.referenceWAV,
		Language:       *values.language,
		Device:         *values.device,
		ServerPort:     *values.serverPort,
		AutoLaunch:     *values.autoLaunch,
		SpeakReplies:   *values.speakReplies,
	}
}

func configureTTSModuleAndExit(
	store *config.Store,
	loadStatus config.LoadStatus,
	configuration ttsModuleConfiguration,
	stdout io.Writer,
) (bool, error) {
	if strings.TrimSpace(configuration.Provider) == "" {
		return false, nil
	}
	configureErr := configureTTSModule(store, loadStatus, configuration)
	closeErr := store.Close()
	if configureErr != nil {
		return true, configureErr
	}
	if closeErr != nil {
		return true, closeErr
	}
	_, err := fmt.Fprintf(stdout, "TTS module settings updated: provider=%s auto_launch=%t\n",
		configuration.Provider, configuration.AutoLaunch)
	return true, err
}

func configureTTSModule(
	store *config.Store,
	loadStatus config.LoadStatus,
	configuration ttsModuleConfiguration,
) error {
	if loadStatus.Recovered && loadStatus.UsingDefaults {
		return errors.New("TTS settings cannot be changed while recovered defaults are active; review and save recovered settings in the app first")
	}
	switch configuration.Provider {
	case config.VoiceTTSProviderFasterQwen, config.VoiceTTSProviderChatterbox:
	default:
		return fmt.Errorf("unsupported scripted TTS provider %q", configuration.Provider)
	}
	if strings.TrimSpace(configuration.ModuleRoot) == "" {
		return errors.New("tts-module-root is required")
	}
	if configuration.ServerPort < 1 || configuration.ServerPort > 65535 {
		return errors.New("tts-server-port must be between 1 and 65535")
	}
	if configuration.BaseURL == "" {
		configuration.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", configuration.ServerPort)
	}
	if configuration.Provider == config.VoiceTTSProviderFasterQwen {
		if configuration.Model == "" {
			configuration.Model = config.DefaultFasterQwenModel
		}
		if configuration.Voice == "" {
			configuration.Voice = config.DefaultFasterQwenVoice
		}
	}
	if configuration.Provider == config.VoiceTTSProviderChatterbox {
		if configuration.Model == "" {
			configuration.Model = config.DefaultChatterboxModel
		}
		if configuration.Voice == "" {
			configuration.Voice = config.DefaultChatterboxVoice
		}
		if configuration.HealthPath == "" || configuration.HealthPath == config.DefaultTTSHealthPath {
			configuration.HealthPath = config.DefaultChatterboxHealthPath
		}
	}

	_, _, err := store.Update(func(settings config.Settings) (config.Settings, error) {
		sameProvider := settings.Voice.TTSProvider == configuration.Provider
		settings.Voice.Enabled = true
		settings.Voice.TTSProvider = configuration.Provider
		settings.Voice.TTSWorkerPath = ""
		settings.Voice.TTSWorkerArgs = nil
		settings.Voice.TTSModuleRoot = strings.TrimSpace(configuration.ModuleRoot)
		settings.Voice.TTSBaseURL = strings.TrimRight(strings.TrimSpace(configuration.BaseURL), "/")
		settings.Voice.TTSModel = strings.TrimSpace(configuration.Model)
		settings.Voice.TTSVoice = strings.TrimSpace(configuration.Voice)
		settings.Voice.TTSResponseFormat = strings.TrimSpace(configuration.ResponseFormat)
		settings.Voice.TTSHealthPath = strings.TrimSpace(configuration.HealthPath)
		if !sameProvider {
			settings.Voice.TTSReferenceWAV = ""
			settings.Voice.TTSReferenceText = ""
		}
		referenceWAV := strings.TrimSpace(configuration.ReferenceWAV)
		if configuration.Provider == config.VoiceTTSProviderChatterbox && referenceWAV != "" {
			settings.Voice.TTSReferenceWAV = referenceWAV
		}
		settings.Voice.TTSLanguage = strings.TrimSpace(configuration.Language)
		settings.Voice.TTSDevice = strings.TrimSpace(configuration.Device)
		settings.Voice.TTSServerPort = configuration.ServerPort
		settings.Voice.TTSAutoLaunch = configuration.AutoLaunch
		settings.Voice.SpeakReplies = configuration.SpeakReplies
		return settings, nil
	})
	if err != nil {
		return fmt.Errorf("save TTS module settings: %w", err)
	}
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

func configureLanguagesAndExit(store *config.Store, loadStatus config.LoadStatus, uiLocale, chatLocale string, completeSetup bool, stdout io.Writer) (bool, error) {
	if uiLocale == "" && chatLocale == "" {
		return false, nil
	}
	configureErr := configureLanguages(store, loadStatus, uiLocale, chatLocale, completeSetup)
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

func configureLanguages(store *config.Store, loadStatus config.LoadStatus, uiLocale, chatLocale string, completeSetup bool) error {
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
		settings.UI.SetupCompleted = completeSetup
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
