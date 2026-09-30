package main

import (
	"flag"
	"testing"

	"github.com/mapledaemon/MagicHandy/internal/netaccess"
)

func TestRemoteFlagsOverrideSavedDisabledListener(t *testing.T) {
	security := serverSecurity{BaseURL: "http://127.0.0.1:49717", NetworkPolicy: &netaccess.Policy{Config: netaccess.Config{Mode: netaccess.Local, RemotePort: -1}}}
	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, ""}, {[]string{"-remote-port", "0"}, "127.0.0.1:49718"},
		{[]string{"-remote-port", "51000"}, "127.0.0.1:51000"}, {[]string{"-remote-port", "-1"}, ""},
	} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		remote := addRemoteFlags(flags)
		if err := flags.Parse(test.args); err != nil {
			t.Fatal(err)
		}
		_, address, err := prepareRemoteServer("127.0.0.1:49717", security, remote)
		if err != nil || address != test.want {
			t.Fatalf("%v: %q %v", test.args, address, err)
		}
	}
}
