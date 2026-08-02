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
		setup: flags.Bool("setup", false, "open the first-run setup route when launching a browser"),
	}
}

func launchBrowserWhenReady(open, setup bool, address string, logger *slog.Logger) {
	if !open {
		return
	}
	route := "#/chat"
	if setup {
		route = "#/setup"
	}
	go openLocalAppWhenReady(address, route, logger)
}

func openLocalAppWhenReady(address, route string, logger *slog.Logger) {
	host := address
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	baseURL := "http://" + host
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/healthz") // #nosec G107 -- the address is the validated loopback listener.
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				if err := openSystemBrowser(fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), route)); err != nil {
					logger.Warn("default browser did not open", "error", err, "url", baseURL)
				}
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	logger.Warn("default browser was not opened because the local server did not become ready", "url", baseURL)
}
