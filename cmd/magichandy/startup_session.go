package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/startup"
)

type launchSession struct {
	dataDir            string
	instance           *startup.Instance
	recorder           *startup.Recorder
	logger             *slog.Logger
	ready              bool
	portHint           *startup.PortHint
	preferredURL       string
	preferredRemoteURL string
}

func beginLaunch(dataDir, override, policy string, security serverSecurityFlags, remote remoteFlags, browser browserLaunchFlags, simulate bool, logger *slog.Logger, stdout io.Writer) (*launchSession, bool, error) {
	resolved, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return nil, false, err
	}
	instance, err := startup.Acquire(resolved)
	if errors.Is(err, startup.ErrRunning) {
		if simulate {
			return nil, false, errors.New("MagicHandy already runs for this data directory; close it or use a separate data directory before requesting motion simulation")
		}
		if !canReuseLocalLaunch(security, override) {
			return nil, false, errors.New("MagicHandy already runs for this data directory; close it before applying explicit authentication, TLS or hosted network flags")
		}
		return nil, true, openExistingInstance(resolved, override, policy, remote, browser, stdout)
	}
	launch := &launchSession{dataDir: resolved, instance: instance, recorder: startup.NewRecorder(resolved, version, commit), logger: logger}
	if err != nil {
		return nil, false, launch.finish(err)
	}
	return launch, false, nil
}

func canReuseLocalLaunch(security serverSecurityFlags, override string) bool {
	if override != "" {
		if _, loopback, err := validateListenHost(override); err != nil || !loopback {
			return false
		}
	}
	mode := flagString(security.networkMode)
	return !*security.requireAuth && (mode == "" || mode == "local") &&
		flagString(security.certificate) == "" && flagString(security.privateKey) == "" &&
		flagString(security.publicURL) == "" && flagString(security.trustedProxies) == ""
}

func explicitNetworkFlags(security serverSecurityFlags) bool {
	return flagString(security.networkMode) != "" || flagString(security.certificate) != "" || flagString(security.privateKey) != "" ||
		flagString(security.publicURL) != "" || flagString(security.trustedProxies) != ""
}

func openExistingInstance(dataDir, override, policy string, remote remoteFlags, browser browserLaunchFlags, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	existing, err := startup.FindExisting(ctx, dataDir)
	if err != nil {
		return err
	}
	origin, _ := url.Parse(existing.URL)
	if override != "" && policy != "auto" && origin.Host != override {
		return fmt.Errorf("MagicHandy already runs for this data directory at %s; the fixed address %s was not started", existing.URL, override)
	}
	if policy == "fixed" && override == "" && existing.PreferredURL != existing.URL {
		return fmt.Errorf("MagicHandy already runs at %s after port recovery; the fixed preferred address was not started", existing.URL)
	}
	if (policy == "fixed" || (override != "" && policy != "auto")) && existing.PreferredRemoteURL != existing.RemoteURL {
		return errors.New("MagicHandy already runs with a different or unverifiable remote endpoint; close it before applying fixed ports")
	}
	if err := checkExistingRemote(existing, remote); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "MagicHandy is already running at %s\n", existing.URL)
	if *browser.open {
		return openSystemBrowser(existing.URL + "/" + browserLaunchRoute(*browser.setup))
	}
	return nil
}

func checkExistingRemote(existing startup.Existing, flags remoteFlags) error {
	var portSet, urlSet bool
	flags.set.Visit(func(value *flag.Flag) {
		portSet = portSet || value.Name == "remote-port"
		urlSet = urlSet || value.Name == "remote-public-url"
	})
	origin, err := url.Parse(existing.RemoteURL)
	if err != nil {
		return errors.New("existing remote endpoint is invalid")
	}
	if portSet && ((*flags.port == -1 && existing.RemoteURL != "") || (*flags.port > 0 && origin.Port() != strconv.Itoa(*flags.port))) {
		return errors.New("MagicHandy already runs for this data directory with a different remote endpoint; the explicit remote port was not applied")
	}
	if urlSet && flagString(flags.publicURL) != existing.RemoteURL {
		return errors.New("MagicHandy already runs for this data directory with a different remote public URL")
	}
	return nil
}

func printRunningInstance(dataDir string, stdout io.Writer) error {
	resolved, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	existing, err := startup.FindExisting(ctx, resolved)
	if err != nil {
		return json.NewEncoder(stdout).Encode(nil)
	}
	return json.NewEncoder(stdout).Encode(existing)
}

func (l *launchSession) finish(err error) error {
	if !l.ready && err != nil {
		if path := l.saveReport(err); path != "" {
			err = fmt.Errorf("%w\nStartup report: %s\nRetry startup or share this report for help. No system network services were reset", err, path)
		}
	}
	if l.instance != nil {
		l.instance.Close()
	}
	return err
}

func (l *launchSession) saveReport(err error) string {
	path, saveErr := l.recorder.Finish(err)
	if saveErr != nil {
		l.logger.Warn("startup report could not be saved")
		return ""
	}
	if err != nil || l.recorder.Report.Outcome == "recovered" {
		l.logger.Warn("startup diagnostic report saved", "report", path, "outcome", l.recorder.Report.Outcome,
			"message", "If startup failed, retry or share this credential-free report. Windows port denial does not by itself identify HNS as the cause.")
	}
	return path
}

func (l *launchSession) announce(appURL, remoteURL string) error {
	l.recorder.Report.Stage = "readiness"
	if err := l.instance.PublishWithPreference(appURL, remoteURL, l.preferredURL, l.preferredRemoteURL); err != nil {
		return fmt.Errorf("publish startup endpoint: %w", err)
	}
	l.saveReport(nil)
	if l.portHint != nil {
		if err := startup.SavePortHint(l.dataDir, *l.portHint); err != nil {
			l.logger.Warn("could not remember local startup ports")
		}
	}
	l.ready = true
	return nil
}
