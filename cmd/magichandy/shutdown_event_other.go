//go:build !windows

package main

import "time"

func listenForInstallerShutdown() (<-chan struct{}, func(), error) {
	return nil, func() {}, nil
}

func requestInstallerShutdown(time.Duration) (bool, error) {
	return false, nil
}
