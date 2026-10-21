package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/mapledaemon/MagicHandy/internal/config"
	"github.com/mapledaemon/MagicHandy/internal/netaccess"
	"github.com/mapledaemon/MagicHandy/internal/startup"
)

type startupListeners struct {
	main   net.Listener
	remote net.Listener
}

func (l startupListeners) close() {
	if l.main != nil {
		_ = l.main.Close()
	}
	if l.remote != nil {
		_ = l.remote.Close()
	}
}

func automaticLocalPort(policy, override string, settings config.Settings, security serverSecurity, address string, explicitNetwork bool) (bool, error) {
	if policy != "default" && policy != "auto" && policy != "fixed" {
		return false, errors.New("port-policy must be default, auto or fixed")
	}
	host, _, err := net.SplitHostPort(address)
	local := err == nil && net.ParseIP(host).IsLoopback() && security.TLSConfig == nil &&
		(security.NetworkPolicy == nil || security.NetworkPolicy.Config.Mode == netaccess.Local)
	if policy == "auto" && !local {
		// The source launcher opts ordinary local installs into recovery. A
		// previously saved hosted policy still owns its fixed listener settings.
		if security.NetworkPolicy != nil && !explicitNetwork {
			return false, nil
		}
		return false, errors.New("automatic ports require a literal loopback HTTP address; HTTPS, LAN and proxy deployments keep fixed ports")
	}
	if !local || policy == "fixed" {
		return false, nil
	}
	return policy == "auto" || (override == "" && settings.Server.Port == config.DefaultServerPort && security.NetworkPolicy == nil), nil
}

// bindStartup keeps the winning socket open. Only bind denial and port conflicts
// permit fallback; malformed addresses and other errors stay visible.
func bindStartup(address, name string, automatic bool, hint int, recorder *startup.Recorder) (net.Listener, error) {
	return bindStartupUsing(net.Listen, address, name, automatic, hint, recorder)
}

func bindStartupUsing(listen func(string, string) (net.Listener, error), address, name string, automatic bool, hint int, recorder *startup.Recorder) (net.Listener, error) {
	host, preferred, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if automatic && hint > 0 {
		address = net.JoinHostPort(host, strconv.Itoa(hint))
	}
	listener, err := listen("tcp", address)
	retryable := recorder.Bind(name, address, err)
	if err == nil || !automatic || !retryable {
		return listener, err
	}
	// A cached origin can disappear. Try the preferred port before asking the OS.
	if hint > 0 && strconv.Itoa(hint) != preferred {
		address = net.JoinHostPort(host, preferred)
		listener, err = listen("tcp", address)
		retryable = recorder.Bind(name, address, err)
		if err == nil || !retryable {
			return listener, err
		}
	}
	address = net.JoinHostPort(host, "0")
	listener, err = listen("tcp", address)
	if listener != nil {
		address = listener.Addr().String()
	}
	recorder.Bind(name, address, err)
	return listener, err
}

func localSecurityAt(security serverSecurity, address string) serverSecurity {
	security.BaseURL = "http://" + address
	if security.NetworkPolicy != nil {
		policy := *security.NetworkPolicy
		policy.Config.ListenAddress = address
		security.NetworkPolicy = &policy
	}
	return security
}

func bindApplicationListeners(address string, security serverSecurity, flags remoteFlags, automatic bool, launch *launchSession) (_ startupListeners, main, remote serverSecurity, err error) {
	var listeners startupListeners
	defer func() {
		if err != nil {
			listeners.close()
		}
	}()
	remotePort, remoteURL := configuredRemote(security, flags)
	preference := fmt.Sprintf("%s/%d/%s", address, remotePort, remoteURL)
	hint := startup.PortHint{}
	if automatic {
		hint = startup.ReadPortHint(launch.dataDir, preference)
	}
	// Reserve an explicitly pinned remote first, so an OS-selected app port
	// cannot accidentally consume it during local recovery.
	if automatic && remotePort > 0 {
		launch.recorder.Report.Stage = "bind_remote"
		_, remoteAddress, prepareErr := prepareRemoteServer(address, security, flags)
		if prepareErr != nil {
			return listeners, security, remote, prepareErr
		}
		listeners.remote, err = bindStartup(remoteAddress, "remote", false, 0, launch.recorder)
		if err != nil {
			return listeners, security, remote, fmt.Errorf("remote listener: %w", err)
		}
	}
	launch.recorder.Report.Stage = "bind_app"
	listeners.main, err = bindStartup(address, "app", automatic, hint.Main, launch.recorder)
	if err != nil {
		return listeners, security, remote, fmt.Errorf("app listener: %w", err)
	}
	if automatic {
		security = localSecurityAt(security, listeners.main.Addr().String())
	}
	launch.recorder.Report.Stage = "bind_remote"
	remote, launch.preferredRemoteURL, err = bindRemoteListener(&listeners, security, flags, automatic && remotePort == 0, hint.Remote, launch.recorder)
	if err != nil {
		return listeners, security, remote, err
	}
	if automatic {
		launch.portHint = &startup.PortHint{Preference: preference, Main: listenerPort(listeners.main), Remote: listenerPort(listeners.remote)}
	}
	return listeners, security, remote, nil
}

func bindRemoteListener(listeners *startupListeners, main serverSecurity, flags remoteFlags, automatic bool, hint int, recorder *startup.Recorder) (serverSecurity, string, error) {
	address := listeners.main.Addr().String()
	remote, remoteAddress, err := prepareRemoteServer(address, main, flags)
	preferredURL := remote.BaseURL
	if automatic {
		// The app can receive 65535 from the OS; it has no adjacent remote port.
		host, _, _ := net.SplitHostPort(address)
		if listenerPort(listeners.main) == 65535 {
			remoteAddress, err = net.JoinHostPort(host, "0"), nil
		}
	}
	if err != nil || remoteAddress == "" {
		return remote, preferredURL, err
	}
	if listeners.remote == nil {
		listeners.remote, err = bindStartup(remoteAddress, "remote", automatic, hint, recorder)
		if err != nil {
			return remote, preferredURL, fmt.Errorf("remote listener: %w; choose -remote-port or use -remote-port -1", err)
		}
	}
	if automatic {
		remote = localSecurityAt(main, listeners.remote.Addr().String())
		remote.NetworkPolicy, err = netaccess.RemotePolicy(main.NetworkPolicy, listeners.remote.Addr().String(), remote.BaseURL)
	}
	return remote, preferredURL, err
}

func listenerPort(listener net.Listener) int {
	if listener == nil {
		return 0
	}
	_, raw, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(raw)
	return port
}
