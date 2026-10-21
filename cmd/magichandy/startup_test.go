package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
	"github.com/mapledaemon/MagicHandy/internal/startup"
)

func TestLocalPortPolicyKeepsPinnedAndHostedDeploymentsStrict(t *testing.T) {
	for _, test := range []struct {
		name, policy, override string
		custom, saved, tls     bool
		mode                   string
		explicitNetwork        bool
		want, fail             bool
	}{
		{name: "default local", policy: "default", want: true},
		{name: "explicit address", policy: "default", override: "127.0.0.1:49717"},
		{name: "saved custom port", policy: "default", custom: true},
		{name: "saved network deployment", policy: "default", saved: true},
		{name: "explicit fixed", policy: "fixed"},
		{name: "launcher opts in", policy: "auto", override: "127.0.0.1:49717", want: true},
		{name: "TLS remains strict", policy: "default", tls: true},
		{name: "TLS cannot opt in", policy: "auto", tls: true, fail: true},
		{name: "saved proxy stays fixed with launcher auto", policy: "auto", saved: true, mode: netaccess.TrustedProxy},
		{name: "saved HTTPS stays fixed with launcher auto", policy: "auto", saved: true, tls: true, mode: netaccess.DirectHTTPS},
		{name: "explicit proxy cannot opt in", policy: "auto", saved: true, mode: netaccess.TrustedProxy, explicitNetwork: true, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := config.DefaultSettings()
			if test.custom {
				settings.Server.Port = 48800
			}
			security := serverSecurity{}
			if test.saved {
				mode := test.mode
				if mode == "" {
					mode = netaccess.Local
				}
				security.NetworkPolicy = &netaccess.Policy{Config: netaccess.Config{Mode: mode}}
			}
			if test.tls {
				security.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			}
			got, err := automaticLocalPort(test.policy, test.override, settings, security, "127.0.0.1:49717", test.explicitNetwork)
			if got != test.want || (err != nil) != test.fail {
				t.Fatalf("auto=%v err=%v", got, err)
			}
		})
	}
}

func TestWindowsBindDenialRecoversOnlyInAutomaticMode(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		recorder := startup.NewRecorder(t.TempDir(), "test", "commit")
		calls := 0
		listen := func(network, address string) (net.Listener, error) {
			calls++
			if calls == 1 {
				return nil, fmt.Errorf("Windows bind: %w", syscall.Errno(10013))
			}
			if address != "127.0.0.1:0" {
				t.Fatalf("fallback address=%s", address)
			}
			return net.Listen(network, address)
		}
		listener, err := bindStartupUsing(listen, "127.0.0.1:49717", "app", automatic, 0, recorder)
		if automatic {
			if err != nil {
				t.Fatal(err)
			}
			_ = listener.Close()
			if calls != 2 || recorder.Report.Attempts[0].NativeCode != 10013 {
				t.Fatal("denial was not recorded before recovery")
			}
		} else if err == nil || calls != 1 {
			t.Fatal("fixed bind silently moved")
		}
	}
}

func TestCachedPortFallsBackToPreferenceBeforeAnAssignedPort(t *testing.T) {
	for _, preferredBusy := range []bool{false, true} {
		root := t.TempDir()
		recorder := startup.NewRecorder(root, "test", "commit")
		var attempts []string
		listen := func(network, address string) (net.Listener, error) {
			attempts = append(attempts, address)
			if address == "127.0.0.1:49718" || (preferredBusy && address == "127.0.0.1:49717") {
				return nil, syscall.EADDRINUSE
			}
			return net.Listen(network, "127.0.0.1:0")
		}
		listener, err := bindStartupUsing(listen, "127.0.0.1:49717", "app", true, 49718, recorder)
		if err != nil {
			t.Fatal(err)
		}
		_ = listener.Close()
		want := "127.0.0.1:49718,127.0.0.1:49717"
		if preferredBusy {
			want += ",127.0.0.1:0"
		}
		if strings.Join(attempts, ",") != want {
			t.Fatalf("retry order: %v", attempts)
		}
		if _, err := recorder.Finish(nil); err != nil {
			t.Fatal(err)
		}
		if startup.ReadReports(root).Current.Outcome != "recovered" {
			t.Fatal("a failed cached bind followed by automatic success must remain diagnosable")
		}
	}
}

func TestAutomaticListenersRetainBothSocketsAndActualOrigins(t *testing.T) {
	blocked, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocked.Close() }()
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	remote := addRemoteFlags(flags)
	root := t.TempDir()
	launch := &launchSession{dataDir: root, recorder: startup.NewRecorder(root, "test", "commit"), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	listeners, main, secondary, err := bindApplicationListeners(blocked.Addr().String(), serverSecurity{BaseURL: "http://" + blocked.Addr().String()}, remote, true, launch)
	if err != nil {
		t.Fatal(err)
	}
	defer listeners.close()
	if main.BaseURL != "http://"+listeners.main.Addr().String() || secondary.BaseURL != "http://"+listeners.remote.Addr().String() || main.BaseURL == secondary.BaseURL {
		t.Fatalf("wrong origins main=%s remote=%s", main.BaseURL, secondary.BaseURL)
	}
	for _, listener := range []net.Listener{listeners.main, listeners.remote} {
		if other, err := net.Listen("tcp", listener.Addr().String()); err == nil {
			_ = other.Close()
			t.Fatal("winning socket was released")
		}
	}
}

