package main

import (
	"log/slog"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/httpapi"
	"github.com/mapledaemon/MagicHandy/web"
)

// createApplicationAPI transfers the prepared store and certificate service to
// one API graph, cleaning them up if either listener's preparation fails.
func createApplicationAPI(store *config.Store, runtime httpapi.Runtime, security serverSecurity, address string, flags remoteFlags, logger *slog.Logger) (_ *httpapi.Server, remoteSecurity serverSecurity, remoteAddress string, err error) {
	defer func() {
		if err != nil {
			if security.Automation != nil {
				security.Automation.Close()
			}
			_ = store.Close()
		}
	}()
	remoteSecurity, remoteAddress, err = prepareRemoteServer(address, security, flags)
	if err != nil {
		return nil, remoteSecurity, "", err
	}
	runtime.RemoteURL = remoteSecurity.BaseURL
	api, err := httpapi.New(web.FS(), logger, store, runtime, httpapi.VersionInfo{Version: version, Commit: commit})
	return api, remoteSecurity, remoteAddress, err
}
