package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type browserLaunchFlags struct {
	open  *bool
	setup *bool
}

func addBrowserFlags(flags *flag.FlagSet) browserLaunchFlags {
	return browserLaunchFlags{
		open:  flags.Bool("open-browser", false, "open the local app in the default browser after startup"),
		setup: flags.Bool("setup", false, "open guided setup for explicit reconfiguration when launching a browser"),
	}
}

func launchBrowserWhenReady(open, setup bool, baseURL string, logger *slog.Logger) {
	if !open {
		return
	}
	go openLocalAppWhenReady(baseURL, browserLaunchRoute(setup), logger)
}

func browserLaunchRoute(setup bool) string {
	route := "#/chat"
	if setup {
		route = "#/setup/reconfigure"
	}
	return route
}

func openLocalAppWhenReady(baseURL, route string, logger *slog.Logger) {
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/healthz") // #nosec G107 -- startup produced the validated server URL.
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				if err := openSystemBrowser(fmt.Sprintf("%s/%s", baseURL, route)); err != nil {
					logger.Warn("default browser did not open", "error", err, "url", baseURL)
				}
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	logger.Warn("default browser was not opened because the local server did not become ready", "url", baseURL)
}
