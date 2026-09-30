package netaccess

import "testing"

func TestRemoteEndpointPreservesBoundaryAndSeparatesPorts(t *testing.T) {
	for _, tc := range []struct {
		name, address, base, public, wantAddress, wantURL string
		port                                              int
		invalid                                           bool
	}{
		{"local", "127.0.0.1:49717", "http://127.0.0.1:49717", "", "127.0.0.1:49718", "http://127.0.0.1:49718", 0, false},
		{"ipv6", "[::1]:51000", "http://[::1]:51000", "", "[::1]:51001", "http://[::1]:51001", 0, false},
		{"managed TLS", "192.168.1.10:49717", "https://control.example.test", "", "192.168.1.10:49718", "https://control.example.test:49718", 0, false},
		{"proxy", "127.0.0.1:49717", "https://app.example.test", "https://remote.example.test", "127.0.0.1:55000", "https://remote.example.test", 55000, false},
		{"disabled", "127.0.0.1:49717", "http://127.0.0.1:49717", "", "", "", -1, false},
		{"same port", "127.0.0.1:49717", "http://127.0.0.1:49717", "", "", "", 49717, true},
		{"overflow", "127.0.0.1:65535", "http://127.0.0.1:65535", "", "", "", 0, true},
		{"public HTTP", "127.0.0.1:49717", "http://127.0.0.1:49717", "https://remote.example.test", "", "", 0, true},
		{"same origin", "127.0.0.1:49717", "https://app.example.test", "https://app.example.test", "", "", 0, true},
		{"skip external collision", "127.0.0.1:49717", "https://app.example.test:49718", "", "127.0.0.1:49719", "https://app.example.test:49719", 0, false},
		{"reject equivalent default port", "127.0.0.1:49717", "https://app.example.test:443", "https://app.example.test", "", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address, origin, err := RemoteEndpoint(tc.address, tc.base, tc.port, tc.public)
			if (err != nil) != tc.invalid || (err == nil && (address != tc.wantAddress || origin != tc.wantURL)) {
				t.Fatalf("%q %q %v", address, origin, err)
			}
		})
	}
}

func TestRemoteManagedCertificateRetainsMainIssuanceOrigin(t *testing.T) {
	parent, err := Validate(Config{Mode: DirectHTTPS, ListenAddress: "192.168.1.10:49717", PublicURL: "https://control.example.com", Scope: "public", CertificateMode: AutomaticPublic, AcceptedTerms: "https://letsencrypt.org/documents/LE-SA-v1.5-February-24-2025.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := RemotePolicy(parent, "192.168.1.10:49718", "https://control.example.com:49718")
	if err != nil {
		t.Fatal(err)
	}
	if parent.Origin.Port() != "" || child.Origin.Port() != "49718" || child.Config.Mode != DirectHTTPS {
		t.Fatal("remote mutated parent certificate policy")
	}
	if _, err = RemotePolicy(parent, "192.168.1.10:49718", "https://other.example.com"); err == nil {
		t.Fatal("certificate hostname changed")
	}
}