func TestFixedBindFailureRetainsReportBeforeApplicationStarts(t *testing.T) {
	blocked, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocked.Close() }()
	root := t.TempDir()
	err = run([]string{"-addr", blocked.Addr().String(), "-data-dir", root, "-simulate-motion", "-remote-port", "-1"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("fixed occupied port recovered")
	}
	reports := startup.ReadReports(root)
	if reports.LastFailure == nil || reports.LastFailure.Stage != "bind_app" || reports.LastFailure.Outcome != "failed" {
		t.Fatalf("failure not retained: %+v", reports)
	}
	if _, err := os.Stat(filepath.Join(root, "startup", "instance.json")); !os.IsNotExist(err) {
		t.Fatal("failed startup published readiness")
	}
	owner, err := startup.Acquire(root)
	if err != nil {
		t.Fatalf("failure leaked profile ownership: %v", err)
	}
	owner.Close()
}

func TestRemotePortConflictRecoversOrReleasesMainAccordingToPolicy(t *testing.T) {
	var address string
	var blocker net.Listener
	for attempt := 0; attempt < 20; attempt++ {
		main, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listenerPort(main)
		if port < 65535 {
			blocker, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port+1)))
			if err == nil {
				address = main.Addr().String()
			}
		}
		_ = main.Close()
		if blocker != nil {
			break
		}
	}
	if blocker == nil {
		t.Fatal("could not reserve a remote conflict fixture")
	}
	defer func() { _ = blocker.Close() }()
	for _, automatic := range []bool{false, true} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		remote := addRemoteFlags(flags)
		root := t.TempDir()
		launch := &launchSession{dataDir: root, recorder: startup.NewRecorder(root, "test", "commit"), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
		listeners, _, remoteSecurity, err := bindApplicationListeners(address, serverSecurity{BaseURL: "http://" + address}, remote, automatic, launch)
		if automatic {
			if err != nil {
				t.Fatal(err)
			}
			if listeners.main.Addr().String() != address || listeners.remote.Addr().String() == blocker.Addr().String() || remoteSecurity.BaseURL != "http://"+listeners.remote.Addr().String() {
				t.Fatal("remote conflict changed the working app endpoint or published an incorrect remote")
			}
			assertFixedDuplicatesRejectRecoveredRemote(t, launch, listeners.main, address, remoteSecurity.BaseURL)
			listeners.close()
		} else {
			if err == nil {
				listeners.close()
				t.Fatal("fixed remote port silently moved")
			}
			probe, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatalf("failed remote bind leaked main listener: %v", err)
			}
			_ = probe.Close()
		}
	}
}

func assertFixedDuplicatesRejectRecoveredRemote(t *testing.T, launch *launchSession, listener net.Listener, address, remoteURL string) {
	t.Helper()
	instance, err := startup.Acquire(launch.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	launch.instance, launch.preferredURL = instance, "http://"+address
	server := httptest.NewUnstartedServer(instance.Handler(http.NotFoundHandler()))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	if err := launch.announce(server.URL, remoteURL); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-data-dir", launch.dataDir, "-port-policy", "fixed"}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "fixed") {
		t.Fatalf("fixed duplicate accepted a recovered remote: %v", err)
	}
	if err := run([]string{"-data-dir", launch.dataDir, "-addr", address}, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "fixed") {
		t.Fatalf("explicit-address duplicate accepted a recovered remote: %v", err)
	}
}

func TestDuplicateLaunchAndMachineDiscoveryDoNotOpenDatastore(t *testing.T) {
	root := t.TempDir()
	instance, err := startup.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	server := httptest.NewServer(instance.Handler(http.NotFoundHandler()))
	defer server.Close()
	if err := instance.Publish(server.URL, ""); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-data-dir", root}, &stdout, &stderr); err != nil || !strings.Contains(stdout.String(), server.URL) {
		t.Fatalf("reuse: %v %s", err, stdout.String())
	}
	stdout.Reset()
	if err := run([]string{"-data-dir", root, "-running-instance"}, &stdout, &stderr); err != nil || strings.Contains(stdout.String(), "token") || !strings.Contains(stdout.String(), server.URL) {
		t.Fatalf("discovery: %v %s", err, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "magichandy.db")); !os.IsNotExist(err) {
		t.Fatal("second launch opened datastore")
	}
	if _, err := startup.Acquire(root); !errors.Is(err, startup.ErrRunning) {
		t.Fatal("second launch released existing owner's lock")
	}
	origin, _ := url.Parse(server.URL)
	_, port, _ := net.SplitHostPort(origin.Host)
	number, _ := strconv.Atoi(port)
	wrong := net.JoinHostPort("127.0.0.1", strconv.Itoa(number%60000+1))
	if err := run([]string{"-data-dir", root, "-addr", wrong}, &stdout, &stderr); err == nil {
		t.Fatal("fixed-address duplicate ignored requested endpoint")
	}
	if err := run([]string{"-data-dir", root, "-require-auth"}, &stdout, &stderr); err == nil {
		t.Fatal("duplicate silently ignored an authentication change")
	}
	if err := run([]string{"-data-dir", root, "-simulate-motion"}, &stdout, &stderr); err == nil {
		t.Fatal("duplicate silently ignored explicit motion simulation")
	}
	if err := run([]string{"-data-dir", root, "-remote-port", "48799"}, &stdout, &stderr); err == nil {
		t.Fatal("duplicate silently ignored a fixed remote endpoint")
	}
	if err := run([]string{"-data-dir", root, "-addr", "192.168.1.2:49717", "-port-policy", "auto"}, &stdout, &stderr); err == nil {
		t.Fatal("duplicate silently ignored a nonlocal listen request")
	}
}
