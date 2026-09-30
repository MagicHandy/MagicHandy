package console

import (
	"strings"
	"testing"
	"time"
)

func TestRemoteAddressAndKeysAreIndependentFromApp(t *testing.T) {
	opened, copied := make(chan struct{}, 1), make(chan struct{}, 1)
	d, term := startedDashboard(t, Actions{OpenRemote: func() error { opened <- struct{}{}; return nil }, CopyRemote: func() error { copied <- struct{}{}; return nil }})
	d.SetRemote("https://remote.example.test:49718")
	waitFor(t, func() bool { return strings.Contains(visible(term.output()), "https://remote.example.test:49718") })
	term.keys <- 'r'
	term.keys <- 'l'
	for _, ch := range []chan struct{}{opened, copied} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("remote shortcut did not run")
		}
	}
	waitFor(t, func() bool { return strings.Contains(visible(term.output()), "https://remote.example.test:49718") })
}
