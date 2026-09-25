package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/mapledaemon/MagicHandy/internal/console"
	"github.com/mapledaemon/MagicHandy/internal/httpapi"
	"github.com/mapledaemon/MagicHandy/internal/logging"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

const (
	consoleAuto  = "auto"
	consolePlain = "plain"
)

func addConsoleFlag(flags *flag.FlagSet) *string {
	return flags.String("console", consoleAuto, "launch console: auto shows the interactive MagicHandy window when started in a Windows console; plain writes structured JSON logs")
}

// launchConsole is the interactive window Windows opens with the app. A nil
// launchConsole means plain structured logs, so every method accepts nil.
type launchConsole struct {
	dashboard *console.Dashboard
	quit      chan struct{}
	quitOnce  sync.Once
}

// startLaunchConsole shows the interactive console when the app owns a real
// console. Redirected output, configuration-only runs and -console plain keep
// the structured JSON logs that scripts and diagnostics read.
func startLaunchConsole(mode string, configOnly bool, stdout, stderr io.Writer) (*launchConsole, error) {
	switch mode {
	case consoleAuto, "":
	case consolePlain:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown console mode %q; use %s or %s", mode, consoleAuto, consolePlain)
	}
	if configOnly || stdout != io.Writer(os.Stdout) || stderr != io.Writer(os.Stderr) {
		return nil, nil
	}
	term, err := console.Open()
	if err != nil {
		return nil, nil
	}
	dashboard := console.New(term, version)
	dashboard.Start()
	return &launchConsole{dashboard: dashboard, quit: make(chan struct{})}, nil
}

func (c *launchConsole) logger(stderr io.Writer, level slog.Level) *slog.Logger {
	if c == nil {
		return logging.New(stderr, level)
	}
	return slog.New(c.dashboard.Handler(level))
}

func (c *launchConsole) serverPrepared(security serverSecurity, simulated bool) {
	if c == nil {
		return
	}
	c.dashboard.SetServer(security.BaseURL, accessDescription(security), simulated)
}

// connect gives the console keys their actions. Stop uses the same
// emergency-stop path as the app's Stop button.
func (c *launchConsole) connect(api *httpapi.Server, baseURL string) {
	if c == nil {
		return
	}
	c.dashboard.SetActions(console.Actions{
		Open: func() error {
			return openSystemBrowser(strings.TrimRight(baseURL, "/") + "/" + browserLaunchRoute(false))
		},
		Copy: clipboardCopier(baseURL),
		Stop: func(ctx context.Context) (console.StopOutcome, error) {
			result, err := api.ConsoleStop(ctx)
			switch {
			case err != nil:
				return console.StopUnconfirmed, err
			case result.Confirmed:
				return console.StopConfirmed, nil
			case !result.Available:
				return console.StopNothingConnected, nil
			default:
				return console.StopUnconfirmed, nil
			}
		},
		Quit: func() { c.quitOnce.Do(func() { close(c.quit) }) },
	})
}

func (c *launchConsole) ready() func() {
	if c == nil {
		return nil
	}
	return c.dashboard.SetRunning
}

func (c *launchConsole) stopping() {
	if c != nil {
		c.dashboard.SetStopping()
	}
}

// quitRequested closes when the person confirms Quit in the console. It is
// nil, and so never ready, with plain logs.
func (c *launchConsole) quitRequested() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.quit
}

// finish restores the terminal and reports whether the console showed err.
func (c *launchConsole) finish(err error) bool {
	if c == nil {
		return false
	}
	return c.dashboard.Finish(err)
}

// configurationOnly reports a run that changes settings and exits, which
// never shows the launch console.
func configurationOnly(languages languageConfigurationFlags, tts ttsModuleFlagValues) bool {
	return *languages.ui != "" || *languages.chat != "" || strings.TrimSpace(*tts.provider) != ""
}

// reportedError is an error the launch console already showed, so main does
// not print it again.
type reportedError struct{ error }

func (e reportedError) Unwrap() error { return e.error }

// accessDescription says who can reach the app, in the terms Setup and
// Access settings use.
func accessDescription(security serverSecurity) string {
	access := "Local only (this computer)"
	policy := security.NetworkPolicy
	direct := policy != nil && policy.Config.Mode == netaccess.DirectHTTPS
	switch {
	case policy != nil && policy.Config.Mode == netaccess.TrustedProxy:
		access = "Through a trusted proxy"
	case direct && policy.Config.Scope == "lan":
		access = "LAN + local, over HTTPS"
	case direct && policy.Config.Scope == "public":
		access = "Public, over HTTPS"
	case security.TLSConfig != nil:
		access = "Network, over HTTPS"
	}
	if security.AuthenticationRequired {
		access += "; sign-in required"
	}
	return access
}
