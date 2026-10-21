package main

import (
	"log/slog"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/httpapi"
	"github.com/mapledaemon/MagicHandy/web"
)

func closePreparedStore(store *config.Store, security serverSecurity) {
	if security.Automation != nil {
		security.Automation.Close()
	}
	_ = store.Close()
}

type serverLaunchOptions struct {
	address    string
	portPolicy string
	security   serverSecurityFlags
	remote     remoteFlags
	simulate   bool
	browser    browserLaunchFlags
}

func startApplication(store *config.Store, settings config.Settings, options serverLaunchOptions, logger *slog.Logger, consoleUI *launchConsole, installerShutdown <-chan struct{}, launch *launchSession) error {
	runtime := applicationRuntime(options.simulate)
	if options.simulate {
		logger.Info("motion simulation enabled", "transport", runtime.MotionTransport.Diagnostics().Name)
	}
	launch.recorder.Report.Stage = "network_configuration"
	runtime, security, address, err := prepareServerRuntime(store, settings, options.address, options.security, runtime)
	if err != nil {
		_ = store.Close()
		return err
	}
	automatic, err := automaticLocalPort(options.portPolicy, options.address, settings, security, address, explicitNetworkFlags(options.security))
	if err != nil {
		closePreparedStore(store, security)
		return err
	}
	launch.recorder.Report.Policy = "fixed"
	if automatic {
		launch.recorder.Report.Policy = "auto"
	}
	launch.preferredURL = security.BaseURL
	listeners, security, remoteSecurity, err := bindApplicationListeners(address, security, options.remote, automatic, launch)
	if err != nil {
		closePreparedStore(store, security)
		return err
	}
	defer listeners.close()
	launch.recorder.Report.Stage = "application"
	api, err := createApplicationAPI(store, runtime, security, remoteSecurity, logger)
	if err != nil {
		return err
	}
	defer api.Close()
	consoleUI.serverPrepared(security, options.simulate)
	consoleUI.remotePrepared(remoteSecurity.BaseURL)
	consoleUI.connect(api, security.BaseURL, remoteSecurity.BaseURL)
	server := newHTTPServer(listeners.main.Addr().String(), launch.instance.Handler(api.Handler()), security.TLSConfig)
	remoteAddress := ""
	if listeners.remote != nil {
		remoteAddress = listeners.remote.Addr().String()
	}
	remoteServer := makeRemoteHTTPServer(api, remoteAddress, remoteSecurity)
	return serveUntilStopped(server, remoteServer, api, security, remoteSecurity, logger, consoleUI, installerShutdown, options.browser, listeners, launch)
}

// createApplicationAPI transfers the prepared store and certificate service to
// one API graph, cleaning them up if API construction fails.
func createApplicationAPI(store *config.Store, runtime httpapi.Runtime, security, remoteSecurity serverSecurity, logger *slog.Logger) (_ *httpapi.Server, err error) {
	defer func() {
		if err != nil {
			if security.Automation != nil {
				security.Automation.Close()
			}
			_ = store.Close()
		}
	}()
	runtime.RemoteURL = remoteSecurity.BaseURL
	runtime.NetworkPolicy = security.NetworkPolicy
	api, err := httpapi.New(web.FS(), logger, store, runtime, httpapi.VersionInfo{Version: version, Commit: commit})
	return api, err
}
